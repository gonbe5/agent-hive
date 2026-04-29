# hitl-choice-type-registry Specification

## Purpose
TBD - created by archiving change hitl-choice-type-registry. Update Purpose after archive.
## Requirements
### Requirement: ChoiceType field orthogonal to InputRequestType

The `internal/master.InputRequest` struct SHALL introduce a new field `ChoiceType string` with JSON tag `choice_type,omitempty`, positioned orthogonally to the existing `Type InputRequestType` enum. The new field SHALL encode the business-decision sub-semantic of an `input_request` event, while `Type` continues to encode the protocol interaction shape (approval / clarification / confirmation / choice / permission). Introducing the field MUST NOT modify, rename, reorder, or drop any existing field of `InputRequest`, and MUST NOT modify the 5 existing values of `InputRequestType`.

#### Scenario: Field coexistence
- **WHEN** a caller constructs `InputRequest{Type: InputChoice, ChoiceType: "account_selector", ...}`
- **THEN** both fields MUST be preserved end-to-end (struct field → JSON → deserialized struct)
- **AND** the JSON output MUST include both `"type": "choice"` and `"choice_type": "account_selector"`

#### Scenario: Empty ChoiceType omitted
- **WHEN** a caller constructs `InputRequest{Type: InputApproval, ChoiceType: "", ...}`
- **THEN** the JSON output MUST NOT include the `choice_type` key (driven by `omitempty`)
- **AND** consumers that do not read `choice_type` MUST observe byte-identical JSON vs the pre-change schema

#### Scenario: Existing InputRequestType values untouched
- **WHEN** the change is deployed
- **THEN** `InputApproval`, `InputClarification`, `InputConfirmation`, `InputChoice`, `InputPermission` MUST each continue to evaluate to their original string values (`approval`, `clarification`, `confirmation`, `choice`, `permission`)
- **AND** no new `InputRequestType` enum constants MUST be added by this change

### Requirement: ChoiceType registry lifecycle

The system SHALL expose a thread-safe registry in package `internal/master` with the following API, hereafter referred to as the **`choice_type_registry`**:

- `func RegisterChoiceType(spec ChoiceTypeSpec) error`
- `func IsRegisteredChoiceType(name string) bool`
- `func ListChoiceTypes() []ChoiceTypeSpec`
- `func MustRegisterChoiceType(spec ChoiceTypeSpec)` (helper wrapping `RegisterChoiceType` with panic-on-error, for `init()` use)

Where:
```
type ChoiceTypeSpec struct {
    Name        string
    Description string
    PayloadHint map[string]string
}
```

The registry SHALL be initialized at package load time and SHALL persist for the lifetime of the process. Registered entries MUST NOT be removable during process lifetime.

#### Scenario: Register new choice type
- **WHEN** an uninitialized registry receives `RegisterChoiceType(ChoiceTypeSpec{Name: "my_type", Description: "..."})`
- **THEN** the call MUST return nil error
- **AND** `IsRegisteredChoiceType("my_type")` MUST return `true`
- **AND** `ListChoiceTypes()` MUST include an entry with `Name == "my_type"`

#### Scenario: Re-registration of identical spec is idempotent
- **WHEN** `RegisterChoiceType(spec)` is called twice with structurally-equal specs
- **THEN** the second call MUST return nil error
- **AND** the registry MUST contain exactly one entry for that `Name`

#### Scenario: Re-registration of conflicting spec rejected
- **WHEN** `RegisterChoiceType(ChoiceTypeSpec{Name: "my_type", Description: "A"})` succeeds
- **AND** a subsequent `RegisterChoiceType(ChoiceTypeSpec{Name: "my_type", Description: "B"})` is attempted
- **THEN** the second call MUST return a non-nil error that identifies name conflict
- **AND** the registry MUST retain the first-registered entry unchanged

#### Scenario: Invalid name rejected
- **WHEN** `RegisterChoiceType(ChoiceTypeSpec{Name: "MyType"})` is called (camelCase)
- **THEN** the call MUST return a non-nil error citing the naming convention
- **AND** `IsRegisteredChoiceType("MyType")` MUST return `false`

#### Scenario: Concurrent registration safety
- **WHEN** 100 goroutines each call `RegisterChoiceType` with 100 distinct names concurrently
- **THEN** all 10000 entries MUST be registered
- **AND** the race detector (`go test -race`) MUST NOT report a data race
- **AND** concurrent `ListChoiceTypes()` calls MUST each return a consistent snapshot

#### Scenario: No unregister API
- **WHEN** a consumer searches the package exported symbols
- **THEN** no `UnregisterChoiceType`, `DeleteChoiceType`, or equivalent MUST exist
- **AND** the registry MUST only grow monotonically during process lifetime

### Requirement: Built-in choice type registrations

The `internal/master` package SHALL register the following 3 `ChoiceTypeSpec` entries during its `init()` function, preserving semantic continuity with the closed-set whitelist previously declared in `add-spec-driven-cognition/permission-minimalism`:

1. `account_selector` — "User selects which upstream account to use for a multi-account skill"
2. `ambiguity_clarification` — "Master detected intent ambiguity and asks user to disambiguate"
3. `confirmation_before_irreversible_business_action` — "Skill/Tool asks for explicit confirmation before irreversible external side-effects"

This capability SHALL NOT include `skill_install_confirmation` or any other downstream-change-specific value; those SHALL be registered by their owning changes.

#### Scenario: Built-in values available at boot
- **WHEN** the `internal/master` package is loaded in any Go binary
- **THEN** `IsRegisteredChoiceType("account_selector")` MUST return `true`
- **AND** `IsRegisteredChoiceType("ambiguity_clarification")` MUST return `true`
- **AND** `IsRegisteredChoiceType("confirmation_before_irreversible_business_action")` MUST return `true`

#### Scenario: skill_install_confirmation NOT built-in
- **WHEN** this change is deployed in isolation (without `hive-skill-on-demand`)
- **THEN** `IsRegisteredChoiceType("skill_install_confirmation")` MUST return `false`
- **AND** `ListChoiceTypes()` MUST NOT include an entry with that name

### Requirement: InputResponse subscription API

The system SHALL introduce a new pair of APIs to enable per-request response subscription, required by `EmitInputRequest`'s await-loop. Grep verification confirms neither symbol exists prior to this change, therefore both SHALL be added as part of this change's scope.

- `func (eb *EventBus) BroadcastInputResponse(resp *InputResponse)` — symmetric to existing `BroadcastInputRequest`, routes `InputResponse` through the event bus so subscribers can observe it
- `func (m *Master) SubscribeInputResponse(reqID string) <-chan *InputResponse` — returns a buffered channel (capacity 1) that receives the first `InputResponse` whose `RequestID == reqID`, then closes

The `HITLBroker.SubmitInput` method SHALL additionally invoke `hb.eventBus.BroadcastInputResponse(resp)` after delivering the response to its existing `pendingInputChans` map, so the new subscription API observes all responses without changing legacy behavior.

#### Scenario: Subscriber receives matching response
- **WHEN** a caller subscribes via `Master.SubscribeInputResponse("req-42")`
- **AND** `HITLBroker.SubmitInput(&InputResponse{RequestID: "req-42", Action: "approve"})` is called
- **THEN** the subscription channel MUST emit one `*InputResponse` with `RequestID == "req-42"`
- **AND** the channel MUST close after the single emission

#### Scenario: Subscriber ignores non-matching responses
- **WHEN** a caller subscribes via `Master.SubscribeInputResponse("req-99")`
- **AND** `HITLBroker.SubmitInput(&InputResponse{RequestID: "req-42", ...})` is called
- **THEN** the subscription channel MUST NOT emit anything
- **AND** the channel MUST remain open awaiting `req-99`

#### Scenario: Legacy HITLBroker pendingInputChans path unchanged
- **WHEN** this change is deployed
- **THEN** existing consumers of `HITLBroker.pendingInputChans` (e.g., `WaitForInput`) MUST continue receiving responses as before
- **AND** the new broadcast MUST be an additive side-effect that does not replace or drop the existing delivery

#### Scenario: No goroutine leak on ctx drop
- **WHEN** a caller subscribes and then abandons the returned channel without a matching response ever arriving
- **THEN** the subscription infrastructure MUST not retain a reference forever (use a weak registration pattern, a TTL, or a cleanup tied to the matching request's lifecycle)
- **AND** `goleak.VerifyNone` MUST pass in the subscription unit tests

### Requirement: EmitInputRequest closed-loop helper

The `Master` type SHALL expose a new method:

```go
func (m *Master) EmitInputRequest(
    ctx context.Context,
    req InputRequest,
    opts ...EmitInputRequestOptions,
) (*InputResponse, error)
```

The method SHALL encapsulate the full HITL closed loop: construct missing metadata → validate → broadcast → subscribe response → await with timeout/cancel → return. Skill / Tool authors SHALL use this method rather than composing the primitives themselves.

#### Scenario: ChoiceType validation before broadcast
- **WHEN** `EmitInputRequest(ctx, InputRequest{ChoiceType: "unregistered_name"})` is called
- **AND** `IsRegisteredChoiceType("unregistered_name") == false`
- **THEN** the call MUST return a non-nil error identifying the unregistered choice type
- **AND** no broadcast MUST have been emitted (verifiable via a stub `Master.broadcast` recording)

#### Scenario: Empty ChoiceType bypasses registry check
- **WHEN** `EmitInputRequest(ctx, InputRequest{ChoiceType: "", Type: InputApproval, ...})` is called
- **THEN** the registry check MUST be skipped
- **AND** the request MUST be broadcast normally

#### Scenario: Auto-fill ID and CreatedAt
- **WHEN** `EmitInputRequest(ctx, InputRequest{ID: "", CreatedAt: time.Time{}, ...})` is called with otherwise-valid fields
- **THEN** the broadcast payload MUST contain a non-empty `ID`
- **AND** MUST contain a non-zero `CreatedAt` close to the call time
- **AND** the returned `InputResponse` (on success) MUST reference that generated ID via `RequestID`

#### Scenario: Context cancellation
- **WHEN** `EmitInputRequest` is awaiting a response
- **AND** the caller cancels `ctx`
- **THEN** the method MUST return `ctx.Err()` within 100ms
- **AND** MUST NOT leak a goroutine

#### Scenario: Timeout behavior
- **WHEN** `EmitInputRequest` is called with `EmitInputRequestOptions{Timeout: 50 * time.Millisecond}`
- **AND** no `InputResponse` arrives within 50ms
- **THEN** the method MUST return a sentinel error `ErrInputRequestTimeout`
- **AND** MUST NOT leak a goroutine

#### Scenario: Successful response path
- **WHEN** `EmitInputRequest(ctx, req)` broadcasts with generated `ID = "req-123"`
- **AND** a consumer posts `InputResponse{RequestID: "req-123", Action: "approve"}`
- **THEN** the method MUST return `*InputResponse` with `Action == "approve"`
- **AND** MUST return `err == nil`

### Requirement: Broadcast protocol compatibility

`Master.BroadcastInputRequest(req *InputRequest)` SHALL continue to operate without any behavioral change for callers that do not populate `ChoiceType`. The new field SHALL flow through the existing serialization path via `encoding/json`, and the existing broadcast subscribers (Feishu, WeChat, Web UI, test harnesses) MUST NOT require code changes to continue functioning.

#### Scenario: Old consumer byte-compat
- **WHEN** `BroadcastInputRequest(&InputRequest{Type: InputApproval, ChoiceType: "", Prompt: "..."})` is called
- **THEN** the serialized JSON MUST be byte-identical (modulo map ordering within `Data`) to what the same call produced before this change
- **AND** existing snapshot tests for pre-change shape MUST pass unchanged

#### Scenario: New field flows through broadcast
- **WHEN** `BroadcastInputRequest(&InputRequest{ChoiceType: "account_selector", ...})` is called
- **THEN** all WS subscribers (`SubscribeWSBroadcast`) MUST receive a `BroadcastMessage` whose payload contains `"choice_type": "account_selector"`

#### Scenario: Raw broadcast bypasses registry check
- **WHEN** `BroadcastInputRequest(&InputRequest{ChoiceType: "unregistered_value"})` is called (i.e., the raw primitive, not `EmitInputRequest`)
- **THEN** the broadcast MUST proceed without error
- **AND** no registry lookup MUST block the primitive path (the helper layer is responsible for validation, not the primitive)

### Requirement: Proposal-impact protocol stability

The shape of the existing `BroadcastInputRequest` JSON message SHALL remain backwards compatible. Subscribers that ignore unknown JSON fields MUST continue to function with no code change after this change is deployed.

#### Scenario: SSE subscriber compatibility
- **WHEN** a pre-change SSE subscriber connects after this change is deployed
- **AND** the server emits an `InputRequest` with non-empty `ChoiceType`
- **THEN** the subscriber MUST successfully parse the message (the extra key `choice_type` is ignored)
- **AND** no subscriber MUST crash or disconnect due to the schema addition

