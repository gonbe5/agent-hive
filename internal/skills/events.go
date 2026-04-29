package skills

import "time"

// SkillEvent 表示 skill 系统的结构化事件，可通过 master 事件系统发布
type SkillEvent struct {
	Type      string    `json:"type"`                 // "skill.invoked", "tool.called", "permission.asked"
	SkillName string    `json:"skill_name,omitempty"` // 相关 skill 名称
	ToolName  string    `json:"tool_name,omitempty"`  // 相关工具名称
	DurationMs int64    `json:"duration_ms"`          // 耗时（毫秒）
	Success   bool      `json:"success"`              // 是否成功
	Error     string    `json:"error,omitempty"`      // 错误信息（失败时）
	Timestamp time.Time `json:"timestamp"`            // 事件时间
}

// EventPublisher 用于发布 SkillEvent 的接口（由 master 实现）
type EventPublisher interface {
	PublishSkillEvent(event SkillEvent)
}
