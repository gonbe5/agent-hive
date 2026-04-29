import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ApiClient, ApiRequestError } from '../client';

// mock store/auth 模块，避免 zustand 副作用
vi.mock('../../store/auth', () => ({
  useAuthStore: {
    getState: () => ({ clearAuth: vi.fn() }),
  },
  refreshToken: vi.fn(),
}));

import { refreshToken } from '../../store/auth';
const mockRefreshToken = vi.mocked(refreshToken);

// 辅助：构造 mock Response
function mockResponse(status: number, body: unknown, ok?: boolean): Response {
  return {
    status,
    ok: ok ?? (status >= 200 && status < 300),
    statusText: String(status),
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as unknown as Response;
}

describe('ApiClient 401 retry logic', () => {
  let client: ApiClient;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
let fetchSpy: ReturnType<typeof vi.spyOn<any, any>>;

  beforeEach(() => {
    client = new ApiClient('', 30000);
    fetchSpy = vi.spyOn(globalThis, 'fetch');
    mockRefreshToken.mockReset();
    localStorage.clear();
    // 重置 isRedirecting flag（通过重新导入模块无法做到，但 5s timeout 会自动重置）
    // 测试中不触发 redirect 路径，所以不需要特殊处理
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('401 → refreshToken 成功 → retry 成功 → 返回数据', async () => {
    const data = { id: 1 };
    fetchSpy
      .mockResolvedValueOnce(mockResponse(401, {}, false))
      .mockResolvedValueOnce(mockResponse(200, data));
    mockRefreshToken.mockResolvedValueOnce('new-token');

    const result = await client.get('/api/test');

    expect(result).toEqual(data);
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    // 第二次请求应带新 token
    const secondCall = fetchSpy.mock.calls[1];
    expect((secondCall[1] as RequestInit & { headers: Record<string, string> }).headers['Authorization']).toBe('Bearer new-token');
  });

  it('401 → refreshToken 成功 → retry 失败 → 抛出实际错误（非 Unauthorized）', async () => {
    fetchSpy
      .mockResolvedValueOnce(mockResponse(401, {}, false))
      .mockResolvedValueOnce(mockResponse(403, { error: 'forbidden', code: 403 }, false));
    mockRefreshToken.mockResolvedValueOnce('new-token');

    await expect(client.get('/api/test')).rejects.toMatchObject({
      message: 'forbidden',
      code: 403,
    });
  });

  it('401 → refreshToken 返回 null → 抛出 Unauthorized 401', async () => {
    // 模拟 window.location.href 赋值（jsdom 中不会真正跳转）
    const locationSpy = vi.spyOn(window, 'location', 'get').mockReturnValue({
      ...window.location,
      href: '',
    } as Location);

    fetchSpy.mockResolvedValueOnce(mockResponse(401, {}, false));
    mockRefreshToken.mockResolvedValueOnce(null);

    await expect(client.get('/api/test')).rejects.toMatchObject({
      code: 401,
    });

    locationSpy.mockRestore();
  });

  it('非 401 错误 → 直接抛出，不触发 refresh', async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(500, { error: 'server error', code: 500 }, false));

    await expect(client.get('/api/test')).rejects.toMatchObject({
      message: 'server error',
      code: 500,
    });
    expect(mockRefreshToken).not.toHaveBeenCalled();
  });

  it('/auth/ 路径的 401 不触发 refresh（防循环）', async () => {
    fetchSpy.mockResolvedValueOnce(mockResponse(401, {}, false));

    await expect(client.get('/api/v1/auth/refresh')).rejects.toBeInstanceOf(Error);
    expect(mockRefreshToken).not.toHaveBeenCalled();
  });

  it('ApiRequestError 包含正确的 name 和 code', () => {
    const err = new ApiRequestError('test error', 422);
    expect(err.name).toBe('ApiRequestError');
    expect(err.code).toBe(422);
    expect(err.message).toBe('test error');
  });
});
