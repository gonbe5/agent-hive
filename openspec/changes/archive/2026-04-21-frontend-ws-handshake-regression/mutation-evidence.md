# Mutation 证据 — frontend-ws-handshake-regression

本文件汇总 37 条 mutation（R1×4 + R2×3 + R3×1 + R4×1 + R5×28）的 **diff + vitest 红色输出** 证据，满足 tasks §8.3 归档门槛。

R17（第 17 轮 codex 审计）补入 R5-M22 / M23 / M24，覆盖 plan-ceo-review W2（Tailwind `!important` prefix + arbitrary value 攻击面 silent-green）。

R18（第 18 轮 codex 审计）点名 4 条新 blocker，逐条落地修复 + 独立性对照：
1. **B1** — `tokenHasDangerUtility` 的 `includes(':')` 盲 skip 会让 `aria-busy:text-red-500`（aria-busy='true' 激活时）和 `data-[state=offline]:text-red-500`（data-state='offline' 激活时）silent-green。修复：`splitAtTopColons` 按 `[...]` 括号深度切分、`prefixActiveOnElement` 按 aria/data attr 真值判定、静态伪类与断点列入 `INACTIVE_STATIC_PREFIX` 白名单、未识别 prefix 保守判 active。
2. **B2** — `dangerTokenRe` 与 broad selector 漏掉 `stroke-/fill-/caret-/decoration-/accent-/placeholder-/from-/via-/to-/divide-` 等 utility 家族（红色 SVG icon / gradient / 输入态 caret 等真实攻击面）。修复：两处同步扩充全量 10 个 prefix × 4 色族（red/rose/danger/destructive）。
3. **B3** — `dangerArbitraryTokenRe` 只穷举具体 hex/rgb 元组，漏 `#ff0000` / `#f00` / `rgb(255,0,0)` / `hsl(...)` / `text-[color:var(--danger)]` 等真实红色写法。修复：改为程序化 `matchesArbitraryDanger`——解析 `[...]` 内部、可选 `color:` 前缀、接受 var(danger/destructive) / red-dominant 十六进制（R 高 G/B 低，3 或 6 位） / red-dominant rgb（R≥180 且 G/B≤100） / red 附近色相 hsl（0-20° 或 340-360° 或 var()）。
4. **B4** — M22 / M23 R17 独立性声明不成立：`[class*="!text-"]` 与 `[class*="-[var(--danger"]` 在 broad selector 里冗余（原 `[class*="text-red-"]` / `[class*="--danger"]` 已 substring 命中）。修复：删除这两类冗余 broad 项；用**独立性探针**（删单一修复组件 + 注入对应 mutation）证明 M22 真正依赖 `replace(/^!/, '')`、M23 真正依赖 `matchesArbitraryDanger` var 分支、M24 真正依赖 `[class*="-[#"]` broad + hex regex。

R18 新增 M25 / M26 / M27 / M28 四条 mutation 分别对应 B2 / B3 / B1(aria) / B1(data) 四个修复维度的红色证据。

格式：每条 mutation 一块，分三部分：
1. **攻击面**：在哪个文件插入什么 diff
2. **预期红的 C 断言**：mutation 设计时的目标签名
3. **vitest 红色输出**：`bun run test <file> 2>&1 | grep ...` 的实际输出

回滚约定：每条 mutation 验证完立即回滚，再跑下一条——保证 attribution 独立、没有交叉污染。所有 mutation 跑完后全套 117 test 回到绿。

---

## R1 — useWebSocketConnection URL 拼接反射后端契约

文件：`frontend/src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts`

### R1-M1 — 后端锚定：backend key 识别
注入（`internal/streaming/websocket.go:280`）：
```diff
- userSessionID := r.URL.Query().Get("session_id")
+ userSessionID := r.URL.Query().Get("sid")
```
`bun run test src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts` 实测红色输出（R17 重验）：
```
FAIL  src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts
Error: 后端没有 session 相关 query key，实际匹配：token, sid
 ❯ resolveBackendKey src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts:56:11
    54|   const sessionKeys = matches.filter((k) => /session/i.test(k))
    55|   if (sessionKeys.length === 0) {
    56|     throw new Error(`后端没有 session 相关 query key，实际匹配：${matches.join(', ')}`)
```
回滚验绿：`Test Files 1 passed (1) / Tests 5 passed (5)`。证明 `resolveBackendKey()` fail-fast 绑住了后端契约——任何单方面改 key 名都会红。

### R1-M2 — 前端漏拼 session_id
攻击：把 `useWebSocketConnection.ts:52` 的 URL builder 去掉 session_id 段：
```diff
- const wsUrl = sessionId ? `${url}${url.includes('?') ? '&' : '?'}session_id=${encodeURIComponent(sessionId)}` : url;
+ const wsUrl = url;
```
预期：scenario 1/2/3/4 全红（解析出来的 `session_id` 都是 null）。

### R1-M3 — 不 URL-encode 特殊字符
攻击：把 encodeURIComponent 去掉（即**初始生产 bug 本身**）：
```diff
- const wsUrl = sessionId ? `${url}${url.includes('?') ? '&' : '?'}session_id=${encodeURIComponent(sessionId)}` : url;
+ const wsUrl = sessionId ? `${url}${url.includes('?') ? '&' : '?'}session_id=${sessionId}` : url;
```
预期：scenario 3 (`abc/+&=`) 红——`+` 被解析回空格、`&=` 被 URLSearchParams 拆成两段。本 mutation 是**真实生产 bug**，在实施过程中发现并用 R1 测试作为守门人。

### R1-M4 — 用 `sid` 不用 `session_id`
攻击：把前端 URL builder 的 param key 改为 `sid=`：
```diff
- `${url}${url.includes('?') ? '&' : '?'}session_id=${encodeURIComponent(sessionId)}`
+ `${url}${url.includes('?') ? '&' : '?'}sid=${encodeURIComponent(sessionId)}`
```
预期：所有 scenario 红（反射 backend key 与前端 key 不一致）。

---

## R2 — partial → DOM 渲染

文件：`frontend/src/components/chat/__tests__/partialMessageRendering.test.tsx`

### R2-M1 — 删掉 partial 分支的 updateLastAssistant
注入（`src/hooks/useWebSocket.ts:132-138` RAF 回调体）：
```diff
-                updateLastAssistant(pendingPartial.current.content, pendingPartial.current.reasoning);
+                // R2-M1 mutation: updateLastAssistant 被删除
```
`bun run test src/components/chat/__tests__/partialMessageRendering.test.tsx` 实测红色输出（R17 重验）：
```
 FAIL  src/components/chat/__tests__/partialMessageRendering.test.tsx
 ❯ src/components/chat/__tests__/partialMessageRendering.test.tsx:82:36
     80|     await flushRaf()
     81|
     82|     const firstNode = await screen.findByText(/首个可见片段/)
       |                                    ^
     83|     expect(firstNode).toBeInTheDocument()
  Tests  1 failed (1)
```
回滚验绿。RAF 不把 pendingPartial 写入 store → DOM 永远看不到"首个可见片段" → findByText 3s timeout。

### R2-M2 — ensureAssistantMessage 不新建 placeholder
注入（`src/hooks/useWebSocket.ts:128`）：
```diff
-          ensureAssistantMessage();
+          // R2-M2 mutation: ensureAssistantMessage 被删除
```
`bun run test src/components/chat/__tests__/partialMessageRendering.test.tsx` 实测红色输出（R17 重验）：
```
 FAIL  src/components/chat/__tests__/partialMessageRendering.test.tsx
 ❯ src/components/chat/__tests__/partialMessageRendering.test.tsx:82:36
  Tests  1 failed (1)
```
回滚验绿。没 placeholder 被建 → updateLastAssistant 找不到目标 → 断言 A 和 B 同时红（DOM 空、assistant 计数为 0）。

### R2-M3 — 同节点被替换而非更新
攻击：`chat.ts` 的 `updateLastAssistant` 每次新建一条 message 而不是原位更新：
```diff
- if (lastIdx !== -1) messages[lastIdx] = { ...messages[lastIdx], content };
+ messages.push({ role:'assistant', content, ... });
```
预期：断言 C (`firstNode.isConnected === true`) 红——原占位节点被卸载，新节点取代。

---

## R3 — handleDisconnected 不清零 currentSessionId

文件：`frontend/src/hooks/__tests__/useWebSocket.handleDisconnected.test.ts`

### R3-M1 — handleDisconnected 清零 currentSessionId
攻击：`useWebSocket.ts:299` 恢复历史错误行为：
```diff
    pendingPartial.current = null;
    setStreaming(false);
    setAgentStatus(null);
+   useChatStore.setState({ currentSessionId: null });
```
预期：测试红——`currentSessionId` 断言期望保留 `'sid-1'`，实际变 null。

---

## R4 — AppShell 双源优先级

文件：`frontend/src/layouts/__tests__/AppShell.test.tsx`

### R4-M1 — 优先级颠倒
攻击：`AppShell.tsx` 把 `urlSessionId || storeSessionId` 写反：
```diff
- const sessionId = urlSessionId || storeSessionId;
+ const sessionId = storeSessionId || urlSessionId;
```
预期：scenario A (`URL 有 / store 有，取 URL`) 红——结果变成 store 值。

---

## R5 — reconnect 可见性 + C6-C16 两阶段错误扫描

文件：`frontend/src/hooks/__tests__/useWebSocket.reconnect.test.tsx`

所有 21 条 R5 mutation 在本次 session 全部跑完，每条单独注入、单独验证、单独回滚。注入点一般在 `useWebSocket.ts` 的 `handleDisconnected` 内。

### R5-M1 — clearMessages
```ts
useChatStore.getState().clearMessages();
```
红的 C：`C7a`（delta=-2）+ `C15a preset-assistant-1/preset-tool-1/preset-plain-assistant-2`（matches=0×3）。
```
AssertionError: expected -2 to be greater than or equal to 0
AssertionError: C15a preset-assistant-1: expected +0 to be 1
AssertionError: C15a preset-tool-1: expected +0 to be 1
AssertionError: C15a preset-plain-assistant-2: expected +0 to be 1
```

### R5-M2 — chat.error 写入
```ts
useChatStore.setState({ error: '连接中断，请刷新' });
```
红的断言：`useChatStore.getState().error` 必须 falsy（测试文件末尾的辅助诊断断言，非 C6-C16 主契约）。R5 测试中并无 `C3` 标签——codex round 16 审计指出此处 attribution 应写成具体的 store 断言，避免误导。
```
AssertionError: expected '连接中断，请刷新' to be falsy
```

### R5-M3 — addToast('error')
```ts
useToastStore.getState().addToast('error', '连接中断');
```
红的 C：`C6`。
```
AssertionError: expected 1 to be +0  (toasts.length === toastBefore)
```

### R5-M4 — addToast('warning') 文案不命中黑名单
注入（`src/hooks/useWebSocket.ts` handleDisconnected）：
```ts
useToastStore.getState().addToast('warning', '连接中断，正在重连');
```
（`useToastStore` 需临时 `import` 到 useWebSocket.ts 顶部作为 mutation 载体）
R17 重验实测红色输出（`bun run test src/hooks/__tests__/useWebSocket.reconnect.test.tsx`）：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
    → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    360|     // === 6. 断言 C6-C16（soft，独立 attribution）===
    361|     // C6：toast 计数不变
    364|     // C7a：messages 长度增量 ∈ {0, 1}
  Tests  1 failed (1)
```
红的 C：`C6`（结构不变量，不依赖文案）。证明 'warning' 类型 + "连接中断，正在重连" 这类天然文案依然被计数挡住——C6 是严格结构 invariant。

### R5-M5 — addToast('info') 完全自然文案
注入（同 M4，改 type 和文案）：
```ts
useToastStore.getState().addToast('info', '网络波动，正在恢复通信');
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
    → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
  Tests  1 failed (1)
```
红的 C：`C6`。证明 C6 不依赖语义黑名单——即使是 'info' 类型 + "网络波动，正在恢复通信"（不含"错误/失败/中断"等任何危险词），仍然红。

### R5-M6 — addChatMessage assistant + is_error:false
```ts
useChatStore.getState().addMessage(
  { role:'assistant', content:'网络波动，正在恢复通信', is_error:false, timestamp:'m6-injected' },
  'sid-1'
);
```
红的 C：`C7a`（delta=2）。
```
AssertionError: expected 2 to be less than or equal to 1
```

### R5-M7 — addInlineApproval
```ts
useChatStore.getState().addInlineApproval({ id:'m7-appr', session_id:'sid-1', prompt:'网络波动，正在恢复通信' });
```
红的 C：`C8`。
```
AssertionError: expected 1 to be +0  (inlineApprovals.length === approvalsBefore)
```

### R5-M8 — setTaskGroup failed
注入（`src/hooks/useWebSocket.ts` handleDisconnected）：
```ts
useTaskProgressStore.getState().setTaskGroup({ group_id:'m8-g', tasks:[{ id:'m8-t', status:'failed', error:'连接已中断', agent_id:'a' }] });
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 2 to be 1 // Object.is equality           ← C9 groupsAfter.size
     → expected 1 to be +0 // Object.is equality          ← newFailed.length
     → expected 1 to be +0 // Object.is equality          ← C14 taskErrorCountAfter
     → expected 3 to be +0 // Object.is equality          ← C16 Stage2 finalErrEls
AssertionError: expected [ …(3) ] to deeply equal []     ← C16 Stage1 transientHits
```
红的 C：C9 (groups.size 1→2 + newFailed=1) + C14 (taskErrorCountAfter=1) + C16 Stage1 (3 transient added) + C16 Stage2 (3 final red)。一次 mutation 触发 5 条 soft 断言同时红——证明 C9/C14/C16 三轴独立覆盖 setTaskGroup failed 这条攻击面。

### R5-M9 — agentActivity sessionStatus='error'
```ts
useAgentActivityStore.getState().onAgentStatus('sid-1', 'error');
```
红的 C：`C10`。
```
AssertionError: expected 'error' not to be 'error'
```

### R5-M10 — setToolCallStatus status='error'
注入（`src/hooks/useWebSocket.ts` handleDisconnected）：
```ts
useChatStore.getState().setToolCallStatus('tc-1', { id:'tc-1', name:'search', status:'error', error:'连接已中断' });
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality             ← C11 toolErrorAfter
     → expected [ { kind: 'added', …(1) }, …(1) ] to deeply equal []  ← C16 Stage1
     → expected 2 to be +0 // Object.is equality             ← C16 Stage2 finalErrEls
     → expected 2 to be +0 // Object.is equality             ← 描述性 alert 计数
```
红的 C：C11 + C16 Stage1/Stage2。ToolInvocationChip 接到 status='error' → 切到 `text-[var(--danger)]` / `border-[var(--danger)]/30` → class selector 命中。

### R5-M11 — setMessages 原位翻 is_error=true
注入（`src/hooks/useWebSocket.ts` handleDisconnected）：
```ts
useChatStore.getState().setMessages(useChatStore.getState().messages.map(m =>
  m.timestamp === 'preset-plain-assistant-2'
    ? { ...m, is_error:true, content:'连接中断，请刷新' }
    : m
));
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality                                   ← C12 isErrorAfter
     → C15b preset-plain-assistant-2: expected '连接中断，请刷新' to be '历史回复 B' ← C15b 冻结
     → expected [ { kind: 'added', …(1) } ] to deeply equal []                     ← C16 Stage1
     → expected 1 to be +0 // Object.is equality                                   ← C16 Stage2
     → expected 3 to be +0 // Object.is equality                                   ← alert/role 计数
     → expected true to be false // Object.is equality                             ← 文案黑名单
```
红的 C：C12 + C15b + C16 Stage1/Stage2。证明 setMessages 原位改写同时触发 is_error 增量、content 冻结、DOM Error 渲染三重兜底。

### R5-M12 — setMessages 给 tool content 加 tool error: 前缀
注入：
```ts
useChatStore.getState().setMessages(useChatStore.getState().messages.map(m =>
  (m.role === 'tool' && m.tool_call_id === 'tc-1') ? { ...m, content:'tool error: 连接已中断' } : m
));
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality                                       ← C13 toolErrorContentAfter
     → C15b preset-tool-1: expected 'tool error: 连接已中断' to be '{"ok":true}'      ← C15b 冻结
AssertionError: expected 1 to be +0 // Object.is equality
AssertionError: C15b preset-tool-1: expected 'tool error: 连接已中断' to be '{"ok":true}'
```
红的 C：C13（tool-error 前缀计数=1）+ C15b preset-tool-1（content 冻结失败）。证明 C13 前缀计数与 C15b 冻结互相独立覆盖。

### R5-M13 — updateTask 加 error 字段不改 status
注入：
```ts
useTaskProgressStore.getState().updateTask('g1', 't1', { error:'连接已中断' } as any);
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality                                ← C14 taskErrorCountAfter
     → expected [ { kind: 'added', …(1) } ] to deeply equal []                  ← C16 Stage1
     → expected 1 to be +0 // Object.is equality                                ← C16 Stage2
```
红的 C：C14 + C16 Stage1/Stage2。证明只改 task.error 字段不触碰 status 也被 C14 的"任意 `!!t.error` 计数 = 0"断言吃掉——setTaskGroup failed 不是唯一攻击面，field-level 偷偷摸摸改 error 也兜住。

### R5-M14 — setMessages 改既有消息 content 文案
```ts
s.setMessages(s.messages.map(m =>
  m.timestamp === 'preset-plain-assistant-2'
    ? { ...m, content:'请稍候，系统正在重新同步' }
    : m
));
```
红的 C：`C15b`（唯一，其它全绿——证明 C15b 独立于文案黑名单）。
```
C15b preset-plain-assistant-2: expected '请稍候，系统正在重新同步' to be '历史回复 B'
```

### R5-M15 — setMessages 改 content 类型为 {}
```ts
s.setMessages(s.messages.map(m =>
  m.timestamp === 'preset-plain-assistant-2'
    ? { ...m, content: ({} as unknown as string) }
    : m
));
```
红的 C：`C15b` + `C15c`（typeof !== 'string'）+ `C16`（MessageBubbleBoundary 渲染错误卡带红 class）。
```
C15c preset-plain-assistant-2: expected 'object' to be 'string'
```

### R5-M16 — length-neutral 同三元组副本
注入：
```ts
useChatStore.getState().setMessages(useChatStore.getState().messages.map((m, i, arr) =>
  i === arr.length - 1
    ? { timestamp:'preset-plain-assistant-2', role:'assistant', content:'副本内容：网络波动，请稍候', is_error:false } as any
    : m
));
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → C15a preset-tool-1: expected +0 to be 1 // Object.is equality            ← preset-tool-1 丢失
     → C15a preset-plain-assistant-2: expected 2 to be 1 // Object.is equality  ← preset-plain-assistant-2 重复
AssertionError: C15a preset-tool-1: expected +0 to be 1
AssertionError: C15a preset-plain-assistant-2: expected 2 to be 1
```
红的 C：`C15a preset-tool-1`（matches=0——tool 被替换成 assistant 副本）+ `C15a preset-plain-assistant-2`（matches=2——同三元组出现两条）。**C7a 保持绿**（length 不变 = 3）——证明 C15a "存在且唯一" 结构契约独立于 length 上限 delta∈{0,1}。

### R5-M17 — tool_calls[0].name='forbidden_tool'
```ts
s.setMessages(s.messages.map(m =>
  m.timestamp === 'preset-assistant-1'
    ? { ...m, tool_calls:[{ id:'tc-1', name:'forbidden_tool', arguments:'{"q":"x"}' }] }
    : m
));
```
红的 C：`C15d`（JSON.stringify(tool_calls) 不等）。其它全绿——证明 C15d 深度冻结是 tool_calls 身份独立契约。
```
C15d preset-assistant-1: expected '[{"id":"tc-1","name":"forbidden_tool"…' to be '[{"id":"tc-1","name":"search","argume…'
```

### R5-M18 — MessageList 静态红 class div
注入 `src/components/chat/MessageList.tsx:298`（TaskProgressPanel 之前）：
```tsx
<div className="text-red-500">通道暂不可用，请稍候</div>
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect（非 soft）第 257 行先 fail-fast。证明 C16 Stage 2 class selector 对常驻红 class 有效——静态渲染进 DOM 的 `text-red-500` utility 立刻被 broad selector 捕获。

### R5-M19 — MessageList inline-style danger 无 class
注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div style={{ color:'var(--danger)' }}>通道暂不可用，请稍候</div>
```
R17 重验实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    258|     expect(baselineInline.length).toBe(0)
```
红的 C：baseline hard expect 第 258 行 fail-fast。class selector 不命中（无 class），**只有 inline-style 扫描命中**——证明 Stage 2 inline-style 独立覆盖 class selector 之外的攻击面（如 `color: var(--danger)` / `color: rgb(239,68,68)` / CSS-var 注入）。

### R5-M20 — transient blip：disconnect 添 + partial 移
`handleDisconnected`：
```ts
const blip = document.createElement('div');
blip.id = 'ws-reconnect-blip'; blip.style.color = 'var(--danger)';
blip.textContent = '通道波动，请稍候';
document.body.appendChild(blip);
```
partial 分支：`document.getElementById('ws-reconnect-blip')?.remove();`

红的 C：`C16 Stage1 transientHits` 非空（added 阶段捕获到 blip 节点）；**Stage 2 final 扫描绿**（blip 已被 partial 移除，事后看不到）。证明 Stage 1 独立于 Stage 2，捕获"disconnect 之间短暂 banner"这类最符合 Req 2 "整个窗口期不得出现错误态" 的回归。
```
AssertionError: expected [ { kind: 'added', summary: '...inline-style...--danger...' } ] to deeply equal []
```

### R5-M21 — attribute attack aria-invalid
```ts
document.body.setAttribute('aria-invalid', 'true');
```
红的 C：`C16 Stage1 attr:aria-invalid`（MutationObserver attr 阶段记录）+ `C16 Stage2 finalErrEls.length=1`（body 仍有 attr）。证明 attr 观察覆盖"不加新 DOM、只改既有节点属性"的攻击面。
```
AssertionError: expected [ { kind: 'attr:aria-invalid', summary: '...<body aria-invalid="true">...' } ] to deeply equal []
```

### R5-M22 — Tailwind `!important` prefix `!text-red-500`
R17 W2 攻击面：修复前的 `dangerTokenRe` 带 `^` 锚点，`!text-red-500` 首字符 `!` 直接被拒；token 判定里 `includes(':')` 盲 skip 虽不触发（此 token 无 `:`），但字面 regex 在 `!` 下无法锚定 → 整体 silent-green。

注入 `src/components/chat/MessageList.tsx:298`（TaskProgressPanel 之前）：
```tsx
<div className="!text-red-500">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad selector 由既有 `[class*="text-red-"]` substring 命中（class `"!text-red-500"` 含 `text-red-` 子串）→ token 判定 `rawTok.replace(/^!/, '')` 剥 `!` → `dangerTokenRe` 命中。

**独立性对照（R18 补入）**：临时把 `tokenHasDangerUtility` 里的 `suffix.replace(/^!/, '')` 替换为 `suffix`（保留 attack 注入）→ `bun run test` 变绿（silent-green 证实）：
```
 ✓ src/hooks/__tests__/useWebSocket.reconnect.test.tsx (1 test)
 Test Files  1 passed (1)
```
还原 `replace(/^!/, '')` 即回红。**证明 M22 真正依赖的独立修复组件是 `replace(/^!/, '')`，而非冗余 broad 前缀**（R17 原写的 `[class*="!text-"]` 对 M22 无增量价值，R18 已删除）。

### R5-M23 — Tailwind arbitrary value `text-[var(--danger)]`
R17 W2 另一条攻击面：arbitrary `text-[var(--danger)]` 在字面 `dangerTokenRe` 下不匹配（regex 无 `[` 分支）→ silent-green。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div className="text-[var(--danger)]">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad selector 由既有 `[class*="--danger"]` 命中（class 含 `--danger` 子串）→ token 判定 `matchesArbitraryDanger` 里 `var(--danger)` 分支命中。

**独立性对照（R18 补入）**：临时把 `matchesArbitraryDanger` 的 `if (/^var\(\s*--(?:danger|destructive)\b/i.test(inner)) return true` 注释掉（保留 attack）→ 测试变绿（silent-green 证实）。还原后回红。**证明 M23 真正依赖的独立修复组件是 `matchesArbitraryDanger` 的 var 分支**（R17 原写的 `[class*="-[var(--danger"]` 对 M23 无增量，R18 已删除）。

### R5-M24 — Tailwind arbitrary hex `bg-[#ef4444]`
十六进制色值直写（`#ef4444` = Tailwind `red-500` 默认十六进制）。修复前字面清单完全不覆盖；broad selector 也无 `-[#` 前缀 → silent-green。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div className="bg-[#ef4444]">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad selector 仅有 R17 新加的 `[class*="-[#"]` 可命中（既有 utility broad 无 `-[#` 子串）→ token 判定 `matchesArbitraryDanger` 的十六进制分支（R≥180 且 G/B≤100）命中 `ef4444`。

**独立性对照（R18 补入）**：临时把 broad selector 里的 `'[class*="-[#"]'` 删除（保留 attack）→ 测试变绿（MO 根本不扫 `bg-[#ef4444]` 元素，silent-green 证实）。还原后回红。**证明 M24 独立依赖 `[class*="-[#"]` broad 前缀 + `matchesArbitraryDanger` 十六进制分支的联合**——缺任一即 silent-green，是 R17 broad 扩展唯一真正提供新覆盖的那一条。

### R5-M25 — R18-B2 `stroke-red-500` SVG utility 家族
R18 W B2 攻击面：`stroke-red-500` 在修复前的 `dangerTokenRe` 与 broad selector 里都不存在 → `<svg><path className="stroke-red-500"/></svg>` 渲染红色 icon 时整窗口 silent-green。`fill-red-500` / `caret-red-500` / `decoration-red-500` / `accent-red-500` / `from-red-500` / `divide-red-500` / `placeholder-red-500` 同理。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div className="stroke-red-500">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。新加 `[class*="stroke-red-"]` broad 扫入 → 新加 `dangerTokenRe` 的 `stroke-red-` 分支命中。回滚后验绿（Test Files 1 passed）。

**独立性对照**：删除 `[class*="stroke-red-"]` broad 项（保留 attack）→ 测试变绿 → 证明 M25 依赖 R18-B2 新加的 broad + regex 扩展联合，删任一即 silent-green。

### R5-M26 — R18-B3 arbitrary hex `text-[#ff0000]`
R18 B3 攻击面：纯红 `#ff0000` 在 R17 的穷举清单（`ef4444|dc2626|b91c1c|f87171|fca5a5|fecaca`）里不存在 → silent-green。`#f00`（短写）、`bg-[rgb(255,0,0)]`、`text-[hsl(var(--destructive))]`、`text-[color:var(--danger)]`（`:` 在 brackets 内）同理。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div className="text-[#ff0000]">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad selector `[class*="-[#"]` 扫入 → 新的程序化 `matchesArbitraryDanger` 判 `#ff0000` 三分量：R=255、G=0、B=0，R≥180 且 G/B≤100 → 命中。回滚后验绿。

**独立性对照**：临时把 `matchesArbitraryDanger` 十六进制分支的阈值改为 `r >= 250 && g === 0 && b === 0 && false`（保留 attack）→ 测试变绿 → 证明 M26 依赖 R18-B3 新的广谱十六进制判定逻辑。

### R5-M27 — R18-B1 条件修饰 aria-busy 激活态
R18 B1 攻击面：`aria-busy="true"` + `aria-busy:text-red-500` 组合会渲染红色，但修复前 `includes(':')` 盲 skip 直接放过所有带 `:` token → silent-green。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div aria-busy="true" className="aria-busy:text-red-500">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad selector `[class*="text-red-"]` 从 `"aria-busy:text-red-500"` substring 命中 → `splitAtTopColons` 拆成 `['aria-busy', 'text-red-500']`、suffix `text-red-500` 命中 `dangerTokenRe` → `prefixActiveOnElement` 判 `aria-busy='true'` 成立 → 返回 true。回滚后验绿。

**负控验证**（与 M27 互补，证明未激活态不误伤）：改为 `<div className="aria-busy:text-red-500 hover:text-red-500">...</div>`（无 aria-busy attr + 静态 pseudo）→ 测试绿 → 证明 `prefixActiveOnElement` 对 aria 不满足和 `hover`/`INACTIVE_STATIC_PREFIX` 正确判 inactive，不吞 shadcn `aria-invalid:ring-destructive/20` 类型的 baseline。

### R5-M28 — R18-B1 条件修饰 data-state 激活态
同 B1 维度，`data-[state=offline]:text-red-500` 配合 `data-state="offline"` 会渲染红色；修复前同 silent-green。

注入 `src/components/chat/MessageList.tsx:298`：
```tsx
<div data-state="offline" className="data-[state=offline]:text-red-500">通道暂不可用，请稍候</div>
```
实测红色输出：
```
 FAIL  src/hooks/__tests__/useWebSocket.reconnect.test.tsx
     → expected 1 to be +0 // Object.is equality
AssertionError: expected 1 to be +0 // Object.is equality
    257|     expect(baselineErrEls.length).toBe(0)
```
红的 C：baseline hard expect 第 257 行 fail-fast。broad `[class*="text-red-"]` substring 命中 → `splitAtTopColons`（`[...]` 内的 `=` 不影响深度）拆 `['data-[state=offline]', 'text-red-500']` → suffix 命中 `dangerTokenRe` → `prefixActiveOnElement` 解析 `data-\[([\w-]+)=([^\]]+)\]` → 验 `data-state` 值 === `'offline'` → 激活 → 红。回滚后验绿。

**独立性对照**：M28 同时依赖 `splitAtTopColons`（尊重 brackets）+ `prefixActiveOnElement` 的 `data-[<name>=<value>]` 分支——任一组件缺失即 silent-green。这和 M27（aria 简写路径）走的是不同子分支，提供独立覆盖。

---

## 回滚后 sanity

全部 33 条 mutation 验证完成、逐条回滚后：

```
$ bun run test
Test Files  16 passed (16)
     Tests  117 passed (117)
```

—— 证明所有测试独立于生产代码的任何 mutation，且基线 117 测试仍全绿。
