# Proposal: Spec-Driven Cognition for Hive Master Agent

> **⚠️ STATUS (2026-04-18)**: 本 change 的 Phase 2（"Hidden Spec Layer"）已由 [`harden-spec-driven-phase2`](../harden-spec-driven-phase2/README.md) **取代并加固**。Phase 2 的五处致命反例（FM-1..FM-5：静默 MRU / last-writer-wins 数据腐败 / planner schema drift / 锁重入死锁 / streaming 分叉）在本 proposal 原稿下不可上线；硬护栏 + eval harness 先行方为 Phase 2 dual-flag rollout 的准入闸门。Phase 1（权限极简重构）与 Phase 3（SubAgent 契约升维）不受影响，继续以本 proposal 为准。

## Why

Hive 当前的 Master Agent 是 ReAct 裸奔架构（`internal/master/react_processor.go:202`）：想一步做一步，意图不持久化，长任务断层，多 Agent 协作靠 prompt 拼接。复杂任务完成率低、跑偏率高、失败不可解释。

同时终端用户绝大多数不懂技术，**任何工具调用审批弹窗对他们都是价值毁灭**。现有 `lifecycle.go:170` 的 `im-*` 前缀自动放行是正确方向，但顺序不严谨 —— 破坏性检查在 IM 短路之后，IM 用户理论上能打穿 `DROP TABLE`。

OpenSpec 的 spec-driven 思想给出了答案：让 Agent 自己在内部走 `propose → apply → archive` 流程，**把规划纪律注入认知架构、把意图持久化为文件、把工作流作为跨 session 契约**。用户完全看不见这套机制，只感受到：任务更准、长任务可续、失败可追溯、权限不烦。

## What Changing

1. **权限极简重构（复用现有 SafeExecutor）**：`createPermissionPromptFn` 默认 `Granted: true`，但在返回前先调用 `security.SafeExecutor.MatchPolicy()`（复用 `internal/security/builtin_rules.go` 的 `BuiltinDangerousRules`）。`PolicyDeny` 一律拦截（IM 也不例外），`PolicyAsk` 在非 IM 会话进入 HITL、IM 会话继续放行并打 warn。业务决策（账号选择、歧义澄清）HITL 保留。**不新建 `destructive_guard.go`**，避免双源真相。
2. **隐藏式 Spec Layer 内嵌到 Master**：接收复杂意图时自动生成 proposal + tasks（存 `internal/storage/specs/{session_id}/`，不上前端），ReAct 循环按 tasks.md checklist 推进，completion 时 archive 归档，30 天后 blob 压缩避免磁盘无限增长。Planner 强制用 cheap model（haiku 4.5 级）+ per-plan token budget + overbudget fallback，避免成本爆炸。
3. **Change 作为跨 Session 意图载体**：`SessionState` 新增 `ChangeID` 持久字段 + `specCtx *Context` 运行时指针（参考现有 `activeLLM *llm.Client` 模式）；session_compact 保留 `ChangeID`；用户说"昨天那个继续"时 `Continuation.Resolve` 作为意图入口的第一级分流，命中后绕开 classifier 与 planner 直接续跑。
4. **SubAgent 契约升维（零签名变更）**：复用现有 `subagent.AgentInput.Context` map 携带 `spec_ref`；`AgentLoop.Run` 与 `AgentInput` struct 签名不变，保持所有现有 caller 编译通过。Skills discovery 增加 spec-requirements 语义路由，失败回退 keyword 匹配。
5. **失败自动纠偏走显式信号**：用户点 IM 卡片上的 👎 按钮（或 `/redo` 命令）触发 `Verifier.Diff`，**不**依赖 LLM 分类用户自由文本（避免误判）。

## 关键改动边界（避免与现有代码冲突）

- **`runReActLoop` 签名不变**：`specCtx` 走 `SessionState` 字段，函数内部读取。保持现有 9 参签名，避免打断 `im-streaming-reply` 等并行 change。
- **`SessionState`（不是 `Session`）**：类型名对齐 `internal/master/session.go:12`。
- **IM 短路顺序修正**：`SafeExecutor.MatchPolicy` 在 `strings.HasPrefix(sessionID, "im-")` 检查**之前**运行。
- **reuse > create**：`builtin_rules.go` 已有 `PolicyDeny` + `PolicyAsk` 两级 + 19 条规则，直接扩展，不起新文件。（原稿误写 20 条，由 `harden-spec-driven-phase2` TG11.2 勘误；实际数字以 `internal/security/builtin_rules.go` 源码为准）

## Non-goals

- **不**给终端用户暴露 proposal/specs/tasks 任何前端视图 —— 永远隐藏
- **不**给用户加 OpenSpec 命令面板 —— 这是纯内部认知机制
- **不**要求用户审批工具调用 —— 违反核心用户体验原则
- **不**参照过时的 `DESIGN.md` 做 UI 决策
- **不**替换 ReAct，而是在 ReAct **之前**加规划层（Continuation + Classifier + Planner）、**之后**加验收层（Verifier）
- **不**改动 LLM 模型选择、tokenizer、compaction 算法本体
- **不**废除 `im-*` 前缀自动放行（这是 feature 不是 bug，只修检查顺序）
- **不**引入外部 OpenSpec CLI 作为运行时依赖 —— 只复用其 artifact 格式与工作流思想，Go 侧原生实现 `internal/specdriven/` 包
- **不**改动 `runReActLoop` / `AgentLoop.Run` / `AgentInput` 的公共签名 —— 所有扩展走 struct 字段或 Context map
- **不**新建 `destructive_guard.go` —— 复用 `internal/security/builtin_rules.go`
