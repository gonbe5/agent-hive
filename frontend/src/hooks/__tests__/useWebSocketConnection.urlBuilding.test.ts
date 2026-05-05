import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, cleanup, waitFor } from '@testing-library/react'
import { useWebSocketConnection } from '../useWebSocketConnection'
import * as fs from 'node:fs'
import * as path from 'node:path'
import { fileURLToPath } from 'node:url'

// 反射后端契约：key 名以 internal/streaming/websocket.go 里的
//   r.URL.Query().Get("<key>") 字面量为单一 source of truth。
// 前端改 key 同时改这里的字面量——若没改后端，测试仍红。
function resolveBackendKey(): string {
  const here = fileURLToPath(import.meta.url)
  // src/hooks/__tests__/<this-file>.ts → repoRoot 向上 4 级
  let repoRoot = path.resolve(here, '../../../../..')
  // find-up go.mod 兜底（防止目录深度变动）
  let cur = path.dirname(here)
  let found = false
  for (let i = 0; i < 10; i++) {
    if (fs.existsSync(path.join(cur, 'go.mod'))) {
      repoRoot = cur
      found = true
      break
    }
    const parent = path.dirname(cur)
    if (parent === cur) break
    cur = parent
  }
  if (!found) {
    throw new Error(`R1 测试无法 find-up 仓库根 (go.mod 未定位)，当前起点 ${here}`)
  }
  const backendPath = path.join(repoRoot, 'internal/streaming/websocket.go')
  if (!fs.existsSync(backendPath)) {
    throw new Error(`R1 测试锚定的后端 Go 源文件不存在：${backendPath}`)
  }
  const rawSrc = fs.readFileSync(backendPath, 'utf-8')
  // 去 Go 注释 / raw string：攻击面是任何人加一行
  //   `// r.URL.Query().Get("session_legacy")`
  // 就能让 resolveBackendKey 抛"多个 session key"制造 false red。
  // 顺序：block → line → backtick raw string。
  const src = rawSrc
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:"`])\/\/[^\n]*/g, '$1')
    .replace(/`[^`]*`/g, '``')
  const re = /r\.URL\.Query\(\)\.Get\("([^"]+)"\)/g
  const matches: string[] = []
  let m: RegExpExecArray | null
  while ((m = re.exec(src)) !== null) {
    matches.push(m[1])
  }
  if (matches.length === 0) {
    throw new Error('后端 WS handler 契约源码未找到 r.URL.Query().Get(...)，请更新 R1 反射路径')
  }
  // 过滤 token 等认证参数，session 标识符的 key 名固定以 session 开头
  const sessionKeys = matches.filter((k) => /session/i.test(k))
  if (sessionKeys.length === 0) {
    throw new Error(`后端没有 session 相关 query key，实际匹配：${matches.join(', ')}`)
  }
  if (sessionKeys.length > 1) {
    throw new Error(`后端匹配到多个 session key，需显式选择：${sessionKeys.join(', ')}`)
  }
  return sessionKeys[0]
}

interface CapturedCall {
  url: string
  protocols?: string | string[]
}
const capturedCalls: CapturedCall[] = []

class MockWS {
  readyState = 0
  onopen: ((e?: unknown) => void) | null = null
  onmessage: ((e?: unknown) => void) | null = null
  onclose: ((e?: unknown) => void) | null = null
  onerror: ((e?: unknown) => void) | null = null
  constructor(url: string, protocols?: string | string[]) {
    capturedCalls.push({ url, protocols })
  }
  close() {}
  send() {}
}
Object.assign(MockWS, {
  CONNECTING: 0,
  OPEN: 1,
  CLOSING: 2,
  CLOSED: 3,
})

describe('useWebSocketConnection URL 拼接（R1）', () => {
  const expectedKey = resolveBackendKey()

  beforeEach(() => {
    capturedCalls.length = 0
    vi.stubGlobal('WebSocket', MockWS)
    localStorage.clear()
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it(`后端契约锁定：key 名来自 websocket.go 反射，值 === '${expectedKey}'`, () => {
    expect(expectedKey).toBe('session_id')
  })

  it('场景 1: 无 query 的 url → ?<key>=<value>', async () => {
    renderHook(() =>
      useWebSocketConnection({ url: 'ws://h/api', sessionId: 'abc-123', enabled: true })
    )
    await waitFor(() => expect(capturedCalls.length).toBe(1))
    const captured = new URL(capturedCalls[0].url)
    expect(captured.searchParams.get(expectedKey)).toBe('abc-123')
    // 补充字面正则锁定以防重复 key / 多余字段
    expect(capturedCalls[0].url).toMatch(new RegExp(`^ws:\\/\\/h\\/api\\?${expectedKey}=abc-123$`))
  })

  it('场景 2: 带 query 的 url → 用 & 追加', async () => {
    renderHook(() =>
      useWebSocketConnection({ url: 'ws://h/api?foo=1', sessionId: 'abc-123', enabled: true })
    )
    await waitFor(() => expect(capturedCalls.length).toBe(1))
    expect(capturedCalls[0].url).toMatch(new RegExp(`\\?foo=1&${expectedKey}=abc-123$`))
  })

  it('场景 3: 特殊字符值必须 URL-encoded（可 decodeURIComponent 反解）', async () => {
    const original = 'abc/+&='
    renderHook(() =>
      useWebSocketConnection({ url: 'ws://h/api', sessionId: original, enabled: true })
    )
    await waitFor(() => expect(capturedCalls.length).toBe(1))
    const parsed = new URL(capturedCalls[0].url)
    const gotValue = parsed.searchParams.get(expectedKey)
    expect(gotValue).toBe(original)
    // 拼接字符串不得出现未编码的 & / = / + 导致 query 被截断
    const rawAfterKey = capturedCalls[0].url.split(`${expectedKey}=`)[1] ?? ''
    expect(rawAfterKey).toBe(encodeURIComponent(original))
    expect(decodeURIComponent(rawAfterKey)).toBe(original)
  })

  it('场景 4: sessionId undefined → url 不含 key', async () => {
    renderHook(() =>
      useWebSocketConnection({ url: 'ws://h/api', sessionId: undefined, enabled: true })
    )
    await waitFor(() => expect(capturedCalls.length).toBe(1))
    expect(capturedCalls[0].url).not.toContain(`${expectedKey}=`)
    expect(capturedCalls[0].url).toBe('ws://h/api')
  })
})
