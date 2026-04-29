## ADDED Requirements

### Requirement: Subscriber-side session filter (extended for subagent + lifecycle)

The session-scope event filter (originally specified in `im-streaming-reply` Sprint 12.4) SHALL be extended to cover three additional event sources that were previously left to implicit defaults: (1) subagent stream chunks via `StreamCallback`, (2) subagent progress events via `ProgressCallback`, and (3) agent lifecycle events (`EventTypeAgentCreated`, `EventTypeAgentDestroyed`, `EventTypeToolListChanged`). Each event MUST explicitly declare a session-scoped or broadcast-scoped routing decision. Implicit defaults are forbidden.

#### Scenario: Subagent StreamCallback carries sessionID
- **WHEN** a subagent emits a stream chunk via `StreamCallback`
- **THEN** the new callback signature `func(agentID, sessionID, content, reasoning string)` MUST be used (sessionID positioned as 2nd parameter)
- **AND** `master.CreateAgentStreamCallback` MUST invoke `m.eventBus.BroadcastSessionMessage(sessionID, EventTypeAgentProgress, payload)` (NOT `BroadcastGenericMessage`)
- **AND** the resulting `BroadcastMessage.SessionID` envelope field MUST equal the sessionID from the callback

#### Scenario: Subagent ProgressCallback carries sessionID
- **WHEN** a subagent emits a progress event via `ProgressCallback`
- **THEN** the `ProgressEvent` struct MUST contain a non-empty `SessionID string` field
- **AND** `master.CreateAgentProgressCallback` MUST invoke `m.eventBus.BroadcastSessionMessage(event.SessionID, EventTypeAgentProgress, ...)` (NOT raw `Broadcast(BroadcastMessage{...})`)
- **AND** the resulting envelope `SessionID` MUST equal `event.SessionID`

#### Scenario: Lifecycle events explicit routing
- **WHEN** `master.go` emits `EventTypeAgentCreated`, `EventTypeAgentDestroyed`, or `EventTypeToolListChanged`
- **THEN** the call site MUST use either `BroadcastSessionMessage(sessionID, ...)` (if event semantically belongs to a session) or `BroadcastGenericMessage(...)` with a code comment `// no session scope by design — <reason>` (if event is global)
- **AND** raw `eventBus.Broadcast(BroadcastMessage{...})` calls without sessionID and without justification comment MUST NOT exist in `internal/master/`

#### Scenario: Cross-tenant leak protection
- **GIVEN** two concurrent sessions A (UserID=u1) and B (UserID=u2) each running a subagent
- **WHEN** session A's subagent emits a stream chunk
- **THEN** session B's WebSocket subscription MUST NOT receive the chunk
- **AND** session B's IM EventRenderer MUST drop the message at the session-mismatch filter (logged at debug level with `broadcast_session=A.ID`, `conn_session=B.ID`, `event=agent_progress`)

### Requirement: BREAKING subagent callback signatures

The internal subagent package SHALL break its callback contracts to enforce sessionID propagation. This is an internal-interface BREAKING change (no external API consumers).

#### Scenario: StreamCallback signature change
- **WHEN** any caller imports `internal/subagent.StreamCallback`
- **THEN** the type MUST be `func(agentID, sessionID, content, reasoning string)` (4 parameters)
- **AND** the previous 3-parameter form `func(agentID, content, reasoning string)` MUST NOT compile

#### Scenario: ProgressEvent struct extended
- **WHEN** any caller constructs or consumes `internal/subagent.ProgressEvent`
- **THEN** the struct MUST contain a `SessionID string` field
- **AND** the field MUST be populated at every emission site (verified by go vet custom check or static grep)

#### Scenario: Registration sites updated
- **WHEN** the codebase is grepped for `CreateAgentStreamCallback|CreateAgentProgressCallback`
- **THEN** all 3 registration points (master / cli / bootstrap) MUST use the new signatures
- **AND** all 5 invocation points MUST pass a non-empty sessionID
