## ADDED Requirements

### Requirement: Eval harness package and case schema

The system SHALL provide a Go package `internal/specdriven/eval/` that defines:

```go
type Case struct {
    Name             string
    UserID           string
    SessionState     SessionSpecState
    Input            string
    WantContinuation WantContinuation
    WantPlan         *Plan
    WantFallback     bool
}

type WantContinuation struct {
    Decision      string // "resume" | "ask" | "new"
    ChangeID      string // expected when Decision == "resume"
    AskReason     string // expected when Decision == "ask"
}

type Runner interface {
    ResolveContinuation(ctx context.Context, c Case) (Decision, error)
    Plan(ctx context.Context, c Case) (*Plan, error)
    ExecuteFallback(ctx context.Context, c Case) error
}
```

Fixture files SHALL live under `internal/specdriven/eval/testdata/*.json`. Each fixture file SHALL deserialize directly into `Case`.

#### Scenario: Required fixture set covers FM-1 to FM-8
- **GIVEN** the testdata directory
- **WHEN** the harness loads fixtures
- **THEN** at least 8 fixture files MUST be present, named `fm01_*.json` through `fm08_*.json`
- **AND** each MUST exercise the failure mode it is named after (wrong-continuation, CAS conflict, FS/DB divergence, planner drift, spec_ref poisoning, lock reentrancy, compaction loss, eval gate)

#### Scenario: Fixture decoding is strict
- **WHEN** a fixture file contains an unknown JSON field
- **THEN** the loader MUST fail the test run with a clear error pointing at the file and field
- **AND** the harness MUST use `json.Decoder.DisallowUnknownFields`

### Requirement: Table-driven test runs all required fixtures

The system SHALL provide `func TestEvalFixtures(t *testing.T)` in `internal/specdriven/eval/runner_test.go` that:
- discovers all `testdata/*.json` fixture files
- runs each as a `t.Run(fixture.Name, ...)` subtest
- fails the parent test if any fixture in the `required` set fails
- emits a summary line with `passed / required / total` counts

#### Scenario: Required fixture failure fails the build
- **GIVEN** fixture `fm01_wrong_continuation.json` in the required set
- **WHEN** the resolver returns `resume` but the fixture expects `ask`
- **THEN** the subtest MUST fail with a diff between expected and actual decision
- **AND** the parent test MUST report nonzero exit
- **AND** `make test-specdriven` MUST return nonzero exit code

#### Scenario: Optional fixture failure does not block CI
- **GIVEN** a fixture marked with `"required": false` in its JSON
- **WHEN** the subtest fails
- **THEN** the parent test MUST log a warning but MUST NOT fail
- **AND** the failure MUST be reported in the summary line under an `optional_failed` count

### Requirement: Coverage and CI gate for dual-flag rollout

The system SHALL add a `Makefile` target `test-specdriven` that:
- runs `go test ./internal/specdriven/... ./internal/master/... -race -coverprofile=coverage.out`
- runs the eval harness via `go test ./internal/specdriven/eval/...`
- enforces minimum line coverage of 75% on `internal/specdriven/...` packages
- exits nonzero if either condition fails

CI workflow configuration SHALL gate the `dual` feature flag promotion on a green `test-specdriven` run.

#### Scenario: Coverage below threshold blocks rollout
- **WHEN** `internal/specdriven/...` line coverage is < 75%
- **THEN** `make test-specdriven` MUST exit nonzero
- **AND** the CI step that promotes `spec_driven.mode` from `legacy` to `dual` MUST NOT run

#### Scenario: Required fixture failure blocks rollout
- **WHEN** any required eval fixture fails
- **THEN** `make test-specdriven` MUST exit nonzero
- **AND** CI MUST refuse to promote the feature flag
- **AND** an audit log entry MUST record the failing fixture names

### Requirement: Regression fixture growth policy

For every production incident attributable to spec-driven code paths, the team SHALL add a new regression fixture under `internal/specdriven/eval/testdata/regression_<incident-id>.json` and mark it `required: true` before closing the incident.

#### Scenario: Post-incident fixture is required
- **GIVEN** an incident report tagged with a spec-driven root cause
- **WHEN** the incident close-out PR is merged
- **THEN** the PR MUST add a new fixture file matching `regression_*.json`
- **AND** the fixture MUST have `"required": true` in its JSON
- **AND** the fixture MUST reproduce the original failure (verified by running the harness against the pre-fix commit)
