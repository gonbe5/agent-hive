// Package journal 提供结构化的 Agent 会话开发日志。
// 每个会话记录做了什么、调用了哪些工具、改了哪些文件、为什么做出关键决策。
package journal

import (
	"context"
	"errors"
	"time"
)

// ErrJournalNotAvailable journal 功能未启用时返回此 sentinel error
var ErrJournalNotAvailable = errors.New("journal not available")

// Journal 是开发日志的核心接口。
// 所有实现必须是 nil 安全的（调用方可直接 if j != nil { j.Xxx() }）。
type Journal interface {
	// StartSession 开始一个新的会话日志
	StartSession(ctx context.Context, sessionID string, task string) error

	// LogToolCall 记录一次工具调用
	LogToolCall(ctx context.Context, entry ToolCallEntry) error

	// LogFileChange 记录一次文件变更
	LogFileChange(ctx context.Context, entry FileChangeEntry) error

	// LogDecision 记录一次关键决策
	LogDecision(ctx context.Context, entry DecisionEntry) error

	// EndSession 结束会话日志并写入摘要
	EndSession(ctx context.Context, sessionID string, summary string) error

	// GetJournal 查询指定会话的完整日志（limit<=0 表示不限制）
	GetJournal(ctx context.Context, sessionID string, limit int) (*SessionJournal, error)

	// DeleteSession 删除指定会话的所有日志数据（级联清理）
	DeleteSession(ctx context.Context, sessionID string) error

	// GetJournalEvents 返回统一事件流（UNION ALL 合并三表，按时间排序）。
	// limit<=0 表示不限制；after 非零时只返回 timestamp > after 的事件（增量查询）。
	GetJournalEvents(ctx context.Context, sessionID string, limit int, after time.Time) ([]JournalEvent, error)

	// GetJournalStats 批量查询多个 session 的 journal 统计摘要（画廊页用）
	GetJournalStats(ctx context.Context, sessionIDs []string) (map[string]*JournalStats, error)
}

// ToolCallEntry 工具调用日志条目
type ToolCallEntry struct {
	SessionID  string        `json:"session_id"`
	ToolName   string        `json:"tool_name"`
	ToolCallID string        `json:"tool_call_id"`
	Arguments  string        `json:"arguments,omitempty"` // JSON 字符串，可截断
	Result     string        `json:"result,omitempty"`    // 结果摘要，可截断
	IsError    bool          `json:"is_error,omitempty"`
	Duration   time.Duration `json:"duration"`
	Timestamp  time.Time     `json:"timestamp"`
}

// FileChangeEntry 文件变更日志条目
type FileChangeEntry struct {
	SessionID string    `json:"session_id"`
	FilePath  string    `json:"file_path"`
	Action    string    `json:"action"` // "create", "edit", "delete"
	Summary   string    `json:"summary,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// DecisionEntry 关键决策日志条目
type DecisionEntry struct {
	SessionID string    `json:"session_id"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SessionJournal 一个会话的完整日志
type SessionJournal struct {
	SessionID   string            `json:"session_id"`
	Task        string            `json:"task"`
	Summary     string            `json:"summary,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	EndedAt     *time.Time        `json:"ended_at,omitempty"`
	ToolCalls   []ToolCallEntry   `json:"tool_calls,omitempty"`
	FileChanges []FileChangeEntry `json:"file_changes,omitempty"`
	Decisions   []DecisionEntry   `json:"decisions,omitempty"`
}

// JournalEvent 统一事件类型（三类事件合并后的扁平结构，用于回放时间线）
type JournalEvent struct {
	Type      string    `json:"type"` // "tool_call" | "file_change" | "decision"
	Timestamp time.Time `json:"timestamp"`
	// tool_call 字段
	ToolName   string `json:"tool_name,omitempty"`
	Arguments  string `json:"arguments,omitempty"`
	Result     string `json:"result,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	// file_change 字段
	FilePath string `json:"file_path,omitempty"`
	Action   string `json:"action,omitempty"`
	Summary  string `json:"summary,omitempty"`
	// decision 字段
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// JournalStats 画廊页统计摘要（批量查询用）
type JournalStats struct {
	ToolCallCount         int        `json:"tool_call_count"`
	FileChangeCount       int        `json:"file_change_count"`
	DecisionCount         int        `json:"decision_count"`
	StartedAt             time.Time  `json:"started_at"`
	EndedAt               *time.Time `json:"ended_at,omitempty"`
	HasError              bool       `json:"has_error"`
	QualityErrorCount     int        `json:"quality_error_count,omitempty"`
	DangerousCount        int        `json:"dangerous_count,omitempty"`
	DelegationCount       int        `json:"delegation_count,omitempty"`
	ACPCount              int        `json:"acp_count,omitempty"`
	ContextPollutionCount int        `json:"context_pollution_count,omitempty"`
}
