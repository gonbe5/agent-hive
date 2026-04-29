## Why

CEO 评审（2026-04-18，3 轮 Codex 对抗辩论 + 30+ 处代码核验）裁定 `add-spec-driven-cognition` 的 Phase 2 在当前形态下**不可上线**：会引入静默错写（wrong-continuation resumption）、cross-session last-writer-wins 数据腐败、planner schema drift 直通工具执行、以及 ReAct 内部锁重入死锁。本 change 是 Phase 2 的"前置加固"——把 5 道硬护栏 + eval harness 当作 Phase 2 dual-flag rollout 的准入闸门。底层逻辑：correctness gate 必须先于 functionality gate 落地。

## What Changes

- **新增 eval harness 包** `internal/specdriven/eval/`：fixtures + table-driven test + CI 准入门槛（≥75% 覆盖率，required fixture 100% pass，否则禁止进 dual flag）
- **重塑 SessionState spec 字段**：`ChangeID string` 标量 → `SessionSpecState{ActiveChangeID, FocusMRU []string, Changes map[string]ChangeRef}`，task 标识用 `task_key string` 而非数字（防 `1.10`/`1.1` 坍塌）
- **runtime specCtx 走 atomic.Pointer**：避免 `session.mu` 重入死锁（runReActLoop 已在 `react_processor.go:191` 持锁）
- **Intake 决策下移**：从 `public_api.go:64`（thin layer，将与 streaming 分叉）移至 `session_loop.go:757`（`processTaskDirectExec` 之前的内部 task path）
- **Continuation 默认 OFF + fail-closed**：背景/subagent 工作禁止改写 `ActiveChangeID`/`FocusMRU`；MRU 单一信号且无显式提及 → 必须 ASK
- **Planner schema gate**：strict typed struct + `json.Decoder.DisallowUnknownFields`，验证失败 → 重试一次 → 再失败回退 direct execution（在工具构造前）
- **Storage 改为 store-canonical**：扩展 `internal/store` 加 `SpecChangeRecord` + 修订号 CAS（仿 `skill_store.go:62`）；FS 仅作带 `exported_revision` 标记的可选物化缓存
- **BREAKING（仅对未发布的 Phase 2）**：取消原 proposal 中 `internal/storage/specs/` 文件系统作为 source of truth 的设计；取消 `ChangeID string` 标量字段

## Capabilities

### New Capabilities
- `spec-eval-harness`: 评测套件 + CI 准入门槛，作为 dual-flag rollout 前的 correctness gate
- `spec-state-store`: Store-canonical 的 change/task 持久化层，含修订号 CAS 与审计事件

### Modified Capabilities
- `hidden-spec-layer`: 重塑状态形状（SessionSpecState 替代 ChangeID 标量）；runtime specCtx 改 atomic.Pointer；intake 决策下移；continuation 默认 OFF；planner schema gate 加固

## Impact

- **代码**：`internal/master/{session,session_loop,react_processor,public_api}.go`、`internal/store/`（新增 spec_store.go）、`internal/specdriven/`（新建包含 eval/ 子包）、`internal/subagent/input.go`（仅 Phase 3 启用时）
- **DB schema**：新增 `hive_spec_changes`、`hive_spec_change_events` 表（含 revision 列）；session_metadata 不动
- **CI**：`Makefile` 新增 `test-specdriven` target；CI workflow 在 dual-flag 切换前必须跑此 target
- **配置**：`spec_driven.continuation.default = "off"`、`spec_driven.planner.token_budget = 800`（从 proposal 原值 2000 收紧）
- **依赖**：无新增第三方依赖
- **回滚**：所有 5 道护栏均可单独 feature-flag；planner gate 失败回退到 direct execution，不影响现有 Phase 1 行为
