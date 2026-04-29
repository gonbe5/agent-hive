import { create } from 'zustand';
import type { InputRequest, InputResponse } from '../types/api';
import type { NodeClient } from '../api/node-client';
import { useChatStore } from './chat';

interface HITLState {
  pendingRequests: InputRequest[];
  loading: boolean;
  error: string | null;
  // 操作
  addRequest: (req: InputRequest) => void;
  removeRequest: (requestId: string) => void;
  submitResponse: (client: NodeClient, taskId: string, resp: InputResponse) => Promise<void>;
  fetchPending: (client: NodeClient, taskId: string) => Promise<void>;
  clearAll: () => void;
}

export const useHITLStore = create<HITLState>((set) => ({
  pendingRequests: [],
  loading: false,
  error: null,

  addRequest: (req) =>
    set((s) => {
      // 去重
      if (s.pendingRequests.some((r) => r.id === req.id)) return s;
      return { pendingRequests: [...s.pendingRequests, req] };
    }),

  removeRequest: (requestId) =>
    set((s) => ({
      pendingRequests: s.pendingRequests.filter((r) => r.id !== requestId),
    })),

  submitResponse: async (client, taskId, resp) => {
    try {
      await client.submitInput(taskId, resp);
      set((s) => ({
        pendingRequests: s.pendingRequests.filter((r) => r.id !== resp.request_id),
      }));
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : '提交响应失败';
      set({ error: errorMsg });
    }
  },

  fetchPending: async (client, taskId) => {
    set({ loading: true });
    try {
      const pending = (await client.getPendingInput(taskId)) as InputRequest[];
      set({ pendingRequests: pending, loading: false });
      // 同步到 chat store，确保审批卡在消息列表中内联渲染
      const chatStore = useChatStore.getState();
      for (const req of pending) {
        chatStore.addInlineApproval(req);
      }
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : '获取待处理请求失败';
      set({ error: errorMsg, loading: false });
    }
  },

  clearAll: () => set({ pendingRequests: [], error: null }),
}));
