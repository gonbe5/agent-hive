# harden-spec-driven-phase2

> OpenSpec change — Phase 2 前置加固：在 `add-spec-driven-cognition` Phase 2 进入 dual flag rollout **之前**，把 5 道硬护栏 + eval harness 落地为准入闸门。

## 触发背景：CEO 评审裁定（2026-04-18）

**评审报告**：[`~/.gstack/workspace/company/ceo-plans/2026-04-18-add-spec-driven-cognition-review.md`](../../../../.gstack/workspace/company/ceo-plans/2026-04-18-add-spec-driven-cognition-review.md)
（3 轮 Codex 对抗辩论 + 30+ 处代码核验）

> **裁定结论**：`add-spec-driven-cognition` 的 Phase 2 在当前形态下**不可上线**。若直接进 dual flag 灰度，以下 5 处致命反例必然触发：

| 反例 | 问题 | 加固位置 |
|------|------|---------|
| **FM-1** | Subagent 在无显式信号下静默 resume 老 change（wrong-continuation） | tasks.md §6 Continuation 默认 OFF + fail-closed |
| **FM-2** | Cross-session 并发 update 触发 last-writer-wins 数据腐败 | tasks.md §2 SpecChangeStore CAS（单事务 + revision 校验） |
| **FM-3** | Streaming wrapper 绕过 intake，spec/legacy 路径分叉 | tasks.md §5 Intake 下移到纯函数共享入口 |
| **FM-4** | Planner schema drift（数字 task_key / int→float64 坍塌）直通工具执行 | tasks.md §7 Planner schema gate（strict JSON decoder） |
| **FM-5** | ReAct 内部持 `session.mu` 调 `setSpecCtx` → sync.RWMutex 不可重入死锁 | tasks.md §4 specCtx 走 `atomic.Pointer`（零锁读写） |

## 本 change 的交付物

```
harden-spec-driven-phase2/
├── proposal.md      — Why + What Changes + Capabilities + Impact
├── design.md        — 5 道 guard 逐项的实现选择 + 拒绝方案（含 Codex Q1-Q10 对抗）
├── specs/           — spec-eval-harness / spec-state-store / hidden-spec-layer 能力 delta
├── tasks.md         — 12 个 TG（task group），每个 TG 对应一个可独立验收的交付
└── README.md        — 本文件（CEO review link + 导航）
```

### 对其他 change 的影响

- **`add-spec-driven-cognition`** Phase 2 被本 change **取代并加固**（proposal.md 顶部已加 STATUS 标记）
- Phase 1（权限极简）+ Phase 3（SubAgent 契约）不受影响，继续以原 proposal 为准
- `add-spec-driven-cognition/specs/permission-minimalism/spec.md` 已追加 "BuiltinDangerousRules 条目数 = 19"（源码为准）不变量 scenario

## 关联产物

- **Runbook（灰度）**：[`docs/runbooks/spec-driven-rollout.md`](../../../docs/runbooks/spec-driven-rollout.md)
- **Runbook（回退）**：[`docs/runbooks/spec-driven-rollback.md`](../../../docs/runbooks/spec-driven-rollback.md)
- **Lock 规约/Invariant 对齐**：[`MEMORY.md`](../../../MEMORY.md)
- **Eval fixture**：`internal/specdriven/eval/fixtures/fm01_*.json` ~ `fm08_*.json`（8 个 required）
- **Feature flag 默认值锁定**：`internal/config/defaults.go:DefaultSpecDrivenConfig` + `TestDefaultSpecDrivenConfig_SystemLevelInvariant`

## 状态快照

- `openspec status --change harden-spec-driven-phase2` → `isComplete: true`（所有 artifacts done）
- Task 进度：见 `tasks.md`；其中 `TG10.5/10.6` 标记为 plumbing 就位、runner 未接（`ErrSpecRunnerNotImplemented` 桩），待独立 task 完成 spec runner 接 LLM client 后下线

## 导航

- 想知道"为什么要做"→ [`proposal.md`](./proposal.md)
- 想知道"怎么做 / 为什么这么做"→ [`design.md`](./design.md)
- 想知道"做到哪了 / 待办是什么"→ [`tasks.md`](./tasks.md)
- 想知道"能力契约"→ [`specs/`](./specs/)
- 想在生产落地/回退 → [`docs/runbooks/`](../../../docs/runbooks/)
