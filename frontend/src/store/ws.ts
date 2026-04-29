import { create } from 'zustand';
import type { WSMessage } from '../types/api';

interface WsState {
  connected: boolean;
  send: ((msg: WSMessage) => void) | null;
  setConnected: (connected: boolean) => void;
  setSend: (fn: ((msg: WSMessage) => void) | null) => void;
}

/**
 * wsStore — 全局 WebSocket 状态共享
 *
 * AppShell 建立 WebSocket 后写入 send 和 connected，
 * 子组件/hook 读取后可直接使用 WS 通道，无需 prop drilling。
 */
export const useWsStore = create<WsState>((set) => ({
  connected: false,
  send: null,
  setConnected: (connected) => set({ connected }),
  setSend: (fn) => set({ send: fn }),
}));
