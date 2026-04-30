package agentquality

import "time"

type EventName string

const (
	EventAgentTurn          EventName = "quality.agent_turn"
	EventToolDecision       EventName = "quality.tool_decision"
	EventContextBuild       EventName = "quality.context_build"
	EventPermissionDecision EventName = "quality.permission_decision"
	EventDelegation         EventName = "quality.delegation"
)

type FailureType string

const (
	FailureNone       FailureType = "none"
	FailurePrompt     FailureType = "prompt"
	FailureTool       FailureType = "tool"
	FailureSkill      FailureType = "skill"
	FailureContext    FailureType = "context"
	FailureModel      FailureType = "model"
	FailurePermission FailureType = "permission"
	FailureRuntime    FailureType = "runtime"
	FailureUserInput  FailureType = "user_input"
)

type FinalStatus string

const (
	StatusPass      FinalStatus = "pass"
	StatusFail      FinalStatus = "fail"
	StatusBlocked   FinalStatus = "blocked"
	StatusNeedsUser FinalStatus = "needs_user"
)

type Decision string

const (
	DecisionExpected   Decision = "expected"
	DecisionAllowed    Decision = "allowed"
	DecisionUnexpected Decision = "unexpected"
	DecisionRejected   Decision = "rejected"
)

type PromptRef struct {
	Key      string `json:"key,omitempty"`
	Version  string `json:"version,omitempty"`
	Source   string `json:"source,omitempty"`
	Language string `json:"language,omitempty"`
}

type ToolDecision struct {
	Expected []string `json:"expected,omitempty"`
	Actual   string   `json:"actual,omitempty"`
	Decision Decision `json:"decision,omitempty"`
	ArgsHash string   `json:"args_hash,omitempty"`
}

type Delegation struct {
	ParentTraceID string   `json:"parent_trace_id,omitempty"`
	ChildTraceID  string   `json:"child_trace_id,omitempty"`
	AgentID       string   `json:"agent_id,omitempty"`
	AgentType     string   `json:"agent_type,omitempty"`
	GroupID       string   `json:"group_id,omitempty"`
	SpawnDepth    int      `json:"spawn_depth,omitempty"`
	MaxTurns      int      `json:"max_turns,omitempty"`
	ToolWhitelist []string `json:"tool_whitelist,omitempty"`
	StopReason    string   `json:"stop_reason,omitempty"`
}

type ContextBuild struct {
	MessageCount       int      `json:"message_count"`
	Compressed         bool     `json:"compressed"`
	MemoryInjected     bool     `json:"memory_injected"`
	MemoryIDs          []int64  `json:"memory_ids,omitempty"`
	SkippedMemoryIDs   []int64  `json:"skipped_memory_ids,omitempty"`
	SkippedExpired     int      `json:"skipped_expired,omitempty"`
	SkippedLowTrust    int      `json:"skipped_low_trust,omitempty"`
	SkippedCrossUser   int      `json:"skipped_cross_user,omitempty"`
	SkippedTokenBudget int      `json:"skipped_token_budget,omitempty"`
	SkippedMemoryTotal int      `json:"skipped_memory_total,omitempty"`
	AttachmentCount    int      `json:"attachment_count"`
	PromptVersions     []string `json:"prompt_versions,omitempty"`
	EstimatedTokens    int      `json:"estimated_tokens,omitempty"`
	ContaminationCheck string   `json:"contamination_check,omitempty"`
}

type Event struct {
	Name          EventName      `json:"name"`
	CaseID        string         `json:"case_id,omitempty"`
	SessionIDHash string         `json:"session_id_hash,omitempty"`
	Route         string         `json:"route,omitempty"`
	Prompt        PromptRef      `json:"prompt,omitempty"`
	ToolDecision  ToolDecision   `json:"tool_decision,omitempty"`
	ContextBuild  ContextBuild   `json:"context_build,omitempty"`
	Delegation    Delegation     `json:"delegation,omitempty"`
	FailureType   FailureType    `json:"failure_type,omitempty"`
	RetryReason   string         `json:"retry_reason,omitempty"`
	FinalStatus   FinalStatus    `json:"final_status,omitempty"`
	ReplayRef     string         `json:"replay_ref,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`
	Ts            time.Time      `json:"ts"`
}

func MetricLabels(ev Event) map[string]any {
	labels := map[string]any{
		"route":        emptyAsUnknown(ev.Route),
		"failure_type": emptyAsUnknown(string(ev.FailureType)),
		"status":       emptyAsUnknown(string(ev.FinalStatus)),
	}
	if ev.ToolDecision.Actual != "" {
		labels["tool_name"] = ev.ToolDecision.Actual
	}
	if ev.ToolDecision.Decision != "" {
		labels["decision"] = ev.ToolDecision.Decision
	}
	if ev.RetryReason != "" {
		labels["retry_reason"] = ev.RetryReason
	}
	return labels
}

func emptyAsUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
