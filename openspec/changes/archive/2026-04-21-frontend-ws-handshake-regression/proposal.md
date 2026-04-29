## Why

`im-streaming-reply` Sprint 12.9 在前端 WS 链路上落了两条**关键 invariant**，靠手工 runbook (`docs/runbooks/im-streaming-reply-live-smoke.md` 11.8) 保护，**无任何自动化回归屏障**。一旦被日常 refactor/"顺手清理"回退，用户看到的就是：发完消息只见自己输入，LLM 回复静默消失——code review 阶段几乎无法发现。

三轮 CEO 评审 + 读码核查 + 两轮 codex 独立审计后，真正的盲区收敛到 5 条（R5 由 codex 审计过程中识别补入），覆盖程度如下：

| # | 盲区 | 今日自动化防护 |
|---|---|---|
| R1 | `useWebSocketConnection.ts` 唯一 URL 拼接位置把 `session_id` 键名改错 / 编码漏字符 / 拼接符错 | **零** |
| R2 | `useWebSocket.handleMessage` 的 partial 分支链路（`ensureAssistantMessage` → RAF `updateLastAssistant` → MessageList selector → Streamdown 渲染）任一环被改成只写 store 不触发 DOM | **零** |
| R3 | `useWebSocket.handleDisconnected` 被后人"优化"重新清空 currentSessionId（现有长注释解释了为什么不能清，但注释会被忽视）——**必要但不充分**条件，是 R5 的 fast-fail unit | **零** |
| R4 | `AppShell.tsx` URL-params → store-fallback 优先级被反过来，或两源同时为空仍握手 | **零** |
| R5 | disconnect → reconnect → 后续消息首个 partial 到达 DOM 的端到端链路（spec Requirement 2 的可见性部分，codex 审计补入） | **零** |

`session-scope-regression-matrix` 之前预计提供 e2e harness 承载这些断言，但实测仓库 Playwright 配置只覆盖 Streamdown/shiki 叶子原语（`frontend/e2e/fixture/main.tsx` 是 standalone Streamdown mock，无 WS、无后端、无 AppShell 装载），**真实 e2e harness 尚未存在**。等一个不存在的依赖把 R1-R5 继续裸奔是错误决策。

## What Changes

### 1. 黑盒行为契约（spec.md，完全重写）

3 条 invariant，完全脱敏到 user-observable 层：

- **WS handshake URL 必带 session 标识符**（key 名由后端 HTTP handler 的 `r.URL.Query().Get(...)` 作为 single source of truth；同时加 URL-encode 子条款）
- **disconnect→reconnect 后下一条消息必须可见**（DOM 渲染，不是 store 写入；删除"browser idle timeout"等不可测时窗）
- **partial chunk 必须渲染到 DOM**（store 写了 DOM 没动视同 regression；用 `findByText` 默认异步超时替代"within one animation frame"）

删除项：
- 原 spec 里所有行号引用（`useWebSocket.ts:242-260` 等）——代码漂移即假红
- 所有 hook/store 名（`useParams`、`useChatStore.currentSessionId`、`handleDisconnected`）——违反黑盒自洽
- 后端日志 literal `WebSocket session-mismatch drop`——日志措辞变化即假红
- 原 Requirement #4 "Playwright case landing condition"——这是排期策略不是系统契约，放 spec 必然腐烂
- 原 "within one animation frame" / "before the browser idle timeout"——vitest JSDOM 无 RAF shim、Playwright 也没有 idle timeout 测试原语，codex 审计确认不可操作

### 2. 五条 vitest 断言覆盖 R1-R5（tasks.md）

落 vitest + `@testing-library/react` 级别保护，不等 e2e harness：

| 盲区 | 测试文件 | 手法 |
|---|---|---|
| R4 | `AppShell.test.tsx` | 渲染 AppShell，mock `useParams` + chat store 双源组合（A=URL 有/store 有、B=URL 空/store 有、C=URL 有/store 空、D=两空），断言 `useWebSocket` 收到的 `sessionId` prop |
| R3 | `useWebSocket.handleDisconnected.test.ts` | mock `useWebSocketConnection` 用 module-level ref 捕获 `onDisconnected`；`act()` 触发；断言 `useChatStore.getState().currentSessionId` 不变 |
| R1 | `useWebSocketConnection.urlBuilding.test.ts` | 全局 stub `WebSocket` 构造函数捕获 url 参数；renderHook 满足 `enabled && url && sessionId` 前提；**expectedKey 运行时 `fs.readFileSync` 后端 `internal/streaming/websocket.go` 用正则反射提取**（前端代码+前端测试同改无法自洽绕过）；校验 `?` vs `&` 拼接 + URL-encode |
| R2 | `partialMessageRendering.test.tsx` | mock `useWebSocketConnection` 捕获 `onMessage`；推真实 `WSMessage` partial payload 走完 `handleMessage` → `ensureAssistantMessage` → RAF → `updateLastAssistant` → MessageList → Streamdown 链路；`findByText` 断言 DOM 可见；**第二段 partial 后额外断言 assistant 消息计数未增 + 同节点更新 + 旧占位未残留**（钉死 spec Req 3 "update same visible message region / no duplicated or orphaned"） |
| R5 | `useWebSocket.reconnect.test.tsx` | 同上 mock 模式，额外捕获 `onConnected`；顺序 `disconnect → reconnect → push partial → findByText`，端到端覆盖 spec Requirement 2；**无可见错误态用结构性不变量做契约主力**（codex 五/六/七/八/九/十/十一/十二/十三/十四次审计收敛）：**C6 `useToastStore.toasts` 计数增量 = 0** + **C7 `useChatStore.messages` 增量 ∈ {0,1} 且新增必须是非 error 的流式 assistant placeholder** + **C8 `useChatStore.inlineApprovals` 增量 = 0** + **C9 `useTaskProgressStore.activeGroups` 不新增 group 且不引入新 failed 任务** + **C10 `useAgentActivityStore.sessionStatus[sessionId] !== 'error'`** + **C11 `useChatStore.toolCallStatuses` 错误态计数增量 = 0** + **C12 `messages.filter(m => m.is_error === true).length` 增量 = 0** + **C13 `messages.filter(role='tool' 且 content 错误前缀).length` 增量 = 0** + **C14 `activeGroups.flatMap(tasks).filter(!!t.error).length` 增量 = 0** + **C15 既有 preset 消息完整冻结**（按 harness 赋予的 timestamp+role+tool_call_id 三元组对位：**C15a 三元组唯一** `matches.length === 1` 堵同三元组副本 + **C15b content 字面冻结** + **C15c content 类型 string** 堵 `{} as any` 触发 `MessageBubbleBoundary` "此消息渲染失败" + **C15d tool_calls 深度冻结** `JSON.stringify === JSON.stringify` 堵 `tool_calls.arguments` / `name` 等字段改写造成工具卡漂移或渲染异常） + **C16 两阶段 DOM 错误状态兜底**（**Stage 1 MutationObserver** 在 baseline snapshot 之后启动，observe `childList/subtree/attributes`，整个 disconnect→connect→partial→findByText 窗口内捕获所有新增节点 + attr 变更，过滤 error signal selector（`[role=alert|status|banner]`、`[aria-live]`、`[aria-invalid]`、`[data-state=error]`、`text-red-/rose-/pink-/orange-/amber-/yellow-`、`bg-*/border-*/ring-*` 红系、`[class*=--danger]`、`[class*=--destructive]`、`dialog[open]`）+ inline-style 正则 `--danger|--destructive|color:red|rgb()` 命中 → `transientHits` 非空即违规（堵 transient attack：中途出现、最终被移除的错误元素）；**Stage 2 Final 扫描** 在 findByText 后跑一次 `querySelectorAll` + `[style]` inline-style 正则扫描 → absolute 计数 = 0（堵静态常驻元素）；MutationObserver 启动前有硬 `expect` baseline sanity（baseline 不干净 fail fast）；harness preset task `status='running'` + `error=undefined` 确保 `TaskProgressPanel` 所有红色分支不触发）。辅助保留 C1/C2/C3/C4a/C4b 文案/type 断言作诊断。**所有 C6-C16 断言使用 `expect.soft` 软断言**，便于 M14/M15/M16/M17/M18 同时 red 多条 C 时 attribution 清晰（vitest soft 失败不中断后续断言）。明确排除 Header / Sidebar / AdminSidebar 连接状态指示器（合法状态显示，不写入任何 C6-C16 覆盖的 store、也不在 MessageList DOM 子树内），Sidebar `sending || streaming` 禁用态是 chrome 级 UX 不是 error state。映射 spec Req 2 "no visible error state SHALL be shown"。**Scope 边界**：只覆盖 disconnect→reconnect 循环；initial connection failure 和 cleanup close 不在 Req 2 范围 |

### 3. CI gate（codex 审计后补入）

**codex 审计发现**：原 scaffold 声称"CI 跑通"，但仓库 `.github/workflows/` 下仅 `test-specdriven.yml`（Go gates）和 `e2e-session-scope.yml`（Playwright），`grep -l -E "vitest|bun run test|npm.*test|frontend.*test"` 无匹配——**无任何 workflow 跑 frontend vitest**。没有 CI 的 vitest 等于没跑过。

本 change 明确要求新增 `.github/workflows/frontend-vitest.yml`（或把 job 并入现有 workflow），PR required check 上 gate 五条 vitest。CI gate 本身有效性须在本次 PR 里反向证明：故意让一条测试红 → CI 红 → 修复 → CI 绿，证据贴 PR 描述。

### 3.1 测试基座前置（codex 二次审计补入）

`MessageList.tsx:103` 使用 `new IntersectionObserver(...)`，当前 `frontend/src/test-setup.ts` 未 polyfill，JSDOM 也不原生提供。R2/R5 若不前置 stub，effect 执行时即抛异常，测试红于基座而非红于契约。改动 `test-setup.ts` 补最小 `IntersectionObserver` + `ResizeObserver` stub（见 tasks 1.7）。

### 4. 不做的事（明确划界）

- **不**改 `setCurrentSessionId` 的类型签名。仓库 grep 证据：今日 0 个 caller 传 null（唯一引用在 `useWebSocket.ts:283-297` 的注释里）——为幽灵加类型护栏是防御过拟合
- **不**加 zustand middleware "WS 活跃期间写空 throw"。上一版 CEO 评审 round 3 指出 middleware 无法区分 disconnect-clear vs route-leave vs logout，必然误伤；且读码确认今日无合法 clear 路径（logout 走 `window.location.href` 全页重载，Chat.tsx 卸载仅调 `clearMessages`）
- **不**加 CI grep 规则检测 `setCurrentSessionId(null)`。可被 `destructure+alias / setState 直写 / clearSession helper / undefined` 至少四种绕过，是剧场而非契约
- **不**现在落 Playwright e2e。Playwright config 存在但无 WS harness；搭 mock WS server + AppShell fixture 是独立工程，开新 change `frontend-ws-e2e-harness` 承载

## Capabilities

### New Capabilities

- `frontend-ws-handshake-regression`: 前端 WS 握手 / session 身份连续性 / 流式可见性的黑盒行为契约，及 vitest + CI 级别的回归防护

### Modified Capabilities
（无——本 change 不改既有 capability）

## Dependency Graph

```
本 change（spec + 5 条 vitest + CI workflow）
  ↓ 独立闭环，无外部依赖
（后续）frontend-ws-e2e-harness
  ↓ 需先搭 mock WS server + AppShell fixture
  → 承载 network-layer + DOM 级 Playwright 回归
```

与在途 change 的协调：
- 与 `chat-ui-polish`（amber→blue）：本 change 的 vitest 用 store action / onMessage 回调触发 + DOM 文本断言，不绑 className，不受颜色/样式影响
- 与 `chat-ui-migrate-ai-elements`（已归档）：spec 现在不引用 `useHiveAgentEvents`、也不引用 `useWebSocket`，两条实现路径都满足同一组 invariant

## Impact

- **代码**：
  - `openspec/specs/frontend-ws-handshake-regression/spec.md`：3 条黑盒 invariant（archive 后落）
  - `frontend/src/layouts/__tests__/AppShell.test.tsx`：新增
  - `frontend/src/hooks/__tests__/useWebSocket.handleDisconnected.test.ts`：新增
  - `frontend/src/hooks/__tests__/useWebSocketConnection.urlBuilding.test.ts`：新增
  - `frontend/src/components/chat/__tests__/partialMessageRendering.test.tsx`：新增
  - `frontend/src/hooks/__tests__/useWebSocket.reconnect.test.tsx`：新增
  - `.github/workflows/frontend-vitest.yml`：新增（或合并到已有 workflow 作为新 job）
  - 无前端生产代码改动
- **测试**：5 条 vitest 全部本地 + CI 跑通，且对 `AppShell.tsx:32` / `useWebSocket.ts:274` / `useWebSocketConnection.ts:52` / `useWebSocket.ts:95-139 partial 分支` 的故意 mutation 必须转红
- **兼容性**：纯新增，无破坏
- **依赖 change**：无外部阻塞（`session-scope-regression-matrix` 不再是前置）
- **回滚**：删掉五个测试文件 + workflow 即可；spec 可直接撤销

## Verification

- `openspec validate frontend-ws-handshake-regression --strict` 通过
- 五条 vitest 本地 `cd frontend && bun run test` 绿
- CI workflow 本次 PR 首跑绿（CI gate 本身有效性反证已贴 PR）
- **红军验证**（mutation testing 手动版，codex 十四轮审计后扩展到 30 条）：
  1. `AppShell.tsx:32` 改成 `storeSessionId || urlSessionId` → R4 单测红
  2. `useWebSocket.ts:274` handleDisconnected 里加 `useChatStore.setState({ currentSessionId: null })` → R3 单测红
  3. `useWebSocketConnection.ts:52` 把 `session_id` 改成 `sessionId`（**同时**修改 R1 测试里前端字面量） → R1 单测**仍然转红**（因 expectedKey 从后端 `internal/streaming/websocket.go` 反射而来）
  4. `useWebSocketConnection.ts:52` 把 `session_id` 改成 `sessionId`（不改后端） → R1 单测红
  5. `useWebSocketConnection.ts:52` 去掉 `url.includes('?') ? '&' : '?'` 分支永远用 `?` → R1 单测红
  6. `useWebSocketConnection.ts:52` value 直接插入不走 `encodeURIComponent` → R1 单测红
  7. `useWebSocket.ts:95-139` partial 分支改成"只有 streamingMessageId 已存在才 updateLastAssistant，否则什么都不做" → R2 单测红（codex 点名的真实攻击路径）
  8. 删除 `useWebSocket.ts:128` 的 `ensureAssistantMessage()` 调用 → R2 单测红
  9. 让 partial 分支每次都新建 placeholder（`chat.ts:178-193` 或 useWebSocket partial 分支改成每次都 create） → R2 单测红（同节点/无重复占位断言捕获）
  10. `useWebSocket.ts` handleDisconnected 加 `useChatStore.getState().clearMessages()` → R5 断言 B 红
  11. handleDisconnected 加 `useChatStore.setState({ error: '...' })` 或推入 role=system 错误消息 → R5 断言 C1/C3 红
  12. handleDisconnected 加 `useToastStore.getState().addToast('error', '...')`（codex 三轮审计补入，攻击路径 1-2） → R5 断言 C4a + C6 红
  13. handleDisconnected 加 `useToastStore.getState().addToast('warning', '连接中断...')` 或 `'info'`（codex 四轮审计：type-only 断言绕过） → R5 断言 C4b + C6 红
  14. handleDisconnected 加 `useToastStore.getState().addToast('info', '网络波动，正在恢复通信')`（codex 五轮审计：文案完全不命中 C1/C4b 关键字） → R5 断言 C1/C4a/C4b 全绿，**C6 红**（结构不变量兜住自然文案绕过）
  15. handleDisconnected 加 `addChatMessage({ role: 'assistant', content: '网络波动，正在恢复通信', is_error: false })`（codex 五轮审计：消息气泡形式绕过） → R5 断言 C1/C2/C3/C4 全绿，**C7 红**（新增 chat 消息计数增量超过流式 placeholder 允许的 1）
  16. handleDisconnected 加 `addInlineApproval({ id:'x', prompt: '网络波动，正在恢复通信', ...})`（codex 六轮审计：审批卡承载错误） → R5 断言 C6/C7 全绿，**C8 红**（inlineApprovals 增量 > 0）
  17. handleDisconnected 加 `useTaskProgressStore.getState().setTaskGroup({ group_id:'x', tasks:[{ id:'t1', status:'failed', error:'连接已中断' }] })`（codex 六轮审计：taskProgress 红字失败面板绕过） → R5 断言 C6/C7/C8 全绿，**C9 红**（activeGroups 新增或出现 failed 任务）
  18. handleDisconnected 加 `useAgentActivityStore.getState().onAgentStatus(currentSessionId, 'error')`（codex 七轮审计：Sidebar `SessionStatusDot` 红点绕过） → R5 断言 C6/C7/C8/C9 全绿，**C10 红**（`sessionStatus[sid]` 被写成 `'error'`）
  19. handleDisconnected 加 `useChatStore.getState().setToolCallStatus('tc-1', { status: 'error', error: '连接已中断' })`（codex 八轮审计：harness 需预置 `tool_calls` + running 状态的 tool call；ToolAdapter/ToolHeader 红色 Error 徽标绕过） → R5 断言 C6/C7/C8/C9/C10 全绿，**C11 红**（toolCallStatuses error 态增量 > 0，`ai-elements/tool.tsx:47` 渲染为 output-error 红色头）
  20. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === presetTs ? { ...m, is_error: true, content: '连接中断，请刷新' } : m))`（codex 九轮审计：harness 预置一条 `is_error:false` 的既有 assistant 消息，messages.length 不增但 `MessageBubble.tsx:309` 渲染 ErrorCard） → R5 断言 C6/C7a/C7b/C8/C9/C10/C11/C13/C14 全绿，**C12 红**（`messages.filter(is_error===true).length` 从 0 变 1）
  21. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map(m => m.role === 'tool' && m.tool_call_id === 'tc-1' ? { ...m, content: 'tool error: 连接已中断' } : m))`（codex 十轮审计：harness 预置一条 `role:'tool',content:'{"ok":true}',is_error:false` 消息；`MessageBubble.tsx:175` 按 content 前缀判红色） → R5 断言 C6/C7/C8/C9/C10/C11/C12 全绿（`is_error` 没翻），**C13 红**（tool 错误前缀计数从 0 变 1）
  22. handleDisconnected 加 `useTaskProgressStore.getState().updateTask('g1', 't1', { error: '连接已中断' })`（codex 十轮审计：harness 预置 group `{ g1: [{ t1: running, error: undefined }] }`；`TaskProgressPanel.tsx:80` 渲染红字 error 文案，status 仍 running） → R5 断言 C6/C7/C8/C9/C10/C11/C12/C13 全绿（group 数/failed 数不变），**C14 红**（任务 error 非空计数从 0 变 1）
  23. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-plain-assistant-2' ? { ...m, content: '请稍候，系统正在重新同步' } : m))`（codex 十一轮审计：harness 预置一条 is_error:false content:'历史回复 B' 消息；MessageBubble 渲染正常气泡但内容已被改写成 reconnect 提示） → R5 断言 C6/C7/C8/C9/C10/C11/C12/C13/C14 全绿（length 不增、is_error 没翻、tool 前缀无、task.error 无），**C15b 红**（既有消息 content 字面不等）
  24. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-plain-assistant-2' ? { ...m, content: ({} as unknown as string) } : m))`（codex 十一轮审计：content 类型破坏，`artifactParser.ts:58` raw.slice 异常，`MessageBubbleBoundary.tsx:60` 渲染 "此消息渲染失败"） → R5 断言 C6-C14 全绿（C12 只查 is_error），**C15b 或 C15c 红**（preset content typeof 不是 string 或字面不等）
  25. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map((m, i, arr) => i === arr.length - 1 ? { timestamp:'preset-plain-assistant-2', role:'assistant', tool_call_id: undefined, content:'副本内容：网络波动，请稍候', is_error:false, tool_calls: undefined } : m))`（codex 十二/十三轮审计收敛：**length-neutral 替换末条 preset-tool-1 为 preset-plain-assistant-2 的三元组副本**，保持 `messages.length` 不变从而 C7a 绿，纯用 store 身份污染证明 C15a 独立有效） → R5 断言 C6/C7a/C7b/C8/C9/C10/C11/C12/C13/C14/C15b/C15c/C15d/C16 全绿（toast/approval/task/agent/tool 所有 store 未动，length 增量=0，content 字面和类型未动，tool_calls 未改，DOM 无红色元素），**C15a 红**（对 `preset-plain-assistant-2` 三元组 `matches.length === 2`；对 `preset-tool-1` 三元组 `matches.length === 0`）
  26. handleDisconnected 加 `useChatStore.getState().setMessages(messages.map(m => m.timestamp === 'preset-assistant-1' ? { ...m, tool_calls: [{ id:'tc-1', name:'forbidden_tool', arguments:'{"q":"x"}' }] } : m))`（codex 十二/十三轮审计收敛：**仅改 tool_calls[0].name，arguments 保留合法字符串不触发 `ToolExecutionBlock.tsx:45` raw.trim 异常也不走 `MessageBubbleBoundary` "此消息渲染失败" 兜底**，纯用字段身份漂移证明 C15d 独立有效） → R5 断言 C1/C2/C4a/C4b 全绿（无"失败"文案）、C6-C14/C15a/C15b/C15c/C16 全绿（三元组唯一、content 字面和类型未动，is_error 没翻，tool-error-content 无，toolCallStatuses 未改，DOM 无红色元素），**C15d 红**（`JSON.stringify([{id:'tc-1',name:'forbidden_tool',arguments:'{"q":"x"}'}]) !== JSON.stringify([{id:'tc-1',name:'search',arguments:'{"q":"x"}'}])`）
  27. `frontend/src/components/chat/MessageList.tsx:298` 附近静态插入 `<div className="text-red-500">通道暂不可用，请稍候</div>`（codex 十三轮审计：**纯 DOM 静态红元素绕过——文案完全不命中 C1/C4b 关键字且不写任何 store**；这是 Requirement 2 "no visible error state" 最本质的违规路径，之前 R5 只有 store 层 C6-C15 断言全绿，唯一 DOM 文案黑名单 C1 也被绕过） → R5 断言 C1/C2/C4a/C4b 全绿（文案无"连接中断/断连/失败"）、C3/C6/C7/C8/C9/C10/C11/C12/C13/C14/C15a/C15b/C15c/C15d 全绿（不写任何 store），**C16 红**（Stage 2 `errorStyledElements.length === 1` 不等于 0）
  28. `frontend/src/components/chat/MessageList.tsx:299` 附近静态插入 `<div style={{ color: 'var(--danger)' }}>通道暂不可用</div>`（codex 十四轮审计：**inline-style danger 色值绕过——不走 Tailwind `text-red-*` 类，而走 CSS 变量 `--danger`/`--destructive` 内联样式**；C16 原始选择器只看 class 名会漏） → R5 断言 C1/C2/C4/C6-C15 全绿，**C16 红**（Stage 2 `inlineDangerElements.length === 1`，inline-style 正则命中 `--danger`）
  29. `useWebSocket.ts:274` handleDisconnected 里 `document.body.appendChild` 一个 `<div role="alert">通道异常</div>`，同时在 `useWebSocket.ts:128` partial 分支入口 `document.querySelector('[role=alert]')?.remove()`（codex 十四轮审计：**瞬态 DOM 元素攻击——元素在 handleDisconnected 同步添加，在第一个 partial 帧到达后被移除**；C16 原始 Stage 2 最终快照 `querySelectorAll` 检测不到中间窗口曾经出现过红色元素，但用户已看到） → R5 断言 C1-C15 全绿（断言时 DOM 已清空），**C16 红**（Stage 1 MutationObserver `transientHits.length === 1`，因为 observer 在整个 act 窗口监听 `childList` 变化并保留引用副本）
  30. `useWebSocket.ts:274` handleDisconnected 里对既有 `<div data-testid="chat-root">` 设置 `setAttribute('aria-invalid','true')` 或 `setAttribute('data-state','error')`（codex 十四轮审计：**属性级错误信号攻击——不插入新元素，而是在既有 DOM 节点上挂错误属性**；ARIA `aria-invalid=true` 对辅助技术立即宣告错误，shadcn `data-state=error` 是组件库标准错误态） → R5 断言 C1-C15 全绿（无新消息/无新元素/无 class 变化），**C16 红**（Stage 1 MutationObserver `attributes` filter 命中 `aria-invalid`/`data-state` 变更并进入 `transientHits`）

共 30 条 mutation；全部必须让对应 vitest 失败，才算本 change 落地成立。
