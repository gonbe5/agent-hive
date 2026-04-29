import { useEffect, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/auth';

export function AuthGuard({ children }: { children: ReactNode }) {
  const { loading, authEnabled, authError, user, token } = useAuthStore();
  const checkAuthEnabled = useAuthStore((s) => s.checkAuthEnabled);
  const checkAuth = useAuthStore((s) => s.checkAuth);
  const navigate = useNavigate();

  useEffect(() => {
    const init = async () => {
      // 1. 检测 auth 是否启用
      if (authEnabled === null) {
        const enabled = await checkAuthEnabled();
        if (!enabled) return; // auth 未启用或错误，不继续
        // 用 enabled 而非闭包里的 authEnabled（此时 authEnabled 仍是 null）
        if (enabled && !user) {
          const valid = await checkAuth();
          if (!valid) navigate('/login', { replace: true });
        }
        return;
      }
      // 2. auth 已知启用，检查 token
      if (authEnabled && !user) {
        const valid = await checkAuth();
        if (!valid) {
          navigate('/login', { replace: true });
        }
      }
    };
    init();
  }, [authEnabled, user, checkAuthEnabled, checkAuth, navigate]);

  // 错误页
  if (authError) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <p className="text-[var(--text-secondary)] mb-4">{authError}</p>
          <button
            onClick={() => window.location.reload()}
            className="px-4 py-2 rounded-[10px] bg-[var(--accent-600)] text-white"
          >
            重试
          </button>
        </div>
      </div>
    );
  }

  // loading
  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="animate-pulse text-[var(--text-secondary)]">加载中...</div>
      </div>
    );
  }

  // auth 未启用 → 直接渲染
  if (authEnabled === false) {
    return <>{children}</>;
  }

  // auth 启用但未登录 → 等待 redirect（useEffect 中处理）
  if (!user && !token) {
    return null;
  }

  return <>{children}</>;
}
