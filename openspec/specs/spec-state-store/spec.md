# spec-state-store Specification

## Purpose
TBD - created by archiving change harden-spec-driven-phase2. Update Purpose after archive.
## Requirements
### Requirement: SpecChangeStore with revisioned CAS

The system SHALL extend `internal/store` with a new file `spec_store.go` that defines:

```go
type SpecChangeRecord struct {
    ID, Status, Title string
    CurrentTaskKey    string    // task_key string, never numeric
    Revision          int
    UpdatedBy         string
    UpdatedAt         time.Time
    ParentID          string    // optional, for derived changes
}

type SpecChangeEvent struct {
    ChangeID    string
    Sequence    int64
    EventType   string // "task_complete" | "task_fail" | "status_change" | "rollback"
    PrevTaskKey string // for inverse rollback
    NewTaskKey  string
    PrevStatus  string
    NewStatus   string
    ActorID     string
    Timestamp   time.Time
}

type SpecChangeStore interface {
    Get(ctx context.Context, id string) (*SpecChangeRecord, bool, error)
    UpsertWithCAS(ctx context.Context, rec SpecChangeRecord, expectedRevision int) error
    AppendEvent(ctx context.Context, event SpecChangeEvent) error
    ListByUser(ctx context.Context, userID string, limit int) ([]SpecChangeRecord, error)
}

var ErrSpecChangeConflict = errors.New("spec change revision mismatch")
```

The store SHALL be backed by two PostgreSQL tables `hive_spec_changes` (current state) and `hive_spec_change_events` (append-only audit). The CAS implementation SHALL follow the pattern at `internal/store/skill_store.go:62`.

#### Scenario: Successful CAS update increments revision
- **GIVEN** a change record with revision 5
- **WHEN** `UpsertWithCAS` is called with `expectedRevision: 5` and a new status
- **THEN** the DB row MUST be updated with revision 6
- **AND** `rows_affected` MUST equal 1
- **AND** the call MUST return nil

#### Scenario: Stale CAS write returns conflict
- **GIVEN** two callers both holding revision 5 of the same change
- **WHEN** caller A successfully writes (revision becomes 6)
- **AND** caller B then attempts `UpsertWithCAS` with `expectedRevision: 5`
- **THEN** caller B MUST receive `ErrSpecChangeConflict`
- **AND** caller B MUST reload the record before retrying
- **AND** no merge of caller B's intent MUST happen automatically

#### Scenario: Every CAS write appends an event
- **WHEN** `UpsertWithCAS` succeeds
- **THEN** `AppendEvent` MUST be called within the same transaction
- **AND** the event MUST capture both `PrevTaskKey/PrevStatus` and `NewTaskKey/NewStatus`
- **AND** the event sequence MUST be strictly monotonic per `ChangeID`

### Requirement: SessionSpecState persistence

The system SHALL provide a Go struct on `SessionState`:

```go
type SessionSpecState struct {
    ActiveChangeID string                `json:"active_change_id,omitempty"`
    FocusMRU       []string              `json:"focus_mru,omitempty"` // newest first, max length 16
    Changes        map[string]ChangeRef  `json:"changes,omitempty"`
}

type ChangeRef struct {
    ID, Status, Title, ParentID string
    LastTaskKey string
    LastTouched time.Time
}
```

`SessionSpecState` SHALL be persisted to a NEW table `hive_spec_session_state` keyed by `session_id`, NOT mixed into the existing `hive_session_metadata` JSON blob. Writes SHALL happen ONLY at task ingress (`internal/master/session_loop.go:712 processTask` entry, before `processTaskDirectExec`), with no outer `session.mu` held.

#### Scenario: Multi-change session preserved across restarts
- **GIVEN** a session with `ActiveChangeID = "X"` and `Changes` map containing X, Y, Z
- **WHEN** the process restarts and the session is rehydrated
- **THEN** `ActiveChangeID` MUST equal `"X"`
- **AND** `len(Changes)` MUST equal 3
- **AND** `FocusMRU` MUST preserve original order

#### Scenario: FocusMRU bounded to 16 entries
- **WHEN** the 17th change is added to a session
- **THEN** the oldest entry MUST be evicted from `FocusMRU`
- **AND** the entry's `ChangeRef` MUST remain in `Changes` (only MRU is bounded)

#### Scenario: Background work cannot mutate ActiveChangeID
- **GIVEN** an in-flight subagent task
- **WHEN** the subagent code path attempts to write `SessionSpecState.ActiveChangeID`
- **THEN** the write MUST be rejected by a runtime guard (panic in test, error in prod)
- **AND** `FocusMRU` MUST NOT be mutated either
- **AND** only code reachable from `session_loop.go:712 processTask` (user ingress) MUST be allowed to mutate

### Requirement: Hybrid storage invariant for optional FS export

If a future change adds a filesystem materialization layer, that layer SHALL embed `exported_revision` in every emitted artifact. Readers SHALL trust the FS view ONLY when `exported_revision == store_revision`. Otherwise readers SHALL fall back to the store or fail closed.

#### Scenario: Stale FS export rejected by reader
- **GIVEN** an exported `tasks.md` with `exported_revision: 5`
- **AND** the canonical store record at revision 7
- **WHEN** a subagent reads the FS file
- **THEN** the reader MUST detect the mismatch and either reload from store or return an error
- **AND** the reader MUST NOT silently trust the stale content

#### Scenario: Export failure does not block DB commit
- **GIVEN** a CAS write succeeds in the store
- **WHEN** the async FS export fails (disk full, permission denied)
- **THEN** the store revision MUST remain authoritative
- **AND** the export error MUST surface as a metric `specdriven.export_failed_total`
- **AND** the user-visible operation MUST still report success

### Requirement: Retention safe for in-progress changes

The retention sweeper SHALL NOT delete `hive_spec_changes` rows whose `Status` is `planning`, `active`, or `blocked`. Only `done` or `archived` rows older than 30 days SHALL be eligible for compaction.

#### Scenario: Active change survives retention sweep
- **GIVEN** a change in status `active`, not touched for 60 days
- **WHEN** the retention sweeper runs
- **THEN** the row MUST NOT be deleted or compacted
- **AND** `LastTouched` MUST be unaffected

#### Scenario: Done change compacted to event log only
- **GIVEN** a change in status `done`, last touched 31 days ago
- **WHEN** the retention sweeper runs
- **THEN** the current-state row MAY be moved to a compacted snapshot table
- **AND** the event log MUST retain at least the terminal state event for ≥ 1 year

