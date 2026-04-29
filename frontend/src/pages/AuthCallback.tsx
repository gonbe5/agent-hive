import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuthStore } from '../store/auth';

// 错误码 → 友好提示映射
const ERROR_MESSAGES: Record<string, string> = {
  state_mismatch: '授权超时，请重新登录',
  auth_failed: '登录失败，请重试',
  provider_error: '服务暂时不可用，请稍后重试',
  internal_error: '系统错误，请联系管理员',
  user_denied: '您取消了授权',
  user_disabled: '账号已被禁用，请联系管理员',
  rate_limited: '操作过于频繁，请稍后重试',
};

export function AuthCallback() {
  const { token, user } = useAuthStore();
  const setAuth = useAuthStore((s) => s.setAuth);
  const navigate = useNavigate();
  const [error, setError] = useState('');

  useEffect(() => {
    // 已登录 → 直接跳首页（浏览器后退场景）
    if (token && user) {
      navigate('/', { replace: true });
      return;
    }

    const hash = window.location.hash.slice(1); // 去掉 #
    // 立即清除 hash（安全：防止 token 留在浏览器历史）
    window.history.replaceState(null, '', window.location.pathname);

    if (!hash) {
      // 无 hash → 超时后跳转登录页
      const timer = setTimeout(() => navigate('/login', { replace: true }), 5000);
      return () => clearTimeout(timer);
    }

    const params = new URLSearchParams(hash);
    const tokenValue = params.get('token');
    const errorValue = params.get('error');

    if (errorValue) {
      setError(ERROR_MESSAGES[errorValue] || '登录失败，请重试');
      return;
    }

    if (tokenValue) {
      // 先调 /auth/me 获取 user，成功后再 setAuth
      // 注意：此处直接用 fetch 而非 apiClient，因为 token 尚未存入 localStorage
      (async () => {
        try {
          const resp = await fetch('/api/v1/auth/me', {
            headers: { Authorization: `Bearer ${tokenValue}` },
          });
          if (!resp.ok) {
            setError('登录验证失败，请重试');
            return;
          }
          const userData = await resp.json();
          setAuth(tokenValue, userData);
          console.info('[auth] callback: token verified, navigating to /');
          navigate('/', { replace: true });
        } catch {
          setError('网络错误，请重试');
        }
      })();
    } else {
      // hash 存在但既无 token 也无 error（格式异常）→ 跳转登录页
      navigate('/login', { replace: true });
    }
  }, []);

  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--bg-primary)]">
      <div className="text-center">
        {error ? (
          <>
            <p className="text-[var(--text-primary)] mb-4">{error}</p>
            <button
              onClick={() => navigate('/login', { replace: true })}
              className="px-4 py-2 rounded-[10px] bg-[var(--accent-600)] text-white text-sm"
            >
              重新登录
            </button>
          </>
        ) : (
          <div className="animate-pulse text-[var(--text-secondary)]">正在登录...</div>
        )}
      </div>
    </div>
  );
}
