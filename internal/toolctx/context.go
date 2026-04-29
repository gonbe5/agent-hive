// Package toolctx 提供工具调用上下文，供 tools 和 subagent 包共享，
// 避免两个包之间产生循环依赖。
package toolctx

import "context"

// CallerType 调用者类型
type CallerType string

const (
	CallerMaster     CallerType = "master"      // Master Agent
	CallerSubAgent   CallerType = "subagent"    // 动态 SubAgent
	CallerFixedAgent CallerType = "fixed_agent" // 固定 Agent（可委托任务）
)

// ToolContext 工具调用上下文
type ToolContext struct {
	CallerType CallerType // 调用者类型
	CallerName string     // 调用者名称（如 "research", "general"）
	Depth      int        // 调用深度（防止递归）
}

// contextKey 用于在 context.Context 中存储 ToolContext
type contextKey string

const ToolContextKey contextKey = "tool_context"

// SessionIDKey 用于在 context 中传递 sessionID，供权限审批时关联到正确的会话。
// Master 和 AgentLoop 共用此 key。
const SessionIDKey contextKey = "session_id"

// SkipPermissionKey 用于在 context 中标记跳过权限检查（同任务内已审批过的 tool+args 组合）
const SkipPermissionKey contextKey = "skip_permission"

// WithSkipPermission 标记此次工具调用跳过权限检查
func WithSkipPermission(ctx context.Context) context.Context {
	return context.WithValue(ctx, SkipPermissionKey, true)
}

// ShouldSkipPermission 检查是否应跳过权限检查
func ShouldSkipPermission(ctx context.Context) bool {
	v, _ := ctx.Value(SkipPermissionKey).(bool)
	return v
}

// WithToolContext 将 ToolContext 注入到 context.Context
func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return context.WithValue(ctx, ToolContextKey, tc)
}

// WithSessionID 将 sessionID 注入到 context.Context
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, SessionIDKey, sessionID)
}

// GetSessionID 从 context.Context 获取 sessionID，未设置时返回空字符串
func GetSessionID(ctx context.Context) string {
	if id, ok := ctx.Value(SessionIDKey).(string); ok {
		return id
	}
	return ""
}

// GetToolContext 从 context.Context 获取 ToolContext
// 如果未设置，返回默认的 Master 上下文
func GetToolContext(ctx context.Context) *ToolContext {
	if tc, ok := ctx.Value(ToolContextKey).(*ToolContext); ok {
		return tc
	}
	return &ToolContext{
		CallerType: CallerMaster,
		CallerName: "master",
		Depth:      0,
	}
}
