import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useChatStore } from '../../store/chat'

// 捕获 useWebSocketConnection 被传入的 options.onDisconnected
const captured: { onDisconnected: (() => void) | null; onMessage: ((m: unknown) => void) | null; onConnected: (() => void) | null } = {
  onDisconnected: null,
  onMessage: null,
  onConnected: null,
}

vi.mock('../useWebSocketConnection', () => ({
  useWebSocketConnection: (opts: { onMessage?: (m: unknown) => void; onConnected?: () => void; onDisconnected?: () => void }) => {
    captured.onDisconnected = opts.onDisconnected ?? null
    captured.onMessage = opts.onMessage ?? null
    captured.onConnected = opts.onConnected ?? null
    return { connected: false, send: vi.fn() }
  },
}))

describe('useWebSocket.handleDisconnected 不清零 currentSessionId（R3）', () => {
  beforeEach(() => {
    useChatStore.setState({ currentSessionId: 'keep-me' })
    // 三个 ref 全部置空：防止未来新增 case 时 onMessage/onConnected 跨用例泄漏
    captured.onDisconnected = null
    captured.onMessage = null
    captured.onConnected = null
  })

  it('onDisconnected 触发后 currentSessionId 保持不变', async () => {
    const { useWebSocket } = await import('../useWebSocket')
    renderHook(() => useWebSocket({ url: 'ws://test', sessionId: 'keep-me', enabled: true }))

    expect(captured.onDisconnected).toBeTypeOf('function')
    act(() => captured.onDisconnected?.())

    expect(useChatStore.getState().currentSessionId).toBe('keep-me')
  })
})
