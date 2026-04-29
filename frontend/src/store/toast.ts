import { create } from 'zustand';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface Toast {
  id: string;
  type: ToastType;
  message: string;
  duration?: number;
}

interface ToastState {
  toasts: Toast[];
  addToast: (type: ToastType, message: string, duration?: number) => void;
  removeToast: (id: string) => void;
  clearAll: () => void;
}

// 存储每个 toast 的定时器 ID，用于手动移除时清理
const timerMap = new Map<string, ReturnType<typeof setTimeout>>();

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],

  addToast: (type, message, duration = 5000) => {
    const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
    const toast: Toast = { id, type, message, duration };

    set((s) => ({
      toasts: [...s.toasts, toast],
    }));

    // 自动移除
    if (duration > 0) {
      const timer = setTimeout(() => {
        timerMap.delete(id);
        set((s) => ({
          toasts: s.toasts.filter((t) => t.id !== id),
        }));
      }, duration);
      timerMap.set(id, timer);
    }
  },

  removeToast: (id) => {
    // 清除对应的定时器，防止重复删除
    const timer = timerMap.get(id);
    if (timer) {
      clearTimeout(timer);
      timerMap.delete(id);
    }
    set((s) => ({
      toasts: s.toasts.filter((t) => t.id !== id),
    }));
  },

  clearAll: () => {
    // 清除所有定时器
    timerMap.forEach((timer) => clearTimeout(timer));
    timerMap.clear();
    set({ toasts: [] });
  },
}));
