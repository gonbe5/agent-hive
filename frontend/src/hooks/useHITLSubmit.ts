import { useCallback } from 'react';
import { useHITLStore } from '../store/hitl';
import { useChatStore } from '../store/chat';
import { useWsStore } from '../store/ws';
import { useToastStore } from '../store/toast';
import { useNodeClient } from './useNodeClient';
import type { InputRequest, InputResponse } from '../types/api';

/**
 * useHITLSubmit — 审批响应提交 hook
 *
 * 提交逻辑：WebSocket 在线时直接发送，断线时走 REST API fallback。
 * 提交成功后同时从 HITL store 和 chat store 移除该审批请求。
 */
export function useHITLSubmit() {
  const client = useNodeClient();
  const removeRequest = useHITLStore((s) => s.removeRequest);
  const removeInlineApproval = useChatStore((s) => s.removeInlineApproval);
  const addToast = useToastStore((s) => s.addToast);
  // 从全局 WS store 读取连接状态，无需 prop drilling
  const connected = useWsStore((s) => s.connected);
  const wsSend = useWsStore((s) => s.send);

  const submitApproval = useCallback(async (req: InputRequest, resp: InputResponse) => {
    if (connected && wsSend) {
      // WebSocket 在线时直接发送
      wsSend({ type: 'input_response', payload: resp });
      removeRequest(req.id);
      removeInlineApproval(req.id);
    } else {
      // WebSocket 断线时通过 REST API 提交
      try {
        await client.submitInput(req.task_id, resp);
        removeRequest(req.id);
        removeInlineApproval(req.id);
      } catch (e) {
        const msg = e instanceof Error ? e.message : '提交审批响应失败';
        addToast('error', msg + '，请稍后重试');
      }
    }
  }, [connected, wsSend, removeRequest, removeInlineApproval, client, addToast]);

  return { submitApproval };
}
