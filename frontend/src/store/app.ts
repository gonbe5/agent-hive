import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import type { NodeClient } from '../api/node-client';
import { LocalNodeClient } from '../api/node-client';
import { applyTheme } from '../utils/applyTheme';

type AppMode = 'standalone' | 'hub';
type Theme = 'light' | 'dark';
type Language = 'en' | 'zh';

interface AppState {
  mode: AppMode;
  currentNodeId: string;
  nodeClient: NodeClient;
  sidebarOpen: boolean;
  theme: Theme;
  language: Language;
  // 操作
  setSidebarOpen: (open: boolean) => void;
  toggleSidebar: () => void;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
  setLanguage: (lang: Language) => void;
}

export const useAppStore = create<AppState>()(
  persist(
    (set, get) => ({
      mode: 'standalone',
      currentNodeId: 'local',
      nodeClient: new LocalNodeClient(),
      sidebarOpen: true,
      theme: 'dark',
      language: 'zh',

      setSidebarOpen: (open) => set({ sidebarOpen: open }),
      toggleSidebar: () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),

      setTheme: (theme) => {
        set({ theme });
        applyTheme(theme);
      },

      toggleTheme: () => {
        const newTheme = get().theme === 'light' ? 'dark' : 'light';
        get().setTheme(newTheme);
      },

      setLanguage: (language) => set({ language }),
    }),
    {
      name: 'app-storage',
      partialize: (state) => ({
        theme: state.theme,
        language: state.language,
        sidebarOpen: state.sidebarOpen,
      }),
      // 恢复状态后，同步主题到 DOM
      onRehydrateStorage: () => (state) => {
        if (state?.theme) {
          applyTheme(state.theme);
        }
      },
    }
  )
);
