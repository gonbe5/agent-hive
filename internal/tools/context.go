package tools

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/toolctx"
)

// CallerType 调用者类型（从 toolctx 包重新导出，保持向后兼容）
type CallerType = toolctx.CallerType

const (
	CallerMaster     CallerType = toolctx.CallerMaster
	CallerSubAgent   CallerType = toolctx.CallerSubAgent
	CallerFixedAgent CallerType = toolctx.CallerFixedAgent
)

// ToolContext 工具调用上下文（从 toolctx 包重新导出，保持向后兼容）
type ToolContext = toolctx.ToolContext

// WithToolContext 将 ToolContext 注入到 context.Context
func WithToolContext(ctx context.Context, tc *ToolContext) context.Context {
	return toolctx.WithToolContext(ctx, tc)
}

// GetToolContext 从 context.Context 获取 ToolContext
// 如果未设置，返回默认的 Master 上下文
func GetToolContext(ctx context.Context) *ToolContext {
	return toolctx.GetToolContext(ctx)
}
