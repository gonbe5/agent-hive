import { useEffect, useRef, useCallback } from 'react';
import { useWebSocketConnection } from './useWebSocketConnection';
import { useReplayStore } from '../store/replay';
import { useNodeClient } from './useNodeClient';
import type { WSMessage } from '../types/api';
import type { JournalEvent } from '../types/journal';

interface AgentStatusPayload {
  session_id?: string;
  status: 'thinking' | 'tool_calling' | 'warning' | 'completed' | 'error';
  error?: string;
  warning?: string;
}

interface UseReplayWebSocketOptions {
  url: string;
  sessionId: string;
  enabled?: boolean;
}

/**
 * 回放专用 WebSocket hook。
 * 基于 useWebSocketConnection，监听 journal_event + agent_status，
 * 含指数退避对账逻辑。
 */
export function useReplayWebSocket({ url, sessionId, enabled = true }: UseReplayWebSocketOptions) {
  const client = useNodeClient();
  const wsEventCount = useRef(0);
  const reconciling = useRef(false);

  const handleMessage = useCallback((msg: WSMessage) => {
    if (msg.type === 'journal_event') {
      const event = msg.payload as JournalEvent;
      useReplayStore.getState().appendLiveEvent(event);
      wsEventCount.current++;
    }

    if (msg.type === 'agent_status') {
      const payload = msg.payload as AgentStatusPayload;
      if (payload.session_id && payload.session_id !== sessionId) return;

      if (payload.status === 'completed' || payload.status === 'error') {
        // session 结束，触发对账
        reconcile();
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId]);

  const reconcile = useCallback(async () => {
    if (!client || reconciling.current) return;
    reconciling.current = true;

    const delays = [1000, 2000, 4000];
    const currentWsCount = wsEventCount.current;

    for (const delay of delays) {
      await sleep(delay);
      try {
        const res = await client.getSessionJournal(sessionId);
        if (res.events.length >= currentWsCount) {
          useReplayStore.getState().setEvents(res.events);
          reconciling.current = false;
          return;
        }
      } catch {
        // 继续重试
      }
    }

    // 超过重试次数，以最后一次 REST 结果为准
    try {
      const res = await client.getSessionJournal(sessionId);
      useReplayStore.getState().setEvents(res.events);
    } catch {
      // 静默失败
    }
    reconciling.current = false;
  }, [client, sessionId]);

  const onConnected = useCallback(() => {
    // 连接建立后，如果 session 正在执行，切换到 live 模式
    const store = useReplayStore.getState();
    if (store.mode === 'ready' || store.mode === 'empty') {
      // 保持当前模式，等 journal_event 到来时自动进入 live
    }
  }, []);

  const { connected } = useWebSocketConnection({
    url,
    sessionId,
    enabled,
    onMessage: handleMessage,
    onConnected,
  });

  // 收到第一个 journal_event 时自动切换到 live 模式
  useEffect(() => {
    const unsub = useReplayStore.subscribe((state, prev) => {
      if (state.events.length > prev.events.length && state.mode !== 'live' && state.mode !== 'playing') {
        // 有新事件追加且不在播放中，切换到 live
        if (wsEventCount.current > 0) {
          useReplayStore.getState().setMode('live');
        }
      }
    });
    return unsub;
  }, []);

  // 清理
  useEffect(() => {
    return () => {
      wsEventCount.current = 0;
      reconciling.current = false;
    };
  }, [sessionId]);

  return { connected };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
