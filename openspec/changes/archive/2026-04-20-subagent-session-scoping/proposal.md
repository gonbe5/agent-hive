## Why

`im-streaming-reply` 通过 X-1/X-1d/X-1e 把 master session 作用域事件的跨会话泄漏堵上了——`react_processor.go` 的 message / tool_call / agent_progress 全部改走 `BroadcastSessionMessage(session.ID, ...)`。但归档审阅时遗留两条 follow-up（原 `im-streaming-reply/tasks.md` 12.6 / 12.7）没进这次合并，因为它们跨 subagent / cli / bootstrap 多包，颗粒度太大，当时判定"不阻塞 streaming 主线"。

**第一次 proposal（2026-04-19）的盲区**（Codex + P10 双线评审 2026-04-20 揭出）：仅声明改 `StreamCallback` 签名 + 5 处审计点，但漏了三个**实际承载跨会话泄漏的核心代码点**：

1. **`internal/subagent/agentloop.go:44` `ProgressCallback` 类型**——和 `StreamCallback` 平级的另一个回调签名，同样不带 sessionID，同样会把 sub-agent 进度事件广播到错误会话
2. **`internal/master/master.go:573-587` `CreateAgentProgressCallback`**——内部直接调 `m.eventBus.Broadcast(BroadcastMessage{...})` **裸路径**，envelope 里 `SessionID` 空，前端 / IM renderer 的 session-mismatch drop 失效
3. **`internal/master/master.go:591-603` `CreateAgentStreamCallback`**——内部用 `BroadcastGenericMessage(EventTypeAgentProgress, payload)`，泛型路径**不带 session scope**，与 spec 12.4 约束冲突

第一版 proposal 不修这三处 = "看起来在动但没真正闭环"，是技术债中最毒的一类——spec 上是 red，代码上是 green，**最危险的 spec drift**。

现在 streaming 主线归档完成，这两条必须有单独的承载 change，否则 X-1 的 spec 约束（session-scoped event types MUST go through BroadcastSessionMessage）会在 subagent 与 lifecycle 的盲区上长期飘绿。

## What Changes

### 1. Subagent 回调签名 BREAKING（双签名同步改）

- **`StreamCallback`**（`internal/subagent/agentloop.go:25`）：
  - 旧：`func(agentID, content, reasoning string)`
  - 新：`func(agentID, sessionID, content, reasoning string)`
- **`ProgressCallback`**（`internal/subagent/agentloop.go:44`）：
  - 旧：`func(event ProgressEvent)`
  - 新：`func(event ProgressEvent)` ← 形态保留，但 `ProgressEvent` struct **新增 `SessionID string` 必填字段**
- 同步改 master / cli / bootstrap **3 处注册点 + 5 处调用点**（grep `CreateAgentStreamCallback|CreateAgentProgressCallback|StreamCallback|ProgressCallback`）

### 2. master.go 两处裸 Broadcast 改 session-scoped

- **`master.go:573-587` `CreateAgentProgressCallback`**：
  - 旧：`m.eventBus.Broadcast(BroadcastMessage{Type: EventTypeAgentProgress, Payload: ...})` ← 无 SessionID
  - 新：`m.eventBus.BroadcastSessionMessage(event.SessionID, EventTypeAgentProgress, ...)` ← envelope 携带 SessionID（从 `ProgressEvent.SessionID` 取）
- **`master.go:591-603` `CreateAgentStreamCallback`**：
  - 旧：`m.eventBus.BroadcastGenericMessage(EventTypeAgentProgress, payload)` ← 无 session scope
  - 新：`m.eventBus.BroadcastSessionMessage(sessionID, EventTypeAgentProgress, payload)` ← sessionID 从新签名第二参取

### 3. 非 session-scoped emit 审计完整闭环

`session_loop.go:33`（startup error，pre-subscribe）、`session_loop.go:389/640`（session_title，renderer 不 dispatch）、`master.go:500/531`（EventTypeAgentCreated/Destroyed）、`lifecycle.go:156`（EventTypeToolListChanged）——5 处当前无渗透但违反 spec 12.4 约束的发射点，按需补 sessionID 或明确用 `BroadcastGenericMessage` 并在注释中说明 **"no session scope by design"** + 对应 spec scenario 引用。

### 4. Spec 契约升级

在 `specs/im-streaming-reply/spec.md`（归档后继承进 `specs/` 主目录）的 "Subscriber-side session filter" 场景追加 AND clauses：

- **subagent 发起的 stream chunk**（`StreamCallback` 路径）MUST 携带 sessionID
- **subagent 发起的 progress event**（`ProgressCallback` 路径）MUST 携带 sessionID
- **agent 生命周期事件**（Created / Destroyed / ToolListChanged）MUST 显式选择 session-scoped 或 broadcast-scoped 路径，**不允许隐式默认**

## Capabilities

### New Capabilities
（无）

### Modified Capabilities
- `im-streaming-reply`: 追加 subagent 层 session 作用域约束；扩展 "Subscriber-side session filter" scenario 至双 callback + lifecycle 三类事件。

## Dependency Graph（CEO + Codex 双线评审强约束）

```
hitl-choice-type-registry (55/55, 待归档)
  ↓ 必须先归档（unblocks permission-minimalism + hive-skill-on-demand）
subagent-session-scoping (本 change)
  ↓ 必须先合并（前置基建，unblocks hive-skill-on-demand 的 SessionID 携带要求）
harden-spec-driven-phase2 (100/103, 收尾)
  ↓ 5 道护栏 + eval harness 准入闸门必须先过
add-spec-driven-cognition Phase 3 (subagent input.go 改造)
  ↓ 与本 change 写同一个文件！强制 owner 拉通会议 + 同 PR 或显式串行
spec-driven Phase 3 dual-flag rollout
```

**强约束**：
- 本 change 与 `add-spec-driven-cognition` Phase 3 都改 `internal/subagent/agentloop.go`——**owner 拉通会议必开**，决定 (a) 同 PR 落地 还是 (b) 本 change 先 merge、Phase 3 rebase
- 本 change 必须在 `harden-spec-driven-phase2` 5 道护栏合并之后、`spec-driven Phase 3 dual-flag` 切换之前完成

## Impact

- **代码**：
  - `internal/subagent/agentloop.go`（双签名改造，break 本包接口）
  - `internal/master/master.go`（CreateAgentStreamCallback `:591` + CreateAgentProgressCallback `:573` + lifecycle emit 5 处）
  - `internal/master/session_loop.go`（startup error `:33` + session_title `:389/640` 共 3 处）
  - `internal/master/lifecycle.go`（tool list changed `:156`）
  - `cmd/*` 注册点（3 处，grep `CreateAgent.*Callback` 命中）
- **测试**：
  - subagent 单元测试需更新 callback 签名
  - 新增 session-scope 渗透回归用例覆盖 双 callback + agent 生命周期事件
  - 红测：构造一个故意泄漏的 broadcast，CI 应**红**（保护 spec 12.4 不被静默回退）
- **兼容性**：
  - 双 callback 签名变更 + ProgressEvent struct 新增字段 = **BREAKING**（仅内部接口，无外部依赖）
- **依赖 change**：
  - **Blocks (unblocks)**：`hive-skill-on-demand`（其 spec.md:141/346/351 已要求 `skill.install.progress` 携带 SessionID，本 change 是其前置基建）
  - **Blocked by**：`hitl-choice-type-registry`（先归档保证 spec 主目录 hitl 协议层稳定）
  - **Co-edit conflict**：`add-spec-driven-cognition` Phase 3（同改 `internal/subagent/`，owner 必须拉通）
  - **Sequential**：在 `harden-spec-driven-phase2` 准入闸门后、`spec-driven Phase 3 dual-flag` 前
- **回归矩阵**：与 `session-scope-regression-matrix` change 配套——本 change 改代码，矩阵 change 加 CI 红测保护

## Verification

- `openspec validate subagent-session-scoping --strict` 通过
- `go build ./...` 全绿
- `go test ./internal/subagent/... ./internal/master/...` 全绿
- 红测：**故意写一个不带 sessionID 的 BroadcastSessionMessage 调用**，CI 必须红（防止未来回退）
- 渗透矩阵：跨租户 session A 的 subagent 进度事件**不应**到达 session B 的 WebSocket 订阅
