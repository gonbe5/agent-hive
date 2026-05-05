import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { useWebSocketConnection } from '../useWebSocketConnection';
import { ensureFreshToken } from '../../store/auth';

vi.mock('../../store/auth', () => ({
  ensureFreshToken: vi.fn(),
}));

const mockEnsureFreshToken = vi.mocked(ensureFreshToken);
const capturedCalls: Array<{ url: string; protocols?: string | string[] }> = [];
const instances: MockWS[] = [];

class MockWS {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState = MockWS.CONNECTING;
  onopen: ((event?: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event?: Event) => void) | null = null;

  constructor(url: string, protocols?: string | string[]) {
    capturedCalls.push({ url, protocols });
    instances.push(this);
  }

  close() {}
  send() {}
}

describe('useWebSocketConnection token refresh', () => {
  beforeEach(() => {
    capturedCalls.length = 0;
    instances.length = 0;
    localStorage.clear();
    vi.clearAllMocks();
    vi.stubGlobal('WebSocket', MockWS);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('建连前先确保 token 新鲜，并用刷新后的 token 创建 WebSocket', async () => {
    localStorage.setItem('auth_token', 'expired-token');
    mockEnsureFreshToken.mockResolvedValueOnce('fresh-token');

    renderHook(() => useWebSocketConnection({ url: 'ws://h/api', sessionId: 'sid-1', enabled: true }));

    await waitFor(() => expect(capturedCalls).toHaveLength(1));
    expect(mockEnsureFreshToken).toHaveBeenCalledWith();
    expect(capturedCalls[0].protocols).toEqual(['bearer-fresh-token', 'v1']);
  });

  it('4401 close 后强制 refresh，再用新 token 重连', async () => {
    mockEnsureFreshToken
      .mockResolvedValueOnce('first-token')
      .mockResolvedValueOnce('second-token');

    renderHook(() => useWebSocketConnection({ url: 'ws://h/api', sessionId: 'sid-1', enabled: true }));

    await waitFor(() => expect(capturedCalls).toHaveLength(1));
    act(() => {
      instances[0].onclose?.({ code: 4401 } as CloseEvent);
    });
    await waitFor(() => expect(capturedCalls).toHaveLength(2));

    expect(mockEnsureFreshToken).toHaveBeenNthCalledWith(1);
    expect(mockEnsureFreshToken).toHaveBeenNthCalledWith(2, { force: true });
    expect(capturedCalls[1].protocols).toEqual(['bearer-second-token', 'v1']);
  });
});
