package mcphost

import "context"

// PromptDefinition MCP 提示定义
type PromptDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument 提示参数
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage 提示消息
type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PromptExecutor 提示执行器
type PromptExecutor func(ctx context.Context, args map[string]string) ([]PromptMessage, error)

type promptEntry struct {
	def      PromptDefinition
	executor PromptExecutor
}
