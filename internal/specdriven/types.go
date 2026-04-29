// Package specdriven 定义 spec-driven cognition 的核心数据类型。
// 本包的其它子包（planner/continuation/intake/eval）共享这些类型。
// 详见 openspec/changes/harden-spec-driven-phase2/design.md。
package specdriven

import "time"

// SessionSpecState 持久化到 hive_spec_session_state 表（keyed by session_id）。
// 仅在 user ingress 路径修改（session_loop.go:712 processTask 入口），
// subagent / 后台路径禁止写入（参见 hidden-spec-layer spec.md MODIFIED 段）。
type SessionSpecState struct {
	ActiveChangeID string               `json:"active_change_id,omitempty"`
	FocusMRU       []string             `json:"focus_mru,omitempty"`
	Changes        map[string]ChangeRef `json:"changes,omitempty"`
}

// ChangeRef 是 SessionSpecState 对某个 change 的引用。
// 不是 canonical——canonical 在 hive_spec_changes 表中，修订号 CAS 保护。
type ChangeRef struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Title       string    `json:"title,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	LastTaskKey string    `json:"last_task_key,omitempty"`
	LastTouched time.Time `json:"last_touched"`
}

// Context 是运行时 specCtx 的形状，通过 atomic.Pointer 挂在 SessionState 上。
// 发布后 IMMUTABLE，更新必须分配新 Context 并 Store()。
type Context struct {
	ChangeID    string
	CurrentTaskKey string
	Revision    int
}

// Plan 是 planner 输出的严格 schema。
// task_key 强制 string，禁止数字（防 1.10/1.1 坍塌）。
type Plan struct {
	Steps []PlanStep `json:"steps"`
}

// PlanStep 是 Plan 的单步。
type PlanStep struct {
	TaskKey  string `json:"task_key"`
	ToolName string `json:"tool_name"`
	// Args 故意不解析，保留 raw 给下游工具层处理
	Args any `json:"args,omitempty"`
}

// Decision 是 continuation 或 intake 的决策结果。
type Decision struct {
	Kind      DecisionKind `json:"kind"`
	ChangeID  string       `json:"change_id,omitempty"`
	AskReason string       `json:"ask_reason,omitempty"`
}

// DecisionKind 枚举 continuation 的 3 种决策。
type DecisionKind string

const (
	DecisionResume DecisionKind = "resume"
	DecisionAsk    DecisionKind = "ask"
	DecisionNew    DecisionKind = "new"
)
