// Package taskboard 提供持久化的工作项管理。
// Agent 可以创建、查询、更新任务，跨会话追踪工作进度。
package taskboard

import (
	"context"
	"fmt"
	"time"
)

var validStatuses = map[Status]bool{
	StatusPending: true, StatusInProgress: true, StatusDone: true,
	StatusBlocked: true, StatusCancelled: true,
}

var validPriorities = map[Priority]bool{
	PriorityLow: true, PriorityMedium: true, PriorityHigh: true,
}

func validateStatus(s Status) error {
	if !validStatuses[s] {
		return fmt.Errorf("invalid status: %q", s)
	}
	return nil
}

func validatePriority(p Priority) error {
	if !validPriorities[p] {
		return fmt.Errorf("invalid priority: %q", p)
	}
	return nil
}

// TaskBoard 是工作项管理的核心接口。
// 所有实现必须是并发安全的。
type TaskBoard interface {
	Create(ctx context.Context, task *Task) (string, error)
	Get(ctx context.Context, id string) (*Task, error)
	Update(ctx context.Context, id string, patch TaskPatch) error
	List(ctx context.Context, filter TaskFilter) ([]*Task, error)
	Delete(ctx context.Context, id string) error
}

// Status 任务状态
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	StatusBlocked    Status = "blocked"
	StatusCancelled  Status = "cancelled"
)

// Priority 任务优先级
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

// Task 工作项
type Task struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`            // 创建该任务的会话
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      Status    `json:"status"`
	Priority    Priority  `json:"priority"`
	Assignee    string    `json:"assignee,omitempty"`     // agent ID 或用户标识
	ParentID    string    `json:"parent_id,omitempty"`    // 父任务 ID（支持子任务）
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TaskPatch 任务更新补丁（nil 字段不更新）
type TaskPatch struct {
	Title       *string   `json:"title,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *Status   `json:"status,omitempty"`
	Priority    *Priority `json:"priority,omitempty"`
	Assignee    *string   `json:"assignee,omitempty"`
	Tags        []string  `json:"tags,omitempty"` // 非 nil 时替换
}

// TaskFilter 任务查询过滤器
type TaskFilter struct {
	SessionID string   `json:"session_id,omitempty"`
	Status    Status   `json:"status,omitempty"`
	Priority  Priority `json:"priority,omitempty"`
	Assignee  string   `json:"assignee,omitempty"`
	ParentID  string   `json:"parent_id,omitempty"`
	Tags      []string `json:"tags,omitempty"` // 任一匹配
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}
