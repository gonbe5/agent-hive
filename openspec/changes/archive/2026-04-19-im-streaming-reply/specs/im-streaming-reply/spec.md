## ADDED Requirements

### Requirement: Channel EventRenderer abstraction

The system SHALL define an optional `EventRenderer` interface in `internal/channel` that extends `ChannelPlugin` with a single method `RenderEventStream(ctx, scope, eventCh)` consuming `master.BroadcastMessage` events from the harness EventBus. Plugins implementing `EventRenderer` SHALL render harness events to platform-specific API calls. Plugins NOT implementing `EventRenderer` SHALL keep using the existing one-shot `Send` path with zero behavior change.

#### Scenario: Plugin implements EventRenderer
- **WHEN** the router resolves a plugin and `plugin.(EventRenderer)` succeeds
- **AND** `cfg.<platform>.Renderer.Enabled` is `true`
- **THEN** the router MUST take the renderer path
- **AND** MUST NOT invoke the legacy `Send(TaskResponse.Content)` path

#### Scenario: Plugin does not implement EventRenderer
- **WHEN** the router resolves a plugin and `plugin.(EventRenderer)` fails
- **THEN** the router MUST take the existing one-shot `Send` path
- **AND** behavior MUST be bit-identical to the pre-change path
- **AND** MUST NOT subscribe to the harness EventBus for that message

#### Scenario: Renderer disabled by configuration
- **WHEN** `cfg.<platform>.Renderer.Enabled` is `false`
- **THEN** the router MUST take the legacy `Send` path even if the plugin implements `EventRenderer`

#### Scenario: SessionScope shape
- **WHEN** the router constructs `SessionScope` for a renderer call
- **THEN** the scope MUST include `SessionID`, `ChatID`, `ReplyToID`, `UserID`, and `MessageID`
- **AND** all fields except `ReplyToID` and `UserID` MUST be non-empty

### Requirement: Router subscriber-based orchestration

The router SHALL subscribe to the harness `EventBus` via `Master.SubscribeWSBroadcast()` whenever it takes the renderer path, and SHALL forward the event channel to the renderer. The router SHALL NOT introduce any new master-side API such as `ProcessMessageStream`. Subscription lifetime MUST be bounded by the underlying `ProcessMessage` call.

#### Scenario: Subscription lifecycle
- **WHEN** the router takes the renderer path for an inbound message
- **THEN** the router MUST call `master.SubscribeWSBroadcast()` before invoking `ProcessMessage`
- **AND** MUST call `master.UnsubscribeWSBroadcast(subID)` after `ProcessMessage` returns AND a 200 ms drain window has elapsed
- **AND** MUST close the renderer's event channel after unsubscribing

#### Scenario: ProcessMessage signature unchanged
- **WHEN** the renderer path is implemented
- **THEN** `MessageProcessor.ProcessMessage(ctx, sessionID, input) (TaskResponse, error)` MUST remain unchanged
- **AND** no `ProcessMessageStream` or equivalent method MUST be added to `MessageProcessor`

#### Scenario: Subscriber-side session filter
- **WHEN** the router or renderer reads from the EventBus channel
- **THEN** events with non-empty `SessionID` that does not match `scope.SessionID` MUST be skipped
- **AND** session-scoped event types (`message`, `tool_call`, `agent_progress`, `error`, `input_request`, `input_received`) MUST be emitted via `EventBus.BroadcastSessionMessage` so their top-level `SessionID` is populated
- **AND** events with empty `SessionID` MUST only originate from harness-global sources (e.g., startup errors, tool-list-changed); renderers MAY pass them through but MUST NOT treat them as belonging to their scoped session

#### Scenario: No cross-session leak in shared chat
- **WHEN** two distinct sessions `S1` and `S2` are bound to the same IM chat (e.g., same Feishu `chat_id` with different `sender_id`s routed to different harness sessions)
- **AND** both sessions have concurrent renderer subscriptions active
- **THEN** LLM `message` / `tool_call` / `agent_progress` chunks emitted for session `S1` MUST NOT be dispatched to the renderer scoped to session `S2`
- **AND** this MUST hold even when the renderer's event handler does not re-filter by payload's `session_id` field (i.e., the top-level `BroadcastMessage.SessionID` is the authoritative filter)

#### Scenario: Renderer failure fallback
- **WHEN** `RenderEventStream` returns a non-nil error
- **THEN** the router MUST fall back to `plugin.Send` with the last known full message content (captured by the renderer and exposed via the error or scope)
- **AND** MUST log the renderer error at warn level
- **AND** MUST NOT surface a half-rendered card as the final state

#### Scenario: Goroutine safety
- **WHEN** the router or renderer goroutine panics
- **THEN** a `defer recover` MUST capture it
- **AND** the recovery path MUST log at error level
- **AND** MUST NOT crash other in-flight messages

### Requirement: Harness input_received event

The harness SHALL broadcast an `input_received` event to the EventBus immediately upon receiving a user message into the master session loop. The event payload SHALL include `SessionID` and `ChannelMessageID` (the platform-side message ID, propagated by the channel plugin via `SessionRequest`). This event SHALL replace channel-private ack logic (such as the hard-coded `"Get"` reaction in `longconn.go`).

#### Scenario: Event broadcast on inbound message
- **WHEN** master `session_loop` receives a user message for session `S` with `ChannelMessageID` `M`
- **THEN** the harness MUST broadcast `BroadcastMessage{Type: EventTypeInputReceived, SessionID: S, Payload: InputReceivedEvent{SessionID: S, ChannelMessageID: M}}` BEFORE invoking the LLM
- **AND** the broadcast MUST happen within 50 ms of receiving the message

#### Scenario: ChannelMessageID propagation
- **WHEN** a channel plugin invokes `MessageProcessor.ProcessMessage`
- **THEN** the plugin MUST set `req.ChannelMessageID` to the platform-side message ID (or empty string if unavailable)
- **AND** the master MUST propagate this value into the `input_received` event payload unchanged

#### Scenario: Event constant exported
- **WHEN** other packages consume harness events
- **THEN** `master.EventTypeInputReceived` MUST be exported as `"input_received"`
- **AND** `master.InputReceivedEvent` MUST be exported with fields `SessionID string` and `ChannelMessageID string`

### Requirement: Feishu EventRenderer with event-to-card mapping

The Feishu plugin SHALL implement `EventRenderer.RenderEventStream` by mapping harness events to interactive card lifecycle operations. The renderer SHALL maintain a single card per session and update it via `client.PatchCard(messageID, cardJSON)` as events arrive. The mapping SHALL cover at minimum: `message` (partial + final), `tool_call` (start/success/error), `input_request` (HITL), and `error`. Throttling SHALL be 300 ms minimum interval between PATCHes for `message` partial events; final events and tool_call/hitl/error events SHALL flush immediately.

#### Scenario: First event creates card
- **WHEN** the first event of type `message` or `tool_call` arrives for a session
- **THEN** the renderer MUST call `client.ReplyMessage(scope.ReplyToID, "interactive", BuildCardJSON(state))` (or `SendMessage(scope.ChatID, ...)` if `ReplyToID` is empty)
- **AND** MUST capture the returned `messageID` into renderer state for subsequent PATCHes

#### Scenario: Message partial events are throttled
- **WHEN** consecutive `message` events with `partial=true` arrive
- **AND** less than 300 ms has elapsed since the last PATCH
- **THEN** the renderer MUST coalesce the new content into pending state
- **AND** MUST NOT issue a PATCH

#### Scenario: Message final event flushes immediately
- **WHEN** a `message` event with `partial=false` arrives
- **THEN** the renderer MUST PATCH the card to terminal state (title `"✅ 完成"`, full content) within 100 ms
- **AND** MUST NOT be subject to the 300 ms throttle

#### Scenario: Tool call events update tool section
- **WHEN** a `tool_call` event with `Status="start"` arrives
- **THEN** the renderer MUST add a tool line `🔧 调用工具：{tool_name}` with a spinner glyph
- **AND** MUST PATCH immediately (no throttle)
- **WHEN** a follow-up `tool_call` event with `Status="success"` for the same `tool_call_id` arrives
- **THEN** the renderer MUST update that line to a green checkmark with duration
- **WHEN** `Status="error"` arrives
- **THEN** the line MUST become red with error message

#### Scenario: HITL renders approval buttons
- **WHEN** an `input_request` event arrives for the active session
- **THEN** the renderer MUST PATCH the card to include approve / reject buttons at the bottom
- **AND** the buttons MUST carry the `request_id` as their `card_action_callback` value

#### Scenario: Error event renders failure card
- **WHEN** an `error` event arrives
- **THEN** the renderer MUST PATCH the card to a terminal red state with the error hint
- **AND** subsequent events for the same session MUST NOT be rendered

#### Scenario: Context cancellation triggers final flush
- **WHEN** the renderer's `ctx` is canceled
- **THEN** the renderer MUST attempt one final PATCH using the latest accumulated content within 3 seconds
- **AND** then return `ctx.Err()`

#### Scenario: PATCH failure falls back to plain Send
- **WHEN** any `PatchCard` call returns a non-nil error
- **AND** subsequent retries within 1 second also fail
- **THEN** the renderer MUST surface the last full content via the returned error so the router can call `plugin.Send(lastFullContent)` as fallback
- **AND** MUST log a warn entry with `message_id` and the underlying error

#### Scenario: Idle heartbeat indicates thinking state
- **WHEN** the card has been created (non-empty `messageID`) AND the card is in generating state AND no terminal error has occurred AND the card is not awaiting human input (no pending HITL request)
- **AND** the elapsed time since the last PATCH exceeds the renderer's `thinkingHeartbeat` threshold (default 5 seconds)
- **THEN** the renderer MUST PATCH the card with title `"💭 思考中…"` while preserving existing body, tool lines, and HITL buttons
- **AND** MUST NOT leave the visible card stuck in the thinking title once a real event (`message` / `tool_call` / `agent_progress` / `final` / `error`) resumes — subsequent PATCHes MUST reflect the event's semantic title (generating / done / error)
- **AND** MUST suppress further heartbeat PATCHes once the card reaches a terminal state (`done` or `error`)
- **AND** MUST NOT use the heartbeat's PATCH timestamp as the throttle gate for message/agent_progress partials — the heartbeat MUST NOT consume the partial throttle window

### Requirement: Feishu input_received to ack reaction

The Feishu `EventRenderer` SHALL react to `input_received` events by calling `client.AddReaction(payload.ChannelMessageID, cfg.AckEmoji)` asynchronously with a 5-second timeout. This SHALL be the SOLE place in the codebase that issues the ack reaction; the prior hard-coded call at `longconn.go:215` SHALL be removed in favor of this event-driven path. The webhook path SHALL benefit automatically without per-path code duplication.

#### Scenario: Renderer reacts to input_received
- **WHEN** the renderer receives an `input_received` event with non-empty `ChannelMessageID`
- **AND** `cfg.AckEmoji` is not `"none"` and not empty
- **THEN** the renderer MUST spawn a goroutine that calls `client.AddReaction(ctx5s, ChannelMessageID, cfg.AckEmoji)`
- **AND** MUST NOT block subsequent event processing

#### Scenario: Empty MessageID or disabled emoji
- **WHEN** `ChannelMessageID` is empty
- **OR** `cfg.AckEmoji` is `"none"` or empty
- **THEN** the renderer MUST skip the reaction call

#### Scenario: Webhook path uniformity
- **WHEN** an inbound message arrives via the webhook path
- **AND** the message reaches the master session loop
- **THEN** the same `input_received` event MUST be broadcast
- **AND** the renderer MUST issue the reaction
- **AND** behavior MUST be observationally identical to the longconn path

#### Scenario: Legacy hard-coded reaction removed
- **WHEN** the change is fully applied
- **THEN** `internal/channel/feishu/longconn.go` MUST NOT contain any direct `AddReaction` call
- **AND** the only `AddReaction` call site MUST be inside the renderer's `input_received` handler

#### Scenario: Reaction failure does not affect rendering
- **WHEN** `AddReaction` returns a non-nil error
- **THEN** the renderer MUST log a warn entry
- **AND** MUST continue processing subsequent events normally

### Requirement: Feishu renderer configuration

The configuration schema SHALL expose under `FeishuConfig`:
- `ack_emoji` (string, default `"Get"`, allowed: `"Get"`, `"Typing"`, `"none"`). Values MUST match the Feishu reactions API `emoji_type` enum (CamelCase). Legacy values `"GET"` and `"KEYBOARD"` from earlier releases MUST be silently migrated to `"Get"` and `"Typing"` respectively during `Normalize` — this is a compatibility concession, not an accepted spelling.
- `renderer.enabled` (bool, default `true`)
- `renderer.throttle_ms` (int, default `300`)
- `renderer.show_agent_progress` (bool, default `false`)

#### Scenario: Default configuration enables renderer
- **WHEN** the config omits all renderer fields
- **THEN** the system MUST behave as if `renderer.enabled=true`, `renderer.throttle_ms=300`, `renderer.show_agent_progress=false`, `ack_emoji="Get"`

#### Scenario: Legacy emoji value is silently migrated
- **WHEN** `ack_emoji` is loaded from DB as `"GET"` or `"KEYBOARD"` (old releases wrote these by mistake)
- **THEN** `Normalize` MUST rewrite the value to `"Get"` or `"Typing"` respectively
- **AND** MUST NOT emit a warn entry (this is a silent upgrade, not a user mistake)

#### Scenario: Invalid emoji value falls back to Get
- **WHEN** `ack_emoji` is set to a value other than `"Get"`, `"Typing"`, `"none"`, empty, or a legacy-migrated value
- **THEN** the system MUST log a warn entry at startup
- **AND** MUST treat the value as `"Get"`

#### Scenario: agent_progress hidden by default
- **WHEN** `renderer.show_agent_progress=false`
- **AND** an `agent_progress` event arrives
- **THEN** the renderer MUST NOT update the card based on this event

### Requirement: EventBus subscription cleanup

The router SHALL guarantee that every renderer-path subscription is released within 1 second of `ProcessMessage` returning, regardless of success or error. The system SHALL NOT rely on `PruneDeadSubscribers` as the primary cleanup mechanism for renderer subscriptions.

#### Scenario: Successful cleanup on normal return
- **WHEN** `ProcessMessage` returns successfully
- **THEN** the router MUST call `UnsubscribeWSBroadcast(subID)` within 1 second (200 ms drain + bookkeeping)
- **AND** MUST close the renderer event channel

#### Scenario: Cleanup on error or panic
- **WHEN** `ProcessMessage` returns an error
- **OR** the renderer goroutine panics
- **THEN** the router MUST still call `UnsubscribeWSBroadcast(subID)` and close the channel via `defer`
- **AND** MUST NOT leave the subscription dangling

#### Scenario: Cleanup on context cancellation
- **WHEN** the request `ctx` is canceled mid-flight
- **THEN** the router MUST cancel the renderer's child context
- **AND** MUST unsubscribe within 1 second

### Requirement: Backward compatibility for non-renderer platforms

Platforms that do not implement `EventRenderer` (dingtalk, wecom, wechat variants at the time of this change) SHALL continue to use the existing one-shot `Send` path with zero changes to their plugin code beyond shared interface stubs (if any). The `input_received` event SHALL still be broadcast for these platforms but SHALL have no observable effect (no plugin consumes it).

#### Scenario: DingTalk plugin unchanged
- **WHEN** a message arrives on the DingTalk platform
- **THEN** the router MUST call `DingTalkPlugin.Send(ctx, msg)` exactly once with the final content
- **AND** MUST NOT subscribe to the EventBus for that message
- **AND** MUST NOT call any renderer method

#### Scenario: WeCom plugin unchanged
- **WHEN** a message arrives on the WeCom platform
- **THEN** the router MUST call `WeComPlugin.Send(ctx, msg)` exactly once with the final content

#### Scenario: input_received event is benign for non-renderer platforms
- **WHEN** harness broadcasts `input_received` for a session whose channel has no renderer
- **THEN** the EventBus broadcast MUST succeed without error
- **AND** MUST have no observable side effect on the IM platform

### Requirement: Cross-surface contract uniformity

The `EventRenderer` abstraction SHALL be the same contract used by the frontend WebSocket subscriber (`internal/streaming/websocket.go`) and any future renderer (MCP server, AG-UI encoder, dingtalk renderer, etc). All renderers SHALL consume `master.BroadcastMessage` from the same `Subscribe()` API. The frontend `useHiveAgentEvents` hook (defined in `chat-ui-migrate-ai-elements`) SHALL be informed by but NOT depend on this change.

#### Scenario: Single subscription contract
- **WHEN** any new surface (IM, MCP, AG-UI) is added in the future
- **THEN** it MUST consume events via `Master.SubscribeWSBroadcast()`
- **AND** MUST NOT introduce a parallel pub-sub mechanism

#### Scenario: WebSocket path unchanged
- **WHEN** the change is applied
- **THEN** `internal/streaming/websocket.go` MUST require zero modifications
- **AND** the existing session-filter logic at lines 346-355 MUST remain authoritative for the frontend
- **AND** every frontend subscriber (notably `frontend/src/layouts/AppShell.tsx`) MUST pass the current session id via `?session_id=` at WS handshake time; failure to do so causes the filter to silently drop 100% of session-scoped broadcasts (streaming chunks, tool_call events, agent_progress) because `userSessionID == ""` combined with any non-empty `broadcastMsg.SessionID` hits the drop branch. This contract is mandatory once Sprint 12 migrated the emit path to `BroadcastSessionMessage`.
- **AND** the session-mismatch drop branch at line 351-355 MUST emit a Debug-level log with `broadcast_session`, `conn_session`, and event `type` so future subscriber-contract regressions become observable within seconds rather than blind production outages.
- **AND** the AppShell `sessionId` value passed into `useWebSocket` MUST resolve via the precedence `useParams().id || useChatStore.currentSessionId`, so that the ChatLanding → `/sessions/:id` → first LLM chunk transition cannot produce a WS-reconnect race window where `sessionId` is `undefined` when the backend begins streaming. URL params are the earlier signal during in-app navigation, but `chat.store.currentSessionId` (set synchronously by `sendMessage` / `loadMessages`) MUST serve as fallback whenever the URL has not yet matched `:id` — for example when a sibling layout or admin shell mounts before the Chat page.
- **AND** `useWebSocket.handleDisconnected` MUST NOT call `useChatStore.setCurrentSessionId(null)`. The WS `onclose` fires on every session-scoped reconnect triggered by AppShell `sessionId` prop changes; nullifying the store invalidates the `chat.addMessage` sessionId guard, perturbs downstream handlers that key off `currentSessionId`, and creates a three-way race between WS reconnect, backend first chunk emit, and Chat.tsx `loadMessages` re-setting the store. The source of truth for `currentSessionId` SHALL be `sendMessage` / `loadMessages`, not the WS lifecycle.
