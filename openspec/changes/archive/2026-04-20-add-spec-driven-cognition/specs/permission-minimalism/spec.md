## ADDED Requirements

### Requirement: Default-allow permission gate

The system SHALL reverse `createPermissionPromptFn` in `internal/master/lifecycle.go:163-236` so that its DEFAULT return is `PermissionResponse{Granted: true}` for all tool calls, file I/O, search operations, and MCP invocations. The only paths that MAY deny or ask are (a) bash-family tool calls that `security.SafeExecutor.MatchPolicy` classifies as `PolicyDeny` / `PolicyAsk`, and (b) explicit business-decision `input_request` events emitted by Skills/Tools (e.g., `account_selector`, `ambiguity_clarification`).

The gate SHALL only invoke `MatchPolicy` when `req.ToolName` is a shell-family tool. Shell-family tools SHALL be defined as: the tool `bash` (the only currently-registered built-in shell tool, per `internal/tools/tools.go:753`) PLUS any tool name listed in the config array `security.shell_tool_names` (default empty; extensible without code change). For all other ToolNames (`write_file`, `read_file`, `generate_image`, `search_files`, `WebFetch`, etc.), the gate SHALL skip `MatchPolicy` entirely and return `{Granted: true}`, because `PermissionRequest.Input` is `json.RawMessage` of structured tool args and MUST NOT be regex-matched as shell command text.

For shell-family tools, the gate SHALL extract the command string from `req.Input` by JSON-unmarshaling into a local struct `{Command string }` (or equivalent wrapper matching the bash tool's known schema). If unmarshaling fails, the gate SHALL deny safe-by-default and log a warn (this is a defensive branch, not expected in normal operation).

#### Scenario: Normal bash command passes silently
- **WHEN** an Agent invokes `bash: ls -la *.log && tar czf logs.tar.gz *.log`
- **THEN** `createPermissionPromptFn` MUST invoke `SafeExecutor.MatchPolicy(cmd)` first
- **AND** if the policy is `PolicyAllow` (the default for non-matching commands) the function MUST return `{Granted: true}` with zero UI prompt
- **AND** the end user MUST NOT see any permission dialog

#### Scenario: Search and read-only tool calls always pass
- **WHEN** an Agent invokes any of `search_files`, `read_file`, `list_dir`, `grep`, `WebFetch`, `WebSearch`
- **THEN** the gate MUST return `{Granted: true}` without invoking `MatchPolicy` (these tools' ToolName is not in the shell-family set, and their `Input` is structured JSON not shell text)
- **AND** latency overhead MUST be < 1ms

#### Scenario: Non-shell tool with JSON input never regex-matched
- **GIVEN** `req.ToolName = "write_file"` and `req.Input = {"path": "/tmp/x.txt", "content": "rm -rf /"}`
- **WHEN** the gate evaluates the request
- **THEN** the gate MUST NOT invoke `MatchPolicy` (ToolName is not in shell-family)
- **AND** MUST return `{Granted: true}` (the string `rm -rf /` inside file content is data, not a command to execute)
- **AND** a false positive from regex-matching structured JSON MUST be impossible

#### Scenario: Write operations pass unless destructive
- **WHEN** an Agent invokes `write_file` or `edit_file` on a non-system path
- **THEN** the gate MUST return `{Granted: true}`

### Requirement: Destructive check runs before IM short-circuit

The system SHALL reorder `createPermissionPromptFn` so that `SafeExecutor.MatchPolicy` is evaluated BEFORE the `strings.HasPrefix(sessionID, "im-")` auto-allow branch at `internal/master/lifecycle.go:170`. When the policy is `PolicyDeny`, the gate MUST abort regardless of session type; when `PolicyAsk`, IM sessions MUST still auto-allow (IM has no approval UI and user intent is inferred from message sending).

#### Scenario: IM session cannot bypass PolicyDeny
- **GIVEN** session `im-wechatbot-user42`
- **WHEN** the Agent attempts `rm -rf /*` or `mkfs.ext4 /dev/sda`
- **THEN** `MatchPolicy` MUST return `PolicyDeny`
- **AND** the gate MUST return `{Granted: false}` with structured error `destructive_command_blocked`
- **AND** the audit log MUST record the block with `session_id`, pattern, and hashed command
- **AND** the metric `security.policy_deny_total{pattern=<name>}` MUST increment

#### Scenario: IM session still auto-allows PolicyAsk commands
- **GIVEN** session `im-feishu-user7`
- **WHEN** the Agent attempts `rm -rf /tmp/build-cache` (matches `PolicyAsk` recursive delete rule)
- **THEN** `MatchPolicy` MUST return `PolicyAsk`
- **AND** because the session prefix is `im-`, the gate MUST log a warn and return `{Granted: true}`
- **AND** the metric `security.policy_ask_im_autoallow_total` MUST increment for observability

#### Scenario: Non-IM session surfaces PolicyAsk to HITL
- **GIVEN** session `web-dashboard-s12`
- **WHEN** the Agent attempts `DROP TABLE orders`
- **THEN** `MatchPolicy` MUST return `PolicyAsk`
- **AND** the gate MUST enter the existing HITL flow (register pending input, broadcast to channel, wait up to 60 minutes)

### Requirement: Reuse existing SafeExecutor, do not duplicate

The system SHALL NOT introduce a new `internal/security/destructive_guard.go` or any parallel blocklist. All destructive-command matching SHALL reuse `internal/security/builtin_rules.go:BuiltinDangerousRules` and `internal/security/exec.go:SafeExecutor.MatchPolicy`. New patterns SHALL be added to `BuiltinDangerousRules` directly or appended via `config.json > security.destructive_patterns` (which `NewSafeExecutor` already composes).

The system SHALL promote the currently local variable `safeExec` at `internal/master/master.go:340` and `master.go:1211` to a `Master` struct field `safeExecutor *security.SafeExecutor`, populated during `New` and refreshed during hot-reload (`master.go:1211` already re-creates on reload and SHALL update the field in place). `createPermissionPromptFn` SHALL reference `m.safeExecutor.MatchPolicy(...)` rather than re-constructing a SafeExecutor per request.

#### Scenario: No duplicate rule source
- **WHEN** this change is merged
- **THEN** the repository MUST contain exactly one authoritative destructive-pattern list (`BuiltinDangerousRules`)
- **AND** grep for `rm -rf /` across `internal/` MUST return a single definition site
- **AND** the gate at `lifecycle.go` MUST call the shared `SafeExecutor` instance, not a forked matcher

#### Scenario: Builtin rule count matches source of truth
- **WHEN** any doc (proposal.md, README, tasks.md, design.md) references the count of `BuiltinDangerousRules`
- **THEN** the referenced number MUST equal the exact entry count in `internal/security/builtin_rules.go`
- **AND** at time of `harden-spec-driven-phase2` merge the canonical count is **19** (previously misdocumented as 20 in the original `add-spec-driven-cognition` proposal)
- **AND** any PR that changes the entry count in `builtin_rules.go` MUST update every doc reference in the same PR, enforced by a pre-merge grep check

#### Scenario: Operator adds custom pattern via config
- **WHEN** `config.json` contains `"security": { "destructive_patterns": [{"pattern": "aws s3 rb --force", "policy": "deny"}] }`
- **THEN** `NewSafeExecutor` MUST compose builtin + custom rules (builtin first, custom appended)
- **AND** attempting to override a builtin pattern's policy via config MUST NOT weaken it (builtin match wins due to ordering)
- **AND** the warn log `security.user_rule_shadowed_by_builtin` MUST fire if a user pattern duplicates a builtin

### Requirement: Business-decision HITL preserved orthogonally

The system SHALL preserve HITL for explicit business-decision `input_request` events where a Skill or Tool emits an ask whose `choice_type` is **registered via the `choice_type_registry`** (see `hitl-choice-type-registry` change). At boot time the registry contains the three built-in values `account_selector`, `ambiguity_clarification`, and `confirmation_before_irreversible_business_action`; downstream changes (e.g. `hive-skill-on-demand`) MAY register additional values through `RegisterChoiceType`. These asks SHALL surface via channel-native UI (Feishu card, WeChat text, Web UI prompt) and SHALL NOT be confused with `SafeExecutor.MatchPolicy` policy returns.

#### Scenario: Account selector HITL still fires
- **GIVEN** a user has bound multiple Xiaohongshu accounts
- **WHEN** a Skill emits `input_request{choice_type: "account_selector", options: [A, B, C]}`
- **THEN** the gate MUST transition to HITL flow (NOT auto-approve)
- **AND** the user MUST receive a channel-native choice prompt

#### Scenario: Ambiguity clarification HITL still fires
- **WHEN** Master detects intent ambiguity and emits `input_request{choice_type: "ambiguity_clarification"}`
- **THEN** the gate MUST surface the clarification to the user
- **AND** MUST NOT auto-answer

### Requirement: Audit logging on policy deny

The system SHALL log every `PolicyDeny` match as a structured warn-level entry with `change_id` (if set), `session_id`, pattern description, and hashed command (not full command to avoid logging secrets).

#### Scenario: Deny produces audit line
- **WHEN** `SafeExecutor.MatchPolicy` returns `PolicyDeny`
- **THEN** logs MUST contain `zap.Warn("destructive command blocked", zap.String("pattern", desc), zap.String("session_id", sid), zap.String("cmd_hash", h), zap.String("change_id", cid))`
- **AND** MUST NOT include the raw command text

### Requirement: Feature flag for rollback

The system SHALL gate the permission-minimalism behavior behind `config.json > security.permission_mode` with values `minimal` (default) and `strict`. In `strict` mode the gate reverts to the pre-change behavior (HITL for most tool calls). This is the one-line rollback path.

#### Scenario: Strict mode restores legacy HITL
- **GIVEN** `security.permission_mode = "strict"`
- **WHEN** any tool call arrives
- **THEN** the gate MUST use the pre-change flow (no default-allow)
- **AND** the change MUST be reversible without a code rollback
- **AND** `PolicyDeny` matches MUST still block even in strict mode (strict reverts default-allow, NOT the destructive blocklist)

### Requirement: IM PolicyAsk auto-allow is a known risk with SLO

The system SHALL document that IM-session `PolicyAsk` auto-allow (e.g., `rm -rf /tmp/build-cache` sent from WeChat) is an INTENTIONAL tradeoff: IM channels have no approval UI and blocking every recursive delete would break common user flows (log cleanup, cache clearing). This risk SHALL be bounded by a configurable SLO: the alert threshold SHALL be read from `config.json > security.im_autoallow_alert_threshold_per_day_per_session` (default 10, allowing high-frequency ops groups to tune upward without code change). Crossing the threshold MUST trigger an operator alert via the existing observability pipeline.

#### Scenario: High-frequency PolicyAsk auto-allow alerts
- **GIVEN** `security.im_autoallow_alert_threshold_per_day_per_session = 10` (default) and a session issues 11 `PolicyAsk` commands within 24 hours, all auto-allowed via the IM branch
- **WHEN** the metric threshold is crossed
- **THEN** an alert `security.policy_ask_im_autoallow_rate_exceeded{session_id}` MUST fire
- **AND** the operator MUST have a documented runbook entry to investigate the session's command history

#### Scenario: High-frequency ops group tunes threshold upward
- **GIVEN** `security.im_autoallow_alert_threshold_per_day_per_session = 50` in `config.json`
- **WHEN** a session issues 30 `PolicyAsk` commands in 24 hours
- **THEN** NO alert MUST fire (below the configured threshold)
- **AND** the raw counter MUST still increment for observability
