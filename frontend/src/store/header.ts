import { type ReactNode } from 'react';
import { create } from 'zustand';

interface HeaderSlots {
  /** 侧栏按钮右侧插入的额外元素（如返回按钮） */
  leftExtra: ReactNode | null;
  /** 覆盖中间标题的自定义内容 */
  centerOverride: ReactNode | null;
  /** 插在右侧固定图标（语言/主题/连接状态）左边的额外内容 */
  rightExtra: ReactNode | null;
  setSlots: (slots: Partial<Omit<HeaderSlots, 'setSlots' | 'clearSlots'>>) => void;
  clearSlots: () => void;
}

export const useHeaderStore = create<HeaderSlots>((set) => ({
  leftExtra: null,
  centerOverride: null,
  rightExtra: null,
  setSlots: (slots) => set(slots),
  clearSlots: () => set({ leftExtra: null, centerOverride: null, rightExtra: null }),
}));
