## 1. 类型契约 BREAKING（先改类型，让编译失败暴露所有 callsite）

- [x] 1.1 修改 `internal/subagent/agentloop.go:25` `StreamCallback` 签名为 `func(agentID, sessionID, content, reasoning string)`
- [x] 1.2 修改 `internal/subagent/agentloop.go:34-41` `ProgressEvent` struct 新增 `SessionID string \`json:"session_id"\`` 字段
- [x] 1.3 `go build ./...` 跑一遍，记录所有编译失败 callsite（预期：5 处以上）

## 2. AgentLoop emit 现场注入 sessionID（D4 single source）

- [x] 2.1 修改 `internal/subagent/agentloop.go:153-157` `emitProgress`：在调用 `a.progressFn(event)` 前补 `event.SessionID = a.sessionID`
- [x] 2.2 修改 `internal/subagent/agentloop.go:197` `a.streamFn(a.agentID, content, reasoning)` 为 `a.streamFn(a.agentID, a.sessionID, content, reasoning)`
- [x] 2.3 验证 agentloop.go 内 3 个 emitProgress 调用点（259/315/340）无需改动 —— SessionID 由 emitProgress 内部统一注入

## 3. master.go 5 个 emission site 显式分类

- [x] 3.1 改 `internal/master/master.go:573-587` `CreateAgentProgressCallback`：把 `m.eventBus.Broadcast(BroadcastMessage{...})` 替换为 `m.eventBus.BroadcastSessionMessage(event.SessionID, BroadcastMessage{Type: EventTypeAgentProgress, SessionID: event.SessionID, Payload: ...})`
- [x] 3.2 改 `internal/master/master.go:591-603` `CreateAgentStreamCallback`：签名变为 `func(agentID, sessionID, content, reasoning string)`，把 `BroadcastGenericMessage` 替换为 `BroadcastSessionMessage(sessionID, BroadcastMessage{Type: EventTypeAgentProgress, SessionID: sessionID, Payload: payload})`，并在 payload 内加 `"session_id": sessionID`
- [x] 3.3 在 `internal/master/master.go:516` `EventTypeAgentCreated` BroadcastGenericMessage 调用上方加注释：`// no session scope by design — agent registry 是全局视图，所有用户共享 agent 元数据`
- [x] 3.4 在 `internal/master/master.go:547` `EventTypeAgentDestroyed` BroadcastGenericMessage 调用上方加注释：`// no session scope by design — agent 销毁通知所有 session（agent 是跨 session 共享资源）`
- [x] 3.5 在 `internal/master/lifecycle.go:156` `EventTypeToolListChanged` 调用上方加注释：`// no session scope by design — tool catalog 是全局共享视图`

## 4. 3 个 callback registration site 同步签名

- [x] 4.1 编译验证 `internal/master/master.go:464-465` `m.agentFactory.SetProgressCallback/SetStreamCallback` —— closure 通过 master 方法包装，签名变化对外不可见
- [x] 4.2 编译验证 `internal/bootstrap/server.go:581-582` `ProgressFn`/`StreamFn` 字段赋值 —— 类型签名匹配新 callback type
- [x] 4.3 编译验证 `internal/cli/app.go:392-393` 同上

## 5. factory.go 类型转发（编译验证为主）

- [x] 5.1 验证 `internal/subagent/factory.go:47-48` 字段类型继续匹配新 ProgressCallback / StreamCallback
- [x] 5.2 验证 `internal/subagent/factory.go:97-114` SetProgressCallback / SetStreamCallback setter 自动适配
- [x] 5.3 验证 `internal/subagent/factory.go:244,247,262,265` 转发到 AgentLoop 的 setter 调用 —— 类型自然透传

## 6. 单元测试（覆盖跨租户 invariant）

- [x] 6.1 新增 `internal/subagent/agentloop_session_scoping_test.go`：构造 AgentLoop 设 sessionID="sA" → 触发 emitProgress → 验证 progressFn 收到的 event.SessionID == "sA"
- [x] 6.2 同文件：触发 streamFn 路径（mock LLM 返回 streaming chunk）→ 验证 streamFn 第 2 参 == "sA"
- [x] 6.3 新增 `internal/master/master_subagent_broadcast_test.go`：构造 Master + EventBus → 调用 CreateAgentProgressCallback() 拿到回调 → 触发回调 → 验证 BroadcastMessage.SessionID == event.SessionID（非空）
- [x] 6.4 同文件：CreateAgentStreamCallback() → 触发 → 验证 BroadcastSessionMessage 被调用且 sessionID 非空

## 7. 集成验证 + spec 闭环

- [x] 7.1 全工程 `go build ./...` 必须 0 错误
- [x] 7.2 全工程 `go test ./internal/subagent/... ./internal/master/...` 全绿（pre-existing failures 不在本 change 范围）
- [x] 7.3 grep 自审：`grep -rn "eventBus.Broadcast(BroadcastMessage" internal/master/` 输出 0 例外（含 BroadcastGenericMessage 注释或 BroadcastSessionMessage）
- [x] 7.4 `openspec validate subagent-session-scoping --strict` 绿
- [x] 7.5 spec 反向验证：手工 revert task 3.1（恢复裸 Broadcast）→ master_subagent_broadcast_test.go 必须红（已验证 RED → 已恢复 GREEN）

## 8. 拉通 + 归档

- [x] 8.1 与 `add-spec-driven-cognition` Phase 3 owner 对齐 master.go hunk 顺序，确保不冲突
  - **2026-04-20 冲突检测**：`grep -n "master.go" openspec/changes/add-spec-driven-cognition/tasks.md` 命中 `:340`（safeExec 提升）+ `:1211`（热重载原子替换）；本 change 动 `:464/:516/:547/:573/:591`——**line ranges 完全不相交**，两 change 可独立 merge
- [x] 8.2 PR 描述中标注：unblocks `hive-skill-on-demand`, `frontend-ws-handshake-regression`, `session-scope-regression-matrix`
  - **2026-04-20 unblocks 清单**（单人 repo，无 PR，记入此处作 paper trail）：
    - `hive-skill-on-demand` spec.md:141/346/351 要求 `skill.install.progress` 携带 SessionID——本 change 是前置
    - `frontend-ws-handshake-regression` 需要 subagent 事件带 SessionID 才能验证 WS 重连语义
    - `session-scope-regression-matrix` R-1/R-2/R-3 fixture 需要本 change 修复后才能验真
- [x] 8.3 merge 后通知 `session-scope-regression-matrix` owner 启动红测落地（其 R-1/R-2/R-3 fixture 需要本 change 的修复后才能验真）
  - **2026-04-20 依赖解除公告**：已写入 `openspec/changes/session-scope-regression-matrix/proposal.md`——该 change owner 可启动红测
- [x] 8.4 `openspec archive subagent-session-scoping -y` 完成归档
