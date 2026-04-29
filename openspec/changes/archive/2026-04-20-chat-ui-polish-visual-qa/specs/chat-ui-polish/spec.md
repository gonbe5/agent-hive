## ADDED Requirements

### Requirement: ToolInvocationChip is stateless

`ToolInvocationChip` SHALL be a purely props-driven component. Its signature SHALL be `{ name: string; status?: 'running' | 'success' | 'error' }` and it SHALL NOT subscribe to any store, read any global state, or accept an `id` prop. Live running/success status resolution SHALL be performed by the container component (`ToolCallRow` or equivalent) and injected via the `status` prop.

#### Scenario: Chip props signature
- **WHEN** inspecting `frontend/src/components/chat/ToolInvocationChip.tsx`
- **THEN** the `ToolInvocationChipProps` interface MUST contain exactly two fields: `name: string` and `status?: 'running' | 'success' | 'error'`
- **AND** MUST NOT contain an `id` field

#### Scenario: Chip does not subscribe to store
- **WHEN** running `rg -n 'useChatStore|toolCallStatuses' frontend/src/components/chat/ToolInvocationChip.tsx`
- **THEN** the output MUST be empty
- **AND** the file MUST NOT import from `../../store/chat`

#### Scenario: Live status resolution lifted to container
- **WHEN** `MessageBubble` renders a tool call with live `toolCallStatuses[id].status === 'running'`
- **THEN** a container component (e.g., `ToolCallRow`) MUST subscribe to the store once per tool_call_id
- **AND** MUST compute the effective status (giving `hasError` highest priority, otherwise the live store status)
- **AND** MUST pass the resolved status to `<ToolInvocationChip status={...}/>` as a prop
- **AND** no `id` prop MUST be passed to the chip

## MODIFIED Requirements

### Requirement: Inline code uses light-blue token styling

Chat-rendered Markdown inline code (not fenced blocks) SHALL render with a light-blue tinted background and light-blue text, consistent with the new brand palette. The scope SHALL be tightened to the chat message container via the `.message-content` class so that `.prose`-wrapped content outside chat (Canvas `MarkdownRenderer`, Guide pages, settings docs viewer) does NOT receive the inline-code light-blue styles. Fenced code blocks (`pre code`) SHALL be unaffected and MUST continue to use the existing `CodeBlockHeader` / deep theme.

#### Scenario: Inline code in light mode
- **WHEN** a chat message contains inline ``` `whoami` ```
- **THEN** the `<code>` element MUST have background `var(--accent-subtle)` (resolves to `rgba(59, 130, 246, 0.08)`)
- **AND** text color `var(--accent-700)` (resolves to `#1D4ED8`)
- **AND** font family `JetBrains Mono`
- **AND** border-radius 4px
- **AND** padding 2px 6px
- **AND** font-size `0.9em`
- **AND** font-weight `500`

#### Scenario: Inline code in dark mode
- **WHEN** the same inline code renders in dark mode
- **THEN** text color MUST be `var(--accent-300)` (resolves to `#93C5FD`)
- **AND** background MUST remain `var(--accent-subtle)`

#### Scenario: Fenced code blocks untouched
- **WHEN** the message contains a fenced code block
- **THEN** the inner `<code>` MUST NOT receive the inline-code light-blue styles
- **AND** MUST continue to use the existing `CodeBlockHeader` / deep theme

#### Scenario: Selector uses .message-content scope
- **WHEN** inspecting `frontend/src/index.css` for the inline-code rule
- **THEN** the selector MUST be `.message-content code:not(pre code)` (and its `html.dark` counterpart)
- **AND** MUST NOT be `.prose :not(pre) > code` or any other selector rooted at `.prose`

#### Scenario: Canvas MarkdownRenderer unaffected
- **WHEN** Canvas `MarkdownRenderer` renders inline code inside a `.prose` wrapper without `.message-content`
- **THEN** the inline code MUST retain the default prose inline-code style (no light-blue pill)
- **AND** background MUST NOT be `var(--accent-subtle)`

#### Scenario: Guide page unaffected
- **WHEN** the Guide page renders inline code inside a `.prose` wrapper without `.message-content`
- **THEN** the inline code MUST retain the default prose style (no light-blue pill)

### Requirement: Section heading spacing and scale

Markdown `#`, `##`, and `###` rendered inside a chat message body (`.message-content` container) SHALL use the DESIGN.md D5 typographic scale with explicit breathing room. The rule SHALL be scoped to `.message-content` selector so that non-chat `.prose` containers retain default heading styles.

#### Scenario: H1 rendering
- **WHEN** a chat message contains a `# Title` heading
- **THEN** the rendered `<h1>` MUST have computed `font-size: 24px` (`text-2xl`)
- **AND** `font-weight: 700`
- **AND** `margin-top: 40px` (`mt-10` = `2.5rem`)
- **AND** `margin-bottom: 20px` (`mb-5` = `1.25rem`)

#### Scenario: H2 rendering
- **WHEN** a chat message contains a `## 主要差异` heading
- **THEN** the rendered `<h2>` MUST have computed `font-size: 20px` (`text-xl`)
- **AND** `font-weight: 600`
- **AND** `margin-top: 32px` (`mt-8` = `2rem`)
- **AND** `margin-bottom: 16px` (`mb-4` = `1rem`)
- **AND** `scroll-margin-top: 64px` (`scroll-mt-16` = `4rem`)

#### Scenario: H3 rendering
- **WHEN** a chat message contains a `### 子项` heading
- **THEN** the rendered `<h3>` MUST have computed `font-size: 18px` (`text-lg`)
- **AND** `font-weight: 600`
- **AND** `margin-top: 24px` (`mt-6` = `1.5rem`)
- **AND** `margin-bottom: 12px` (`mb-3` = `0.75rem`)

#### Scenario: First-child margin collapse
- **WHEN** a heading (`h1`, `h2`, `h3`) is the first child of `.message-content`
- **THEN** its `margin-top` MUST be `0`
- **AND** the message body MUST NOT show a large blank gap before the first heading

#### Scenario: Scoped override
- **WHEN** the same heading appears outside `.message-content` (e.g., in settings prose, docs viewer, tool result inline Markdown)
- **THEN** the `.message-content h*` override MUST NOT apply
- **AND** the heading MUST retain the default prose styles
