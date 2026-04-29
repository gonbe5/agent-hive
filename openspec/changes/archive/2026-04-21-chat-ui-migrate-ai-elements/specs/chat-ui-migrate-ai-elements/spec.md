## ADDED Requirements

### Requirement: Hard prerequisites — chat-ui-polish close-the-loop evidence complete

This change SHALL NOT begin implementation (not even Phase 0 spike) until `chat-ui-polish` is merged AND its close-the-loop evidence is verifiable. PR merge alone does NOT satisfy this prerequisite.

#### Scenario: Site-wide amber purge verified
- **WHEN** Phase 0 is about to start
- **THEN** `rg -l "amber-" frontend/src` MUST return either empty output OR only files where every occurrence is annotated with one of `/* warning semantic */`, `/* replay scene decor */`, or `/* legacy reference */`
- **OR** Phase 0 MUST be blocked

#### Scenario: ChatInput amber removed
- **WHEN** Phase 0 is about to start
- **THEN** `rg "amber" frontend/src/components/chat/ChatInput.tsx` MUST return empty output
- **OR** Phase 0 MUST be blocked

#### Scenario: Tailwind brand-amber removed
- **WHEN** Phase 0 is about to start
- **THEN** `grep "brand-amber" frontend/tailwind.config.js` MUST return empty output
- **OR** Phase 0 MUST be blocked

#### Scenario: Logo SVG asset blue
- **WHEN** Phase 0 is about to start
- **THEN** `rg "stop-color" frontend/src` MUST NOT include `#F59E0B` or `#D97706`
- **OR** Phase 0 MUST be blocked

#### Scenario: Inline code resolved to blue token
- **WHEN** Phase 0 is about to start
- **THEN** `rg "amber" frontend/src/components/chat/MessageBubble.tsx` MUST return empty output
- **AND** inline code MUST resolve to `var(--accent-700)` color and `rgba(59, 130, 246, 0.08)` background
- **OR** Phase 0 MUST be blocked

#### Scenario: iOS Safari evidence attached
- **WHEN** Phase 0 is about to start
- **THEN** `chat-ui-polish/tasks.md` Section 9 MUST include iOS Safari screenshot + video evidence
- **OR** Phase 0 MUST be blocked

#### Scenario: DESIGN.md Decisions Log updated
- **WHEN** Phase 0 is about to start
- **THEN** `DESIGN.md` Decisions Log MUST include entries for the blue-token rebrand and iOS Safari decision
- **OR** Phase 0 MUST be blocked

### Requirement: AI Elements primitive adoption

The chat UI SHALL adopt Vercel AI Elements primitives for generic chat UI concerns (streaming markdown rendering, message bubbles, code blocks, composer, tool call rendering framework, task progress). AI Elements components SHALL be installed via the shadcn method (`npx ai-elements@latest`) and live in `frontend/src/components/ai-elements/`.

#### Scenario: Installation method
- **WHEN** AI Elements is installed for the first time
- **THEN** components MUST be placed in `frontend/src/components/ai-elements/`
- **AND** the installation MUST NOT add AI Elements as a direct npm dependency in `package.json`
- **AND** project-owned files MUST be editable without upstream sync

#### Scenario: Primitive coverage
- **WHEN** the migration completes Phase 1
- **THEN** the following primitives MUST be adopted: `Conversation` (or its responsibility documented as deferred per D5 decision), `Message`, `Response`, `PromptInput`, `Tool`, `Task`
- **AND** `Reasoning` MUST NOT be adopted in this change (deferred — Hive backend does not yet surface reasoning content)

#### Scenario: React 19 compatibility
- **WHEN** AI Elements is installed into the Hive frontend
- **THEN** it MUST be compatible with the project's React 19
- **OR** Phase 0 MUST immediately output a NO-GO decision and the change MUST be archived

### Requirement: Business shell layer preservation

The chat UI SHALL preserve Hive-specific business shell components that have no community equivalent or where the Hive variant carries domain-specific behavior. These components MUST NOT be replaced by AI Elements primitives unless Phase 0 spike explicitly proves the primitive can fully replace them.

#### Scenario: ArtifactCard preservation
- **WHEN** a message contains a parsed artifact segment
- **THEN** `ArtifactCard` MUST still render and link to `useCanvasStore.openArtifact`
- **AND** MUST NOT be replaced by AI Elements `Canvas`

#### Scenario: HITL button preservation
- **WHEN** a message triggers a human-in-the-loop approval
- **THEN** the existing Hive HITL button and approval flow MUST render unchanged
- **AND** MUST NOT be folded into a generic AI Elements action

#### Scenario: ErrorCard preservation
- **WHEN** a message has error state
- **THEN** the existing `ErrorCard` business shell MUST continue to render
- **AND** MUST NOT be replaced unless Phase 0 spike (D3 row #5) PASSES with evidence that AI Elements `Message` variants fully cover the existing ErrorCard behavior

#### Scenario: ToolResultCard content preservation
- **WHEN** a tool produces a result requiring markdown / code / table rendering
- **THEN** the existing `ToolResultCard` business shell MUST be slot-injected into the `Tool` primitive content area
- **AND** MUST NOT have its rendering logic merged into the primitive

#### Scenario: Brain panel preservation
- **WHEN** a message has assistant Brain panel content
- **THEN** the existing Brain panel MUST continue to render at its current mount point in `MessageBubble`
- **AND** MUST NOT be replaced by `Reasoning` primitive in this change

#### Scenario: Parallel tool group badge preservation
- **WHEN** a message contains multiple parallel `tool_calls`
- **THEN** the `并行 ×N` badge (currently at `MessageBubble.tsx:365`) MUST continue to render
- **AND** its mount point MUST follow the Phase 0 spike D3 row #3 decision (business shell vs primitive children)

#### Scenario: MermaidBlock preservation
- **WHEN** a code block has language `mermaid`
- **THEN** `MermaidBlock` MUST continue to render the diagram
- **AND** MUST NOT be replaced

### Requirement: Phase 0 feasibility spike business gap closure gate

Phase 1 (formal migration) SHALL NOT start unless Phase 0 spike produces a written report addressing all 9 business gap rows with PASS / FAIL / DEFERRED judgments and at least 7 of 9 are PASS or DEFERRED with viable mitigations.

#### Scenario: Spike covers all 9 business gaps
- **WHEN** the spike report is produced
- **THEN** it MUST include explicit judgments for: (1) tool status mapping, (2) ToolResultCard content slot viability, (3) parallel badge mount decision, (4) HITL button mount + onApprove routing, (5) ErrorCard primitive coverage, (6) Brain panel mount continuity, (7) Artifact-Canvas slot injection, (8) MessageList container responsibility (D5 three-option recommendation), (9) Replay mode read-only behavior
- **OR** the spike MUST be marked NO-GO

#### Scenario: Pass threshold
- **WHEN** the spike report is reviewed
- **THEN** at least 7 of 9 gaps MUST be PASS or DEFERRED with documented mitigation
- **AND** any FAIL without mitigation MUST trigger NO-GO

#### Scenario: No suspended items
- **WHEN** the spike report is reviewed
- **THEN** the report MUST NOT contain phrases equivalent to "wait until Phase 1 to decide" or "uncertain — verify in production"
- **OR** the spike MUST be marked NO-GO

#### Scenario: Theme alias verified
- **WHEN** the spike runs `Tool` primitive in light + dark mode
- **THEN** the shadcn token aliases (D6) MUST visually apply correctly
- **AND** no token leakage MUST be observed
- **OR** the spike MUST be marked NO-GO

#### Scenario: MessageList recommendation
- **WHEN** the spike report is reviewed
- **THEN** it MUST include a clear recommendation among the D5 three options (A: Conversation replaces MessageList, B: MessageList nests Conversation, C: Keep MessageList without Conversation)
- **AND** the recommendation MUST cite measured LOC + screenshot evidence
- **OR** the spike MUST be marked NO-GO

#### Scenario: Escape hatch on NO-GO
- **WHEN** any spike gate criterion fails
- **THEN** Phase 1 MUST NOT start
- **AND** a new change MUST be proposed (e.g., `chat-ui-salvage-assistant-ui` for route C) OR this change MUST be archived

### Requirement: useHiveAgentEvents hook adapter contract

A new hook `useHiveAgentEvents` SHALL be introduced at `frontend/src/hooks/useHiveAgentEvents.ts`. The hook's relationship to existing WebSocket hooks SHALL be explicit and documented; no double-data-source bug shall arise.

#### Scenario: Hook signature
- **WHEN** the hook is consumed
- **THEN** it MUST accept `{ sessionId: string, mode: 'live' | 'replay' }`
- **AND** return `{ events: HiveEvent[], status: 'idle' | 'streaming' | 'done' | 'error' }`

#### Scenario: HiveEvent shape
- **WHEN** the hook yields events
- **THEN** the `HiveEvent` union MUST include at minimum: `message.delta`, `tool.call`, `tool.result`, `task.update`, `artifact.open`, `hitl.request`
- **AND** event objects MUST be consumable by AI Elements primitives without further transformation in component code

#### Scenario: Live mode wraps useWebSocket
- **WHEN** mode is `'live'`
- **THEN** the hook MUST internally consume `useWebSocket(sessionId)` and map raw `BroadcastGenericMessage` payloads to `HiveEvent`
- **AND** MUST NOT bypass `useWebSocket` to subscribe directly to the raw transport

#### Scenario: Replay mode wraps useReplayWebSocket
- **WHEN** mode is `'replay'`
- **THEN** the hook MUST internally consume `useReplayWebSocket(sessionId)` and map history events to `HiveEvent`
- **AND** the emitted `HiveEvent` shape MUST be identical to live mode (consumers cannot tell the difference)

#### Scenario: Connection layer reuse
- **WHEN** the hook is in operation
- **THEN** the underlying connection management (reconnect, heartbeat, error recovery) MUST flow through `useWebSocketConnection`
- **AND** MUST NOT be reimplemented in `useHiveAgentEvents`

#### Scenario: Single source of truth
- **WHEN** Phase 1 is complete
- **THEN** all chat UI components (primitives, business shells, container) MUST consume events via `useHiveAgentEvents` only
- **AND** MUST NOT directly import `useWebSocket` or `useReplayWebSocket`

#### Scenario: Decoupling from backend payload
- **WHEN** UI components consume events from this hook
- **THEN** they MUST NOT directly reference backend payload shape
- **AND** MUST only reference the `HiveEvent` union type

#### Scenario: Future AG-UI upgrade path
- **WHEN** the Hive backend later adopts AG-UI protocol (route B, out of scope for this change)
- **THEN** only the hook's internal implementation MUST change
- **AND** consuming UI components MUST require no code changes

### Requirement: Tool primitive integration

The `Tool` primitive from AI Elements SHALL replace `ToolInvocationChip` + `ToolExecutionBlock` (produced by `chat-ui-polish`) in Phase 1 Day 2 (after spike GO). The replacement MUST preserve all scenarios defined in `chat-ui-polish`'s tool call spec.

#### Scenario: Functional equivalence
- **WHEN** a tool call is replaced with `Tool` primitive
- **THEN** the collapsed DOM MUST show the tool name and `tools.invoked` label
- **AND** MUST NOT show argument preview
- **AND** clicking the expand control MUST toggle `aria-expanded` and reveal args + result

#### Scenario: Status color tokens
- **WHEN** `Tool` primitive renders with `status: "error"`
- **THEN** the icon color MUST resolve to `#DC2626`
- **AND** MUST NOT fall back to the AI Elements default primary color

#### Scenario: Running spinner
- **WHEN** `status` is `"running"`
- **THEN** a spinner MUST replace the static icon
- **AND** use `var(--accent-600)` in light mode, `var(--accent-500)` in dark mode

#### Scenario: i18n injection
- **WHEN** `Tool` primitive renders
- **THEN** the collapse/expand button label MUST be wired to `t('tools.clickToExpand')` and `t('tools.clickToCollapse')`
- **AND** the invocation label MUST use `t('tools.invoked')`

### Requirement: MessageList container responsibility

The relationship between AI Elements `Conversation` primitive and existing `MessageList.tsx` (337 lines) SHALL be decided in Phase 0 spike (D5 three-option choice) and documented in the spike report.

#### Scenario: D5 decision recorded
- **WHEN** Phase 1 starts
- **THEN** the spike report MUST identify which of D5 options A / B / C was selected
- **AND** Phase 1 implementation MUST follow that selection

#### Scenario: Auto-scroll preserved
- **WHEN** a new message arrives
- **THEN** the chat list MUST auto-scroll to bottom (current `MessageList` behavior)
- **AND** the auto-scroll responsibility MUST be owned by exactly one of: `MessageList` (option B/C) or `Conversation` (option A) — never both

#### Scenario: ErrorCard mount point preserved
- **WHEN** the chat renders an error state
- **THEN** the `ErrorCard` mount point MUST be reachable per the selected D5 option
- **AND** MUST NOT lose its current behavior

### Requirement: Backend absolute zero diff

This change SHALL NOT modify any file under `internal/`, `agents/`, or any other backend directory. The hook adapter layer SHALL be the only place where backend payloads are translated. The proposal SHALL NOT contain any language suggesting "EventBus payload向 AG-UI 事件语义靠近" or any other backend shape change.

#### Scenario: Zero backend diff
- **WHEN** running `git diff --stat main...HEAD -- internal/ agents/` during this change
- **THEN** the diff MUST be empty

#### Scenario: Event payload stability
- **WHEN** this change is deployed
- **THEN** the shape of `BroadcastGenericMessage` payloads MUST remain unchanged
- **AND** existing subscribers (observability, test harnesses, IM channels) MUST NOT break

#### Scenario: No AG-UI alignment work
- **WHEN** the change documents are reviewed
- **THEN** no document MUST contain language proposing backend payload shape changes toward AG-UI semantics
- **AND** any such language MUST be moved to a future change (e.g., `agui-protocol-adoption`)

### Requirement: Shadcn token alias mapping

The `index.css` file SHALL provide alias mappings from shadcn-style AI Elements tokens (`--primary`, `--secondary`, `--accent`, `--muted`, `--border`, `--ring`) to the Hive light-blue token system established by `chat-ui-polish` (`--accent-600`, `--accent-subtle`, etc.). The underlying Hive tokens SHALL remain authoritative; shadcn tokens SHALL only be aliases.

#### Scenario: Primary maps to Hive accent
- **WHEN** `index.css` loads in light mode
- **THEN** `--primary` MUST equal `var(--accent-600)` (i.e., `#2563EB`)
- **AND** `--primary-foreground` MUST equal a contrast-safe value

#### Scenario: No token duplication
- **WHEN** the chat UI renders any AI Elements primitive
- **THEN** the computed colors MUST flow from Hive `--accent-*` tokens only
- **AND** MUST NOT introduce a parallel color pipeline

#### Scenario: Dark mode alias continuity
- **WHEN** the `.dark` class is active
- **THEN** shadcn token aliases MUST track the dark-mode Hive token values
- **AND** produce no contrast regression

### Requirement: Visual QA coverage

Each primitive replacement SHALL be accompanied by light + dark mode screenshot comparison against the baseline (`chat-ui-polish` merged state). Visual regressions SHALL block the associated commit.

#### Scenario: Before/after screenshots
- **WHEN** a primitive replacement commit is opened for review
- **THEN** the PR description MUST include before/after screenshots in both light and dark mode
- **AND** MUST explicitly call out any visual differences and their justification

#### Scenario: No silent degradation
- **WHEN** screenshots show any regression in layout, spacing, contrast, or interaction affordance
- **THEN** the commit MUST be revised
- **OR** the regression MUST be explicitly accepted in the PR description with rationale

#### Scenario: Phase 1 final QA scenes
- **WHEN** Phase 1 closes
- **THEN** the PR MUST include screenshots covering: (a) plain text, (b) artifact card, (c) tool call running/success/error, (d) task progress, (e) fenced code block, (f) inline code (blue), (g) composer focus, (h) ErrorCard, (i) Brain panel, (j) parallel badge, (k) HITL button — for both light and dark = 22 screenshots
- **AND** include a replay mode GIF + an end-to-end Feishu workflow GIF

### Requirement: i18n preservation

All i18n keys used by the legacy chat UI (including `tools.clickToExpand`, `tools.clickToCollapse`, `tools.invoked`, and any ChatInput placeholder/action keys) SHALL continue to work after migration. New keys MAY be added for new primitives' strings.

#### Scenario: Legacy key continuity
- **WHEN** the migrated UI renders
- **THEN** all existing i18n keys in `zh.json` / `en.json` used by chat components MUST continue to resolve
- **AND** no raw key names MUST appear in the UI

#### Scenario: Locale switching
- **WHEN** the user switches between zh and en
- **THEN** all AI Elements primitive labels MUST update correctly
- **AND** no hardcoded English strings MUST remain in the migrated components

### Requirement: No new input capabilities scope creep

This change SHALL NOT introduce new input capabilities not present in the current `ChatInput.tsx` (e.g., new file upload, voice input, slash commands not already present). Existing capabilities (send button, model selection, @mention if present) MUST be preserved through `PromptInput` wrapping.

#### Scenario: Capability parity
- **WHEN** the new `ChatInput.v2` ships
- **THEN** every capability of the legacy `ChatInput.tsx` MUST be present
- **AND** no new capability MUST be added (those belong in a separate change)

#### Scenario: Keyboard shortcuts mapped
- **WHEN** the new `ChatInput.v2` is in focus
- **THEN** `Cmd+Enter` (send), `Esc` (cancel), `Shift+Enter` (newline) MUST work as before
- **AND** the mapping MUST be documented in `ChatInput.v2` source comments
