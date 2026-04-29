# frontend-ws-handshake-regression Specification

## Purpose
TBD - created by archiving change frontend-ws-handshake-regression. Update Purpose after archive.
## Requirements
### Requirement: WebSocket handshake URL carries session identifier

When a session context exists for the active SPA route, the WebSocket handshake URL SHALL carry a query parameter that uniquely identifies the session. The backend broadcast writeLoop filters session-scoped messages against this identifier; omitting it causes all session-scoped traffic (including LLM streaming chunks) to be dropped server-side without user-visible error.

The query parameter key name MUST match the key the backend HTTP handler reads from `r.URL.Query()`. The backend is the single source of truth for the key name. A spec-level test SHALL anchor its key-name assertion to the backend source file, so that a frontend-only rename is caught as a contract violation.

The contract is on **what the network sees**, not on which hook or store produces the value.

#### Scenario: Session route emits session identifier on handshake
- **GIVEN** the user navigates to a chat session route (URL shape carrying a session id)
- **WHEN** the SPA establishes the WebSocket connection
- **THEN** the handshake request URL MUST contain a query parameter whose key equals the key read by the backend WebSocket handler (single source of truth, no scattered string literals)
- **AND** the value MUST equal the session id the user is currently viewing
- **AND** the value MUST be URL-encoded (reversible via `decodeURIComponent` to the original session id, even when the id contains reserved characters)

#### Scenario: Landing without session context omits the identifier
- **GIVEN** the user is on a route with no session context (e.g. landing page, admin shell)
- **WHEN** the SPA establishes the WebSocket connection
- **THEN** the handshake request URL MAY omit the session identifier query parameter
- **AND** the connection SHALL receive only broadcast-scoped events (no session-scoped traffic)

### Requirement: Session identity survives the disconnect → reconnect cycle

The user's session identity SHALL remain stable across an involuntary disconnect + reconnect (network blip, backend restart, browser suspend/resume). After reconnect, the next user-initiated message SHALL produce an LLM response that becomes visible in the chat view DOM, without a page refresh.

This requirement is phrased in terms of **user-observable outcome**. Any internal mechanism — store state, hook-local ref, route re-derivation — that achieves this outcome is acceptable. Future refactors are free to reshape the implementation provided this scenario continues to hold.

#### Scenario: Reconnect preserves conversation continuity
- **GIVEN** a user in an active chat session has successfully exchanged at least one message
- **WHEN** the WebSocket connection is closed by an external event (simulated network drop, server restart)
- **AND** the client auto-reconnects
- **AND** the user sends a subsequent message
- **THEN** at least one partial chunk of the assistant's reply MUST become readable in the chat view DOM (the chunk's text content appears in the rendered message region)
- **AND** no visible error state SHALL be shown between the user message and the assistant's first visible chunk

### Requirement: Streaming chunks reach the rendered chat view

Incoming partial-chunk messages SHALL propagate from the WebSocket transport through to visible DOM text in the chat view. A regression that writes partial chunks to an internal state container without updating the rendered output SHALL be considered a breach of this requirement, even if the internal state reflects the correct content.

The contract is **text visible to the reader**, not any particular store, selector, or component identity.

#### Scenario: First partial chunk renders to the DOM
- **GIVEN** the user has just sent a message in an active session
- **WHEN** the first partial-chunk WebSocket frame for the assistant's reply arrives at the client
- **THEN** the partial content MUST become readable in the chat view DOM (findable by text query) before the test's default async-query timeout
- **AND** subsequent partial chunks for the same reply MUST update the same visible message region (no duplicated or orphaned message rendering)

