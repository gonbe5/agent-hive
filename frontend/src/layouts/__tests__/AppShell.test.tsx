import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { AppShell } from '../AppShell'
import { useChatStore } from '../../store/chat'

const captured: { args: { url: string; sessionId?: string; enabled?: boolean; client?: unknown } | null } = { args: null }

vi.mock('react-router-dom', () => ({
  Outlet: () => null,
  useParams: () => mockParams,
  useNavigate: () => () => {},
  useLocation: () => ({ pathname: '/' }),
  Link: ({ children }: { children?: React.ReactNode }) => children,
  NavLink: ({ children }: { children?: React.ReactNode }) => children,
}))

let mockParams: { id?: string } = {}

vi.mock('../../hooks/useWebSocket', () => ({
  useWebSocket: (opts: { url: string; sessionId?: string; enabled?: boolean; client?: unknown }) => {
    captured.args = opts
    return { connected: false, send: () => {} }
  },
}))

vi.mock('../../hooks/useNodeClient', () => ({
  useNodeClient: () => ({
    getWebSocketUrl: () => 'ws://test/api/ws',
  }),
}))

vi.mock('../Sidebar', () => ({ Sidebar: () => null }))
vi.mock('../../components/common/Header', () => ({ Header: () => null }))
vi.mock('../../components/common/Toast', () => ({ ToastContainer: () => null }))

describe('AppShell sessionId 双源优先级（R4）', () => {
  beforeEach(() => {
    captured.args = null
    useChatStore.setState({ currentSessionId: null })
    mockParams = {}
  })

  it('场景 A: URL abc-123 + store xyz-789 → sessionId = abc-123', () => {
    mockParams = { id: 'abc-123' }
    useChatStore.setState({ currentSessionId: 'xyz-789' })
    render(<AppShell />)
    expect(captured.args?.sessionId).toBe('abc-123')
  })

  it('场景 B: URL 空 + store xyz-789 → sessionId = xyz-789', () => {
    mockParams = {}
    useChatStore.setState({ currentSessionId: 'xyz-789' })
    render(<AppShell />)
    expect(captured.args?.sessionId).toBe('xyz-789')
  })

  it('场景 C: URL abc-123 + store 空 → sessionId = abc-123', () => {
    mockParams = { id: 'abc-123' }
    useChatStore.setState({ currentSessionId: null })
    render(<AppShell />)
    expect(captured.args?.sessionId).toBe('abc-123')
  })

  it('场景 D: URL 空 + store 空 → sessionId = undefined', () => {
    mockParams = {}
    useChatStore.setState({ currentSessionId: null })
    render(<AppShell />)
    expect(captured.args?.sessionId).toBeUndefined()
  })
})
