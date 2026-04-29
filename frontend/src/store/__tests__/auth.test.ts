import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { useAuthStore, refreshToken } from '../auth';
import { apiClient } from '../../api/client';

// mock apiClient
vi.mock('../../api/client', () => ({
  apiClient: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

const mockGet = vi.mocked(apiClient.get);
const mockPost = vi.mocked(apiClient.post);

beforeEach(() => {
  // 重置 store 状态
  useAuthStore.setState({
    user: null,
    token: null,
    loading: true,
    authEnabled: null,
    authError: null,
  });
  localStorage.clear();
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('clearAuth', () => {
  it('清除 localStorage 并重置 state', () => {
    localStorage.setItem('auth_token', 'test-token');
    useAuthStore.setState({ token: 'test-token', user: { id: '1', display_name: '', email: '', avatar_url: '', department: '', role: 'user' as const } });

    useAuthStore.getState().clearAuth();

    expect(localStorage.getItem('auth_token')).toBeNull();
    const { token, user, loading } = useAuthStore.getState();
    expect(token).toBeNull();
    expect(user).toBeNull();
    expect(loading).toBe(false);
  });
});

describe('setAuth', () => {
  it('存 localStorage 并更新 state', () => {
    const user = { id: '1', display_name: 'Test', email: '', avatar_url: '', department: '', role: 'user' as const };
    useAuthStore.getState().setAuth('my-token', user);

    expect(localStorage.getItem('auth_token')).toBe('my-token');
    const { token, user: stateUser, loading, authEnabled } = useAuthStore.getState();
    expect(token).toBe('my-token');
    expect(stateUser).toEqual(user);
    expect(loading).toBe(false);
    expect(authEnabled).toBe(true);
  });
});

describe('checkAuthEnabled', () => {
  it('200 + enabled=true → 返回 true', async () => {
    mockGet.mockResolvedValueOnce({ enabled: true });

    const result = await useAuthStore.getState().checkAuthEnabled();

    expect(result).toBe(true);
    expect(useAuthStore.getState().authEnabled).toBe(true);
  });

  it('200 + enabled=false → 返回 false，loading=false', async () => {
    mockGet.mockResolvedValueOnce({ enabled: false });

    const result = await useAuthStore.getState().checkAuthEnabled();

    expect(result).toBe(false);
    expect(useAuthStore.getState().authEnabled).toBe(false);
    expect(useAuthStore.getState().loading).toBe(false);
  });

  it('404 → 返回 false，authEnabled=false', async () => {
    const err = Object.assign(new Error('Not Found'), { code: 404 });
    mockGet.mockRejectedValueOnce(err);

    const result = await useAuthStore.getState().checkAuthEnabled();

    expect(result).toBe(false);
    expect(useAuthStore.getState().authEnabled).toBe(false);
  });

  it('5xx 重试成功 → 返回 true', async () => {
    const err = Object.assign(new Error('Server Error'), { code: 500 });
    mockGet
      .mockRejectedValueOnce(err)
      .mockResolvedValueOnce({ enabled: true });

    const result = await useAuthStore.getState().checkAuthEnabled();

    expect(result).toBe(true);
    expect(mockGet).toHaveBeenCalledTimes(2);
  });

  it('5xx 重试仍失败 → authError 设置', async () => {
    const err = Object.assign(new Error('Server Error'), { code: 500 });
    mockGet.mockRejectedValue(err);

    const result = await useAuthStore.getState().checkAuthEnabled();

    expect(result).toBe(false);
    expect(useAuthStore.getState().authError).toBeTruthy();
  });
});

describe('checkAuth', () => {
  it('无 token → 返回 false，loading=false', async () => {
    useAuthStore.setState({ token: null });

    const result = await useAuthStore.getState().checkAuth();

    expect(result).toBe(false);
    expect(useAuthStore.getState().loading).toBe(false);
  });

  it('有 token + 200 → 返回 true，user 更新', async () => {
    useAuthStore.setState({ token: 'valid-token' });
    const user = { id: '1', display_name: 'Test', email: '', avatar_url: '', department: '', role: 'user' as const };
    mockGet.mockResolvedValueOnce(user);

    const result = await useAuthStore.getState().checkAuth();

    expect(result).toBe(true);
    expect(useAuthStore.getState().user).toEqual(user);
  });

  it('有 token + 请求失败 → clearAuth 被调用', async () => {
    useAuthStore.setState({ token: 'expired-token' });
    localStorage.setItem('auth_token', 'expired-token');
    mockGet.mockRejectedValueOnce(new Error('Unauthorized'));

    const result = await useAuthStore.getState().checkAuth();

    expect(result).toBe(false);
    expect(useAuthStore.getState().token).toBeNull();
    expect(localStorage.getItem('auth_token')).toBeNull();
  });
});

describe('refreshToken', () => {
  it('成功刷新 → 返回新 token', async () => {
    mockPost.mockResolvedValueOnce({ token: 'new-token' });

    const token = await refreshToken();

    expect(token).toBe('new-token');
    expect(localStorage.getItem('auth_token')).toBe('new-token');
    expect(useAuthStore.getState().token).toBe('new-token');
  });

  it('刷新失败 → clearAuth，返回 null', async () => {
    localStorage.setItem('auth_token', 'old-token');
    useAuthStore.setState({ token: 'old-token' });
    mockPost.mockRejectedValueOnce(new Error('Unauthorized'));

    const token = await refreshToken();

    expect(token).toBeNull();
    expect(useAuthStore.getState().token).toBeNull();
  });
});
