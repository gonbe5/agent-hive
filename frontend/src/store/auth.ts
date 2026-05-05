import { create } from 'zustand';
import { apiClient } from '../api/client';

interface User {
  id: string;
  display_name: string;
  email: string;
  avatar_url: string;
  department: string;
  role: 'user' | 'admin';
}

export interface AuthProvider {
  name: string;
  provider_type: 'feishu' | 'dingtalk' | 'ldap';
  enabled: boolean;
}

interface AuthState {
  user: User | null;
  token: string | null;
  loading: boolean;
  authEnabled: boolean | null; // null=未检测
  authError: string | null;    // 5xx 等不可降级错误

  setAuth: (token: string, user: User) => void;
  clearAuth: () => void;
  logout: () => void;
  checkAuth: () => Promise<boolean>;
  checkAuthEnabled: () => Promise<boolean>;
  fetchProviders: () => Promise<AuthProvider[]>;
}

// refresh 锁：防止并发 refresh
let refreshPromise: Promise<string | null> | null = null;
const DEFAULT_REFRESH_SKEW_MS = 60_000;

function decodeJWTPayload(token: string): { exp?: number } | null {
  const parts = token.split('.');
  if (parts.length < 2) return null;
  try {
    const normalized = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
    return JSON.parse(atob(padded)) as { exp?: number };
  } catch {
    return null;
  }
}

export function shouldRefreshToken(token: string | null, skewMs = DEFAULT_REFRESH_SKEW_MS): boolean {
  if (!token) return false;
  const payload = decodeJWTPayload(token);
  if (!payload?.exp) return true;
  return payload.exp * 1000 <= Date.now() + skewMs;
}

export const useAuthStore = create<AuthState>((set, get) => ({
  user: null,
  token: localStorage.getItem('auth_token'),
  loading: true,
  authEnabled: null,
  authError: null,

  clearAuth: () => {
    localStorage.removeItem('auth_token');
    set({ token: null, user: null, loading: false });
    console.info('[auth] clearAuth: token removed');
  },

  setAuth: (token, user) => {
    localStorage.setItem('auth_token', token);
    set({ token, user, loading: false, authEnabled: true, authError: null });
    console.info('[auth] setAuth: user=%s', user.display_name);
  },

  logout: () => {
    get().clearAuth();
    window.location.href = '/login';
  },

  checkAuthEnabled: async () => {
    try {
      const data = await apiClient.get<{ enabled: boolean }>('/api/v1/auth/status');
      console.info('[auth] checkAuthEnabled: enabled=%s', data.enabled);
      set({ authEnabled: data.enabled });
      if (!data.enabled) set({ loading: false });
      return data.enabled;
    } catch (err) {
      // 404 = 旧版后端，auth 未启用
      if (err instanceof Error && 'code' in err && (err as { code: number }).code === 404) {
        console.info('[auth] checkAuthEnabled: 404, auth not available');
        set({ authEnabled: false, loading: false });
        return false;
      }
      // 5xx = 后端故障，重试 1 次
      console.warn('[auth] checkAuthEnabled: error, retrying...', err);
      try {
        const data = await apiClient.get<{ enabled: boolean }>('/api/v1/auth/status');
        set({ authEnabled: data.enabled });
        if (!data.enabled) set({ loading: false });
        return data.enabled;
      } catch {
        // 重试仍失败 → 显示错误页，不降级
        console.error('[auth] checkAuthEnabled: retry failed, showing error');
        set({ authError: '服务不可用，请稍后重试', loading: false });
        return false;
      }
    }
  },

  checkAuth: async () => {
    const token = get().token;
    if (!token) {
      set({ loading: false });
      return false;
    }
    try {
      const user = await apiClient.get<User>('/api/v1/auth/me');
      set({ user, loading: false });
      console.info('[auth] checkAuth: success, user=%s', user.display_name);
      return true;
    } catch {
      // 401、超时、网络错误 → 统一清除 token
      console.info('[auth] checkAuth: failed, clearing auth');
      get().clearAuth();
      return false;
    }
  },

  fetchProviders: async () => {
    try {
      // 后端返回 { providers: [...] }
      const data = await apiClient.get<{ providers: AuthProvider[] }>('/api/v1/auth/providers');
      return data.providers ?? [];
    } catch {
      return [];
    }
  },
}));

// 导出 refresh 锁方法，供 ApiClient 401 拦截调用
export async function refreshToken(): Promise<string | null> {
  if (refreshPromise) return refreshPromise;
  refreshPromise = (async () => {
    try {
      const data = await apiClient.post<{ token: string }>('/api/v1/auth/refresh');
      localStorage.setItem('auth_token', data.token);
      useAuthStore.setState({ token: data.token });
      console.info('[auth] refreshToken: success');
      return data.token;
    } catch {
      console.info('[auth] refreshToken: failed');
      useAuthStore.getState().clearAuth();
      return null;
    } finally {
      refreshPromise = null;
    }
  })();
  return refreshPromise;
}

export async function ensureFreshToken(options: { force?: boolean; skewMs?: number } = {}): Promise<string | null> {
  const token = localStorage.getItem('auth_token');
  if (!token) return null;
  if (!options.force && !shouldRefreshToken(token, options.skewMs ?? DEFAULT_REFRESH_SKEW_MS)) {
    return token;
  }
  return refreshToken();
}
