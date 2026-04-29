# Hive — Engineering Memory (Lock & Invariant Discipline)

> Quick-reference onboarding doc。新成员入组 1 小时内必读。
> Source of truth for **lock discipline, concurrency invariants, and cross-subsystem 规约**。
> 内容进更新需 PR review；本文件不记临时 decisions，只收**不可逾越的底线**。

## 1. Lock 规约（Master Agent 层）

### 1.1 `session.mu`（`internal/master/session.go`）

- `sync.RWMutex` — **不可重入**。任何持 `s.mu.Lock()` 时调用的函数，其内部绝不允许再次 `s.mu.Lock()` / `RLock()`。
- `runReActLoop` (`internal/master/react_processor.go:202`) 在 `:186` 附近已持锁；循环体内部读取 `specCtx` 必须走 `atomic.Pointer.Load()`，**不持 s.mu**。
- 新增访问器若涉及字段写入，命名必须显式标注 `LockHeld` / `OffLock` 后缀，review 卡这一条。

### 1.2 `SessionState.specCtx` 写入（`atomic.Pointer[specdriven.Context]`）

- **允许写入点**：`session_loop.go` 的 task ingress（`applySpecDrivenIntake` → `StoreSpecCtx(...)`）——持锁外调用。
- **禁止写入点**：subagent 路径、background goroutine、LLM callback、tool handler。违者：
  - test build → `StoreSpecCtxGuarded` panic
  - prod → `spec_state_unauthorized_write_total` counter + warn log
- **允许读取点**：任何 goroutine，任何时刻，零锁；用 `session.LoadSpecCtx()`。

### 1.3 `SessionSpecState` 持久化（`internal/store/spec_session_store.go`）

- Load → mutate → Save 必须在**持锁外完成**（仿 `session_manager.go:741` snapshot pattern）。
- `SpecState` 的任何写入只在 task ingress（`session_loop.go:712 processTask` 入口），subagent **禁止改写** `ActiveChangeID` / `FocusMRU` / `Changes`。
- `FocusMRU` 上限 16 条（`FocusMRUCap`），由 `NormalizeFocusMRU` / `TouchChange` helper 强制。禁止手搓 slice 操作。

### 1.4 `SpecChangeStore` CAS（`internal/store/spec_store.go`）

- `UpsertWithCAS` = 单事务 `SELECT ... FOR UPDATE` → 三路 CAS 冲突判断 → `UPDATE ... WHERE revision = $expected`，`rows_affected=0` → `ErrSpecChangeConflict`。
- **禁止**跨事务先 Read、再 Write；CAS 保护只在单事务内有效。
- `AppendEvent` 必须通过 `appendEventTx`（在同一 tx 内 `MAX(sequence)+1`），**禁止**跨事务拼 sequence。
- `RetentionSweep` 的 `RetentionProtectedStatuses`（`draft/planning/active/in_progress/blocked`）是 **append-only allowlist**——任何删除项的 PR 必须被 review 拦截。

## 2. Spec-Driven 不变量（`harden-spec-driven-phase2`）

### 2.1 Feature flag 默认值（FM-1 / FM-4 反例）

`internal/config/defaults.go` 锁定：

| 字段 | 默认值 | 锁定 test |
|------|--------|----------|
| `spec_driven.mode` | `"legacy"` | `TestDefaultSpecDrivenConfig_SystemLevelInvariant` |
| `spec_driven.continuation.default` | `"off"` | 同上 |
| `spec_driven.planner.token_budget` | `800` | 同上 |

**禁止**任何 PR 把 mode 默认值改非 `legacy`（会把所有老 session 一夜拖进 spec 路径）。

### 2.2 Intake 决策单一入口（FM-3 反例）

- `internal/specdriven/intake/decide.go:ResolveIntakeDecision` 是**唯一**的 intake 纯函数。
- `public_api.go:64` (thin layer) 保持不动，streaming wrapper 未来必须共享同一函数。
- 禁止新增任何并行 intake 分支——任何 "streaming 特殊处理" PR 必须被拒。

### 2.3 Planner schema gate（FM-4 反例）

- `Plan.Steps[].TaskKey` 必须是 `^\d+(\.\d+)+$`（≥2 段），**禁止纯数字**。
- `PlanStep.Args` 字段类型必须是 `json.RawMessage`，**禁止** `any` / `map[string]any`（int→float64 坍塌）。
- Decode 必须 `DisallowUnknownFields + dec.More() 拦尾随 + ToolName 非空`。

### 2.4 BuiltinDangerousRules 数字一致性

- `internal/security/builtin_rules.go` 条目数是 **19**（源码为准）。
- 所有文档（proposal / README / security-model / runbooks）引用此数字时**必须**与源码同步。
- 任何 PR 变更 `BuiltinDangerousRules` 条目数必须在同一 PR 内更新每一处文档引用，pre-merge grep 检查。

## 3. 权限 & 安全（`add-spec-driven-cognition` Phase 1）

### 3.1 HITL 门控顺序

`internal/master/lifecycle.go` 中 **`SafeExecutor.MatchPolicy` 严格早于 `strings.HasPrefix(sessionID, "im-")` 判定**。顺序调换 → IM 用户能打穿 `rm -rf /`。review 卡这一条。

### 3.2 SafeExecutor 单实例

- `Master.safeExecutor *security.SafeExecutor`（struct 字段）+ 热重载 `atomic.Pointer[SafeExecutor]` 原子替换。
- **禁止**在 `createPermissionPromptFn` 里每次请求 re-construct SafeExecutor。
- **禁止**新建 `destructive_guard.go` 或任何并行 blocklist（`add-spec-driven-cognition/specs/permission-minimalism/spec.md` spec 锁定）。

## 4. 观测 & Metrics

### 4.1 Metric naming

- spec-driven 相关 metric 一律 `specdriven.*` 前缀。
- Label 必须取自**有限枚举**（避免 Prom cardinality 漂移）。`IntakeDecision.MetricLabel` 已压扁 path × downshift 组合。
- 禁止把 `session_id` / `user_id` / `change_id` 作为 Prometheus label（高基数爆炸）。写入 metric 的 `Labels` map 里只用作 log correlation，不做聚合。

### 4.2 Metric 入队

- 用 `m.enqueueMetric(observability.Metric{...})`——nil-safe pool + 满队丢弃兜底。
- **禁止**同步写 DB / 同步调外部服务，任何 metric emit 必须 ≤ 50ns。

## 5. 文档 & Runbook 对齐

- `docs/运维手册/spec-driven-rollout.md` + `rollback.md` = spec-driven 生产变更的**唯一指导**。
- CEO review report 存 `~/.gstack/workspace/company/ceo-plans/`。每次 P1 事故必须补一份 RCA 文件到同目录。
- 任何 guard 下线（`ErrSpecRunnerNotImplemented` 被移除等）必须同步更新本文件。

## 6. 非义务

- 本文件**不**收集临时决策、未来愿景、风格偏好——那些去 `~/.gstack/workspace/company/ceo-plans/`。
- 本文件**不**替代 OpenSpec change 文档；它只是提醒"不可逾越的底线"。
- 新员工若发现本文件某条与最新代码不符——**请 PR 修本文件**，不要只在 Slack 里提。
