import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, cleanup } from '@testing-library/react'
import React from 'react'
import { useChatStore } from '../../../store/chat'
import type { WSMessage } from '../../../types/api'

const captured: {
  onMessage: ((m: WSMessage) => void) | null
} = { onMessage: null }

vi.mock('../../../hooks/useWebSocketConnection', () => ({
  useWebSocketConnection: (opts: {
    onMessage?: (m: WSMessage) => void
    onConnected?: () => void
    onDisconnected?: () => void
  }) => {
    captured.onMessage = opts.onMessage ?? null
    return { connected: true, send: vi.fn() }
  },
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, defaultValue?: string) => defaultValue ?? key,
    i18n: { language: 'zh' },
  }),
  initReactI18next: { type: '3rdParty', init: () => {} },
  Trans: ({ children }: { children?: React.ReactNode }) => children,
}))

import { useWebSocket } from '../../../hooks/useWebSocket'
import { MessageList } from '../MessageList'

function Harness() {
  useWebSocket({ url: 'ws://t', sessionId: 'test-sid', enabled: true })
  const messages = useChatStore((s) => s.messages)
  return <MessageList messages={messages} />
}

describe('partial 分支 → DOM 渲染 integration（R2）', () => {
  beforeEach(() => {
    captured.onMessage = null
    useChatStore.setState({
      messages: [],
      currentSessionId: 'test-sid',
      streamingMessageId: null,
      streaming: false,
      sending: false,
      agentStatus: null,
      inlineApprovals: [],
      toolCallStatuses: {},
      toolCallStartTimes: {},
      error: null,
    })
  })

  afterEach(() => cleanup())

  async function flushRaf() {
    await act(async () => {
      await new Promise<void>((r) => requestAnimationFrame(() => r()))
    })
  }

  it('partial 首帧可见（断言 A）+ 同节点更新（断言 B/C/D）', async () => {
    render(<Harness />)
    expect(captured.onMessage).toBeTypeOf('function')

    await act(async () => {
      captured.onMessage!({
        type: 'message',
        payload: {
          session_id: 'test-sid',
          partial: true,
          content: '首个可见片段',
          role: 'assistant',
        },
      } as WSMessage)
    })
    await flushRaf()

    const firstNode = await screen.findByText(/首个可见片段/)
    expect(firstNode).toBeInTheDocument()

    await act(async () => {
      captured.onMessage!({
        type: 'message',
        payload: {
          session_id: 'test-sid',
          partial: true,
          content: '首个可见片段 + 第二段',
          role: 'assistant',
        },
      } as WSMessage)
    })
    await flushRaf()

    await screen.findByText(/首个可见片段 \+ 第二段/)

    // 断言 B：store 层 assistant 消息计数未增加（仍只有 1 条流式 placeholder）
    const assistantCount = useChatStore
      .getState()
      .messages.filter((m) => m.role === 'assistant').length
    expect(assistantCount).toBe(1)

    // 断言 C：同节点被更新而不是被替换
    expect(firstNode.isConnected).toBe(true)
    expect(firstNode.textContent).toContain('第二段')

    // 断言 D：旧占位文本未残留
    const stale = screen.queryAllByText(/^首个可见片段$/)
    expect(stale.length).toBe(0)
  })
})
