import { create } from 'zustand';
import type { Message, InputRequest, FileAttachment, ModelInfo, ToolCallStatus } from '../types/api';
import type { NodeClient } from '../api/node-client';

import { rfc3339Now } from '../utils/date';

interface ChatState {
  messages: Message[];
  sending: boolean;
  streaming: boolean;
  streamingMessageId: string | null; // 当前流式占位符的唯一 ID（timestamp），用于精确定位更新目标
  agentStatus: string | null; // 'thinking', 'tool_calling', 'completed', null
  error: string | null;
  currentSessionId: string | null; // 当前会话 ID，用于过滤消息
  inlineApprovals: (InputRequest & { afterMessageTimestamp: string })[]; // 内联到消息列表中的审批请求，锚定到对应消息的 timestamp
  availableModels: ModelInfo[];
  activeModel: string | null;
  toolCallStatuses: Record<string, ToolCallStatus>; // tool call ID → 实时状态
  toolCallStartTimes: Record<string, number>; // tool call ID → start timestamp (for client-side timing)
  // 操作
  sendMessage: (client: NodeClient, sessionId: string, content: string, options?: { attachments?: FileAttachment[]; deepThinking?: boolean }) => Promise<void>;
  addMessage: (msg: Message, sessionId?: string) => void;
  updateLastAssistant: (content: string, reasoningContent?: string) => void;
  ensureAssistantMessage: () => void; // 确保有 assistant 消息占位符
  setMessages: (messages: Message[]) => void;
  clearMessages: () => void;
  clearError: () => void;
  loadMessages: (client: NodeClient, sessionId: string, limit?: number) => Promise<void>;
  setStreaming: (streaming: boolean) => void;
  setAgentStatus: (status: string | null) => void;
  setCurrentSessionId: (sessionId: string | null) => void;
  addInlineApproval: (req: InputRequest) => void;
  removeInlineApproval: (requestId: string) => void;
  loadModels: (client: NodeClient) => Promise<void>;
  setToolCallStatus: (id: string, status: ToolCallStatus) => void;
  stopTask: (client: NodeClient, sessionId: string) => Promise<void>;
  replaceStreamingMessage: (msg: Message, streamId: string | null) => void;
  confirmUserMessage: (timestamp: string) => void;
}


/** 将消息追加到列表中（去重 + 按 timestamp 升序插入）。
 *  去重键 = timestamp + role + tool_call_id（tool 消息同秒可能多条）。
 *  流式占位符（stream- 前缀）始终排在已确认消息之后。
 */
// 返回消息列表中最后一条已确认消息的时间戳（排除 temp- / stream- 前缀）
// 用于将错误消息锚定在用户消息之后，避免因服务端时钟偏移导致乱序
export function maxConfirmedTimestamp(messages: Message[]): string | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const ts = messages[i].timestamp || '';
    if (ts && !ts.startsWith('temp-') && !ts.startsWith('stream-')) {
      return ts;
    }
  }
  return null;
}

function appendMessage(messages: Message[], msg: Message): Message[] {
  const ts = msg.timestamp || '';
  // 流式占位符、临时用户消息、无时间戳：直接 append
  if (ts.startsWith('stream-') || ts.startsWith('temp-') || !ts) {
    return [...messages, msg];
  }

  // 去重：如果已有相同 timestamp + role + tool_call_id 的消息，跳过
  const isDuplicate = messages.some((m) => {
    if (m.timestamp !== ts) return false;
    if (m.role !== msg.role) return false;
    // tool 消息额外按 tool_call_id 区分
    if (msg.role === 'tool' && m.tool_call_id !== msg.tool_call_id) return false;
    return true;
  });
  if (isDuplicate) return messages;

  // 插入到正确的时间位置（在流式占位符之前、按 timestamp 升序）
  // temp- 消息（待确认的用户消息）视为"末尾锚点"，有真实时间戳的消息不插入其前面
  const msgs = [...messages];
  let insertAt = msgs.length;
  for (let i = msgs.length - 1; i >= 0; i--) {
    const existTs = msgs[i].timestamp || '';
    if (existTs.startsWith('stream-')) {
      insertAt = i; // 跳过流式占位符
    } else if (existTs.startsWith('temp-')) {
      break; // temp- 消息是末尾锚点，不在其前面插入（避免 assistant 消息排到用户消息前）
    } else if (existTs > ts) {
      insertAt = i; // 当前消息应在此之前
    } else {
      break; // 找到第一个 <= ts 的消息，停止
    }
  }
  msgs.splice(insertAt, 0, msg);
  return msgs;
}

function findCurrentUserMessageIndex(messages: Message[], tempId: string, content: string): number {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg.role === 'user' && msg.timestamp === tempId) {
      return i;
    }
  }
  // 用户消息可能已被 WebSocket confirmUserMessage 替换成真实 timestamp。
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (msg.role === 'user' && msg.content === content) {
      return i;
    }
  }
  return -1;
}

function hasFinalAssistantAfterUser(messages: Message[], userIndex: number): boolean {
  if (userIndex < 0) return false;
  return messages.slice(userIndex + 1).some((msg) => {
    const ts = msg.timestamp || '';
    return msg.role === 'assistant'
      && !msg.is_error
      && !ts.startsWith('stream-')
      && msg.content.trim().length > 0;
  });
}

function removeStreamingAssistantAfterUser(messages: Message[], userIndex: number, streamId: string | null): Message[] {
  if (userIndex < 0) return messages;
  const streamIndex = messages.findIndex((msg, index) => {
    if (index <= userIndex || msg.role !== 'assistant') return false;
    const ts = msg.timestamp || '';
    if (streamId) return ts === streamId;
    return ts.startsWith('stream-');
  });
  if (streamIndex < 0) return messages;
  return [
    ...messages.slice(0, streamIndex),
    ...messages.slice(streamIndex + 1),
  ];
}

export const useChatStore = create<ChatState>((set) => ({
  messages: [],
  sending: false,
  streaming: false,
  streamingMessageId: null,
  agentStatus: null,
  error: null,
  currentSessionId: null,
  inlineApprovals: [],
  availableModels: [],
  activeModel: null,
  toolCallStatuses: {},
  toolCallStartTimes: {},

  sendMessage: async (client, sessionId, content, options) => {
    // 使用临时 ID，后端确认后通过 WS 替换为真实时间戳
    const tempId = `temp-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    const userMsg: Message = {
      role: 'user',
      content,
      timestamp: tempId,
      attachments: options?.attachments,
    };
    set((s) => ({
      messages: appendMessage(s.messages, userMsg),
      sending: true,
      error: null,
      currentSessionId: sessionId,
    }));

    try {
      const resp = await client.sendMessage(sessionId, content, options);
      // WebSocket 是优先通道；HTTP 响应是兜底，避免 WS token 过期或缺帧时用户看不到最终回复。
      set((s) => {
        if (!resp?.content || !resp.completed) {
          return { sending: false };
        }
        const userIndex = findCurrentUserMessageIndex(s.messages, tempId, content);
        if (hasFinalAssistantAfterUser(s.messages, userIndex)) {
          return { sending: false };
        }
        const baseMessages = removeStreamingAssistantAfterUser(s.messages, userIndex, s.streamingMessageId);
        const anchor = maxConfirmedTimestamp(s.messages);
        const fallbackTs = anchor
          ? new Date(new Date(anchor).getTime() + 1).toISOString()
          : rfc3339Now();
        return {
          messages: appendMessage(baseMessages, {
            role: 'assistant',
            content: resp.content,
            timestamp: fallbackTs,
          }),
          sending: false,
          streaming: false,
          streamingMessageId: null,
          agentStatus: null,
        };
      });
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : '发送消息失败';
      // 网络错误（fetch 网络故障 / 客户端超时）→ 保留红色错误条
      const isNetworkError = (e instanceof TypeError) ||
        (e instanceof DOMException && e.name === 'AbortError');

      if (isNetworkError) {
        set({ error: errorMsg, sending: false, streaming: false, streamingMessageId: null, agentStatus: null });
      } else {
        // 业务错误（LLM 超时/500/配置错误等）→ AI 角色消息
        // 时间戳锚定在最后一条已确认消息之后，避免服务端时钟偏移导致错误消息排到用户消息前
        set((s) => {
          // 如果 WS agent_status:error 已经写入了错误消息（2秒内），跳过避免重复
          const lastMsg = s.messages[s.messages.length - 1];
          if (lastMsg?.is_error && lastMsg.role === 'assistant') {
            const lastTs = new Date(lastMsg.timestamp || 0).getTime();
            if (Date.now() - lastTs < 2000) {
              return { sending: false, streaming: false, streamingMessageId: null, agentStatus: null };
            }
          }
          const anchor = maxConfirmedTimestamp(s.messages);
          const errorTs = anchor
            ? new Date(new Date(anchor).getTime() + 1).toISOString()
            : rfc3339Now();
          return {
            messages: appendMessage(s.messages, {
              role: 'assistant' as const,
              content: errorMsg,
              timestamp: errorTs,
              is_error: true,
            }),
            sending: false,
            streaming: false,
            streamingMessageId: null,
            agentStatus: null,
          };
        });
      }
    }
  },

  addMessage: (msg, sessionId) => set((s) => {
    // 如果指定了 sessionId，检查是否匹配当前会话
    if (sessionId && s.currentSessionId && sessionId !== s.currentSessionId) {
      return s; // 忽略其他会话的消息
    }
    return { messages: appendMessage(s.messages, msg) };
  }),

  ensureAssistantMessage: () => set((s) => {
    // 如果已有流式占位符且指向最后一条 assistant 消息，直接复用
    const lastMsg = s.messages.length > 0 ? s.messages[s.messages.length - 1] : null;
    if (lastMsg?.role === 'assistant' && !lastMsg.tool_calls) {
      return { streamingMessageId: lastMsg.timestamp || null };
    }
    // 创建新的流式占位符，使用唯一 ID 标识
    const streamId = `stream-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
    return {
      messages: [...s.messages, {
        role: 'assistant',
        content: '',
        timestamp: streamId,
      }],
      streamingMessageId: streamId,
    };
  }),

  updateLastAssistant: (content, reasoningContent?) =>
    set((s) => {
      // 使用 streamingMessageId 精确定位，避免更新错误的 assistant 消息
      const targetId = s.streamingMessageId;
      const msgs = [...s.messages];
      for (let i = msgs.length - 1; i >= 0; i--) {
        if (targetId ? msgs[i].timestamp === targetId : msgs[i].role === 'assistant') {
          const update: Partial<Message> = { content };
          if (reasoningContent !== undefined) {
            update.reasoning_content = reasoningContent;
          }
          msgs[i] = { ...msgs[i], ...update };
          break;
        }
      }
      return { messages: msgs };
    }),

  setMessages: (messages) => set({ messages }),
  clearMessages: () => set({ messages: [], inlineApprovals: [], toolCallStatuses: {}, toolCallStartTimes: {}, streamingMessageId: null }),
  clearError: () => set({ error: null }),

  loadMessages: async (client, sessionId, limit?) => {
    // 切换到不同会话时，重置本会话的 sending/streaming，避免跨会话锁输入框
    set((s) => ({
      error: null,
      currentSessionId: sessionId,
      ...(s.currentSessionId !== sessionId
        ? { sending: false, streaming: false, streamingMessageId: null, agentStatus: null }
        : {}),
    }));
    try {
      const loaded = await client.getMessages(sessionId, limit);
      set({ messages: loaded });
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : '加载消息失败';
      set({ error: errorMsg });
    }
  },

  // 只设置 streaming 标志；streamingMessageId 由 replaceStreamingMessage 负责清除，
  // 避免 agent_status:completed 先于最终消息到达时提前清除占位符 ID
  setStreaming: (streaming) => set({ streaming }),
  setAgentStatus: (agentStatus) => set({ agentStatus }),
  setCurrentSessionId: (currentSessionId) => set({ currentSessionId }),
  addInlineApproval: (req) =>
    set((s) => {
      // 去重，避免重复插入同一审批请求
      if (s.inlineApprovals.some((r) => r.id === req.id)) return s;
      // 根据审批请求的 created_at 找到正确的锚点：
      // 最后一条 timestamp <= created_at 的消息
      let afterMessageTimestamp = '';
      if (req.created_at && s.messages.length > 0) {
        for (let i = s.messages.length - 1; i >= 0; i--) {
          const msgTs = s.messages[i].timestamp || '';
          if (msgTs && msgTs <= req.created_at) {
            afterMessageTimestamp = msgTs;
            break;
          }
        }
      }
      // 没有 created_at 或没找到合适锚点时，回退到最后一条消息
      if (!afterMessageTimestamp) {
        const lastMsg = s.messages.length > 0 ? s.messages[s.messages.length - 1] : null;
        afterMessageTimestamp = lastMsg?.timestamp || '';
      }
      return { inlineApprovals: [...s.inlineApprovals, { ...req, afterMessageTimestamp }] };
    }),
  removeInlineApproval: (requestId) =>
    set((s) => ({
      inlineApprovals: s.inlineApprovals.filter((r) => r.id !== requestId),
    })),

  loadModels: async (client) => {
    try {
      const res = await client.listModels();
      set({ availableModels: res.models || [], activeModel: res.active || null });
    } catch {
      // 忽略错误，保留空列表
    }
  },

  setToolCallStatus: (id, status) => set((s) => {
    const startTimes = { ...s.toolCallStartTimes };
    // 记录开始时间
    if (status.status === 'running') {
      startTimes[id] = Date.now();
    }
    // 如果完成但没有 duration，用客户端计时
    if ((status.status === 'success' || status.status === 'error') && !status.duration && startTimes[id]) {
      status = { ...status, duration: Date.now() - startTimes[id] };
    }
    return {
      toolCallStatuses: { ...s.toolCallStatuses, [id]: status },
      toolCallStartTimes: startTimes,
    };
  }),

  stopTask: async (client, sessionId) => {
    try {
      await client.stopTask(sessionId);
      set({ sending: false, streaming: false, streamingMessageId: null, agentStatus: null });
    } catch (e) {
      console.error('停止任务失败', e);
    }
  },

  replaceStreamingMessage: (msg, streamId) => set((s) => {
    let msgs: Message[];
    if (streamId) {
      msgs = s.messages.filter(m => m.timestamp !== streamId);
    } else {
      // 降级：移除最后一个 stream- 前缀的 assistant 占位符
      const lastStreamIdx = s.messages.reduceRight(
        (found: number, m: Message, i: number) => found >= 0 ? found
          : (m.role === 'assistant' && (m.timestamp || '').startsWith('stream-')) ? i : -1,
        -1
      );
      msgs = lastStreamIdx >= 0
        ? [...s.messages.slice(0, lastStreamIdx), ...s.messages.slice(lastStreamIdx + 1)]
        : [...s.messages];
    }
    return {
      messages: appendMessage(msgs, msg),
      streaming: false,
      streamingMessageId: null,
    };
  }),

  confirmUserMessage: (timestamp) => set((s) => {
    // 原位更新 timestamp，不通过 appendMessage 重新排序
    // 避免服务端时钟偏快时用户消息排到错误消息之后
    const msgs = [...s.messages];
    for (let i = msgs.length - 1; i >= 0; i--) {
      if (msgs[i].role === 'user' && (msgs[i].timestamp || '').startsWith('temp-')) {
        msgs[i] = { ...msgs[i], timestamp };
        return { messages: msgs };
      }
    }
    return s;
  }),
}));
