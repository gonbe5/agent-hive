## ADDED Requirements

### Requirement: Separate tool invocation chip and execution block

The chat UI SHALL render each assistant tool call as two independent DOM elements: a compact, non-expandable `ToolInvocationChip` announcing the invocation, followed by an expandable `ToolExecutionBlock` for the result. The legacy combined `ToolCallCard` SHALL be reduced to a thin shim composing these two components so existing call sites continue to work during a one-release deprecation window.

#### Scenario: Collapsed default state
- **WHEN** an assistant message contains `tool_calls: [{ id: "t1", name: "bash" }]`
- **AND** the corresponding tool result has streamed in with `status: "success"`
- **THEN** the DOM MUST contain one `ToolInvocationChip` element for "bash" AND one `ToolExecutionBlock` element for "bash 执行结果"
- **AND** the execution block MUST start in the collapsed state (no result body visible)
- **AND** the chip MUST NOT render any expand/collapse control

#### Scenario: Chip never expands
- **WHEN** the user clicks on a `ToolInvocationChip`
- **THEN** no state change MUST occur
- **AND** no detail pane MUST appear under the chip

#### Scenario: Shim preserves old API
- **WHEN** any existing caller imports and uses `ToolCallCard` with the previous prop signature
- **THEN** the shim MUST render the composed `ToolInvocationChip` + `ToolExecutionBlock`
- **AND** produce a deprecation warning via `console.warn` in dev mode

### Requirement: Expand control uses labelled button

The expand/collapse control for `ToolExecutionBlock` SHALL be a text button showing the i18n strings `tools.clickToExpand` / `tools.clickToCollapse`, MUST include `aria-expanded` reflecting the current state, and MUST include `aria-controls` pointing to the detail body element id.

#### Scenario: Collapsed state
- **WHEN** the block is collapsed
- **THEN** the button text MUST be "点击展开" (zh) or "Click to expand" (en)
- **AND** `aria-expanded="false"`

#### Scenario: Expanded state
- **WHEN** the block is expanded
- **THEN** the button text MUST be "点击收起" (zh) or "Click to collapse" (en)
- **AND** `aria-expanded="true"`
- **AND** the detail body MUST be visible with id referenced by `aria-controls`

#### Scenario: Keyboard accessibility
- **WHEN** the button has focus
- **AND** the user presses Enter or Space
- **THEN** the block MUST toggle expanded state

### Requirement: Brand color rebrand to light-blue palette

The DESIGN.md brand accent SHALL be rebranded from amber/gold to a light-blue palette. The existing `--accent-*`, `--gradient-*`, and `--card-tool-*` CSS custom properties defined in `frontend/src/index.css` SHALL keep their names but SHALL take new blue values. Additionally, an explicit `--accent-{50,100,300,500,600,700}` stop set SHALL be declared.

#### Scenario: Light-mode accent values
- **WHEN** `frontend/src/index.css` is loaded in light mode
- **THEN** `--accent` / `--accent-600` MUST equal `#2563EB`
- **AND** `--accent-hover` / `--accent-700` MUST equal `#1D4ED8`
- **AND** `--accent-500` MUST equal `#3B82F6`
- **AND** `--accent-300` MUST equal `#93C5FD`
- **AND** `--accent-100` / `--accent-light` MUST equal `#DBEAFE`
- **AND** `--accent-50` MUST equal `#EFF6FF`
- **AND** `--accent-subtle` MUST equal `rgba(59, 130, 246, 0.08)`
- **AND** `--accent-border` MUST equal `rgba(59, 130, 246, 0.2)`

#### Scenario: Light-mode gradient values
- **WHEN** light mode is active
- **THEN** `--gradient-start`, `--gradient-mid`, `--gradient-end` MUST equal `#60A5FA`, `#3B82F6`, `#2563EB` respectively

#### Scenario: Dark-mode accent values
- **WHEN** `.dark` class is applied on `html`
- **THEN** `--accent` / `--accent-600` MUST equal `#60A5FA`
- **AND** `--accent-hover` / `--accent-700` MUST equal `#3B82F6`
- **AND** `--accent-500` MUST equal `#60A5FA`
- **AND** `--accent-300` MUST equal `#93C5FD`
- **AND** `--accent-subtle` MUST equal `rgba(59, 130, 246, 0.08)`

#### Scenario: Page background cool tint (iOS-safe)
- **WHEN** light mode is active
- **THEN** `--bg-primary` MUST equal `#F4F7FB`
- **AND** `body` MUST apply `background-image: linear-gradient(180deg, #F8FAFF 0%, #F2F5FC 100%)`
- **AND** `body` MUST NOT use `background-attachment: fixed` (to avoid iOS Safari 16+ rendering bugs)

#### Scenario: Dark-mode background unchanged
- **WHEN** `.dark` class is active
- **THEN** `--bg-primary` MUST remain `#1C1C1E`
- **AND** no page-level gradient MUST be applied

### Requirement: Semantic colors decoupled from brand rebrand

Warning, success, error, and info semantic colors defined in `DESIGN.md` SHALL NOT be recolored as part of the brand rebrand. Warning SHALL remain amber `#D97706`. Info SHALL remain blue `#2563EB`. Error SHALL be consolidated to `#DC2626`. Success SHALL be consolidated to `#059669`.

#### Scenario: Warning color unchanged
- **WHEN** a component renders a warning banner/toast
- **THEN** the computed color MUST still be `#D97706`
- **AND** MUST NOT adopt the new blue accent

#### Scenario: Error color consolidated
- **WHEN** any component renders an error icon or indicator previously using `#ef4444`
- **THEN** the computed color MUST be updated to `#DC2626`
- **AND** MUST match the `--danger` semantic token value

#### Scenario: Success color consolidated
- **WHEN** any component renders a success icon or indicator previously using `#10b981`
- **THEN** the computed color MUST resolve via `var(--success)` (light: `#059669`, dark: `#10B981`)

#### Scenario: No warning-as-brand conflict
- **WHEN** a UI element previously used `--accent-*` for a warning-adjacent meaning
- **THEN** that reference MUST be audited and MUST be switched to the semantic warning token if the meaning is truly warning
- **OR** kept on `--accent-*` if the meaning is brand decoration

### Requirement: Hardcoded amber purge (site-wide)

All hardcoded amber/gold hex values or Tailwind `amber-*` classes used for brand decoration SHALL be replaced by either a `var(--accent-*)` token reference or an explicit blue equivalent, across the entire `frontend/src/` tree. A grep audit SHALL yield zero un-annotated brand-decorative amber occurrences.

#### Scenario: Site-wide grep audit
- **WHEN** running `rg -n '#F59E0B|#D97706|#B45309|#FEF3C7|amber-\d+|rgba\(217,\s*119,\s*6' frontend/src`
- **THEN** the only remaining matches MUST be (a) semantic warning usages marked with `/* warning semantic */` comment, (b) replay scene decor marked with `/* replay scene decor */` comment, (c) test files under `__tests__/` or `*.test.tsx`, or (d) this spec file and other openspec artifact files
- **AND** MUST NOT include un-annotated brand-decorative usages in components, icons, backgrounds, or gradients

#### Scenario: ChatInput amber removed
- **WHEN** `frontend/src/components/chat/ChatInput.tsx` is inspected after the change
- **THEN** the drag-over overlay (previously `border-amber-400 dark:border-amber-500 ring-2 ring-amber-100 bg-amber-50/30`) MUST use blue equivalents (e.g. `border-[var(--accent-600)] ring-[var(--accent-100)] bg-[var(--accent-subtle)]`)
- **AND** the active-model indicator MUST use `text-[var(--accent-600)] dark:text-[var(--accent-300)]`
- **AND** the deep-thinking toggle MUST use blue token equivalents
- **AND** the send button MUST use `bg-[var(--accent-600)] hover:bg-[var(--accent-700)]`

#### Scenario: Per-directory verification
- **WHEN** the amber sweep is completed for each of these directories: `components/chat/`, `layouts/`, `components/settings/`, `pages/admin/`, `pages/`, `components/hitl/`, `components/common/`, `components/canvas/`, `components/session/`
- **THEN** each directory MUST pass its grep check independently
- **AND** the `components/replay/` directory MAY retain amber occurrences only if each is annotated with `/* replay scene decor */`

### Requirement: Tailwind theme cleanup

The `tailwind.config.js` SHALL NOT retain an active `brand-amber` color palette. The existing `chat.user: '#EBF3FE'` entry MAY be retained as it is already cool-blue-aligned, provided it is annotated to prevent future confusion.

#### Scenario: brand-amber removed
- **WHEN** `frontend/tailwind.config.js` is inspected
- **THEN** the `theme.extend.colors` object MUST NOT contain a `brand-amber` key
- **OR** if kept, it MUST be renamed to `legacy-amber` and carry a `// @deprecated` comment AND have zero references in `frontend/src/` (verifiable via `rg 'brand-amber'` returning zero hits in source)

#### Scenario: chat.user preserved and annotated
- **WHEN** `frontend/tailwind.config.js` is inspected
- **THEN** if `chat.user` is kept, the line MUST carry a comment such as `// light-blue aligned with --accent-100`

### Requirement: Tool-related UI uses token-driven colors

All colors used in tool-related UI SHALL come from CSS custom properties and SHALL NOT reintroduce hardcoded hex values. The prior hardcoded hex values at `MessageBubble.tsx:755, 820, 827, 830, 833, 899` plus the tool accent palette dict at line 732 SHALL all be replaced with token references.

#### Scenario: Default chip icon color
- **WHEN** a `ToolInvocationChip` renders in light mode with status not set
- **THEN** the gear icon color MUST resolve to `var(--accent-600)` (`#2563EB`)

#### Scenario: Dark mode chip icon color
- **WHEN** a `ToolInvocationChip` renders in dark mode
- **THEN** the gear icon color MUST resolve to `var(--accent-500)` (`#60A5FA`)

#### Scenario: Error status overrides to red
- **WHEN** the chip or execution block status is `"error"`
- **THEN** the icon color MUST resolve to `#DC2626` (via `var(--danger)` or equivalent)
- **AND** MUST NOT use the brand accent
- **AND** MUST NOT be `#ef4444` (consolidated away)

#### Scenario: Running status uses animated accent
- **WHEN** the chip status is `"running"`
- **THEN** the icon MUST be replaced by a spinner (`Loader2` with `animate-spin`) using `var(--accent-600)` light / `var(--accent-500)` dark

#### Scenario: Tool accent palette removed
- **WHEN** `MessageBubble.tsx` is inspected after the change
- **THEN** the hardcoded tool-name→hex dict previously at line ~732 MUST be removed or rewritten to return `var(--accent-*)` / semantic tokens
- **AND** there MUST NOT be a fallback literal like `#93c5fd`

### Requirement: Logo gradient follows new brand

The application logo SHALL use the new blue gradient `linear-gradient(135deg, #60A5FA 0%, #3B82F6 100%)` (or equivalent CSS var composition). The hexagonal brand silhouette SHALL be preserved unchanged.

#### Scenario: Logo gradient in light mode
- **WHEN** the logo is rendered in light mode
- **THEN** the fill MUST be the new blue gradient
- **AND** MUST NOT contain any amber/gold color stops

#### Scenario: Logo shape preserved
- **WHEN** the logo renders
- **THEN** the hexagonal honeycomb formation MUST remain identical to the pre-change geometry

#### Scenario: SVG asset grep
- **WHEN** running `rg -n 'stop-color|linearGradient' frontend/src/assets/ frontend/src/components/` after the change
- **THEN** no `stop-color="#F59E0B"`, `#D97706`, or `#B45309` MUST remain in logo/brand SVG definitions

### Requirement: Inline code uses light-blue token styling

Chat-rendered Markdown inline code (not fenced blocks) SHALL render with a light-blue tinted background and light-blue text, consistent with the new brand palette. Fenced code blocks (`pre code`) SHALL be unaffected and MUST continue to use the existing `CodeBlockHeader` / deep theme.

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

#### Scenario: Scope limited to chat
- **WHEN** inline code appears outside a chat message container (e.g., settings docs, admin help text)
- **THEN** the inline light-blue style MUST NOT apply
- **AND** the default prose inline-code style MUST be retained

### Requirement: Section heading spacing and scale

Markdown `##` and `#` rendered inside a chat message body SHALL use the DESIGN.md typographic scale with explicit breathing room.

#### Scenario: H2 rendering
- **WHEN** a chat message contains a `## 主要差异` heading
- **THEN** the rendered `<h2>` MUST have computed `font-size: 24px` (`text-xl`)
- **AND** `font-weight: 600`
- **AND** `margin-top: 32px`
- **AND** `margin-bottom: 16px`

#### Scenario: H1 rendering
- **WHEN** a chat message contains a `# Title` heading
- **THEN** the rendered `<h1>` MUST have computed `font-size: 28px` (`text-2xl`)
- **AND** `margin-top: 40px`
- **AND** `margin-bottom: 20px`

#### Scenario: Scoped override
- **WHEN** the same heading appears outside the chat message container (e.g., in settings, docs viewer)
- **THEN** the override MUST NOT apply
- **AND** the heading MUST retain the default prose styles

### Requirement: Collapsed chip shows no argument summary

`ToolInvocationChip` SHALL NOT render any argument preview, JSON snippet, or inline summary in the collapsed/default state. Argument details SHALL only appear in the expanded `ToolExecutionBlock` detail body under the existing `tools.input` label.

#### Scenario: JSON args not shown on chip
- **WHEN** a tool call has arguments `{"command": "ls -la /var/log"}`
- **THEN** the chip MUST NOT render "ls -la /var/log" or any substring thereof
- **AND** MUST only show the tool display name (e.g. "bash")

#### Scenario: Args visible after expand
- **WHEN** the user expands the `ToolExecutionBlock`
- **THEN** the detail body MUST show an "输入" / "Input" section with formatted JSON
- **AND** an "输出" / "Output" section with the result

### Requirement: i18n keys for expand control

The i18n resource files SHALL define `tools.clickToExpand`, `tools.clickToCollapse`, and `tools.invoked` keys for both Chinese (zh) and English (en) locales.

#### Scenario: Chinese strings
- **WHEN** the locale is zh
- **THEN** `tools.clickToExpand` MUST be `"点击展开"`
- **AND** `tools.clickToCollapse` MUST be `"点击收起"`
- **AND** `tools.invoked` MUST be `"已调用工具"`

#### Scenario: English strings
- **WHEN** the locale is en
- **THEN** `tools.clickToExpand` MUST be `"Click to expand"`
- **AND** `tools.clickToCollapse` MUST be `"Click to collapse"`
- **AND** `tools.invoked` MUST be `"Invoked tool"`

#### Scenario: Missing key fallback
- **WHEN** a key is missing in the active locale
- **THEN** react-i18next MUST fall back to the en resource
- **AND** MUST NOT render the raw key string
