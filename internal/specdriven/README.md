# `internal/specdriven/`

Spec-driven cognition core (Phase 2 hardening per `openspec/changes/harden-spec-driven-phase2`).

## Subpackages

| Package | Purpose | Guard |
|---|---|---|
| `.` (root) | Shared types: `SessionSpecState`, `ChangeRef`, `Context`, `Decision`, `CompareTaskKey` | foundation |
| `continuation/` | Guard 1 — `Resolve(userText, state, now, cfg)` returns `Resume`/`Ask`/`New` with fail-closed defaults (FM-1 reversal) | 1 |
| `planner/` | Guard 4 (business side) — `Decode([]byte) (*Plan, error)` with `DisallowUnknownFields` + regex `^\d+(\.\d+)+$` (FM-4 reversal) | 4 |
| `intake/` | Guard 4/5 — `ResolveIntakeDecision` pure-function decision matrix shared by ProcessMessage / ProcessMessageStream (FM-3 anti-split) | 4 |
| `eval/` | Guard 5 — harness + fixtures; `make test-specdriven` gates promotion | 5 |

Storage layer (`hive_spec_changes`, `hive_spec_change_events`, `hive_spec_session_state`)
lives in `internal/store/` via `SpecChangeStore` + `SpecSessionStateStore` (single-tx CAS,
see `openspec/changes/harden-spec-driven-phase2/design.md` D3).

## Phase 3 deferred work (DO NOT IMPLEMENT IN PHASE 2)

| Hook | Location | Reason |
|---|---|---|
| `spec_ref` capability token | `internal/subagent/input.go` (`AgentInput.Context["spec_ref"]`) | Subagent routing against a spec requires a read-only capability token so subagents can never write `SpecState` / `SpecChangeStore` directly (violates Guard 4 + Guard 6). Phase 2 keeps subagents spec-agnostic; all spec mutation happens in `session_loop.go` ingress. |
| Subagent spec event bus | `internal/specdriven/specref/` (not yet created) | Phase 3 will add a token-redemption API + event-emission path so subagents can report spec progress without owning state writes. |

**Hard rule for Phase 2 code**: `Context["spec_ref"]` is reserved. No code in `internal/`
may read or write this key until Phase 3. Use `session.LoadSpecCtx()` (atomic.Pointer) to
read current spec context from inside `runReActLoop` instead.

See `openspec/changes/add-spec-driven-cognition/specs/hidden-spec-layer/spec.md` §
"Phase 3 spec_ref capability token (deferred, NOT implemented in Phase 2)" for the
normative spec statement and enforcement scenario.

## Storage invariant (canonical = Postgres; no hybrid FS export in Phase 2)

Phase 2's single source of truth is **`hive_spec_changes` + `hive_spec_change_events`**
via `internal/store/SpecChangeStore`. There is no filesystem canonical, and no
`internal/specdriven/store/` package exists — spec persistence is served directly
from the `store` package so every writer goes through one CAS pipeline.

If Phase 3 (or later) adds filesystem export (e.g., `.openspec/changes/<id>.json`
for human review/audit), the hybrid invariant is non-negotiable:

1. **Canonical stays in Postgres.** FS artifacts are read-only projections.
2. Every FS artifact MUST embed `exported_revision: N` — reading code that finds
   `N < db.revision` MUST fail closed, never merge.
3. Retention never touches active work. `SpecChangeStore.RetentionSweep` uses
   `RetentionProtectedStatuses` (`draft`/`planning`/`active`/`in_progress`/`blocked`)
   as an append-only allowlist — status removals require a dedicated review.

Cross-process concurrency is validated by `TestSpecStore_ConcurrentUpdate` (8
goroutines racing `UpsertWithCAS`: exactly 1 wins, 7 return `ErrSpecChangeConflict`,
revision only ticks to 2, events table grows by exactly 1).

## Design references

- `openspec/changes/harden-spec-driven-phase2/proposal.md` — what + why
- `openspec/changes/harden-spec-driven-phase2/design.md` — decision matrix, FM-{1..8} failure modes
- `openspec/changes/harden-spec-driven-phase2/tasks.md` — TG1–TG12 checklist
