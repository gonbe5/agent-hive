# hidden-spec-layer Specification

## Purpose
TBD - created by archiving change harden-spec-driven-phase2. Update Purpose after archive.
## Requirements
### Requirement: specCtx lives on SessionState, not as runReActLoop param

The system SHALL add a private field `specCtx *atomic.Pointer[specdriven.Context]` to `internal/master/session.go:SessionState` alongside the existing `activeModel`/`activeLLM` runtime fields. Master SHALL populate this field at task ingress (`internal/master/session_loop.go:712 processTask` entry, BEFORE `processTaskDirectExec`) when a continuation match or successful Planner run produces a Context. `runReActLoop` SHALL read `session.specCtx.Load()` lock-free and SHALL NOT take an additional positional parameter. This preserves the existing 9-parameter signature.

The atomic-pointer approach replaces the previously proposed `getSpecCtx()/setSpecCtx()` mutex-protected accessors. The mutex approach was rejected because `runReActLoop` already holds `session.mu.Lock()` at `react_processor.go:191-193` to set `activeModel`; a re-entrant lock acquisition on the non-reentrant `sync.RWMutex` would deadlock.

The Context referenced by `specCtx` MUST be IMMUTABLE after publishing. Updates MUST allocate a new Context and `Store()` it; readers obtain a stable snapshot via `Load()`.

#### Scenario: specCtx accessible without signature change
- **GIVEN** `SessionState` with `specCtx.Load()` returning a non-nil `*Context{ChangeID: "xyz"}`
- **WHEN** `m.runReActLoop(ctx, session, ...)` runs with the existing 9-param signature
- **THEN** the function body MUST read `session.specCtx.Load()` lock-free
- **AND** MUST route through the spec-aware branch when the loaded value is non-nil
- **AND** MUST fall through to legacy behavior when the loaded value is nil

#### Scenario: Signature MUST NOT change
- **WHEN** this change is merged
- **THEN** `grep "func.*runReActLoop" internal/master/react_processor.go` MUST show the same 9 parameters as before
- **AND** no downstream caller (including `im-streaming-reply`) MUST require editing

#### Scenario: No re-entrant lock deadlock
- **WHEN** `runReActLoop` reads `specCtx` while already holding `session.mu`
- **THEN** the read MUST be `Load()` only (no mutex acquisition)
- **AND** static analysis (`go vet`, optionally `staticcheck`) MUST pass with no lock-discipline warnings on `react_processor.go`

#### Scenario: Mutations occur only at task ingress
- **GIVEN** any goroutine other than the per-session worker entering `processTask`
- **WHEN** that goroutine attempts `session.specCtx.Store(...)`
- **THEN** a runtime guard (race-detected channel send to a single owner, or panic in test build) MUST signal a violation
- **AND** the writer MUST be in the call-chain of `session_loop.go:712 processTask`

### Requirement: Continuation resolution runs before complexity classification

The system SHALL evaluate `specdriven.Continuation.Resolve(userID, hint, sessionSpecState)` BEFORE `ClassifyComplexity(intent)` in the internal task path at `internal/master/session_loop.go:757`, NOT in `internal/master/public_api.go`. Continuation SHALL be DEFAULT OFF, gated by `config.json > spec_driven.continuation.default = "off"`.

When enabled, the resolver SHALL be FAIL-CLOSED:
- if the user's text explicitly mentions a change ID or change name → resume that change
- if the only signal is `FocusMRU[0]` and the user text contains no change identifier → emit a `spec_continuation_ambiguous` event and ASK the user (do NOT auto-resume)
- if no signal → behave as a new intent

Background and subagent code paths MUST NOT mutate `SessionSpecState.ActiveChangeID` or `SessionSpecState.FocusMRU`. Only code reachable from `session_loop.go:712 processTask` (user ingress) MAY mutate foreground focus. Continuation auto-detected resumption SHALL NOT auto-mutate task state without user confirmation.

#### Scenario: Continuation default off
- **GIVEN** a fresh installation with default config
- **WHEN** any user message arrives
- **THEN** `Continuation.Resolve` MUST NOT be called
- **AND** the system MUST behave as if continuation logic does not exist

#### Scenario: Explicit change mention auto-resumes
- **GIVEN** continuation enabled, `SessionSpecState.Changes` contains `add-spec-driven-cognition` and `harden-spec-driven-phase2`
- **WHEN** the user sends "继续 add-spec-driven-cognition 的任务 1.10"
- **THEN** `Resolve` MUST return a `Decision{Resume: true, ChangeID: "add-spec-driven-cognition"}`
- **AND** the resumption MUST proceed without asking

#### Scenario: MRU-only signal asks instead of auto-resuming
- **GIVEN** continuation enabled, `FocusMRU = ["add-spec-driven-cognition"]`, last touched 6 hours ago
- **WHEN** the user sends "把 tasks.md 里的 1.10 打勾" (no explicit change name)
- **THEN** `Resolve` MUST return a `Decision{Ask: true, AskReason: "ambiguous_mru_only"}`
- **AND** the system MUST broadcast a `spec_continuation_ambiguous` event with candidate change IDs
- **AND** MUST NOT auto-mutate any change state until user confirms

#### Scenario: Subagent cannot rotate foreground focus
- **GIVEN** an active subagent task running under session S
- **WHEN** the subagent code path executes
- **THEN** any attempt to `Store()` to `SessionSpecState.ActiveChangeID` MUST be rejected
- **AND** the subagent MUST be able to read but NOT write foreground focus

#### Scenario: Pre-write resolved-vs-active divergence detection
- **GIVEN** a continuation resolution returned change `Y`
- **AND** `SessionSpecState.ActiveChangeID = "X"`
- **WHEN** the resolver confidence is not "explicit"
- **THEN** the resolver MUST refuse and emit `Decision{Ask: true, AskReason: "active_focus_mismatch"}`
- **AND** no write to change `Y` MUST occur

### Requirement: SpecPlanner component with cost controls

The system SHALL provide `internal/specdriven/planner/Planner.Plan(ctx, intent, sessionState) (*Plan, error)`. The planner SHALL:
- route via `airouter.TaskPlanning` (introduced in `internal/airouter/types.go` by this change) and map to a haiku-tier model by default
- enforce a HARD output schema gate before any returned plan reaches caller code
- use `json.Decoder.DisallowUnknownFields` and decode into a strict typed struct (see "Planner schema gate")
- retry exactly once on schema validation failure with a schema-reinforced prompt
- time out at 5 seconds with fallback
- reject any plan whose total output tokens exceed `spec_driven.planner.token_budget` (DEFAULT 800 tokens, REDUCED from previously planned 2000)

The schema gate decoder SHALL live at `internal/specdriven/planner/decode.go` and SHALL be called from the internal task path at `internal/master/session_loop.go:757`, BEFORE any tool selection or `processTaskDirectExec` invocation. Schema validation failure on the second attempt SHALL fall back to direct execution (legacy ReAct, `specCtx = nil`) with no tool call constructed from the malformed plan.

The `Plan` struct SHALL use `task_key string` (e.g., `"1.10"`), NEVER numeric `task_id`, to prevent `1.10`/`1.1` collapse during JSON deserialization.

#### Scenario: TaskPlanning constant added to airouter
- **WHEN** this change is merged
- **THEN** `internal/airouter/types.go` MUST contain `TaskPlanning LLMTaskType = "planning"`
- **AND** `internal/airouter/selector.go` MUST route `TaskPlanning` to a model whose per-token cost is ≤ the cost of `TaskSummary`
- **AND** absent explicit mapping, fallback MUST be `TaskSummary`'s model (NEVER `TaskChat`)

#### Scenario: Strict decode rejects unknown fields
- **GIVEN** a planner output containing a field not in the `PlanStep` struct
- **WHEN** `decode.Decode` runs
- **THEN** it MUST return an error mentioning the unknown field
- **AND** the planner MUST trigger its single retry

#### Scenario: task_key string format enforced
- **GIVEN** a planner output containing `"task_key": 1.10` (numeric)
- **WHEN** `decode.Decode` runs
- **THEN** it MUST return a type error
- **AND** retry MUST occur with a schema-reinforced prompt that explicitly shows `"task_key": "1.10"` (string)

#### Scenario: Second schema failure falls back to direct execution
- **GIVEN** the planner already retried once on schema failure
- **WHEN** the second response is also malformed
- **THEN** the planner MUST return `ErrPlannerSchemaInvalid`
- **AND** Master MUST proceed with `processTaskDirectExec` setting `specCtx = nil`
- **AND** NO tool call MUST be constructed from the malformed plan
- **AND** `specdriven.plan_fallback_total{reason="schema"}` MUST increment

#### Scenario: Token budget tightened to 800
- **WHEN** a planner invocation produces > 800 output tokens
- **THEN** the planner MUST abort with `ErrPlannerOverBudget`
- **AND** Master MUST degrade to legacy ReAct
- **AND** metric `specdriven.plan_overbudget_total` MUST increment

### Requirement: Change store persistence with retention

The system SHALL provide `specdriven.Store` whose canonical backend is the database (via `internal/store.SpecChangeStore`, defined by the `spec-state-store` capability). The previously proposed filesystem-canonical layout under `internal/storage/specs/` SHALL NOT be the source of truth.

If a future change introduces a filesystem materialization view, that view SHALL satisfy the "Hybrid storage invariant" requirement defined in the `spec-state-store` capability (export carries `exported_revision`; readers fail closed on mismatch). The current change SHALL NOT implement the filesystem export layer.

`Store.Get/Put/Archive` SHALL operate against `SpecChangeStore.UpsertWithCAS` and SHALL NOT touch the filesystem.

#### Scenario: All change writes go through CAS store
- **WHEN** any spec layer code persists a change update
- **THEN** the write path MUST call `SpecChangeStore.UpsertWithCAS` with the loaded revision
- **AND** MUST NOT write to `internal/storage/specs/` or any other filesystem location

#### Scenario: Concurrent updates rejected with conflict
- **GIVEN** two sessions for the same user both updating change `add-spec-driven-cognition`
- **WHEN** both call `Store.Put` within 200ms
- **THEN** the second writer MUST receive `ErrSpecChangeConflict`
- **AND** the second writer MUST reload before retrying
- **AND** task state MUST NOT be silently overwritten

#### Scenario: Disk full of FS export does not affect canonical state
- **GIVEN** an optional FS export layer is enabled
- **AND** the underlying disk is full
- **WHEN** a CAS store write succeeds and the FS export then fails
- **THEN** the operation MUST report success to the caller
- **AND** `specdriven.export_failed_total` MUST increment
- **AND** subsequent reads MUST trust the store, NOT the stale FS file

### Requirement: Session compaction preserves change references

The system SHALL ensure that `internal/master/session_compact.go` preserves the entire `SessionSpecState` (NOT a single `ChangeID` scalar) on its preservation whitelist. Canonical change progress (`current_task_key`, `revision`) SHALL live in the `SpecChangeStore`, NOT in session transcripts. Recovery from compacted sessions SHALL load progress from the canonical store, never derive it from summaries.

The previously proposed `ChangeID string` scalar field on `SessionState` SHALL be REPLACED by the `SessionSpecState` struct.

#### Scenario: Compaction keeps full SessionSpecState
- **GIVEN** a `SessionState` with 50 messages and `SessionSpecState{ActiveChangeID: "X", FocusMRU: ["X","Y","Z"], Changes: {3 entries}}` approaching context limit
- **WHEN** `session_compact` runs
- **THEN** the compacted state MUST still carry all three `Changes` entries
- **AND** `ActiveChangeID` MUST equal `"X"`
- **AND** `FocusMRU` MUST preserve order

#### Scenario: Recovery loads progress from store, not transcript
- **GIVEN** session A summarized; another session B referencing same change `X`
- **WHEN** session B asks "where did A leave off"
- **THEN** the resolver MUST query `SpecChangeStore.Get("X")` for `CurrentTaskKey` and `Revision`
- **AND** MUST NOT parse session A's summary to derive the task pointer

#### Scenario: Retention removed canonical state fails closed
- **GIVEN** retention has removed the canonical `hive_spec_changes` row for change `X` (after status `done` and 30 days)
- **WHEN** a session attempts to resume change `X`
- **THEN** the system MUST surface `ErrChangeArchived` to the user
- **AND** MUST NOT guess progress from any session summary

