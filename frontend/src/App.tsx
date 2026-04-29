import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
import { AppShell } from './layouts/AppShell';
import { AdminShell } from './layouts/AdminShell';
import { ChatLanding } from './pages/ChatLanding';
import { Dashboard } from './pages/Dashboard';
import { Chat } from './pages/Chat';
import { Agents } from './pages/Agents';
import { Skills } from './pages/Skills';
import { Guide } from './pages/Guide';
import { AdminSettings } from './pages/AdminSettings';
import { Login } from './pages/Login';
import { AuthCallback } from './pages/AuthCallback';
import { UserList } from './pages/admin/UserList';
import { UsageStats } from './pages/admin/UsageStats';
import { AuthProviders } from './pages/admin/AuthProviders';
import { PromptManager } from './pages/admin/PromptManager';
import { LLMProviders } from './pages/admin/LLMProviders';
import { SessionReplay } from './pages/SessionReplay';
import { ReplayGallery } from './pages/ReplayGallery';
import { useTheme } from './hooks/useTheme';
import { useLanguage } from './hooks/useLanguage';
import { useAppStore } from './store/app';
import { ErrorBoundary } from './components/common/ErrorBoundary';
import { AuthGuard } from './components/AuthGuard';
import { AdminGuard } from './components/AdminGuard';

export default function App() {
  // 初始化主题和语言 (hooks 内部执行副作用)
  useTheme();
  useLanguage();

  useEffect(() => {
    // 检测系统主题偏好和浏览器语言（仅在首次加载时）
    const stored = localStorage.getItem('app-storage');
    if (!stored) {
      // 没有保存的设置，使用系统默认
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      useAppStore.getState().setTheme(prefersDark ? 'dark' : 'light');

      const browserLang = navigator.language.startsWith('zh') ? 'zh' : 'en';
      useAppStore.getState().setLanguage(browserLang);
    }
  }, []);

  return (
    <ErrorBoundary>
      <BrowserRouter>
        <Routes>
          {/* 公开路由 */}
          <Route path="/login" element={<Login />} />
          <Route path="/auth/callback" element={<AuthCallback />} />

          {/* 受保护路由 — AuthGuard 包裹 */}
          <Route element={<AuthGuard><Outlet /></AuthGuard>}>
            <Route element={<AppShell />}>
              <Route path="/" element={<ChatLanding />} />
              <Route path="/sessions/:id" element={<Chat />} />
              <Route path="/replay" element={<ReplayGallery />} />
              <Route path="/guide" element={<Guide />} />
            </Route>

            {/* 回放页面（独立全屏布局，无 Sidebar） */}
            <Route path="/sessions/:id/replay" element={<SessionReplay />} />

            {/* 旧路由重定向到管理后台 */}
            <Route path="/agents" element={<Navigate to="/admin/agents" replace />} />
            <Route path="/skills" element={<Navigate to="/admin/skills" replace />} />

            {/* 管理后台路由 */}
            <Route element={<AdminShell />}>
              <Route path="/admin" element={<Dashboard />} />
              <Route path="/admin/agents" element={<Agents />} />
              <Route path="/admin/skills" element={<AdminGuard><Skills /></AdminGuard>} />
              <Route path="/admin/settings" element={<AdminGuard><AdminSettings /></AdminGuard>} />
              <Route path="/admin/guide" element={<Guide />} />
              {/* Admin-only 页面 */}
              <Route path="/admin/users" element={<AdminGuard><UserList /></AdminGuard>} />
              <Route path="/admin/usage" element={<AdminGuard><UsageStats /></AdminGuard>} />
              <Route path="/admin/auth-providers" element={<AdminGuard><AuthProviders /></AdminGuard>} />
              <Route path="/admin/prompts" element={<AdminGuard><PromptManager /></AdminGuard>} />
              <Route path="/admin/llm" element={<AdminGuard><LLMProviders /></AdminGuard>} />
            </Route>
          </Route>
        </Routes>
      </BrowserRouter>
    </ErrorBoundary>
  );
}
