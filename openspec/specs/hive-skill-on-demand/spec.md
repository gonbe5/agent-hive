# hive-skill-on-demand Specification

## Purpose
TBD - created by archiving change hive-skill-on-demand. Update Purpose after archive.
## Requirements
### Requirement: SkillScope type and storage layering

The system SHALL introduce a `SkillScope` type with values `ScopePublic` (shared across all users) and `ScopePersonal` (scoped to a single authenticated user). Public skills SHALL be stored under `$HIVE_DATA/skills/public/` and personal skills under `$HIVE_DATA/skills/users/<userID>/`. The existing paths (`.claude/skills`, `~/.claude/skills`, `skills/`) SHALL continue to load as `ScopePublic` for backward compatibility.

#### Scenario: Public skill default
- **WHEN** a SKILL.md is located under `$HIVE_DATA/skills/public/` or legacy paths
- **AND** the SKILL.md frontmatter does not declare a `scope` field
- **THEN** the skill MUST be registered with `ScopePublic`

#### Scenario: Personal skill tied to userID
- **WHEN** a SKILL.md is located under `$HIVE_DATA/skills/users/<userID>/<name>/`
- **AND** the session's `UserID` equals `<userID>`
- **THEN** the skill MUST be registered with `ScopePersonal` and the owning userID

#### Scenario: Cross-tenant isolation enforced
- **WHEN** user `alice` has a personal skill `nuwa`
- **AND** user `bob` invokes `Registry.Get("nuwa", "bob")`
- **THEN** the result MUST NOT include `alice`'s personal `nuwa`

#### Scenario: Explicit scope field in SKILL.md
- **WHEN** a SKILL.md frontmatter declares `scope: personal`
- **THEN** the declared scope MUST override path-based inference
- **AND** registering a `personal` skill without a valid userID MUST fail with a clear error

### Requirement: Tenant-aware Registry lookup with personal override

The `Registry` SHALL expose `Get(name, userID)` and `List(userID)` that return personal skills first (when `userID` is non-empty) and fall back to public skills. `List(userID)` SHALL merge public and personal skills, where personal entries override same-named public entries, and SHALL mark overridden entries on the returned `SkillSummary` so UIs can display them.

#### Scenario: Personal overrides public on Get
- **WHEN** `alice` has a personal skill `nuwa@2.0.0`
- **AND** a public skill `nuwa@1.0.0` exists
- **AND** `Registry.Get("nuwa", "alice")` is called
- **THEN** the returned skill MUST be `nuwa@2.0.0` (personal)

#### Scenario: Public fallback
- **WHEN** `bob` has no personal skill named `nuwa`
- **AND** a public skill `nuwa@1.0.0` exists
- **AND** `Registry.Get("nuwa", "bob")` is called
- **THEN** the returned skill MUST be `nuwa@1.0.0` (public)

#### Scenario: Empty userID ignores personal layer
- **WHEN** `Registry.Get("nuwa", "")` is called (unauthenticated session)
- **THEN** only public skills MUST be considered
- **AND** any personal skill with the same name MUST be invisible

#### Scenario: List exposes override marker
- **WHEN** `alice` has personal `nuwa@2.0.0` overriding public `nuwa@1.0.0`
- **AND** `Registry.List("alice")` is called
- **THEN** the returned `SkillSummary` for `nuwa` MUST include `OverriddenPublic: true`

### Requirement: Version-aware registration

The `Registry.Register` and `Registry.RegisterFromPath` SHALL be version-aware. When multiple versions of the same (name, scope, userID) tuple are present, the entry with the highest semver SHALL be preferred. If `agent.skills.pinned_versions[name]` is configured, the pinned version SHALL be preferred regardless of semver comparison. Same-version re-registration SHALL be idempotent and emit a `skill.registry.dup` metric rather than failing.

#### Scenario: Latest semver wins by default
- **WHEN** `nuwa@1.0.0` is already registered
- **AND** `nuwa@1.1.0` is registered afterward with the same scope and userID
- **THEN** `Get` MUST return `nuwa@1.1.0`

#### Scenario: Pinned version overrides semver
- **WHEN** `agent.skills.pinned_versions["nuwa"]` is `1.0.0`
- **AND** both `nuwa@1.0.0` and `nuwa@2.0.0` are registered
- **THEN** `Get` MUST return `nuwa@1.0.0`

#### Scenario: Same-version re-register is idempotent
- **WHEN** `nuwa@1.0.0` is registered
- **AND** `nuwa@1.0.0` is registered a second time (e.g., Watcher trigger)
- **THEN** the registration MUST NOT fail
- **AND** `skill.registry.dup` metric MUST be incremented

### Requirement: Discovery ResolveByName and ResolveByRequirements

The system SHALL add `Discovery.ResolveByName(ctx, name)` and `Discovery.ResolveByRequirements(ctx, reqs)` that query the configured `agent.skills.marketplace_urls` in list order. Name-based resolution SHALL return the first match; when the same name exists in multiple marketplaces the method SHALL return `errs.CodeSkillAmbiguous` with all candidates. Requirement-based resolution SHALL return all skills whose `provides_requirements` covers any requested requirement, ranked by coverage. The marketplace `index.json` SHALL be cached for 5 minutes; `refresh: true` SHALL force re-fetch. `ResolveByRequirements` MUST NOT read the local `Registry`; local lookup is the exclusive responsibility of `Skills.FindBySpecRequirements` (see separate requirement on method separation of concerns).

#### Scenario: Name hit in first marketplace
- **WHEN** `marketplace_urls: [A, B]` and `nuwa` exists only in A
- **AND** `ResolveByName(ctx, "nuwa")` is called
- **THEN** the result MUST be the `A`-sourced `ResolvedSkill`

#### Scenario: Name conflict across marketplaces
- **WHEN** `nuwa` exists in both `A` and `B`
- **AND** `ResolveByName(ctx, "nuwa")` is called
- **THEN** the error MUST be `errs.CodeSkillAmbiguous`
- **AND** the error payload MUST list both candidates with source URLs

#### Scenario: Requirement-based resolution with coverage ranking
- **WHEN** marketplace has `foo` providing `["image_generation"]` and `nuwa` providing `["image_generation", "chinese_prompt"]`
- **AND** `ResolveByRequirements(ctx, ["image_generation", "chinese_prompt"])` is called
- **THEN** `nuwa` MUST rank before `foo`

#### Scenario: Cache hit avoids re-fetch
- **WHEN** `ResolveByName(ctx, "nuwa")` is called twice within 5 minutes
- **THEN** only one HTTP request to `index.json` MUST be made

#### Scenario: Refresh bypasses cache
- **WHEN** `ResolveByName` or `skill_install` is called with `refresh: true`
- **THEN** the marketplace `index.json` MUST be re-fetched even if cached

### Requirement: Discovery PullOne single-skill download

The system SHALL add `Discovery.PullOne(ctx, source, name)` that downloads a single named skill from one marketplace source into the local cache. Writes MUST be atomic (write `.tmp`, then rename) and failures MUST NOT leave partial files on disk.

#### Scenario: Successful single-skill download
- **WHEN** `PullOne(ctx, "https://marketplace.hive.io/", "nuwa")` is called
- **THEN** all files declared by `index.json` for `nuwa` MUST be written to the cache
- **AND** the returned path MUST contain `SKILL.md`

#### Scenario: Atomic write on failure
- **WHEN** a mid-download network failure occurs
- **THEN** no partially-written files MUST remain in the destination skill directory
- **AND** the error MUST be returned to the caller

### Requirement: Marketplace index.json schema aligned with spec-driven-subagents

The marketplace `index.json` schema SHALL be extended with optional fields `version`, `tags`, `provides_requirements`, `checksum`, and `scope_hint`. The `provides_requirements` field SHALL share exact semantics with the SKILL.md frontmatter field defined in `add-spec-driven-cognition/specs/spec-driven-subagents/spec.md`. Missing fields MUST NOT break parsing; existing marketplaces with only `{name, description, files}` SHALL continue to work.

#### Scenario: Backward-compatible parsing
- **WHEN** an `index.json` contains only `{name, description, files}`
- **THEN** parsing MUST succeed
- **AND** the resulting `ResolvedSkill` MUST have empty `Version`, `Tags`, `ProvidesRequirements`, `Checksum`

#### Scenario: Provides-requirements propagation
- **WHEN** an `index.json` entry declares `provides_requirements: ["image_generation"]`
- **AND** `ResolveByRequirements(ctx, ["image_generation"])` is called
- **THEN** the entry MUST be included in the result

#### Scenario: Schema aligned with SKILL.md
- **WHEN** the same `provides_requirements` value appears in both the marketplace index entry and the downloaded SKILL.md frontmatter
- **THEN** the two MUST be semantically equivalent and consumable by the same `FindBySpecRequirements` / `ResolveByRequirements` logic

### Requirement: skill_install tool

The system SHALL register a `skill_install` MCP tool that accepts `{name, scope?, source?, refresh?}`. The tool SHALL resolve the skill (using `source` if provided, otherwise `Discovery.ResolveByName`), download it via `PullOne`, and register it via `Registry.RegisterFromPath` under the specified scope. Default scope SHALL be `personal`. The tool SHALL broadcast `skill.install.progress` events through the EventBus during execution. The tool SHALL only be registered when `agent.skills.on_demand_enabled: true`.

#### Scenario: Default personal install
- **WHEN** LLM calls `skill_install({"name": "nuwa"})` in a session with `UserID=alice`
- **THEN** the skill MUST be downloaded to `$HIVE_DATA/skills/users/alice/nuwa/`
- **AND** registered with `ScopePersonal` and `UserID=alice`
- **AND** `skill.install.progress` events MUST be broadcast with `SessionID` of the current session

#### Scenario: Explicit source override
- **WHEN** LLM calls `skill_install({"name": "nuwa", "source": "https://custom/"})`
- **THEN** the skill MUST be resolved and downloaded from `https://custom/`
- **AND** other configured `marketplace_urls` MUST NOT be queried

#### Scenario: Ambiguous name without source
- **WHEN** `nuwa` exists in multiple marketplaces and no `source` is provided
- **THEN** the tool MUST return an error payload listing all candidate sources
- **AND** MUST NOT download any skill

#### Scenario: on_demand_enabled=false hides the tool
- **WHEN** `agent.skills.on_demand_enabled` is `false`
- **THEN** the `skill_install` tool MUST NOT be registered on the MCP host
- **AND** existing tool list MUST remain unchanged from pre-change behavior

### Requirement: skill_install HITL via input_request channel

Invoking `skill_install` SHALL NOT rely on the `PermissionRule.Ask` flow because `add-spec-driven-cognition/specs/permission-minimalism` refactors `createPermissionPromptFn` to default `Granted: true` for non-shell tools (silently bypassing `Ask`). Instead, `skill_install` MUST obtain user approval via the business-decision HITL channel `input_request{choice_type: "skill_install_confirmation"}` retained by `permission-minimalism`. `scope=public` SHALL additionally require `AdminChecker.IsAdmin(ctx, userID)` to return true before emitting the approval request. `scope=personal` with empty `userID` MUST be rejected before any HITL prompt is raised. A decline or approval timeout SHALL abort the install and broadcast `skill.install.progress{stage: "error"}`.

#### Scenario: Personal scope triggers input_request
- **WHEN** authenticated user `alice` invokes `skill_install({"name": "nuwa", "scope": "personal"})`
- **THEN** the tool MUST emit `input_request{choice_type: "skill_install_confirmation", payload: {...}}` via the MCP host
- **AND** the tool MUST NOT rely on `PermissionRule.Ask`
- **AND** on `input_response{approved: true}` the install MUST proceed

#### Scenario: Public scope pre-blocked by AdminChecker
- **WHEN** non-admin `alice` invokes `skill_install({"name": "nuwa", "scope": "public"})`
- **THEN** the tool MUST return an error identifying the admin requirement
- **AND** no `input_request` MUST be emitted
- **AND** no download or registration MUST occur

#### Scenario: Public scope still requires user confirmation when admin
- **WHEN** `AdminChecker.IsAdmin(ctx, "ops-user")` returns true
- **AND** `skill_install({"name": "nuwa", "scope": "public"})` is invoked
- **THEN** the tool MUST still emit `input_request{choice_type: "skill_install_confirmation", payload: {scope: "public", admin_required: true}}`
- **AND** the install MUST proceed only after approval
- **AND** on approval the skill MUST be written under `$HIVE_DATA/skills/public/nuwa/`

#### Scenario: Personal scope blocked without userID
- **WHEN** a session has empty `UserID`
- **AND** `skill_install({"name": "nuwa", "scope": "personal"})` is invoked
- **THEN** the tool MUST return an error "personal scope requires authenticated session"
- **AND** no `input_request` MUST be emitted

#### Scenario: User decline aborts install
- **WHEN** `skill_install` emits `input_request` and receives `input_response{approved: false}`
- **THEN** the install MUST abort with a user-decline error
- **AND** a `skill.install.progress{stage: "error", reason: "user_declined"}` event MUST be broadcast

#### Scenario: Compatibility with permission-minimalism default-Granted refactor
- **WHEN** `add-spec-driven-cognition/permission-minimalism` is merged and `createPermissionPromptFn` returns `{Granted: true}` for `skill_install` by default
- **AND** user invokes `skill_install`
- **THEN** the HITL MUST still fire via `input_request{choice_type: "skill_install_confirmation"}`
- **AND** MUST NOT silently auto-execute

### Requirement: skill_search tool

The system SHALL register a `skill_search` MCP tool that accepts `{query, requirements?, scope?, limit?}`. The tool SHALL search locally installed skills (scoped by caller's `UserID`) and configured marketplaces, returning a merged list of candidates with source, scope, version, and overlap score. The tool SHALL have default permission `Allow` (read-only). The tool SHALL only be registered when `agent.skills.on_demand_enabled: true`.

#### Scenario: Combined local + marketplace search
- **WHEN** LLM calls `skill_search({"query": "image"})` for user `alice`
- **THEN** results MUST include matching skills from both `alice`'s personal skills and all marketplaces
- **AND** each entry MUST be tagged with its source (`local-personal` / `local-public` / marketplace URL)

#### Scenario: Requirements-based filter
- **WHEN** `skill_search({"requirements": ["image_generation"]})` is called
- **THEN** only skills declaring `image_generation` in `provides_requirements` MUST be returned

#### Scenario: Cross-tenant isolation in search
- **WHEN** `bob` calls `skill_search({"query": "custom"})`
- **AND** `alice` has a personal skill matching that query
- **THEN** `alice`'s personal skill MUST NOT appear in `bob`'s results

### Requirement: skill tool self-healing not-found response

When `agent.skills.on_demand_enabled: true` and `skill` tool's `registry.Get(name, userID)` fails, the tool SHALL invoke `Discovery.ResolveByName(ctx, name)`. If a candidate is found, the tool result SHALL include a structured `suggested_action` field advising LLM to call `skill_install`. If no candidate is found, the original `not found` error SHALL be returned unchanged. When `on_demand_enabled: false`, the not-found path SHALL remain strictly backward-compatible.

#### Scenario: Suggested action returned on remote hit
- **WHEN** `on_demand_enabled=true` and `skill("nuwa")` fails locally
- **AND** `Discovery.ResolveByName(ctx, "nuwa")` returns a candidate
- **THEN** the tool result MUST include `suggested_action: {tool: "skill_install", args: {name: "nuwa", scope: "personal"}, reason: "..."}`
- **AND** the original error message MUST still be present for LLMs that don't parse `suggested_action`

#### Scenario: Plain not-found when no candidate
- **WHEN** `on_demand_enabled=true` and `skill("unknown")` fails locally
- **AND** `Discovery.ResolveByName(ctx, "unknown")` returns no match
- **THEN** the tool result MUST be the unchanged `not found` error
- **AND** MUST NOT include `suggested_action`

#### Scenario: Feature flag off preserves legacy behavior
- **WHEN** `on_demand_enabled=false` and `skill("nuwa")` fails locally
- **THEN** the tool result MUST be identical to the pre-change `not found` error
- **AND** no marketplace query MUST be made

### Requirement: SpecSkillResolver aggregates local and remote lookup

The system SHALL introduce a `SpecSkillResolver` interface whose `Resolve(ctx, reqs, userID)` method SHALL first call `Skills.FindBySpecRequirements(reqs, userID)` (local Registry, defined by `spec-driven-subagents`), and only when no local skill satisfies the requirements SHALL it fall back to `Discovery.ResolveByRequirements(ctx, reqs)` (remote marketplace, defined by this change). This aggregator SHALL be the single entry point used by the spec planner; the planner MUST NOT call the underlying local/remote methods directly.

#### Scenario: Local hit short-circuits remote
- **WHEN** `Skills.FindBySpecRequirements` returns a non-empty local match for `["image_generation"]`
- **AND** `SpecSkillResolver.Resolve` is invoked
- **THEN** `Discovery.ResolveByRequirements` MUST NOT be called

#### Scenario: Local miss triggers remote fallback
- **WHEN** `Skills.FindBySpecRequirements` returns empty
- **AND** `on_demand_enabled: true` and `specdriven.skills_semantic_routing: true`
- **THEN** `Discovery.ResolveByRequirements(ctx, reqs)` MUST be called
- **AND** remote candidates MUST be returned as `SpecResolveResult.Remote`
- **AND** a `SuggestedAction` advising `skill_install` MUST be attached

#### Scenario: Remote routing disabled by feature flag
- **WHEN** `on_demand_enabled: false` OR `specdriven.skills_semantic_routing: false`
- **AND** local `FindBySpecRequirements` returns empty
- **THEN** `SpecSkillResolver.Resolve` MUST return an empty result WITHOUT calling any marketplace
- **AND** no network request MUST be made

#### Scenario: Method separation of concerns
- **WHEN** code review inspects `Skills.FindBySpecRequirements` implementation
- **THEN** it MUST NOT perform any network call
- **AND** `Discovery.ResolveByRequirements` implementation MUST NOT read the local `Registry`

### Requirement: SubAgent userID and AdminChecker inheritance

When a SubAgent is spawned with `SubAgent.Context["spec_ref"]` (as defined by `spec-driven-subagents`), the child SubAgent SHALL inherit the parent session's `UserID` and the parent's `AdminChecker` reference (same instance, not a copy). If the parent session has a non-empty `UserID` but the child is constructed without it, construction MUST fail with a clear error and a `subagent.userid.missing` metric MUST be incremented. A SubAgent that subsequently invokes `skill_install` or any Registry `Get(name, userID)` call MUST use the inherited `UserID`.

#### Scenario: Child inherits parent userID
- **WHEN** parent session has `UserID=alice`
- **AND** a SubAgent is spawned via `spec_ref`
- **THEN** the SubAgent `SessionState.UserID` MUST equal `alice`
- **AND** `Registry.Get(name, subAgent.UserID)` MUST see alice's personal skills

#### Scenario: Child inherits AdminChecker reference
- **WHEN** parent holds an `AdminChecker` instance with admin rules
- **AND** a SubAgent is spawned
- **AND** admin rules are hot-updated on the parent's `AdminChecker`
- **THEN** the SubAgent's `AdminChecker` MUST reflect the updated rules (same instance)

#### Scenario: Missing inheritance is blocked
- **WHEN** a code path constructs a SubAgent from a non-nil parent
- **AND** the constructor receives an empty `UserID` while parent `UserID` is non-empty
- **THEN** construction MUST fail
- **AND** `subagent.userid.missing` metric MUST be incremented

#### Scenario: SubAgent skill_install uses inherited identity
- **WHEN** a spec_ref SubAgent for parent `alice` invokes `skill_install({"name": "nuwa", "scope": "personal"})`
- **THEN** the resulting skill MUST be written under `$HIVE_DATA/skills/users/alice/nuwa/`
- **AND** `AdminChecker.IsAdmin` calls from the SubAgent MUST go through the parent-inherited instance

### Requirement: Configuration and startup validation

The system SHALL extend `Agent.Skills` with `MarketplaceURLs []string`, `OnDemandEnabled bool`, `PublicSkillsDir string`, `PersonalSkillsDir string`, and `PinnedVersions map[string]string`. If `OnDemandEnabled: true` and `MarketplaceURLs` is empty, bootstrap SHALL fail with a clear configuration error. Default values: `OnDemandEnabled: false`, `PublicSkillsDir: "$HIVE_DATA/skills/public"`, `PersonalSkillsDir: "$HIVE_DATA/skills/users"`.

#### Scenario: Misconfiguration detected at startup
- **WHEN** `on_demand_enabled: true` and `marketplace_urls: []`
- **THEN** bootstrap MUST fail with error message identifying the missing `marketplace_urls`

#### Scenario: Default config preserves legacy behavior
- **WHEN** no new config fields are set
- **THEN** `on_demand_enabled` MUST default to `false`
- **AND** no marketplace query MUST occur
- **AND** no new tools MUST be registered

#### Scenario: Custom skill dirs respected
- **WHEN** `public_skills_dir: "/opt/hive-skills"` is configured
- **THEN** Finder MUST scan `/opt/hive-skills` instead of the default path for public skills

### Requirement: Feature flag combination contract

The runtime behavior of skill discovery, planner routing, SubAgent inheritance, and HITL prompts SHALL be fully determined by the combination of four boolean feature flags: `specdriven.enabled`, `specdriven.subagent_mode`, `specdriven.skills_semantic_routing`, and `agent.skills.on_demand_enabled`. Bootstrap SHALL log the active combination at startup. For any valid combination, the system SHALL exhibit the behavior specified in this requirement's scenarios.

#### Scenario: All flags off preserves pre-change behavior
- **WHEN** `specdriven.enabled: false` and `on_demand_enabled: false`
- **THEN** spec planner MUST use name-based routing
- **AND** no marketplace queries MUST occur
- **AND** SubAgent inheritance checks MUST be skipped
- **AND** `skill_install` / `skill_search` tools MUST NOT be registered

#### Scenario: on_demand-only enables install but no semantic routing
- **WHEN** `specdriven.enabled: false` and `on_demand_enabled: true`
- **THEN** `skill_install` / `skill_search` tools MUST be registered
- **AND** spec planner MUST still use name-based routing
- **AND** `SpecSkillResolver` MUST NOT be consulted

#### Scenario: Full stack enabled with remote fallback
- **WHEN** `specdriven.enabled: true`, `subagent_mode: true`, `skills_semantic_routing: true`, `on_demand_enabled: true`
- **THEN** `SpecSkillResolver.Resolve` MUST be the single planner entry
- **AND** local miss MUST trigger remote `ResolveByRequirements`
- **AND** SubAgent inheritance (D16) MUST be enforced
- **AND** `skill_install` HITL MUST fire via `input_request`

#### Scenario: Semantic routing without on-demand blocks remote fallback
- **WHEN** `skills_semantic_routing: true` and `on_demand_enabled: false`
- **THEN** local `FindBySpecRequirements` MUST be consulted by `SpecSkillResolver`
- **AND** local miss MUST NOT fall back to marketplace
- **AND** planner MUST surface a "requirement unsatisfied" error to the LLM

#### Scenario: Startup logs active combination
- **WHEN** the Hive server starts
- **THEN** logs MUST contain a single line enumerating the values of all four flags
- **AND** operators MUST be able to grep this line for support diagnostics

### Requirement: EventBus progress events for skill installation

The `skill_install` tool SHALL emit `skill.install.progress` broadcast messages via `Master.BroadcastGenericMessage` or `BroadcastSessionMessage` during `resolving`, `awaiting_approval`, `downloading`, `registering`, `done`, and `error` stages. Events SHALL carry the current `SessionID` so that subscriber-side filters (WebSocket, IM EventRenderer) can route appropriately.

#### Scenario: Stage events emitted in order
- **WHEN** `skill_install({"name": "nuwa"})` is called and approved
- **THEN** the EventBus MUST receive events with `Stage` values in order: `resolving`, `awaiting_approval`, `downloading`, `registering`, `done`
- **AND** each event MUST carry the calling session's `SessionID`

#### Scenario: Error stage on failure
- **WHEN** `PullOne` fails mid-download
- **THEN** an event with `Stage: "error"` MUST be emitted
- **AND** the error message MUST be included in the payload

#### Scenario: Error stage on approval decline
- **WHEN** user responds to `input_request{choice_type: "skill_install_confirmation"}` with `approved: false`
- **THEN** an event with `Stage: "error"` and `reason: "user_declined"` MUST be emitted
- **AND** no download or registration MUST occur

#### Scenario: Cross-surface consumption
- **WHEN** a user has both a WebSocket frontend and a Feishu IM renderer subscribed
- **THEN** both MUST receive the same `skill.install.progress` events
- **AND** event payload shape MUST be identical across surfaces

### Requirement: Backward compatibility

All pre-change behaviors SHALL continue to work. Specifically: (a) existing `agent.skills.urls` startup-time bulk-pull SHALL be unaffected; (b) existing SKILL.md files without `scope`, `version`, or `provides_requirements` SHALL load as `ScopePublic` with empty `Version` and empty `ProvidesRequirements`; (c) existing `skill` tool callers SHALL see identical behavior when `on_demand_enabled: false`; (d) existing marketplaces serving old-schema `index.json` SHALL continue to be pullable via `Pull`.

#### Scenario: Legacy bulk-pull still works
- **WHEN** `agent.skills.urls: [A]` is configured
- **AND** A serves old-schema `index.json`
- **THEN** bootstrap MUST successfully bulk-pull all skills from A
- **AND** register them as `ScopePublic`

#### Scenario: Legacy SKILL.md loads unchanged
- **WHEN** a SKILL.md has no `scope`, `version`, or `provides_requirements` in frontmatter
- **THEN** the skill MUST be registered successfully with `ScopePublic`, empty version, empty requirements

#### Scenario: Feature-flag-off path is byte-identical
- **WHEN** `on_demand_enabled: false`
- **AND** the user invokes the `skill` tool with an unknown name
- **THEN** the returned error MUST match the pre-change format exactly (no `suggested_action`, no structured payload)

### Requirement: OverlayRegistry layered composition with tenant awareness

The system SHALL extend `internal/skills/overlay_registry.go:OverlayRegistry` to explicitly override its `Get`, `List`, and `RegisterFromPath` methods with the tenant-aware signatures defined in `Registry` (not relying on Go embedding, which does not re-dispatch newly-introduced method signatures). The `dbCache` field SHALL change from `map[string]*dbEntry` to `map[dbCacheKey]*dbEntry` where `dbCacheKey = struct { Name, UserID string }`. Personal DB skills MUST be stored with non-empty `UserID`; public DB skills MUST use empty `UserID`. `OverlayRegistry.Get(name, userID)` SHALL search in fixed priority order: (1) personal DB, (2) personal FS, (3) public DB, (4) public FS; the first match SHALL short-circuit and return.

#### Scenario: Four-layer priority for same-name skill
- **GIVEN** four entries all named `nuwa`: personal DB `nuwa@1` for alice, personal FS `nuwa@2` for alice, public DB `nuwa@3`, public FS `nuwa@4`
- **WHEN** `OverlayRegistry.Get("nuwa", "alice")` is called
- **THEN** the result MUST be personal DB `nuwa@1`
- **AND** neither personal FS nor any public layer MUST be consulted for the return value

#### Scenario: Cross-tenant DB isolation
- **GIVEN** `dbCache[{Name: "nuwa", UserID: "alice"}] = nuwa@personal-alice`
- **WHEN** `OverlayRegistry.Get("nuwa", "bob")` is called
- **THEN** the result MUST NOT be alice's personal skill
- **AND** the lookup MUST fall through to layers 2–4 without matching alice's entry

#### Scenario: Empty userID skips personal layers
- **WHEN** `OverlayRegistry.Get("nuwa", "")` is called
- **THEN** only layers (3) public DB and (4) public FS MUST be consulted
- **AND** any personal DB or personal FS entries MUST NOT be returned

#### Scenario: List merges four layers with override markers
- **WHEN** `OverlayRegistry.List("alice")` is called
- **THEN** the result MUST merge all four layers
- **AND** alice's personal entries MUST override same-named public entries
- **AND** DB entries MUST override same-scope FS entries
- **AND** each returned `SkillSummary` MUST include a `Source` field identifying `personal-db` / `personal-fs` / `public-db` / `public-fs`

#### Scenario: No regression via Go embedding
- **WHEN** any caller invokes `OverlayRegistry.Get(name, userID)` with both arguments
- **THEN** the tenant-aware override MUST handle the call (not the embedded `*Registry.Get`)
- **AND** a compile-time assertion test MUST verify `OverlayRegistry` explicitly declares `Get(string, string) (*Skill, error)` rather than relying on the embedded method

### Requirement: skill_install_confirmation choice_type registration

The system SHALL register a new `choice_type` named `skill_install_confirmation` with the global registry defined by `hitl-choice-type-registry/spec.md`, using `master.RegisterChoiceType` or `master.MustRegisterChoiceType`. Registration MUST NOT be added to `internal/master/choice_type_registry.go`'s `init()` function (built-in list), because `hitl-choice-type-registry/spec.md` explicitly forbids that. Registration SHALL occur in this change's own code — either in an `init()` of `internal/tools/skill_install.go` or during bootstrap before any `skill_install` handler can emit HITL. The registration MUST occur before the first `skill_install` handler invocation; otherwise `Master.EmitInputRequest` at `internal/master/host_emit.go:31-87` will return `ErrUnregisteredChoiceType`.

#### Scenario: Registration is performed by this change, not built-in
- **WHEN** only `hitl-choice-type-registry` is merged (this change is absent)
- **THEN** `IsRegisteredChoiceType("skill_install_confirmation")` MUST return false
- **AND** `ListChoiceTypes()` MUST NOT include `skill_install_confirmation`

#### Scenario: Registration occurs before first HITL emission
- **WHEN** this change is merged
- **AND** the Hive server starts up
- **THEN** `IsRegisteredChoiceType("skill_install_confirmation")` MUST return true before any `skill_install` tool handler is first invoked
- **AND** the registered `ChoiceTypeSpec` MUST include a `PayloadHint` documenting `name`, `scope`, `source`, and `admin_required` keys

#### Scenario: skill_install emits HITL without runtime failure
- **WHEN** authenticated user `alice` invokes `skill_install({"name": "nuwa", "scope": "personal"})`
- **AND** the handler calls `host.EmitInputRequest(ctx, InputRequest{ChoiceType: "skill_install_confirmation", ...})`
- **THEN** the call MUST NOT return `ErrUnregisteredChoiceType`
- **AND** the HITL event MUST reach the channel-native UI

#### Scenario: Test-level guard against registration removal
- **WHEN** the first case of `skill_install_test.go` runs
- **THEN** it MUST assert `master.IsRegisteredChoiceType("skill_install_confirmation") == true` before any HITL path is exercised
- **AND** removal of the registration init MUST cause this test to fail immediately

### Requirement: SkillService tenant-aware pg_notify

The `skills` database table SHALL add a nullable `user_id VARCHAR` column where NULL represents a public skill and a non-empty string identifies the owning user for a personal skill. A unique index SHALL be created on `(name, COALESCE(user_id, ''), version)` to prevent collision between same-named skills from different users. The `pg_notify` trigger SHALL emit payload `{name, user_id, version, op}` (where `user_id` is `''` for public). `SkillService` SHALL parse the `user_id` field and update `OverlayRegistry.dbCache` using the composite key `{Name, UserID}`. Legacy payloads without `user_id` MUST be parsed as public (backward compatible).

#### Scenario: Two users push same-named personal skill concurrently
- **GIVEN** `skills` table contains entries `(nuwa, alice, 1.0.0)` and `(nuwa, bob, 1.0.0)`
- **WHEN** both entries trigger `pg_notify` concurrently
- **THEN** `dbCache` MUST contain two distinct entries keyed by `{nuwa, alice}` and `{nuwa, bob}`
- **AND** `Get("nuwa", "alice")` and `Get("nuwa", "bob")` MUST return independent skills without cross-contamination

#### Scenario: Unique index blocks duplicate insert
- **WHEN** a second `INSERT INTO skills (name, user_id, version) VALUES ('nuwa', 'alice', '1.0.0')` is attempted
- **THEN** the DB MUST reject the duplicate via the unique index violation

#### Scenario: Legacy payload parses as public
- **WHEN** `pg_notify` emits `{name: "nuwa", op: "UPDATE"}` (no `user_id` field, legacy deploy)
- **THEN** `SkillService` MUST interpret this as a public skill update
- **AND** update `dbCache[{Name: "nuwa", UserID: ""}]`
- **AND** MUST NOT error

#### Scenario: Public skill uses empty user_id
- **WHEN** an admin inserts a public skill via `skill_install({"scope": "public"})`
- **THEN** the row's `user_id` column MUST be NULL
- **AND** the pg_notify payload MUST carry `user_id: ''` (empty string)
- **AND** the cache MUST update at `{Name: name, UserID: ""}`

### Requirement: AdminChecker concurrent safety contract

The `AdminChecker` interface defined at `internal/skills/admin.go` SHALL document that all implementations MUST be safe for concurrent invocation from multiple goroutines. The interface documentation SHALL explicitly state: "IsAdmin MUST be safe for concurrent invocation from multiple goroutines; implementations SHOULD use `sync.RWMutex` or `atomic.Pointer` to protect mutable state". The default `denyAllAdminChecker` implementation SHALL be stateless and therefore trivially goroutine-safe. Production implementations with mutable rule state (e.g., DB-backed rule caches) SHALL protect the cache with `atomic.Pointer[ruleSet]` or `sync.RWMutex`. This is required because D16 mandates SubAgents inherit the parent's `AdminChecker` by reference (same instance) to enable live rule updates.

#### Scenario: Interface documentation declares goroutine-safety
- **WHEN** the `AdminChecker` interface is read
- **THEN** its doc comment MUST contain the phrase "safe for concurrent invocation" or equivalent
- **AND** the phrase MUST be visible in `go doc internal/skills.AdminChecker` output

#### Scenario: Default implementation passes race detector
- **WHEN** `go test -race -count=100 ./internal/skills -run TestAdminCheckerConcurrent` is run
- **THEN** no data race MUST be reported
- **AND** concurrent `IsAdmin` invocations across 100 goroutines MUST all return consistent values

#### Scenario: Hot rule update visible to SubAgent
- **GIVEN** parent session holds an `AdminChecker` implementation with `atomic.Pointer[ruleSet]`
- **WHEN** a SubAgent is spawned and inherits the parent's AdminChecker reference
- **AND** the parent swaps in a new rule set via `atomic.Pointer.Store`
- **THEN** the SubAgent's subsequent `IsAdmin(ctx, userID)` call MUST observe the updated rules
- **AND** no locking or re-spawn MUST be required

### Requirement: skill_install stage-worker goroutine leak bounds

All worker goroutines spawned during the six stages of `skill_install` (resolving, awaiting_approval, downloading, registering, done, error) SHALL monitor `ctx.Done()` and exit promptly when the context is cancelled. The handler's blocking wait on `host.EmitInputRequest` SHALL use the context-aware API provided by `Master.EmitInputRequest` (which already supports cancellation at `internal/master/host_emit.go`). The `skill_install_test.go` suite SHALL wrap each test case with `goleak.VerifyNone(t)` to ensure no goroutines remain after the test completes, with explicit coverage for three paths: (a) user-decline short-circuit, (b) approval timeout, (c) mid-download context cancellation.

#### Scenario: User decline does not leak goroutines
- **WHEN** a test emits `skill_install` → receives `input_response{approved: false}`
- **THEN** after the handler returns and `goleak.VerifyNone(t)` runs
- **AND** no goroutines spawned by the handler MUST remain

#### Scenario: Approval timeout does not leak goroutines
- **WHEN** a test emits `skill_install` and the `input_request` times out
- **THEN** `goleak.VerifyNone(t)` MUST pass
- **AND** no background download or EventBus broadcast goroutine MUST continue

#### Scenario: Mid-download ctx cancel stops all workers
- **WHEN** a test calls `skill_install` with an explicit `ctx`, then cancels the ctx during the `downloading` stage
- **THEN** the HTTP download goroutine MUST observe `ctx.Done()` and exit
- **AND** any in-flight EventBus broadcast MUST not block indefinitely
- **AND** `goleak.VerifyNone(t)` MUST pass within 500ms after cancellation

### Requirement: Host layer EmitInputRequest transfer with SessionID injection

The system SHALL add a pass-through method `func (h *Host) EmitInputRequest(ctx context.Context, req master.InputRequest) (*master.InputResponse, error)` on `internal/mcphost/host.go`. The method SHALL inject the caller's `SessionID` into `req.SessionID` (extracted from `ctx` via a helper such as `sessionIDFromCtx(ctx)`) before delegating to `h.Master.EmitInputRequest(ctx, req)`. The `Host` struct SHALL hold a reference to `*master.Master` (or an `HITLEmitter` interface to avoid import cycles), injected during `NewHost` construction at bootstrap time. The underlying `Master.EmitInputRequest` is already fully implemented at `internal/master/host_emit.go:31-87` by the `hitl-choice-type-registry` change; this requirement only wires up the Host-layer pass-through.

#### Scenario: Host method injects SessionID before delegation
- **WHEN** `skill_install` handler calls `host.EmitInputRequest(ctx, InputRequest{ChoiceType: "skill_install_confirmation"})` with `req.SessionID` empty
- **AND** `ctx` carries a session ID via the standard context key
- **THEN** the call to `Master.EmitInputRequest` MUST receive `req.SessionID` set to the extracted session ID
- **AND** the channel-native UI MUST be able to route the HITL event to the correct session

#### Scenario: Caller-provided SessionID is preserved
- **WHEN** caller explicitly sets `req.SessionID = "explicit-session"` before calling `host.EmitInputRequest`
- **THEN** the explicit value MUST be preserved and passed through without modification

#### Scenario: NewHost construction requires Master reference
- **WHEN** `NewHost` is invoked without a Master reference (nil)
- **THEN** construction MUST fail (or use an explicit no-op HITL emitter that returns a clear "HITL not configured" error on any EmitInputRequest call)
- **AND** production bootstrap at `internal/bootstrap/server.go` MUST always pass the real Master

### Requirement: Flag combination startup validation

Bootstrap SHALL run `validateFlagCombination(cfg)` before printing the active feature flag combination. The validator SHALL reject the six combinations where `specdriven.subagent_mode` or `specdriven.skills_semantic_routing` is true but `specdriven.enabled` is false, because these are dependency violations. The validator SHALL emit a clear error identifying the missing prerequisite. The ten valid combinations (2 with `specdriven.enabled=false, subagent_mode=false, semantic_routing=false, on_demand∈{false,true}`; 8 with `specdriven.enabled=true` × `subagent_mode∈{false,true}` × `semantic_routing∈{false,true}` × `on_demand∈{false,true}`) SHALL pass validation. The union of valid + invalid equals all 2⁴ = 16 bool combinations.

#### Scenario: subagent_mode without specdriven.enabled rejected
- **WHEN** `specdriven.enabled: false` and `specdriven.subagent_mode: true`
- **THEN** bootstrap MUST fail with an error message identifying "subagent_mode requires specdriven.enabled: true"

#### Scenario: skills_semantic_routing without specdriven.enabled rejected
- **WHEN** `specdriven.enabled: false` and `specdriven.skills_semantic_routing: true`
- **THEN** bootstrap MUST fail with an error message identifying "skills_semantic_routing requires specdriven.enabled: true"

#### Scenario: All ten valid combinations pass
- **WHEN** bootstrap is run with each of the ten valid combinations (`specdriven.enabled=false` + `subagent_mode=false` + `semantic_routing=false` with either `on_demand` value, plus all 8 combos under `specdriven.enabled=true`)
- **THEN** each run MUST succeed
- **AND** each run MUST print an active combination log line matching the grep contract `skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W`
- **AND** the total valid+invalid count MUST equal 2⁴ = 16 with no combination double-counted

