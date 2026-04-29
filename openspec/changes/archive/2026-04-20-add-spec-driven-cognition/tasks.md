# Tasks: Spec-Driven Cognition Implementation

> **Archive note (2026-04-20)**：本 change 归档时的真实落地状态。
> - **Phase 1（permission-minimalism）**：1.1–1.9 代码 & 测试已合入主干，spec 随本次 archive 并入 `openspec/specs/permission-minimalism/`；1.10 灰度为运营路径，不阻塞工程归档。
> - **Phase 2（hidden-spec-layer）**：原始方案被 [`harden-spec-driven-phase2`](../archive/2026-04-19-harden-spec-driven-phase2/) 重构并加固。新路径落地的实现（PG `spec_store` 代替文件系统、mode 配置代替复杂度启发式、eval 双跑代替 redo 信号、`intake.Decide` 代替 `resolveIntakeDecision` 等）已在该 change archive 中承载全部 spec 和测试证据。
> - **Phase 3（SubAgent & Skills Routing）**：超出当前取舍范围，**不做**；对应 delta `spec-driven-subagents/spec.md` 已在本次清理中删除，避免悬挂承诺。如需未来推进，另起独立 change。
> - **Phase F**：验收指标继承到 `harden-spec-driven-phase2` 的 runbook 与 SLO 面板，不再由本 change 承载。

## Phase 1 — Permission Minimalism（已落地 + 随 archive 并入主 specs）

- [x] 1.1 `internal/master/lifecycle.go:createPermissionPromptFn` 改造（shell-family 判定 + JSON unmarshal + MatchPolicy 先于 IM 短路 + PolicyDeny/Ask/Allow 分支）
- [x] 1.2 `Master.safeExecutor` 字段提升 + 热重载原子替换 + `safeExecAdapter` 对接
- [x] 1.3 `internal/master/lifecycle_test.go` 7 条路径覆盖（IM×Policy × 业务决策正交）
- [x] 1.4 `internal/config/config.go` 增 `Security.PermissionMode` + `Security.DestructivePatterns`（append 到 `NewSafeExecutor`）
- [x] 1.5 `strict` 模式兜底（一键回滚路径）
- [x] 1.6 `internal/observability/` 注册 `security.policy_deny_total{pattern}` / `security.policy_ask_im_autoallow_total`
- [x] 1.7 `README.md` 权限章节 + `docs/security-model.md` 已更新
- [x] 1.8 TODOS.md 老 P1 IM HITL 审批条目标记 OBSOLETE 并引用本 change
- [x] 1.9 `go test ./internal/security/... ./internal/master/... -race` 全绿
- [x] 1.10 灰度放量 —— **运营路径**，由平台侧跟进；工程归档不阻塞

## Phase 2 — Hidden Spec Layer（被 harden-spec-driven-phase2 重构并加固）

> 下列任务的实际落地全部收敛到 `archive/2026-04-19-harden-spec-driven-phase2/`，包含 PG `spec_store`、mode 配置路由、eval 双跑、intake 分流抽取、SLO 指标链、灰度 runbook 等。原任务编号保留作溯源，勾选代表"该 commitment 已由新路径履行"。

- [x] 2.0 `internal/airouter/types.go` `TaskPlanning` 常量 + `selector.go` 路由（落地，evidence: `internal/airouter/selector.go:41`）
- [x] 2.1 `internal/specdriven/` 包骨架 + schema / types（落地：`internal/specdriven/types.go` / `metrics.go` / `compare.go`）
- [x] 2.2 **替代路径**：不做文件系统 atomic rename；改由 `internal/store/spec_store.go` PG 持久化（harden Round 5 N3 决定）
- [x] 2.3 **替代路径**：retention/blob 压缩随 PG 方案取消，由数据库 TTL + 观测指标 `specdriven.spec_change_store_disabled` 替代
- [x] 2.4 `internal/specdriven/continuation/resolver.go` 模糊 hint 匹配
- [x] 2.5 **替代路径**：复杂度启发式改为 `internal/specdriven/intake/decide.go` 的 mode 配置（legacy/dual/spec），决策路径更明确
- [x] 2.6 `internal/specdriven/planner/plan.go` + `decode.go`：JSON schema 约束 + retry-once + budget（harden task 7.5 / G1）
- [x] 2.7 **替代路径**：verifier 由 `internal/specdriven/eval/` 双跑 + `dual_diff_total` 指标替代显式 redo 信号
- [x] 2.8 单元测试：`continuation_test.go` / `planner/*_test.go` / `eval/bridge_realrunner_test.go`（N2 闭环）/ `compare_test.go` 等均落地
- [x] 2.9 `internal/master/session.go:39,52` `SpecState` + `atomic.Pointer[specdriven.Context]`
- [x] 2.10 `internal/master/session_compact.go` `PreserveSpecStateOnCompact` + mutation test
- [x] 2.11 `internal/master/session_loop_specdriven*.go` 系列承接 ReAct spec-aware 循环（签名不变）
- [x] 2.12 `internal/master/prompt_builder.go:514` 调用 `PreserveSpecStateOnCompact`；spec 注入由新路径承担
- [x] 2.13 `internal/specdriven/intake/decide.go` 作为 single source of truth（代替原 `resolveIntakeDecision` 抽取）；streaming 与非 streaming 路径统一走此入口
- [x] 2.14 outbound sanitizer：PG 路径无本地存储泄露风险，任务自然消化
- [x] 2.15 `internal/config/config.go:63` `SpecDrivenConfig` struct（`Mode` / `PlanTokenBudget` / `PlannerTimeoutMs` 等）
- [x] 2.16 观测指标链：`plan_total` / `plan_fallback_total` / `plan_overbudget_total` / `spec_change_upsert_total` / `spec_change_store_disabled` / `execution_path_total` / `intake_decision_total` / `dual_diff_total` 等（harden Round 5/6 G1+N1+N3 闭环）
- [x] 2.17 集成测试：harden-phase2 evidence 覆盖 happy/fallback/dual-diff 路径
- [x] 2.18 `go test ./internal/specdriven/... ./internal/master/... -race` 全绿（Round 6 审计）
- [x] 2.19 灰度 runbook：`docs/runbooks/spec-driven-rollout.md` single-node-canary + SLO 门槛（harden G3）

## Phase 3 — SubAgent & Skills Routing 升维（不做，已从本 change 移除）

> 2026-04-20 决策：Phase 3 超出当前精力/价值取舍，spec delta `spec-driven-subagents/spec.md` 已删除。若未来需要推进，另起独立 change，避免本 change 悬挂承诺。

- [x] 3.1 取消 —— SubAgent `Context["spec_ref"]` 注入通道未实现
- [x] 3.2 取消 —— `execution_result.md` 写回 + `Store.UpdateTaskStatus` 未实现
- [x] 3.3 取消 —— Skills `ProvidesRequirements` frontmatter 未扩展
- [x] 3.4 取消 —— `FindBySpecRequirements` 未实现
- [x] 3.5 取消 —— Master 语义派活未接入
- [x] 3.6 取消 —— keyword 回退未实现（对应指标 `skills.spec_routing_miss_total` 不注册）
- [x] 3.7 取消 —— 存量 skills frontmatter 未批量补字段
- [x] 3.8 取消 —— 对应单元测试未写
- [x] 3.9 取消 —— 对应集成测试未写
- [x] 3.10 取消 —— Phase 3 不纳入 `go test ./... -race` 验收范围
- [x] 3.11 取消 —— `subagent_mode` 灰度策略不执行

## 最终验收（继承到 harden-spec-driven-phase2）

- [x] F.1 Phase 1 合入 + spec 随 archive 并入主 specs；Phase 2 由 harden-spec-driven-phase2 承载并通过 Round 6 APPROVE；Phase 3 明示不做
- [x] F.2 SLO 门槛 `specdriven.plan_fallback_total / specdriven.plan_total ≤ 5%` 交付给 `docs/runbooks/spec-driven-rollout.md`（harden task 12.8(d)，平台侧灰度 gating）
- [x] F.3 `security.policy_deny_total` 指标已注册，实际触发率由运营侧 dashboard 观测
- [x] F.4 `specdriven.plan_token_cost_total` p99 观测接入平台 dashboard（harden Round 5 G1）
- [x] F.5 用户侧复杂任务完成率 A/B —— 产品/数据侧指标，不在工程 archive 范围
- [x] F.6 PG `spec_store` 替代 archive 压缩，disk-full 类信号由数据库层承担
- [x] F.7 `openspec archive add-spec-driven-cognition` —— 本次即执行
- [x] F.8 `README.md` 架构/定位章节已由 harden archive 中的 runbook + 架构说明承担
