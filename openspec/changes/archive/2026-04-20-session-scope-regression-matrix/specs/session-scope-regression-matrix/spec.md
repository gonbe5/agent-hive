## ADDED Requirements

### Requirement: Adversarial regression tests for session scope leaks

The system SHALL include (a) a set of **envelope-invariant positive tests** under `tests/regression/red_*.go` that verify session-scoped production paths always produce `BroadcastMessage` with non-empty SessionID (i.e., route through `BroadcastSessionMessage`, not raw `Broadcast`), AND (b) a **CI-layer grep-based enforcement script** `scripts/ci/check_session_scope.sh` that scans `internal/master/*.go` source for disallowed call patterns. The script MUST be runnable locally and as a CI step; its exit code MUST be non-zero when any violation is found. This two-layer design (envelope-invariant test + source-time grep) replaces the originally proposed "lint rule / runtime panic / static analyzer" protective layer, because the repository has no pre-existing analyzer infrastructure (`.golangci.yml`, `go/analysis.Analyzer`, `tools/analyzer/` are all absent) and introducing one is out of scope per the proposal's Out-of-Scope declaration.

**Architectural fact (discovered 2026-04-20 via Phase 0 Spike A)**: the WS subscriber filter at `internal/streaming/websocket.go:358-367` drops ONLY when `broadcastMsg.SessionID != ""` AND does not match `userSessionID`. Empty-SessionID messages pass through unfiltered — this is **intentional** because lifecycle/metadata events (`EventTypeAgentCreated` / `EventTypeAgentDestroyed` / `EventTypeToolListChanged`) rely on empty-SessionID broadcast to reach all connections. Therefore runtime "subscriber MUST drop empty-SessionID leak" is **not** a viable regression assertion; the real protection chain is: (1) emit-side — session-scoped paths route through `BroadcastSessionMessage` which injects SessionID; (2) source-time — grep script rejects new raw `eventBus.Broadcast(BroadcastMessage{...})` call sites in `internal/master/`; (3) WS filter — drops when SessionID is non-empty but doesn't match.

The purpose is to prevent silent regression of `subagent-session-scoping` fixes when future PRs refactor the broadcast layer.

#### Scenario: R-1 envelope invariant — session-scoped emit path produces non-empty SessionID
- **GIVEN** test fixture `tests/regression/red_subagent_progress_raw_broadcast_test.go` drives `Master.CreateAgentProgressCallback()` with a `subagent.ProgressEvent{SessionID: "sX", ...}`
- **WHEN** the callback emits via the production path
- **THEN** the resulting `BroadcastMessage` observed on the `eventBus` subscriber channel MUST have `SessionID == "sX"` (proving the emit path routed through `BroadcastSessionMessage`, not raw `Broadcast`)
- **AND** if any future PR rewires this path to raw `Broadcast(BroadcastMessage{...})`, the `BroadcastMessage.SessionID` observed at subscriber MUST become `""` and the test MUST fail, catching the regression
- **NOTE**: the subscriber-side WS filter at `internal/streaming/websocket.go:358-367` does not drop empty-SessionID messages (empty = intentional global broadcast for lifecycle events); this test therefore asserts envelope integrity at the EventBus layer, not WS drop behavior. The IM EventRenderer filter path (`internal/channel/feishu/renderer.go`) is covered by `im-streaming-reply` main spec 12.4 and MUST NOT be duplicated here.

#### Scenario: R-1b grep-based CI guard detects the pattern
- **GIVEN** the shell script `scripts/ci/check_session_scope.sh`
- **WHEN** the script is invoked against the current repo state
- **THEN** it MUST exit 0 on the clean main branch (post `subagent-session-scoping` archive)
- **AND** if any new `internal/master/*.go` line matches `eventBus\.Broadcast\(BroadcastMessage` without a preceding `// no session scope by design` comment, the script MUST exit non-zero with line-number-precise output
- **AND** the script MUST be wired as a required CI check in `.github/workflows/e2e-session-scope.yml`

#### Scenario: R-2 BroadcastGenericMessage misuse — envelope invariant + grep flag
- **GIVEN** test fixture `tests/regression/red_subagent_stream_generic_test.go` drives `Master.CreateAgentStreamCallback()` with `sessionID = "sX"` and a streamed payload
- **WHEN** the callback emits via the production path
- **THEN** the resulting `BroadcastMessage` observed on the subscriber channel MUST have `SessionID == "sX"` AND `payload["session_id"] == "sX"` (proving the stream path did NOT use `BroadcastGenericMessage`, which would produce empty SessionID)
- **AND** `scripts/ci/check_session_scope.sh` MUST additionally flag any new call site of `BroadcastGenericMessage(EventType(AgentProgress|ToolCall|SkillInstallProgress)...)` in `internal/master/` — **only** these progress-family event types are session-scoped by spec 12.4 and MUST NOT use the generic path. Lifecycle / metadata events (`EventTypeAgentCreated`, `EventTypeAgentDestroyed`, `EventTypeToolListChanged`) are legitimately global broadcasts and MUST retain `// no session scope by design` justification comments at each call site (current clean baseline: `master.go:518`, `master.go:550`, `lifecycle.go:157`)

#### Scenario: R-3 Lifecycle event without justification comment
- **GIVEN** `scripts/ci/check_session_scope.sh` scans `internal/master/*.go` for raw `eventBus.Broadcast(BroadcastMessage{...})` calls
- **WHEN** any such call lacks a preceding code comment matching `// no session scope by design`
- **THEN** the script MUST exit non-zero with `file:line` precise output
- **AND** the matching test `tests/regression/red_lifecycle_unjustified_test.go` MUST parse the script output and assert the expected violation set

### Requirement: Cross-session penetration matrix

The system SHALL include `tests/regression/session_scope_matrix_test.go` which exercises an N×M matrix where N = number of concurrent sessions (each with distinct `SessionID`; UserID is only a sample-construction attribute, not a security boundary) and M = number of session-scoped event types. For every (sessionA_emit, sessionB_subscribe) pair where `A.SessionID != B.SessionID`, the matrix MUST verify zero penetration.

**Naming note**: this requirement was originally drafted as "Cross-tenant penetration matrix". Renamed 2026-04-20 because the actual security boundary enforced by `internal/streaming/websocket.go:346-355` is `SessionID`, not `tenant_key`. Feishu `tenant_key` multi-tenancy is Out-of-Scope per proposal declaration — `im-streaming-reply` main spec has not yet modeled a `tenant_key` field (`grep -r tenant internal/master/` currently returns 0 hits).

#### Scenario: Zero-penetration property
- **GIVEN** sessions `A` (SessionID=sA, UserID=u1) and `B` (SessionID=sB, UserID=u2) where `sA != sB`
- **WHEN** session `A` emits session-scoped event types: `message`, `tool_call`, `agent_progress`, `skill_install_progress` (4 progress-family types enumerated by R-2)
- **THEN** session `B`'s WebSocket subscription MUST NOT observe the event — verified by the WS filter at `internal/streaming/websocket.go:358-367` dropping when `SessionID != "" && SessionID != userSessionID`
- **AND** the WS subscriber's debug log MUST show `broadcast_session=sA, conn_session=sB`
- **NOTE 1**: lifecycle/metadata events (`agent_created`, `agent_destroyed`, `tool_list_changed`) are intentionally excluded from this matrix — they are global broadcasts with empty SessionID and MUST reach all sessions; testing their cross-session visibility would violate their design contract.
- **NOTE 2**: IM EventRenderer path isolation is out of scope for this matrix (covered by `im-streaming-reply` main spec 12.4); verification here is WS-subscriber level.

#### Scenario: Same-user cross-session isolation (UserID is not the boundary)
- **GIVEN** sessions `A` (SessionID=sA, UserID=u1) and `C` (SessionID=sC, UserID=u1) — **same UserID, different SessionID**
- **WHEN** session `A` emits any session-scoped event type
- **THEN** session `C`'s subscribers MUST still drop it (sharing UserID MUST NOT weaken the boundary)
- **AND** this scenario MUST be a distinct matrix cell (not skipped by deduplication against the different-user cell)

#### Scenario: Matrix completeness
- **WHEN** the matrix test runs
- **THEN** all N×M×(N-1) cells MUST be exercised (no skipped cells); N MUST be ≥ 3 and include at least one same-UserID pair to exercise the previous scenario
- **AND** any single-cell red MUST block PR merge

### Requirement: WebSocket reconnect race automation (backend scope)

The system SHALL include `tests/regression/ws_reconnect_race_test.go` (Go) which automates the **backend** portion of the runbook 11.8 path: server-side WS handshake routing → `sessionID` propagation → session-filter drop behavior under disconnect/reconnect storm. Three-way race scenarios (reconnect mid-emit, reconnect mid-loadMessages callback delivery, reconnect mid-handleDisconnected) MUST all be covered **as observable at the backend WS envelope layer**.

**Scope split note**: the original `useChatStore` / `setCurrentSessionId(null)` frontend store spy scenarios are delegated to `frontend-ws-handshake-regression` Phase 2 (playwright-driven, runs inside the harness this change provides). This change's Phase 3 delivers Go-side backend race coverage only; cross-stack contract is verified end-to-end by the shared CI harness (this change's Req 4) where both Go tests and FE playwright cases execute.

#### Scenario: Backend WS envelope SessionID preserved across reconnect
- **GIVEN** a backend session with `SessionID = "sX"` and an active fake WS client
- **WHEN** the test triggers `disconnect → reconnect` cycle 3 times rapid-fire on the same session
- **THEN** every `BroadcastMessage` routed to the reconnected client MUST carry `SessionID = "sX"`
- **AND** the subscriber-side filter MUST NOT drop any message due to envelope SessionID going empty mid-reconnect

#### Scenario: Second-message first-chunk delivery at backend
- **GIVEN** completed backend disconnect → reconnect cycle for session `sX`
- **WHEN** a second inbound message triggers LLM streaming (fake LLM emits first chunk within 500ms)
- **THEN** the first chunk MUST reach the fake WS client's recv queue within 5 seconds of inbound receipt
- **AND** no `WebSocket session-mismatch drop` log line MUST appear in the test-captured log stream

#### Scenario: Frontend store spy delegation (cross-reference)
- **GIVEN** the frontend-facing assertion that `useChatStore.currentSessionId` never transitions to `null` during reconnect and that `setCurrentSessionId(null)` is never called
- **WHEN** that assertion is evaluated
- **THEN** it MUST be covered by `frontend-ws-handshake-regression` Phase 2 playwright spec, executed inside the shared `.github/workflows/e2e-session-scope.yml` CI harness provisioned by Req 4 of this change
- **AND** this change MUST NOT duplicate that assertion in Go-side fakes (doing so produces a cross-language承载不匹配 and regresses on the real store semantics)

### Requirement: longconn + browser e2e CI harness

The system SHALL provision a CI harness capable of running longconn (Feishu mock endpoint) + headless browser (playwright) e2e jobs. This harness is the precondition for `frontend-ws-handshake-regression` Phase 2 case landing and for retiring `docs/runbooks/im-streaming-reply-live-smoke.md` from upload-blocking gate to "CI failure recovery reference" status.

#### Scenario: CI workflow boots full stack (Go side, provisioned by this change)
- **GIVEN** a PR triggers the new `.github/workflows/e2e-session-scope.yml` workflow
- **WHEN** the workflow runs
- **THEN** it MUST start `go run ./cmd/server/main.go --config config.test.json`
- **AND** start a mock Feishu longconn endpoint (stub HTTP server or binary)
- **AND** start a headless playwright browser (runtime available, used by delegated FE specs)
- **AND** execute `session_scope_matrix_test.go` + `red_*.go` fixtures + `ws_reconnect_race_test.go` + `scripts/ci/check_session_scope.sh`
- **AND** its `timeout-minutes` MUST be set from `docs/runbooks/ci-baseline.md` (baseline p95 × 1.5, rounded up to nearest minute), not from a spec-hardcoded number
- **AND** the first 7 green CI runs on `main` MUST be logged to `docs/runbooks/ci-baseline.md` to establish the p95 baseline before the harness is marked authoritative

#### Scenario: Frontend playwright specs plug into harness (provisioned here, authored elsewhere)
- **GIVEN** the harness provides a playwright runtime + browser binary + a job step that runs `npx playwright test` against `frontend/playwright/**/*.spec.ts`
- **WHEN** this change first ships the harness
- **THEN** zero playwright spec files are required to exist — the job step MUST pass with "no tests found" (not fail)
- **AND** once `frontend-ws-handshake-regression` Phase 2 lands its `.spec.ts` files into `frontend/playwright/`, the next harness run MUST pick them up automatically without workflow YAML changes
- **AND** the workflow MUST NOT hard-reference any specific FE spec filename (decouples spec delivery from harness delivery)

#### Scenario: Runbook downgrade signal
- **WHEN** the harness has been green for 7 consecutive days on `main`
- **THEN** `docs/runbooks/im-streaming-reply-live-smoke.md` SHOULD be amended with a header note: "本 runbook 自 <date> 起降级为 CI 故障兜底参考——常规 PR 上线流程已由 `.github/workflows/e2e-session-scope.yml` 自动覆盖"
- **AND** the PR template MUST stop requiring on-call signature for routine merges to `internal/channel/feishu/renderer.go` or `internal/master/react_processor.go`
