# Tasks — frontend-ws-handshake-regression

本 change 以 spec + vitest 五件套 + CI gate 为 scope。每一项任务都附：
- **目标盲区**（R1-R5，见 proposal.md）
- **最小断言**（不加夸张覆盖，抓死核心 invariant）
- **mutation 验证**（红军改一行代码必须让该测试转红，否则测试无效）

**Scope 边界（codex 九次审计明确）**：本 change 只覆盖 spec Req 2 定义的 **disconnect → reconnect 循环**——即先建立过 WS 连接、然后被外部事件关闭、再触发 `onDisconnected` 回调 → 重连 → 下一条 partial 可见。**不**覆盖以下 WS 生命周期：
- **initial connection failure**：`useWebSocketConnection.ts:80-103` 中 `new WebSocket()` 构造异常被 catch、直接 setTimeout 排重试的分支（此时 `onDisconnected` 不调）。生产这条路径今日**不会**写入任何 toast/chat/approval/task/agent/toolCall store，但若未来有人在 catch 分支加副作用，本 change 的 R5 vitest **不会抓到**。该场景是 initial failure 不是 disconnect cycle，在 spec Req 2 之外
- **cleanup close**：组件卸载时 `useWebSocketConnection.ts` cleanup 里 `wsRef.current.onclose = null; close()` 主动关闭，此时 `onDisconnected` 也不会调。同样不在 Req 2 范围
- **多 session 切换**：`currentSessionId` 改变导致 WS reconnect 属于正常 UX，不属于回归

如将来发现 initial-failure / cleanup-close 分支被加副作用，开新 change 承载，不回溯补到本 change。

## 1. spec 契约落地

- [x] 1.1 重写 `specs/frontend-ws-handshake-regression/spec.md`，删除所有行号 / hook 名 / 日志 literal / Req #4，保留 3 条黑盒 invariant
- [x] 1.2 重写 `proposal.md`，移除 `chat-ui-migrate-ai-elements` Phase 2a 条件触发（已归档拒绝 `useHiveAgentEvents`）；明确 Playwright 延后到独立 change
- [x] 1.3 `openspec validate frontend-ws-handshake-regression --strict` 通过
- [x] 1.4 codex 审核后修正 spec.md 三处不可测/过软措辞（browser idle timeout / within one animation frame / 后端契约未锁死）
- [x] 1.5 codex 审核后修正 tasks 5（R2 scope 错位）+ 新增 R5（reconnect 可见性 integration）
- [x] 1.6 codex 二次审计后补四缺口：R1 改可执行后端锚定 / R5 加无错误态断言 / R2 加同节点+无重复占位断言 / test-setup 加 IntersectionObserver polyfill
- [x] 1.7 codex 三次审计后扩展 R5 无错误态断言覆盖全局 `useToastStore` error toast（C4）+ 明确排除 Header 状态指示器（C5 反面约束）
- [x] 1.8 codex 四次审计后把 C4 从 type-only 扩成 C4a（type）+ C4b（message 语义），堵住 `addToast('warning','连接中断...')` 绕过
- [x] 1.9 codex 五次审计后把 R5/C 从"文案黑名单"升级为"结构性不变量"（C6 toast 计数增量=0 + C7 messages 增量只允许流式 assistant 回复），堵住"网络波动，正在恢复通信"等自然文案绕过
- [x] 1.10 codex 六次审计后补 C8 inlineApprovals 增量=0 + C9 taskProgress activeGroups 不新增/不新增 failed 任务，堵住"审批卡承载错误文案"和"setTaskGroup failed 显示红字"两条绕过
- [x] 1.11 codex 七次审计后补 C10 useAgentActivityStore.sessionStatus 不进入 'error' 态，堵住 Sidebar SessionStatusDot 红点绕过
- [x] 1.12 codex 八次审计后补 C11 useChatStore.toolCallStatuses 错误态计数增量=0，堵住 ToolAdapter/ToolExecutionBlock 红色 Error 徽标绕过；同时扩展 C5 排除面到 Sidebar/AdminSidebar 连接状态指示器
- [x] 1.13 codex 九次审计后补 C12 是 C7 的姊妹项：messagesBefore 中每条现有消息在流程窗口内不得 is_error 从 false 翻成 true（堵 `setMessages` 原位改写绕过 C7a）；硬定 R1 测试文件必须落 `frontend/src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts` 且反射路径用 `fs.existsSync` fail-fast；硬定 C11 harness 预置必须发生在所有 C6-C11 基线快照之前；在 proposal scope 段明确"本 change 只覆盖 disconnect→reconnect 循环；initial connection failure 与 cleanup close 在范围之外"
- [x] 1.14 codex 十次审计后把 C12 从"按 timestamp 对位"升级为结构性"`messages.filter(is_error===true).length` 增量=0"（堵 setMessages 同时改 timestamp+is_error 的对位错位绕过）；补 C13 "role=tool 且 content 以 `tool error:` / `tool execution failed:` / `\[工具执行失败` 等前缀开头的计数增量=0"（堵 MessageBubble 对 tool 消息按 content 前缀判红色）；补 C14 "`[...activeGroups.values()].flatMap(tasks).filter(!!t.error).length` 增量=0"（堵 updateTask 只加 error 字段不改 status 绕过 C9）
- [x] 1.15 codex 十一次审计后补 C15 "既有 preset 消息 content 字段冻结"（堵 setMessages 原位改既有 assistant 消息 content 为 `'请稍候，系统正在重新同步'` 或 `{} as any` 的 `MessageBubble`/`MessageBubbleBoundary` 可见文案/"此消息渲染失败"兜底绕过）；统一 CI gate 文案：section 7 `tasks.md:213-214` 把 "四+一" / "四条" 改成 "五条"，与 section 3 proposal / section 8 归档门槛保持单一口径
- [x] 1.16 codex 十二次审计后把 C15 从 "三元组存在即可" 收紧为 "三元组在 after 中 matches.length === 1 且 content + tool_calls 字段全部字面相等"（堵 setMessages 追加"同三元组、不同 content" 副本的假绿通道 + `tool_calls.arguments` 被改成 `{} as any` 触发 `ToolExecutionBlock.tsx:45` raw.trim 异常 → boundary "此消息渲染失败" 绕过 C15b 的通道）；scope 段补一句"Sidebar/AdminSidebar 的 `sending || streaming` 禁用态是 chrome 级 UX，不是 error state，本 change 不兜"
- [x] 1.17 codex 十三次审计后四项修复：(a) **harness preset 顺序重排**为 `[preset-plain-assistant-2, preset-assistant-1, preset-tool-1]`——末尾放 role='tool' 的 `preset-tool-1` 使 `chat.ts:178-193` 的 "lastMsg.role==='assistant' && !lastMsg.tool_calls 即复用" 条件不满足、`ensureAssistantMessage` 必然新建第 4 条 placeholder，避免 partial content 覆盖 preset-plain-assistant-2 导致 C15b 自伤；(b) 补 **C16 "DOM 错误样式 absolute 计数 = 0"** 堵 `MessageList.tsx:298` 被加入静态 `<div className="text-red-500">通道暂不可用，请稍候</div>` 的纯 DOM 绕过；(c) 硬约束所有 C6-C16 使用 `expect.soft(...)` 软断言以便 M14/M15/M16/M17 等同时 red 多条 C 时 attribution 清晰可见（vitest soft 失败不中断后续断言）；(d) 改写 M16 为 `setMessages(map((m,i,arr) => i===arr.length-1 ? plainAssistant2Dup : m))`（replace 末条 length-neutral，C7a 绿 C15a 红证明 C15a 独立有效）；改写 M17 为 `tool_calls[0].name = 'forbidden_tool'`（保留合法 arguments 不触 `ToolExecutionBlock.tsx:45` raw.trim 异常，不走 `MessageBubbleBoundary` "此消息渲染失败" 兜底，C1 绿 C15d 红证明 C15d 独立有效）；新增 M18 静态红 div mutation 证明 C16 独立兜住
- [x] 1.19 codex 十七次审计（R17，plan-ceo-review + codex 交叉）暴露 W2 blocker 并落地修复：**C16 Stage 2 broad selector + token regex 覆盖缺口**——修复前 `dangerTokenRe` 的 `^` 锚点拒绝 `!text-red-500`（Tailwind `!important` prefix），`errorClassBroadSelector` 无 `[class*="!text-"]` / `[class*="-[var(--danger"]` / `[class*="-[#"]` 三组前缀，`!text-red-500` / `text-[var(--danger)]` / `bg-[#ef4444]` 三类常见 Tailwind 真实攻击面 silent-green（Node 旁路 regex 实测四条攻击 token 在旧 regex 下全 `false`）。修复：(a) `errorClassBroadSelector` 追加 `!text-` / `!bg-` / `!border-` / `!ring-` / `!outline-` + `-[var(--danger` / `-[var(--destructive` / `-[#` / `-[rgb` / `-[hsl` 共十条 broad 前缀；(b) 新增 `dangerArbitraryTokenRe = /^(?:text|bg|border|ring|outline)-\[(?:var\(--(?:danger|destructive)\)|#(?:ef4444|dc2626|b91c1c|f87171|fca5a5|fecaca)\b|rgb\([^)]*\b(?:239\s*,\s*68\s*,\s*68|220\s*,\s*38\s*,\s*38|185\s*,\s*28\s*,\s*28|248\s*,\s*113\s*,\s*113)\b)/i`；(c) `tokenHasDangerUtility` 加 `rawTok.replace(/^!/, '')` 剥 `!` prefix + 两条 regex 串联判定 + 继续 skip 带 `:` 的条件修饰 token（保留 shadcn Badge `aria-invalid:ring-destructive/20` baseline 绿）；(d) 新增 **M22**（`!text-red-500` → `[class*="!text-"]` + `replace(/^!/)` + `dangerTokenRe` 路径）、**M23**（`text-[var(--danger)]` → `[class*="-[var(--danger"]` + `dangerArbitraryTokenRe` var 分支）、**M24**（`bg-[#ef4444]` → `[class*="-[#"]` + `dangerArbitraryTokenRe` hex 分支）三条 mutation，逐条注入 `MessageList.tsx:298` 跑 vitest 均 fail-fast 于 `reconnect.test.tsx:257` baseline hard expect，回滚后 117 测试全绿；W1（codex 审计次数口径 "codex 十四次" 与 R17 不一致）、W3（AppShell 双源 chrome 级禁用态 scope 边界）列为 non-blocker，继续沿用 §1.17 的原文案
- [x] 1.20 codex 十八次审计（R18，plan-ceo-review + codex 交叉再审）暴露 4 条 blocker 并落地修复：**B1 conditional-modifier silent-green**——`tokenHasDangerUtility` 旧版 `if (rawTok.includes(':')) continue` 把所有带 `:` 的条件修饰 token 一律放过，`aria-busy:text-red-500` / `data-[state=offline]:text-red-500` / `hover:text-red-500` 等条件激活攻击面全 silent-green（M27 / M28 mutation 实测 fail 前旧代码直接跳过）。修复：新增 `splitAtTopColons(token)` 做括号深度敏感的 `:` 切分（`aria-[state=error]:text-red-500` 的 `:` 不在外层才分）+ `INACTIVE_STATIC_PREFIX` 白名单（`hover` / `focus` / `dark` / `sm` / `md` / `group-hover` 等静态修饰当前态不激活）+ `prefixActiveOnElement(el, prefix)` 对 `aria-<name>` / `aria-[<name>=<value>]` / `data-<name>` / `data-[<name>=<value>]` 四条分支查 element attribute 判定当前是否激活，全部 prefix 激活才判红；**B2 missing utility families**——`dangerTokenRe` 旧版只覆盖 `text|bg|border|ring|outline` 五族，`stroke-red-500`（SVG 图标描边）/ `fill-red-500` / `caret-red-500` / `decoration-red-500` / `accent-red-500` / `placeholder-red-500` / `from-red-500` / `via-red-500` / `to-red-500` / `divide-red-500` 十族全 silent-green（M25 mutation 在旧 regex 下实测 `false`）。修复：regex + `errorClassBroadSelector` 同步扩展 `stroke|fill|caret|decoration|accent|placeholder|from|via|to|divide` 十族 × `red|rose|danger|destructive` 四色家族 = 40 个新 broad selector + regex 分支；**B3 arbitrary value gaps**——旧 `dangerArbitraryTokenRe` 只硬编码 `#ef4444|dc2626|b91c1c|f87171|fca5a5|fecaca` 六个十六进制 + 四个 rgb 字面，`#ff0000`（纯红）/ `rgb(255,0,0)` / `hsl(0, 100%, 50%)` / `hsl(var(--danger))` 全 silent-green（M26 实测）。修复：改成程序化 `matchesArbitraryDanger(tok)` 函数——R 通道主导判定（hex 6位 R≥180 且 G/B≤100；hex 3位 R≥12 且 G/B≤6；rgb 同阈值；hsl 色相 0-20° 或 340-360°；`var(--danger)` / `var(--destructive)` / `hsl(var(--danger))` 直通）+ `color:` 前缀剥离，覆盖全色域 R 通道主导攻击面；**B4 M22/M23 独立性过度主张**——codex 审 M22/M23/M24 时质疑 "broad selector 家族重叠导致单点删除仍可被其他 broad 兜住，不是真独立"。Node 旁路实测：`!text-red-500` 含 `text-red-` 子串，旧 `[class*="text-red-"]` 本来就会命中；`text-[var(--danger)]` 含 `--danger` 子串；只有 `bg-[#ef4444]` 真依赖 `[class*="-[#"]`。修复：删掉 R17 时引入的冗余 broad 前缀 `[class*="!text-"]` / `[class*="!bg-"]` / `[class*="!border-"]` / `[class*="!ring-"]` / `[class*="!outline-"]` / `[class*="-[var(--danger"]` / `[class*="-[var(--destructive"]`，保留真独立的 `[class*="-[#"]` / `[class*="-[rgb"]` / `[class*="-[hsl"]`；M22/M23/M24 section 重写为 **独立性 probe**（删掉单一修复组件 → 攻击 silent-green → 恢复 → 红）证据链；(e) 新增 **M25**（`stroke-red-500` SVG 路径→10 族扩展兜底）、**M26**（`text-[#ff0000]` 纯红→程序化 hex R 通道判定）、**M27**（`aria-busy:text-red-500` + harness `setAttribute('aria-busy','true')` → `splitAtTopColons` + `prefixActiveOnElement` aria 简写分支）、**M28**（`data-[state=offline]:text-red-500` + harness `setAttribute('data-state','offline')` → `prefixActiveOnElement` data 精确匹配分支）四条 mutation，逐条注入 `MessageList.tsx:298` 跑 vitest 均 fail-fast 于 `reconnect.test.tsx:257` baseline hard expect，回滚后 117 测试全绿；独立性 probe 证据同步写入 `mutation-evidence.md` R18 段
- [x] 1.18 codex 十四次审计后三项修复：(a) **C16 升级为两阶段**——Stage 1 在 baseline snapshot 之后启动 `MutationObserver`（observe `childList/subtree/attributes`），整个 `disconnect → connect → partial → findByText` 窗口内捕获所有新增/属性变更节点，过滤 error signal selector + inline-style 正则，流程结束断言 `transientHits` 数组为空（堵 codex 十四次点名的 transient attack：handleDisconnected 加 DOM + partial 分支 remove，事后看不到但中途可见；Stage 2 final `querySelectorAll` 扫描 + 遍历所有 `[style]` 节点匹配 `inlineStyleRe` 正则（堵 inline-style `color: var(--danger)` 绕过 Tailwind class selector 的攻击）；(b) **C16 selector 大幅扩展**——纳入 `[role="status"/"banner"]`、`[aria-invalid="true"]`、`[data-state="error"]`、`text-rose-/pink-/orange-/amber-/yellow-`、`bg-rose-/orange-/amber-/yellow-/danger/destructive`、`border-red-/rose-/orange-/amber-`、`ring-red-/outline-red-`、`dialog[open]`；inline-style 正则匹配 `--danger|--destructive|color: red|rgb(...)` 等红色语义，覆盖黄/橙 warning、dialog portal、纯 inline-style、attribute 攻击全谱；(c) **baseline 前提修正**——`MessageList.tsx:300` 总渲 `TaskProgressPanel`，harness preset task `status:'running'` + `error: undefined` 时 TaskProgressPanel 所有红色分支（`TaskProgressPanel.tsx:52` group non-running `text-red-400` / line 81 `task.error` `text-red-400` / `StatusIcon` failed 分支 `text-red-500`）均不触发，所以 baseline absolute 计数确为 0；**加 baseline 硬 `expect`**（非 soft）在 MutationObserver 启动前 sanity check 一次，baseline 不干净直接 fail fast；**调整 harness 的 PR 必须同时审视 C16 baseline**（加到本 tasks.md 条款备注）；(d) 新增 **M19**（inline-style danger div 不含 class）、**M20**（transient 短暂元素，handleDisconnected 创建 + handleMessage partial 移除，证明 Stage 1 MutationObserver 独立兜底）、**M21**（attribute 攻击 `aria-invalid='true'` / `data-state='error'`，证明 attr 观察独立覆盖）三条 mutation

## 1A. 测试基座前置（codex 二次审计补入）

**目标**：R2 / R5 的 integration 测试会渲染 `MessageList`，其内部 `MessageList.tsx:103` 直接 `new IntersectionObserver(...)`；当前 `frontend/src/test-setup.ts` 只导入 `@testing-library/jest-dom`，**没有 IntersectionObserver polyfill**。JSDOM 也不原生提供。若不前置，R2/R5 测试 effect 执行时会抛 `IntersectionObserver is not defined`，测试变"红于基座而非红于契约"。

- [x] 1A.1 在 `frontend/src/test-setup.ts` 补最小 IntersectionObserver stub：
  ```ts
  if (typeof IntersectionObserver === 'undefined') {
    // @ts-expect-error — test-only stub, real IO not needed for our assertions
    globalThis.IntersectionObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
      takeRecords() { return []; }
      root = null;
      rootMargin = '';
      thresholds = [];
    };
  }
  ```
- [x] 1A.2 同时补 `ResizeObserver` 同形 stub（许多 shadcn/headless-ui 依赖的原语；防止以后扩展时反复补）
- [x] 1A.3 R2/R5 测试文件顶部**不得**再重复 stub（避免 double-stub 冲突）；顶部只写 `// IntersectionObserver polyfill 在 test-setup.ts` 注释提示
- [x] 1A.4 改动 test-setup.ts 后在现有 vitest suite 上跑一遍 `bun run test`，确认无现有测试因 stub 反向破坏

## 2. R4 — AppShell 双源优先级单测

**目标盲区**：URL params 与 chat store 的优先级被反过来，或两源同时为空时仍握手。

- [x] 2.1 新增 `frontend/src/layouts/__tests__/AppShell.test.tsx`
- [x] 2.2 断言矩阵（至少 4 个场景）：
  - A: URL 有 `abc-123` + store 有 `xyz-789` → 传递给 useWebSocket 的 sessionId **必须**是 `abc-123`
  - B: URL 空 + store 有 `xyz-789` → sessionId 必须是 `xyz-789`
  - C: URL 有 `abc-123` + store 空 → sessionId 必须是 `abc-123`
  - D: URL 空 + store 空 → sessionId 必须是 `undefined`
- [x] 2.3 mock 策略：`vi.mock('../../hooks/useWebSocket')` 捕获 props；`vi.mock('react-router-dom', ...)` 控制 `useParams` 返回值；通过 `useChatStore.setState` 控制 store
- [x] 2.4 **mutation 验证**：把 `AppShell.tsx:32` 改成 `storeSessionId || urlSessionId` → A 场景转红

## 3. R3 — handleDisconnected 不清零单测

**目标盲区**：有人"优化"WS onclose 回调重新清空 `currentSessionId`，导致 reconnect 首帧丢失（Sprint 12 P0 根因）。这是 R5 reconnect 可见性的**必要条件**（快速 unit 级 fast-fail），不是充分条件。

- [x] 3.1 新增 `frontend/src/hooks/__tests__/useWebSocket.handleDisconnected.test.ts`
- [x] 3.2 mock 策略：`vi.mock('../useWebSocketConnection')` 返回 stub `{ connected: false, send: vi.fn() }`，同时把传入 options 的 `onDisconnected` 回调**用 module-level ref 捕获**（实现参考：mock factory 里 `captured.onDisconnected = options.onDisconnected` 然后 export `captured`）
- [x] 3.3 renderHook 用 `useWebSocket({ url: 'ws://test', sessionId: 'keep-me', client: mockClient })`；`useChatStore.setState({ currentSessionId: 'keep-me' })`
- [x] 3.4 `act(() => captured.onDisconnected())` 触发 handleDisconnected
- [x] 3.5 断言 `useChatStore.getState().currentSessionId === 'keep-me'`
- [x] 3.6 **mutation 验证**：在 `useWebSocket.ts:274` handleDisconnected 里加一行 `useChatStore.setState({ currentSessionId: null })` → 测试转红

## 4. R1 — useWebSocketConnection URL 拼接单测

**目标盲区**：唯一生产 WS URL 的 `useWebSocketConnection.ts:52` 把 key 改成 `sessionId` / `sid`、把值 URL-encode 错、或省略 `?`/`&` 拼接。

- [x] 4.1 新增 `frontend/src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts`
- [x] 4.2 全局 mock `WebSocket` 构造函数（`vi.stubGlobal('WebSocket', MockWS)` where `MockWS = class { constructor(url, protocols) { capturedCalls.push({ url, protocols }); this.readyState = 0; } close() {} }`），捕获 `new WebSocket(url, protocols)` 的 `url` 参数
- [x] 4.3 renderHook 前提：必须 `{ url: 'ws://h/api', sessionId: '...', enabled: true }` 三者都满足 `useWebSocketConnection.ts:42-47` 的 early-return 条件，否则 effect 不触发 `connect()`、`new WebSocket` 不会被调，测试变成假绿
- [x] 4.4 断言矩阵：
  - sessionId `'abc-123'` + url `'ws://h/api'` → 捕获 URL 匹配 `/^ws:\/\/h\/api\?session_id=abc-123$/`
  - sessionId `'abc-123'` + url `'ws://h/api?foo=1'` → 捕获 URL 匹配 `/\?foo=1&session_id=abc-123$/`（正确拼接 `&`）
  - sessionId 含特殊字符 `'abc/+&='` → 捕获 URL 中 value 部分必须与原值 `encodeURIComponent` 等价可反解
  - sessionId `undefined` → 捕获 URL 不含 `session_id=`
- [x] 4.5 **后端契约锁定（可执行反射，codex 九次审计加硬约束）**：
  - **硬约束 A**（文件落点）：测试文件**必须**落在 `frontend/src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts`，不得改名、不得换目录。该路径在 proposal.md "Impact" 和本 tasks 4.1 共同声明，实施者需保持一致
  - **硬约束 B**（解析基点）：反射路径**不得**依赖 `__dirname`（vitest/node 对 `__dirname` 支持差异、且测试文件漂移会把路径算错到仓库外）。**必须**用 vitest 的 `import.meta.url` 或仓库根相对：优先实现 `const repoRoot = path.resolve(fileURLToPath(import.meta.url), '../../../../..')` 或 `process.cwd()` 向上 find-up `go.mod` 作为仓库根锚点，然后 `path.join(repoRoot, 'internal/streaming/websocket.go')`
  - **硬约束 C**（fail-fast）：解析到的路径**必须**先 `fs.existsSync(backendPath) || throw new Error(\`R1 测试锚定的后端 Go 源文件不存在：\${backendPath}。可能仓库结构变动，请检查路径解析逻辑或 find-up 仓库根方式\`)`——任何时候解析不到文件都立即 fail（而不是后续正则 undefined 造成伪绿）
  - **主断言**：`fs.readFileSync(backendPath, 'utf-8')`，用正则 `/r\.URL\.Query\(\)\.Get\("([^"]+)"\)/` 提取**第一个匹配**的 key 字面量作为 `expectedKey`，再断言 `new URL(capturedUrl).searchParams.has(expectedKey)` + `.get(expectedKey) === sessionId`。前端代码改 key 同时改测试里的前端字面量**无法绕过**，因为 expectedKey 来自后端源文件
  - 补充守卫：若正则未匹配（后端代码结构变动）→ 测试 fail fast 提示"后端 WS handler 契约源码已变更，请更新 frontend R1 测试的反射路径"
  - 补充守卫：若匹配到多个 `r.URL.Query().Get(...)` → 测试 fail fast 要求显式选择（避免误抓其他 query key）
- [x] 4.6 **mutation 验证**（至少 4 条）：
  - 把 `useWebSocketConnection.ts:52` 的 `session_id` 改成 `sessionId`（**同时**把 R1 测试里前端字面量也改成 `sessionId`）→ 测试**仍然转红**（因为 expectedKey 来自后端 go 文件，不会被前端 PR 改掉）
  - 把 `useWebSocketConnection.ts:52` 的 `session_id` 改成 `sessionId` 但**不**改后端 → 同上，测试转红
  - 去掉 `url.includes('?') ? '&' : '?'` 的分支（永远用 `?`） → 场景 2 转红
  - 把 value 直接插入不走 URL-encode → 场景 3 转红

## 5. R2 — handleMessage partial 分支 → DOM 渲染 integration 测试

**目标盲区**：`useWebSocket.handleMessage` 的 partial 分支逻辑（见 `useWebSocket.ts:95-139`）。真实回归形态是调用顺序被改坏，**不是 store action 本身**。Codex 指出的 attack path 例证：把 partial 分支改成"只有 streamingMessageId 已存在才 updateLastAssistant"——生产首帧静默消失，但仍通过 R1/R3/R4。

**原 scaffold 的 5.3 直接调 store action 是 scope 错位——绕过了 handleMessage，抓不到真实回归。本次重写。**

- [x] 5.1 新增 `frontend/src/components/chat/__tests__/partialMessageRendering.test.tsx`
- [x] 5.2 mock `useWebSocketConnection`，用 module-level ref 捕获传入的 `onMessage` 回调（同 R3 的 mock 模式）
- [x] 5.3 渲染组合：把 AppShell 的 useWebSocket 调用 + MessageList 组合在一个 test harness 组件里（或直接调 `useWebSocket()` + 渲染 `<MessageList messages={useChatStore((s) => s.messages)} />`），sessionId 预置 `'test-sid'`，chat store 预置 `currentSessionId: 'test-sid'`
- [x] 5.4 通过捕获的 `onMessage` 推一条真实 `WSMessage`：
  ```
  { type: 'message',
    payload: { session_id: 'test-sid', partial: true,
               content: '首个可见片段', role: 'assistant' } }
  ```
  —— **这一步走完整 handleMessage → ensureAssistantMessage → RAF → updateLastAssistant → MessageList → Streamdown 链路**
- [x] 5.5 断言 `await screen.findByText('首个可见片段')` 能在 DOM 中找到。`findByText` 默认 1s 超时，自然等 RAF 排队（无需手动 runAllTimers / advanceTimersByTime；若必要可 `await act(async () => { await new Promise((r) => requestAnimationFrame(r)); })`）
- [x] 5.6 **同节点更新 + 无重复占位断言**（codex 二次审计补入）：
  - 第一段 partial 推入后：`const firstNode = screen.getByText('首个可见片段')`；记录 `firstNode` 引用
  - 再推一条 partial content `'首个可见片段 + 第二段'`
  - 断言 A：`await screen.findByText('首个可见片段 + 第二段')` 可见
  - 断言 B：`useChatStore.getState().messages.filter((m) => m.role === 'assistant').length === 1`（store 层：assistant 消息计数未增加）
  - 断言 C：更新后 `screen.getByText('首个可见片段 + 第二段')` 所在 DOM element 应当**包含**或**等于** `firstNode`（通过 `firstNode.isConnected === true && firstNode.textContent?.includes('第二段')` 验证同节点被更新而不是被替换）
  - 断言 D：`screen.queryAllByText(/首个可见片段(?! \+ 第二段)/)` 应为空数组（旧占位文本未残留）
- [x] 5.7 **mutation 验证**（至少 3 条）：
  - **M1**：`useWebSocket.ts:95-139` partial 分支改成"只有 `streamingMessageId` 存在才 `updateLastAssistant`，否则什么都不做"（即首帧 `ensureAssistantMessage` 调用前先 return）→ 断言 A 红
  - **M2**：删除 `useWebSocket.ts:128` 的 `ensureAssistantMessage()` 调用（保留 `updateLastAssistant` 和 RAF，只去掉 placeholder 创建）→ 断言 A 红
  - **M3**（codex 二次审计补入）：把 `chat.ts:178-193` 改成每次 partial 都新建 placeholder（或把 `useWebSocket.ts:128-138` partial 分支改成每次都调 `ensureAssistantMessage` 并让 `ensureAssistantMessage` 总是新建），DOM 里第二段文本仍能找到但产生重复 assistant 气泡 → 断言 B/C/D 至少一条红

## 6. R5 — reconnect 后下一条消息可见 integration 测试（新增）

**目标盲区**：spec Requirement 2 "Session identity survives the disconnect → reconnect cycle" 的**可见性断言**。R3 只验证 currentSessionId 不清零是必要条件；这条从端到端覆盖"close → reconnect → send → partial → DOM 可见"完整链路。

- [x] 6.1 新增 `frontend/src/hooks/__tests__/useWebSocket.reconnect.test.tsx`
- [x] 6.2 mock `useWebSocketConnection`，用 module-level ref 同时捕获 `onMessage` + `onDisconnected` + `onConnected`
- [x] 6.3 renderHook useWebSocket，sessionId `'sid-1'`；store 预置 `currentSessionId: 'sid-1'`；渲染 MessageList
- [x] 6.4 流程（顺序严格）：
  1. `act(() => captured.onDisconnected())` 模拟 WS 断开
  2. `act(() => captured.onConnected())` 模拟重连
  3. 推一条 partial WSMessage（payload session_id 是 `'sid-1'`）
  4. `await screen.findByText(...)` 断言可见
- [x] 6.5 断言点：
  - **前置硬约束（codex 九/十/十一次审计）**：**任何 harness 预置数据**（C11 `tool_calls` assistant 消息 + `toolCallStatuses['tc-1']='running'`；C12/C15 既有 `is_error:false` assistant 消息；C13 `{role:'tool', content:'{"ok":true}', is_error:false}` 消息；C14 `setTaskGroup({ g1: [{ t1: running, error: undefined }] })`；**C15 要求 harness 为每条 preset 消息赋唯一 timestamp 并记录 presetSnapshot**）**必须在** C6-C15 基线快照**之前**注入 store。顺序：
    1. `useChatStore.setState({ currentSessionId: 'sid-1', streamingMessageId: null, messages: [presetPlainAssistantMsg /* ts:'preset-plain-assistant-2', role:'assistant', content:'历史回复 B', is_error:false, tool_calls:undefined */, presetAssistantMsgWithToolCalls /* ts:'preset-assistant-1', role:'assistant', content:'历史回复 A', is_error:false, tool_calls:[{id:'tc-1', name:'search', arguments:'{"q":"x"}'}] */, presetToolResultMsg /* ts:'preset-tool-1', role:'tool', tool_call_id:'tc-1', content:'{"ok":true}', is_error:false, tool_calls:undefined */], toolCallStatuses: { 'tc-1': { status: 'running' } } })` —— **harness 赋唯一 timestamp** 避免 confirmUserMessage 漂移冲突；**`streamingMessageId: null` + 把 role='tool' 的 `preset-tool-1` 放最后**（codex 十三次审计补入，堵 `chat.ts:178-193` 的"lastMsg.role==='assistant' && !lastMsg.tool_calls 即复用"陷阱——若 plain assistant 在尾部，`ensureAssistantMessage` 会直接复用并让后续 `updateLastAssistant` 覆盖其 `content`，造成 C15b 自伤）。把 tool 置尾后，`lastMsg.role === 'tool'` 条件不满足，`ensureAssistantMessage` 必然 push 第 4 条新 assistant placeholder，C15 preset 全部安全
    2. `useTaskProgressStore.getState().setTaskGroup({ group_id:'g1', tasks:[{ id:'t1', status:'running', error: undefined }] })`
    3. `const presetSnapshot = [{ timestamp:'preset-assistant-1', role:'assistant', tool_call_id: undefined, content:'历史回复 A', tool_calls: [{ id:'tc-1', name:'search', arguments:'{"q":"x"}' }] }, { timestamp:'preset-tool-1', role:'tool', tool_call_id:'tc-1', content:'{"ok":true}', tool_calls: undefined }, { timestamp:'preset-plain-assistant-2', role:'assistant', tool_call_id: undefined, content:'历史回复 B', tool_calls: undefined }]` （C15 baseline；**tool_calls 字段也是契约面**，codex 十二次审计补入——`MessageBubble.tsx:313` 按 `tool_calls.length` 渲染工具卡、`ToolExecutionBlock.tsx:45` 会对 `arguments` 做 `raw.trim()`，任何对 `tool_calls` 的改写都是可见面）
    4. `const toastBefore = useToastStore.getState().toasts.length`
    5. `const messagesLengthBefore = useChatStore.getState().messages.length`
    6. `const isErrorCountBefore = useChatStore.getState().messages.filter(m => m.is_error === true).length` （C12 baseline，预期 0）
    7. `const toolErrorContentBefore = useChatStore.getState().messages.filter(m => m.role === 'tool' && /^(tool error:|tool execution failed:|tool '.*' .*not allowed|ToolBridge not initialized|\[工具调用被中断|\[工具执行失败)/i.test(m.content || '')).length` （C13 baseline，预期 0）
    8. `const approvalsBefore = useChatStore.getState().inlineApprovals.length`
    9. `const groupsBefore = new Map(useTaskProgressStore.getState().activeGroups)`
    10. `const taskErrorCountBefore = [...useTaskProgressStore.getState().activeGroups.values()].flatMap(g => g.tasks).filter(t => !!t.error).length` （C14 baseline，预期 0）
    11. `const agentStatusBefore = useAgentActivityStore.getState().sessionStatus['sid-1']`
    12. `const toolErrorStatusBefore = Object.values(useChatStore.getState().toolCallStatuses).filter((s) => s?.status === 'error').length`
    13. **随后**才进入 disconnect → reconnect → partial 流程。否则 harness baseline 会被 C7a/C12/C13/C14/C15 记成增量，造成假红（自伤）
  - 断言 A：disconnect 后 `useChatStore.getState().currentSessionId === 'sid-1'`（R3 的 subset，此处一并验证）
  - 断言 B：reconnect 后首个 partial 文本到达 DOM（R2 的端到端版本）
  - **断言 C**（codex 二次/三次/四次/五次审计迭代产物，spec Req 2 "no visible error state" 映射）：在 disconnect→reconnect→partial 全流程期间，在 findByText partial 可见**之前**和**之后**各断言一次以下可见错误面全部为空。**核心策略改变**：从"文案黑名单"升级为"结构性不变量"——测试窗口内不得新增任何 toast、不得新增除流式 assistant 回复之外的任何聊天消息。文案关键字断言（C1/C4b）作为**辅助信号**保留（用于错误诊断时定位），但**结构断言**（C6/C7）才是契约主力。
    **断言实现硬约束（codex 十三次审计补入，expect.soft 软断言）**：**所有 C6 - C16 必须使用 `expect.soft(...)` / `expect.soft(actual).toBe(expected)` 形式**（vitest 提供的软断言），而不是硬 `expect(...)`。原因：硬断言在第一条 fail 时抛异常，后续断言不执行；M14 / M15 / M16 / M17 等 mutation 可能同时 red 多条 C（例如 M16 既 red C7a 又 red C15a，M17 既 red C1 又 red C15d），若用硬断言则只能看到"最先一条 fail 的"，无法证明 C15a/C15d 是独立有效的结构兜底。软断言让每条 C 都评估、失败记录汇总，PR 评审时能直接看到 `M16 → C7a red + C15a red` 的完整证据。

    **结构断言（主力，codex 五次/六次审计引入）**：
    - **C6**：测试窗口内 toast 计数增量 = 0。实现：流程开始前快照 `const toastBefore = useToastStore.getState().toasts.length`；全流程结束后断言 `useToastStore.getState().toasts.length === toastBefore`。**堵死所有自然文案 toast**（"网络波动，正在恢复通信" / "通道暂不可用，请稍候" 等都无法绕过）
    - **C7**：测试窗口内聊天消息只允许流式 assistant 回复新增。实现：流程开始前快照 `const messagesBefore = useChatStore.getState().messages.map(m => ({ timestamp: m.timestamp, is_error: m.is_error === true }))`（记录 id + is_error 位图，便于 C12 做位级比对）；partial 可见断言后：
      - C7a：`useChatStore.getState().messages.length - messagesBefore.length ∈ {0, 1}`（最多 +1，对应 assistant 流式 placeholder；若 `ensureAssistantMessage` 复用已有 placeholder 则 0）
      - C7b：新增的那条（若有）必须 `role === 'assistant'` 且 **不是** `is_error === true`
      - **堵死所有 "推一条 system/assistant 说明性消息" 的绕过**
    - **C12**（codex 九/十次审计收敛为结构性增量）：`useChatStore.getState().messages.filter(m => m.is_error === true).length` **增量 = 0**。实现：流程开始前（harness 预置之后、disconnect 之前）快照 `const isErrorCountBefore = useChatStore.getState().messages.filter(m => m.is_error === true).length`；harness 预置要求**全部** messages 带 `is_error: false`（或不设 is_error 等价 falsy），形成 baseline = 0；流程结束后断言 `useChatStore.getState().messages.filter(m => m.is_error === true).length === isErrorCountBefore`。**堵死所有按 timestamp 对位可能错位的攻击**（`confirmUserMessage`（`chat.ts:325`）会改 timestamp；`setMessages`（`chat.ts:214`）能整体改写；消息去重键是 `timestamp + role + tool_call_id` 三元组不是单 timestamp，见 `chat.ts:42/65`）。任何把既有 assistant 消息翻成 `is_error: true` 的写法（包括 `setMessages` 原位改写 + 同时改 timestamp）都会被结构计数兜住
    - **C13**（codex 十次审计补入）：`useChatStore.getState().messages.filter(m => m.role === 'tool' && /^(tool error:|tool execution failed:|tool '.*' .*not allowed|ToolBridge not initialized|\[工具调用被中断|\[工具执行失败)/i.test(m.content || '')).length` **增量 = 0**。实现：harness 预置一条 `{ role: 'tool', tool_call_id: 'tc-1', content: '{"ok":true}', is_error: false }` 消息（非错误前缀）；流程开始前快照 `const toolErrorContentBefore = 计算式`；流程结束后断言计数 === baseline。**堵死 setMessages 把既有 tool 消息 content 改写成错误前缀绕过 C12 的攻击路径**（`MessageBubble.tsx:175-184` 对 `role='tool'` 按 content 前缀判 `isError` 渲染红色 `resultStatus='error'`，不看 `is_error` 字段；C12 查 `is_error` 纯字段不触发）
    - **C14**（codex 十次审计补入）：`[...useTaskProgressStore.getState().activeGroups.values()].flatMap(g => g.tasks).filter(t => !!t.error).length` **增量 = 0**。实现：harness 预置一个 group 带 1 条 `{ id:'t1', status:'running', error: undefined }` 任务（不是 failed 也没有 error 字段）；流程开始前快照 `const taskErrorCountBefore = 计算式`；流程结束后断言计数 === baseline。**堵死 `updateTask(groupId, taskId, { error: '连接已中断' })` 在不改 status 情况下让 `TaskProgressPanel.tsx:80` 渲染红字错误文案绕过 C9 的攻击路径**（C9 只查 group 数量和 `failed` status，不查 `task.error` 字段）
    - **C16**（codex 十三/十四次审计收敛，DOM 错误状态 absolute + transient 双通道）：spec Req 2 要求"no visible error state SHALL be shown between the user message and the assistant's first visible chunk"——意味着**整个窗口期间**不得出现错误态 DOM，**不只是 partial 可见时刻**。因此 C16 采用**两阶段**实现：

      **Stage 1: MutationObserver 动态捕获**（堵 transient attack——DOM 元素在 disconnect→partial 之间创建又在 partial 之前移除，事后扫描看不到）。实现：在 harness setState 完成、C6-C15 baseline snapshot 之后、`captured.onDisconnected()` 之前，启动：
      ```ts
      const errorSignalSelector = [
        '[role="alert"]', '[role="status"]', '[role="banner"]',
        '[aria-live="assertive"]', '[aria-live="polite"]',
        '[aria-invalid="true"]', '[data-state="error"]',
        '[class*="text-red-"]', '[class*="text-rose-"]', '[class*="text-pink-"]',
        '[class*="text-orange-"]', '[class*="text-amber-"]', '[class*="text-yellow-"]',
        '[class*="text-danger"]', '[class*="text-destructive"]',
        '[class*="--danger"]', '[class*="--destructive"]',
        '[class*="bg-red-"]', '[class*="bg-rose-"]', '[class*="bg-orange-"]',
        '[class*="bg-amber-"]', '[class*="bg-yellow-"]', '[class*="bg-danger"]', '[class*="bg-destructive"]',
        '[class*="border-red-"]', '[class*="border-rose-"]', '[class*="border-orange-"]', '[class*="border-amber-"]',
        '[class*="ring-red-"]', '[class*="outline-red-"]',
        'dialog[open]',
      ].join(', ');
      const inlineStyleRe = /(?:--danger|--destructive|color:\s*(?:red|#[fF][0-9a-fA-F]{2}|rgb\([^)]*(?:2[0-9]{2}|1[89][0-9])\b))/i;
      const transientHits: { kind: string; summary: string }[] = [];
      const recordElement = (el: Element, kind: string) => {
        if (el.matches(errorSignalSelector)) transientHits.push({ kind, summary: `${kind}: matches selector (${el.outerHTML.slice(0, 120)})` });
        const style = el.getAttribute('style') || '';
        if (inlineStyleRe.test(style)) transientHits.push({ kind, summary: `${kind}: inline-style danger (${el.outerHTML.slice(0, 120)})` });
        el.querySelectorAll?.(errorSignalSelector).forEach((child) => transientHits.push({ kind: `${kind}:descendant`, summary: child.outerHTML.slice(0, 120) }));
      };
      const mo = new MutationObserver((mutations) => {
        for (const m of mutations) {
          m.addedNodes.forEach((n) => n instanceof Element && recordElement(n, 'added'));
          if (m.type === 'attributes' && m.target instanceof Element) recordElement(m.target, `attr:${m.attributeName}`);
        }
      });
      mo.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['class', 'style', 'role', 'aria-live', 'aria-invalid', 'data-state', 'open'] });
      ```
      完整跑完 `disconnect → connect → partial → findByText`，然后 `mo.disconnect()`，**断言 `expect.soft(transientHits).toEqual([])`**。

      **Stage 2: Final absolute 扫描**（堵 static attack——生产组件被植入常驻错误元素，既在 baseline 存在也在流程结束存在）。实现：findByText 断言完成后：
      ```ts
      const errorElements = Array.from(document.querySelectorAll(errorSignalSelector));
      const inlineDangerElements = Array.from(document.querySelectorAll('[style]')).filter((el) => inlineStyleRe.test(el.getAttribute('style') || ''));
      expect.soft(errorElements.length).toBe(0);
      expect.soft(inlineDangerElements.length).toBe(0);
      ```

      **Baseline 契约前提（codex 十四次审计修正）**：R5 harness 渲染面是 `MessageList`（其尾部**总是**渲染 `TaskProgressPanel`，见 `MessageList.tsx:300`）。harness preset `setTaskGroup({ group_id:'g1', tasks:[{ id:'t1', status:'running', error: undefined }] })` 让 TaskProgressPanel 渲染，但：
        - `StatusIcon` 对 `status:'running'` 走 `Loader2 text-[var(--accent-500)]`（accent 蓝，非红/黄/橙）；
        - group 整体 `status==='running'` 不进入 `TaskProgressPanel.tsx:52` 的 `text-red-400` 分支；
        - `task.error === undefined` 不进入 `TaskProgressPanel.tsx:81` 的 `text-red-400 error` 文本；
        - 所有 preset messages `is_error:false` 不触发 `MessageBubble.tsx:309` 的 ErrorCard。
      所以 baseline absolute 计数 0 成立。**调整 harness 的 PR 必须同时审视 C16 baseline**——把 task.status 改 'failed'、task.error 加字符串、任一 message.is_error 翻 true，baseline 立即失效、C16 假红。

      **Baseline 验证硬约束**：harness setState 完成、MutationObserver 启动**之前**，先跑一次 sanity：`expect(document.querySelectorAll(errorSignalSelector).length).toBe(0); expect(Array.from(document.querySelectorAll('[style]')).filter((el) => inlineStyleRe.test(el.getAttribute('style')||'')).length).toBe(0);`——这是**硬** `expect`（非 soft），因为 baseline 不干净就没资格进后续测试。

      **堵的 attack paths（列举）**：
      - **M18 静态红 class div**（`<div className="text-red-500">通道暂不可用</div>`）→ Stage 2 final 扫描命中
      - **M19 inline-style danger**（`<div style={{ color: 'var(--danger)' }}>通道暂不可用</div>`，**无 class 关键字**）→ Stage 2 inlineDangerElements 命中
      - **M20 transient 短暂元素**（`handleDisconnected` 里 `document.createElement('div') + style.color='var(--danger)' + appendChild`；`handleMessage partial` 里 `remove()`）→ Stage 1 MutationObserver 的 added 阶段已记录，即便事后移除 transientHits 仍非空
      - **M21 attribute 攻击**（preset 消息节点被改 `aria-invalid='true'` 或 `data-state='error'`）→ Stage 1 MutationObserver 的 attr 阶段捕获
      - **warning/amber/yellow 色调** / **border-red-* ring-red-*** / **`<dialog open>` portal** / **`role="status"` 动态 polite 公告** → 全部在 selector 覆盖内

      **这是 spec Req 2 "no visible error state"（贯穿窗口）到 DOM 结构的唯一契约兜底**；C1/C2/C4b 文案黑名单属于诊断辅助，不作为契约主力。
    - **C15**（codex 十一/十二次审计收敛，既有消息完整冻结）：所有 harness preset 消息在流程窗口内不得被改写、不得被重复、不得被追加同三元组副本。实现：harness 预置阶段记录 `const presetSnapshot = [{ timestamp:'preset-assistant-1', role:'assistant', tool_call_id: undefined, content:'历史回复 A', tool_calls: [{ id:'tc-1', name:'search', arguments:'{"q":"x"}' }] }, { timestamp:'preset-tool-1', role:'tool', tool_call_id:'tc-1', content:'{"ok":true}', tool_calls: undefined }, { timestamp:'preset-plain-assistant-2', role:'assistant', tool_call_id: undefined, content:'历史回复 B', tool_calls: undefined }]`（timestamp 由 harness 赋唯一值）；流程结束后对 presetSnapshot 中每一条，在 `useChatStore.getState().messages` 里用三元组 `{timestamp, role, tool_call_id}` **过滤**出 `matches = messages.filter(m => m.timestamp === p.timestamp && m.role === p.role && (m.tool_call_id || undefined) === (p.tool_call_id || undefined))`：
      - **C15a（存在且唯一）**：`matches.length === 1`（堵 mutation 删 preset 消息——matches.length=0 会 fail；堵 mutation 追加同三元组副本——matches.length=2 会 fail）
      - **C15b（content 字面冻结）**：`matches[0].content === preset.content` 严格相等（堵 setMessages 原位改 content 成 "请稍候，系统正在重新同步" 等自然文案）
      - **C15c（content 类型）**：`typeof matches[0].content === 'string'`（堵 setMessages 改 content 为 `{} as any` 触发 `artifactParser.ts:58` 的 `raw.slice` 异常 → `MessageBubbleBoundary.tsx:56-60` 渲染 "此消息渲染失败"）
      - **C15d（tool_calls 冻结）**：`JSON.stringify(matches[0].tool_calls) === JSON.stringify(preset.tool_calls)` 深度相等（堵 mutation 改 `tool_calls[0].arguments = {} as any` 导致 `ToolExecutionBlock.tsx:45` raw.trim 异常 → boundary "此消息渲染失败"；堵 mutation 换 `tool_calls[0].name = 'forbidden_tool'` 造成可见工具卡漂移）
      - **堵死所有"原位改写既有消息"的可见内容漂移通道**（`MessageBubble.tsx:266` 把 `!is_error` assistant content 直接当正常气泡渲染；`MessageBubble.tsx:313` 按 `tool_calls.length` 渲染工具卡；`MessageBubbleBoundary.tsx:56-60` 兜底）
      - **Harness 预置时必须 `useChatStore.setState({ streamingMessageId: null })` 确保 partial 流程走 `ensureAssistantMessage` 新建第 4 条 placeholder 而非追加到 preset 消息**——否则 `updateLastAssistant` 会把 partial content 追加到 preset_plain_assistant_2 上，C15b 自伤
    - **C8**（codex 六次审计补入）：`useChatStore.inlineApprovals` 增量 = 0。实现：快照 `const approvalsBefore = useChatStore.getState().inlineApprovals.length`；断言 `useChatStore.getState().inlineApprovals.length === approvalsBefore`。**堵死用审批卡承载"网络波动"文案的绕过**（`chat.ts:241` 的 `addInlineApproval`；`MessageList.tsx:218/241` 渲染）
    - **C10**（codex 七次审计补入）：`useAgentActivityStore.sessionStatus[currentSessionId]` 不进入 `'error'` 态。实现：快照 `const statusBefore = useAgentActivityStore.getState().sessionStatus[currentSessionId]`；断言 `useAgentActivityStore.getState().sessionStatus[currentSessionId] !== 'error'`（即使原本是 undefined/idle/running，也不得被 disconnect 流程改写为 'error'）。**堵死 Sidebar SessionStatusDot 红点绕过**——`agentActivity.ts:25` 写入 `error`，`Sidebar.tsx:15-113` 渲染红点
    - **C11**（codex 八次审计补入）：`useChatStore.toolCallStatuses` 的 error 态计数增量 = 0。实现：流程开始前先在测试 harness 里预置一条 assistant 消息携带 `tool_calls: [{ id: 'tc-1', name:'x', arguments:'{}' }]` + 一个 running 状态 `toolCallStatuses['tc-1'] = { status:'running' }`（建立可被攻击的 state），快照 `const toolErrorBefore = Object.values(useChatStore.getState().toolCallStatuses).filter((s) => s?.status === 'error').length`；全流程结束后断言 `Object.values(useChatStore.getState().toolCallStatuses).filter((s) => s?.status === 'error').length === toolErrorBefore`。**堵死 ToolAdapter/ToolHeader/ToolExecutionBlock 红色 Error 徽标绕过**——`chat.ts:278` setToolCallStatus 可把已存在 `tool_call_id` 改为 `{ status: 'error' }`，`ToolAdapter.tsx:44` 订阅 `toolCallStatuses?.[id]`，`ai-elements/tool.tsx:47` 把 `output-error` 状态渲染成红色 ToolHeader + 红框。该通道不触发 C6/C7/C8/C9/C10 任何断言
    - **C9**（codex 六次审计补入）：`useTaskProgressStore.activeGroups` 状态不引入新失败态。实现：快照 `const groupsBefore = new Map(useTaskProgressStore.getState().activeGroups)`；断言：
      - C9a：`activeGroups.size === groupsBefore.size`（**不得新增 group**）
      - C9b：对每个仍存在的 groupId，`status !== 'failed'` 的任务数不下降、且 `tasks.filter((t) => t.status === 'failed' && !groupsBefore 里对应 group 的 same-id task 也是 failed).length === 0`（**不得在现有 group 里新制造 failed 任务**）
      - 简化实现：若 mutation 难覆盖 group-level diff，可落一个保守版 `activeGroups.size === groupsBefore.size && [...activeGroups.values()].every((g) => g.tasks.every((t) => t.status !== 'failed' || groupsBefore.get(g.groupId)?.tasks.find((gt) => gt.id === t.id)?.status === 'failed'))`
      - **堵死通过 `setTaskGroup({status: 'failed', error: '连接已中断'})` 在 `TaskProgressPanel.tsx:80` 显示红字失败的绕过**

    **辅助文案断言（诊断信号，保留但非契约主力）**：
    - C1：chat 区文案 `screen.queryByText(/连接中断|连接失败|重连失败|reconnect failed|error|失败/i) === null`——命中即 fail，没命中不代表 pass（C6/C7 负责兜底）
    - C2：ARIA alert role——`screen.queryAllByRole('alert').length === 0`
    - C3：chat store 错误字段——`useChatStore.getState().error == null`（若 store 无 error 字段则跳过）
    - C4a：`useToastStore.getState().toasts.filter((t) => t.type === 'error').length === 0`（被 C6 完全覆盖，但保留作为具体化诊断）
    - C4b：`useToastStore.getState().toasts.filter((t) => /断连|中断|失败|重连|reconnect|connection lost|offline|disconnect/i.test(t.message)).length === 0`（同上，被 C6 完全覆盖）
    - **C5 明确排除**（codex 八次审计扩展）：`Header.tsx:154` 的 `connected ? t('common.connected') : t('common.disconnected')`、`Sidebar.tsx:376` 附近及 `AdminSidebar.tsx:73` 基于 `connected` 的连接状态栏、`AppShell.tsx:43` / `AdminShell.tsx:31` 传入的 `connected={false}` 均是**合法状态指示器**，不是 spec Req 2 所指 "error state"——disconnect 窗口内显示 "已断开" 是正确行为。这些指示器**不渲染在 MessageList 作用域内**、**不写入 toast / chat messages / inlineApprovals / taskProgress / agentActivity / toolCallStatuses**，因此 C6/C7/C8/C9/C10/C11 天然不触及。断言 C **不得**把 `/disconnected|已断开|未连接/` 列为禁词。这是一条**反面约束**：实施者不要把状态指示器当错误态抓
    - 这组断言的核心：用"结构性禁止新增提示"替代"语义黑名单"，任何形式的用户可见提示（无论文案措辞）都会让 C6/C7 红
- [x] 6.6 **mutation 验证**（至少 8 条，最终 28 条 M1-M28）：
  - **M1**：在 handleDisconnected 里加 `useChatStore.getState().clearMessages()` 清掉 streamingMessageId → 后续 partial 的 `updateLastAssistant` 找不到 target，DOM 空白 → 断言 B 红
  - **M2**（codex 二次审计补入）：在 handleDisconnected 里加 `useChatStore.setState({ error: '连接中断，请刷新' })` 或推入一条可见错误消息（role: 'system'，content: '连接失败'）→ 断言 C1/C3 红
  - **M3**（codex 三次审计补入）：在 handleDisconnected 里加 `useToastStore.getState().addToast('error', '连接中断')` → 断言 C4a + C6 红
  - **M4**（codex 四次审计补入）：在 handleDisconnected 里加 `useToastStore.getState().addToast('warning', '连接中断，正在重连')` 或 `addToast('info', '...')` → 断言 C4a 绿、C4b 红、**C6 红**（证明结构不变量比语义黑名单硬）
  - **M5**（codex 五次审计补入）：在 handleDisconnected 里加 `useToastStore.getState().addToast('info', '网络波动，正在恢复通信')`（文案完全不命中 C1/C4b 关键字）→ 断言 C1/C4a/C4b 全绿，**C6 红**（结构断言兜住自然文案绕过）
  - **M6**（codex 五次审计补入）：在 handleDisconnected 里加 `useChatStore.getState().addChatMessage({ role: 'assistant', content: '网络波动，正在恢复通信', is_error: false, ... }, currentSessionId)` → 断言 C1/C2/C3 全绿，**C7a 或 C7b 红**（消息计数增量 > 1，或新增消息内容不对应流式 assistant placeholder）
  - **M7**（codex 六次审计补入）：在 handleDisconnected 里加 `useChatStore.getState().addInlineApproval({ id: 'x', prompt: '网络波动，正在恢复通信', ... })` → 断言 C6/C7/C1/C2/C3 全绿，**C8 红**（inlineApprovals 增量超 0）
  - **M8**（codex 六次审计补入）：在 handleDisconnected 里加 `useTaskProgressStore.getState().setTaskGroup({ group_id: 'x', tasks: [{ id: 't1', status: 'failed', error: '连接已中断' }] })` → 断言 C6/C7/C8 全绿，**C9 红**（activeGroups 新增或出现 failed 任务）
  - **M9**（codex 七次审计补入）：在 handleDisconnected 里加 `useAgentActivityStore.getState().onAgentStatus(currentSessionId, 'error')` → 断言 C6/C7/C8/C9 全绿，**C10 红**（sessionStatus 进入 'error' 态，Sidebar SessionStatusDot 会显示红点）
  - **M10**（codex 八次审计补入）：在 harness 预置好 `tool_calls` 消息 + `toolCallStatuses['tc-1'] = { status: 'running' }` 的前提下，在 handleDisconnected 里加 `useChatStore.getState().setToolCallStatus('tc-1', { status: 'error', error: '连接已中断' })` → 断言 C6/C7/C8/C9/C10 全绿，**C11 红**（toolCallStatuses 错误态计数增 1，ToolAdapter 把该 id 渲染为 output-error，ToolHeader 显示红色徽标）
  - **M11**（codex 九次审计补入）：在 harness 预置一条 `is_error:false` 的既有 assistant 消息（在 C6-C14 基线快照**之前**注入），在 handleDisconnected 里加 `useChatStore.getState().setMessages(useChatStore.getState().messages.map(m => m.timestamp === presetTs ? { ...m, is_error: true, content: '连接中断，请刷新' } : m))` → 断言 C6/C7a/C7b/C8/C9/C10/C11/C13/C14 全绿（length 不增、不新增错 toast、不改 approvals/tasks/agent status/tool status/tool-error-content），**C12 红**（`messages.filter(m => m.is_error === true).length` 从 0 变 1）
  - **M12**（codex 十次审计补入）：在 harness 预置一条 `{ role:'tool', tool_call_id:'tc-1', content:'{"ok":true}', is_error:false }` 消息（在 C6-C14 基线快照**之前**注入），在 handleDisconnected 里加 `useChatStore.getState().setMessages(messages.map(m => m.role === 'tool' && m.tool_call_id === 'tc-1' ? { ...m, content: 'tool error: 连接已中断' } : m))` → 断言 C6/C7/C8/C9/C10/C11/C12 全绿（is_error 没翻、length 不增、其他通道未动），**C13 红**（`role='tool'` 且 content 以 `tool error:` 开头的计数从 0 变 1，`MessageBubble` 渲染 `resultStatus='error'` 红色工具结果卡）
  - **M13**（codex 十次审计补入）：在 harness 预置一个 group `{ group_id:'g1', tasks:[{ id:'t1', status:'running', error: undefined }] }`（在 C6-C15 基线快照**之前**注入），在 handleDisconnected 里加 `useTaskProgressStore.getState().updateTask('g1', 't1', { error: '连接已中断' })`（或等价的 setTaskGroup 保留 status:'running' 只加 error 字段） → 断言 C6/C7/C8/C9/C10/C11/C12/C13/C15 全绿（group 数/failed 数不变），**C14 红**（任务 error 非空计数从 0 变 1，`TaskProgressPanel.tsx:80` 渲染红字文案）
  - **M14**（codex 十一次审计补入，既有消息 content 改写）：在 harness 预置 preset messages（timestamp: 'preset-plain-assistant-2', is_error:false, content:'历史回复 B'），在 handleDisconnected 里加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-plain-assistant-2' ? { ...m, content: '请稍候，系统正在重新同步' } : m))` → 断言 C6/C7/C8/C9/C10/C11/C12/C13/C14 全绿（length 不增、is_error 没翻、tool 前缀无、task.error 无），**C15b 红**（preset 消息 content 从 '历史回复 B' 变成 '请稍候...'）
  - **M15**（codex 十一次审计补入，content 类型破坏）：在 harness 预置 preset messages 前提下，在 handleDisconnected 里加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-plain-assistant-2' ? { ...m, content: ({} as unknown as string) } : m))` → 断言 C6/C7/C8/C9/C10/C11/C12/C13/C14 全绿（C12 只查 is_error 不查 content 类型），**C15b 或 C15c 红**（preset content 不再是字符串，`MessageBubbleBoundary.tsx:60` 渲染 "此消息渲染失败"）
  - **M16**（codex 十二/十三次审计收敛，length-neutral 同三元组副本）：在 harness 预置 preset messages 前提下，在 handleDisconnected 里加 `useChatStore.getState().setMessages(useChatStore.getState().messages.map((m, i, arr) => i === arr.length - 1 ? { timestamp:'preset-plain-assistant-2', role:'assistant', tool_call_id: undefined, content:'副本内容：网络波动，请稍候', is_error:false, tool_calls: undefined } : m))`（**把最后一条 preset-tool-1 替换为 preset-plain-assistant-2 的三元组副本；length 保持不变**） → 断言 C6/C8/C9/C10/C11/C12/C13/C14 全绿（toast/approval/task/agent/tool 所有 store 未动），**C7a 绿**（`messages.length - messagesBefore.length === 0`），**C15a 红**（对 `preset-plain-assistant-2` 三元组 `matches.length === 2`；对 `preset-tool-1` 三元组 `matches.length === 0`——两条路径都违反 C15a）。**因用 expect.soft 断言，C7a 绿 + C15a 红的证据独立可见**。**Attribution**：证明 C15a "存在且唯一" 是 store 层的独立结构契约，与 C7a "length 增量上限" 不重合
  - **M17**（codex 十二/十三次审计收敛，tool_calls.name 改写不触 boundary）：在 harness 预置 preset messages 前提下，在 handleDisconnected 里加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-assistant-1' ? { ...m, tool_calls: [{ id:'tc-1', name:'forbidden_tool', arguments:'{"q":"x"}' }] } : m))`（**仅改 tool_calls[0].name，arguments 保持合法字符串不触发 `ToolExecutionBlock.tsx:45` raw.trim 异常**） → 断言 C1/C2/C4a/C4b 全绿（无 "失败" 文案，`MessageBubbleBoundary` 不兜底）、C6/C7/C8/C9/C10/C11/C12/C13/C14/C15a/C15b/C15c/C16 全绿（content 字面和类型未动，is_error 没翻，tool-error-content 无，toolCallStatuses 未改，三元组唯一，无红色 DOM），**C15d 红**（`JSON.stringify([{id:'tc-1',name:'forbidden_tool',arguments:'{"q":"x"}'}]) !== JSON.stringify([{id:'tc-1',name:'search',arguments:'{"q":"x"}'}])`）。**Attribution**：证明 C15d 深度冻结是对 `tool_calls` 字段的独立契约；即便 `MessageBubble` 渲染一个"看起来正常"的工具卡（name="forbidden_tool" 不抛异常），store 层身份漂移仍是违规
  - **M18**（codex 十三次审计补入，DOM 静态红 class div）：修改 `frontend/src/components/chat/MessageList.tsx:298` 附近**静态插入** `<div className="text-red-500">通道暂不可用，请稍候</div>`（保持无 `role`、无文案黑名单关键字，**不写任何 store**）——C1/C2/C4a/C4b 全绿、C6/C7/C8/C9/C10/C11/C12/C13/C14/C15a/b/c/d 全绿，**C16 Stage 2 final 扫描红**（`document.querySelectorAll('[class*="text-red-"]').length === 1`）。**Attribution**：证明 C16 Stage 2 对常驻静态红 class 攻击有效
  - **M19**（codex 十四次审计补入，inline-style danger 绕过 class selector）：修改 `frontend/src/components/chat/MessageList.tsx:299` 附近插入 `<div style={{ color: 'var(--danger)' }}>通道暂不可用，请稍候</div>`（**无任何 class**，逃掉所有 `[class*="..."]` 选择器） → C1/C2/C4/C6-C15 全绿，C16 Stage 2 的 `errorElements` 数组为空（class selector 不命中），**C16 Stage 2 的 `inlineDangerElements` 红**（inline-style 正则命中 `--danger`，非空）。**Attribution**：证明 Stage 2 的 inline-style 扫描独立于 class selector，堵住"用 inline style 规避 Tailwind class 扫描"的 attack
  - **M20**（codex 十四次审计补入，transient 短暂元素 mid-window 可见）：修改 `frontend/src/hooks/useWebSocket.ts:274`（handleDisconnected 内）加：
    ```ts
    const blip = document.createElement('div');
    blip.id = 'ws-reconnect-blip';
    blip.style.color = 'var(--danger)';
    blip.textContent = '通道波动，请稍候';
    document.body.appendChild(blip);
    ```
    **同时**修改 `useWebSocket.ts:128` 的 partial 分支加 `document.getElementById('ws-reconnect-blip')?.remove();`——元素在 disconnect → connect 期间可见，在首个 partial 到达时被移除，**Stage 2 final 扫描事后看不到** → C16 Stage 2 全绿。**C16 Stage 1 的 MutationObserver 红**（捕获 `added` 阶段的 `blip` 节点，inline-style `--danger` 命中，`transientHits` 非空）。**Attribution**：证明 Stage 1 MutationObserver 独立于 Stage 2 absolute 扫描，堵住"在 disconnect 之间短暂显示 reconnecting banner"这类真实常见的回归（最符合 spec Req 2 文义：整个窗口期不得出现错误态）
  - **M21**（codex 十四次审计补入，attribute 攻击 aria-invalid）：修改 `frontend/src/components/chat/MessageList.tsx:208` 附近，在 `messages.map` 渲染每条消息的 root 元素上条件附加 `aria-invalid={!connected ? 'true' : undefined}`（`connected` 来自 `useWebSocket`），或直接在 handleDisconnected 时 `document.querySelector('[data-testid="chat-messages"]')?.setAttribute('aria-invalid', 'true')` —— C1/C2/C4/C6-C15 全绿，**C16 Stage 1 MutationObserver attr 阶段红**（`attr:aria-invalid` 记录，matches selector）。**Attribution**：证明 Stage 1 attr 观察独立覆盖"不加新 DOM、只改既有节点属性"的 attack（data-state='error' 同理走一条 mutation）
  - **M22**（R17 plan-ceo W2 补入，Tailwind `!important` prefix）：修改 `frontend/src/components/chat/MessageList.tsx:298` 附近**静态插入** `<div className="!text-red-500">通道暂不可用，请稍候</div>`——修复前 `dangerTokenRe` 的 `^` 锚点拒绝 `!` 开头、broad selector 无 `[class*="!text-"]`，silent green 零红色。修复后新 broad `[class*="!text-"]` 扫入 + `tokenHasDangerUtility` 里 `rawTok.replace(/^!/, '')` 剥前缀 + `dangerTokenRe` 命中 → `reconnect.test.tsx:257` baseline hard expect fail-fast 第一红。**Attribution**：证明 W2 `!important` 前缀攻击面已被 Tailwind 语义对齐的剥 prefix + 固定字面清单二步法独立覆盖
  - **M23**（R17 plan-ceo W2 补入，Tailwind arbitrary value CSS var）：修改 `MessageList.tsx:298` 附近静态插入 `<div className="text-[var(--danger)]">通道暂不可用，请稍候</div>`——修复前无任何字面/前缀命中。修复后 `[class*="-[var(--danger"]` broad 扫入 + `dangerArbitraryTokenRe` 的 `text|bg|border|ring|outline-\[var\(--(?:danger|destructive)\)` 分支命中 → baseline 第 257 行 fail-fast。**Attribution**：证明 arbitrary value CSS var 攻击面（设计系统内最常见危险色写法）独立可达
  - **M24**（R17 plan-ceo W2 补入，Tailwind arbitrary hex）：修改 `MessageList.tsx:298` 附近静态插入 `<div className="bg-[#ef4444]">通道暂不可用，请稍候</div>`——`#ef4444` 即 Tailwind `red-500` 默认十六进制。修复后 `[class*="-[#"]` broad 扫入 + `matchesArbitraryDanger` 的 hex 6 位 R 通道主导判定命中 → baseline 第 257 行 fail-fast。**Attribution（R18 校正）**：R17 原文本主张 M22/M23/M24 "三层缺一不可"，R18 codex 审计指出 M22 / M23 被现存 broad selector（`text-red-` 子串 / `--danger` 子串）冗余兜住；真正独立的是 M24 的 `[class*="-[#"]` + 程序化 hex R 通道判定。R18 落地后 M22 / M23 broad 冗余前缀已删，改以独立性 probe（删单一修复组件 → silent-green → 恢复 → 红）在 `mutation-evidence.md` 逐条证明
  - **M25**（R18 codex W2 补入，SVG 描边 stroke-red-500）：修改 `MessageList.tsx:298` 附近静态插入 `<svg width="16" height="16"><circle cx="8" cy="8" r="4" className="stroke-red-500" fill="none"/></svg>`——修复前 `dangerTokenRe` 与 `errorClassBroadSelector` 均无 `stroke|fill|caret|decoration|accent|placeholder|from|via|to|divide` 十族任何一族，silent-green。修复后 broad `[class*="stroke-red-"]` 扫入 + `dangerTokenRe` 的 `stroke-red-\d+` 分支命中 → baseline 第 257 行 fail-fast。**独立性对照**：删掉 regex 中 stroke 分支保留 broad selector → Stage 2 querySelectorAll 命中但 tokenHasDangerUtility 回 false → silent-green，证明 regex 与 broad selector 必须同步扩展，任一侧缺失即漏抓。**Attribution**：证明十族扩展（stroke/fill/caret/decoration/accent/placeholder/from/via/to/divide）独立覆盖 SVG 图标描边这一常见危险色入口
  - **M26**（R18 codex W2 补入，纯红 hex `#ff0000`）：修改 `MessageList.tsx:298` 附近静态插入 `<div className="text-[#ff0000]">通道暂不可用，请稍候</div>`——R17 `dangerArbitraryTokenRe` 仅硬编码 `ef4444|dc2626|b91c1c|f87171|fca5a5|fecaca` 六个 Tailwind 预设色；`#ff0000` 不在清单内，silent-green。修复后 broad `[class*="-[#"]` 扫入 + `matchesArbitraryDanger` 程序化 hex 6 位 R≥180 且 G/B≤100 判定命中（R=255 G=0 B=0） → baseline fail-fast。**独立性对照**：把 `matchesArbitraryDanger` 里的 hex 分支阈值收窄到仅命中 `ef4444` 字面 → M26 silent-green；恢复程序化阈值 → 红。证明 R 通道主导判定相对字面清单的独立覆盖。**Attribution**：证明 hex 全色域 + rgb + hsl + var 程序化判定独立于字面清单
  - **M27**（R18 codex B1 补入，conditional aria-busy 激活路径）：harness 先 `document.body.setAttribute('aria-busy','true')`，再在 `MessageList.tsx:298` 附近静态插入 `<div className="aria-busy:text-red-500">通道暂不可用，请稍候</div>`——修复前 `tokenHasDangerUtility` 的 `if (rawTok.includes(':')) continue` 把 `aria-busy:text-red-500` 整个 token 放过，silent-green。修复后 `splitAtTopColons('aria-busy:text-red-500')` → `['aria-busy','text-red-500']`，`suffix='text-red-500'` 命中 `dangerTokenRe`；`prefixActiveOnElement(el,'aria-busy')` 走 `aria-<name>` 分支读 `el.getAttribute('aria-busy') === 'true'` → true（harness 预置），`allActive=true` → 判红 → baseline fail-fast。**独立性对照（负控制）**：harness 不设置 `aria-busy='true'` 则同 mutation silent-green（`prefixActiveOnElement` 回 false，条件未激活，跳过），证明激活判定本身即独立契约，不是无差别把所有条件 token 都判红。**Attribution**：证明 `splitAtTopColons` + `prefixActiveOnElement` aria 简写分支独立覆盖条件修饰攻击面，且不伤 baseline（shadcn Badge `aria-invalid:ring-destructive/20` baseline 无 `aria-invalid='true'` → 静默，regression 117/117 绿）
  - **M28**（R18 codex B1 补入，conditional data-[state=offline] 精确匹配路径）：harness 先 `document.body.setAttribute('data-state','offline')`，再在 `MessageList.tsx:298` 附近静态插入 `<div className="data-[state=offline]:text-red-500">通道暂不可用，请稍候</div>`——修复前同 M27 被 `includes(':')` 放过，silent-green。修复后 `splitAtTopColons` 需识别 `data-[state=offline]:` 内层 `=offline]` 里的方括号——括号深度判 `:`，只在外层深度=0 才分；切成 `['data-[state=offline]','text-red-500']`，`prefixActiveOnElement` 走 `data-\[<name>=<value>\]` 分支读 `el.getAttribute('data-state') === 'offline'` → true → 判红。**独立性对照**：M28 同时依赖 `splitAtTopColons`（尊重 brackets）+ `prefixActiveOnElement` 的 `data-[<name>=<value>]` 分支——任一组件缺失即 silent-green。和 M27（aria 简写路径）走的是不同子分支，提供独立覆盖。**Attribution**：证明 data-attribute 精确匹配分支独立于 aria 简写分支，且 `splitAtTopColons` 对嵌套方括号的正确处理是该分支能走到底的先决条件

## 7. CI gate（新增）

**Codex 审核发现**：原 proposal 声称"CI 跑通"但 `.github/workflows/test-specdriven.yml` 只跑 Go、`e2e-session-scope.yml` 只跑 Playwright，**无任何 workflow 跑 frontend vitest**。没有 CI 的 vitest 等于没跑过。

- [x] 7.1 新增 `.github/workflows/frontend-vitest.yml`（或把 job 加到 `test-specdriven.yml`，由维护者决定）
- [x] 7.2 触发条件：`pull_request` 或 `push` 匹配 `frontend/**` + `openspec/**` 路径
- [x] 7.3 步骤最小集：
  - `actions/checkout@v4`
  - 安装 bun（`oven-sh/setup-bun@v1` 或 curl 安装）
  - `cd frontend && bun install --frozen-lockfile`
  - `cd frontend && bun run test` （执行**五条** R1-R5 vitest：AppShell / handleDisconnected / urlBuilding / partialMessageRendering / reconnect）
- [x] 7.4 PR CI 必须 required check；**五条** vitest 任一红则 merge block（口径与 section 3 proposal、section 8 归档门槛一致）
- [ ] 7.5 **验证 CI gate 本身有效**：在 PR 里故意让一个测试 fail，CI 必须红；修复后 CI 必须绿。证据贴到 PR 描述

## 8. 归档门槛

- [x] 8.1 五条 vitest（R1-R5）在 `bun run test` 本地跑绿
- [ ] 8.2 CI workflow 在本次 PR 上首次跑绿（证明 job 配置正确）
- [x] 8.3 mutation 证据：每条 R1-R5 对应的 mutation diff + vitest 红色输出贴到 PR 描述。共 37 条 mutation（R1×4 + R2×3 + R3×1 + R4×1 + R5×28：M2 chat.error / M3 error toast / M4 warning+info toast / M5 自然文案 toast (C6) / M6 assistant 消息气泡 (C7) / M7 inlineApproval (C8) / M8 taskProgress failed (C9) / M9 agentActivity sessionStatus='error' (C10) / M10 toolCallStatus error (C11) / M11 setMessages 原位翻 is_error (C12) / M12 setMessages 改 tool content 加 error 前缀 (C13) / M13 updateTask 加 error 字段不改 status (C14) / M14 setMessages 改既有消息 content 文案 (C15b) / M15 setMessages 改既有消息 content 类型破坏 (C15c) / M16 length-neutral 替换末条为同三元组副本 (C15a) / M17 setMessages 改 tool_calls[0].name='forbidden_tool' (C15d) / M18 `MessageList.tsx` 插入静态 `<div className="text-red-500">` (C16 Stage 2 class selector) / M19 `MessageList.tsx` 插入 inline-style `<div style={{ color: 'var(--danger)' }}>` (C16 Stage 2 inline-style 扫描) / M20 `useWebSocket.ts` handleDisconnected 创建 DOM 元素 + partial 分支移除 (C16 Stage 1 MutationObserver transient 捕获) / M21 `MessageList.tsx` 给消息节点加 `aria-invalid='true'` 或 `data-state='error'` (C16 Stage 1 attr 观察) / M22 `MessageList.tsx` 插入 `<div className="!text-red-500">` (R17 W2 Tailwind `!important` prefix，R18 校正：被现存 `text-red-` broad 冗余兜住，独立性靠 `replace(/^!/)` 剥 prefix + regex 分支) / M23 `MessageList.tsx` 插入 `<div className="text-[var(--danger)]">` (R17 W2 arbitrary value CSS var，R18 校正：被 `--danger` 子串冗余兜住，独立性靠 `matchesArbitraryDanger` var 直通分支) / M24 `MessageList.tsx` 插入 `<div className="bg-[#ef4444]">` (R17 W2 arbitrary value 十六进制，R18 真独立：`[class*="-[#"]` broad + `matchesArbitraryDanger` hex R 通道判定) / M25 `MessageList.tsx` 插入 `<svg><circle className="stroke-red-500"/>` (R18 B2 十族扩展 stroke/fill/caret/decoration/accent/placeholder/from/via/to/divide × 4 色家族) / M26 `MessageList.tsx` 插入 `<div className="text-[#ff0000]">` (R18 B3 程序化 hex R 通道主导判定，覆盖 Tailwind 预设清单之外的纯红色) / M27 harness `aria-busy='true'` + `<div className="aria-busy:text-red-500">` (R18 B1 `splitAtTopColons` + `prefixActiveOnElement` aria 简写激活判定) / M28 harness `data-state='offline'` + `<div className="data-[state=offline]:text-red-500">` (R18 B1 `splitAtTopColons` 括号深度感知切分 + `prefixActiveOnElement` data-[<name>=<value>] 精确匹配激活判定)）。**每条 M 的红色 vitest 输出必须逐条贴到 PR 描述，一条不贴视为未完成本检查项**
- [ ] 8.4 PR 描述中显式列出：本 change **不含** Playwright e2e、**不含** 生产代码改动；下一步是开 `frontend-ws-e2e-harness` change 搭 mock WS server 承载 network + DOM 级 e2e
- [ ] 8.5 合并后 `openspec archive frontend-ws-handshake-regression` 加日期前缀
