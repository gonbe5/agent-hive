import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { AuthCallback } from '../AuthCallback';
import { useAuthStore } from '../../store/auth';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return { ...actual, useNavigate: () => mockNavigate };
});

vi.mock('../../store/auth', () => ({
  useAuthStore: vi.fn(),
}));

const mockUseAuthStore = vi.mocked(useAuthStore);

function buildStore(overrides = {}) {
  const defaults = {
    token: null as string | null,
    user: null as object | null,
    setAuth: vi.fn(),
  };
  return { ...defaults, ...overrides };
}

function mockLocationHash(hash: string) {
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    ...window.location,
    hash,
    pathname: '/auth/callback',
  } as Location);
  vi.spyOn(window.history, 'replaceState').mockImplementation(() => {});
}

function renderCallback(storeState: ReturnType<typeof buildStore>) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mockUseAuthStore.mockImplementation((selector?: (s: any) => unknown) => {
    if (typeof selector === 'function') return selector(storeState);
    return storeState;
  });
  return render(<MemoryRouter><AuthCallback /></MemoryRouter>);
}

beforeEach(() => {
  vi.clearAllMocks();
  mockNavigate.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('AuthCallback', () => {
  it('已登录 → 直接 navigate("/")', async () => {
    mockLocationHash('');
    const user = { id: '1', display_name: 'Test', email: '', avatar_url: '', department: '', role: 'user' };
    const store = buildStore({ token: 'existing-token', user });
    await act(async () => { renderCallback(store); });
    expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true });
  });

  it('无 hash → 显示"正在登录..."，5s 后 navigate("/login")', async () => {
    vi.useFakeTimers();
    mockLocationHash('');
    const store = buildStore();
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('正在登录...')).toBeInTheDocument();
    await act(async () => { vi.advanceTimersByTime(5000); });
    expect(mockNavigate).toHaveBeenCalledWith('/login', { replace: true });
    vi.useRealTimers();
  });

  it('hash=error=state_mismatch → 显示"授权超时，请重新登录"', async () => {
    mockLocationHash('#error=state_mismatch');
    const store = buildStore();
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('授权超时，请重新登录')).toBeInTheDocument();
  });

  it('hash=error=auth_failed → 显示"登录失败，请重试"', async () => {
    mockLocationHash('#error=auth_failed');
    const store = buildStore();
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('登录失败，请重试')).toBeInTheDocument();
  });

  it('hash=error=unknown_code → 显示默认错误', async () => {
    mockLocationHash('#error=unknown_code');
    const store = buildStore();
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('登录失败，请重试')).toBeInTheDocument();
  });

  it('hash=token → fetch /auth/me 成功 → setAuth + navigate("/")', async () => {
    mockLocationHash('#token=jwt-token');
    const setAuth = vi.fn();
    const store = buildStore({ setAuth });
    const user = { id: '1', display_name: 'Test', email: '', avatar_url: '', department: '', role: 'user' };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue(user),
    }));
    await act(async () => { renderCallback(store); });
    expect(setAuth).toHaveBeenCalledWith('jwt-token', user);
    expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true });
  });

  it('hash=token → fetch /auth/me 返回 401 → 显示错误', async () => {
    mockLocationHash('#token=invalid-token');
    const store = buildStore();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 401 }));
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('登录验证失败，请重试')).toBeInTheDocument();
  });

  it('hash=token → fetch /auth/me 网络错误 → 显示错误', async () => {
    mockLocationHash('#token=jwt-token');
    const store = buildStore();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Network Error')));
    await act(async () => { renderCallback(store); });
    expect(screen.getByText('网络错误，请重试')).toBeInTheDocument();
  });
});
