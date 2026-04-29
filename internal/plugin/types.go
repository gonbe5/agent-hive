package plugin

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// Plugin 插件入口函数签名
type Plugin func(input PluginInput) (Hooks, error)

// PluginInput 传递给插件的上下文
type PluginInput struct {
	WorkDir string
	Logger  *zap.Logger
}

// Hooks 插件可以注册的钩子集合
type Hooks struct {
	// 工具执行前拦截（可修改参数、可拒绝执行）
	ToolExecuteBefore func(ctx context.Context, input *ToolExecuteInput) error
	// 工具执行后拦截（可修改输出）
	ToolExecuteAfter func(ctx context.Context, input ToolExecuteInput, output *ToolExecuteOutput) error
	// Chat 消息发送前（可修改 system prompt、messages）
	ChatMessageBefore func(ctx context.Context, input *ChatMessageInput) error
	// Chat 消息返回后（可记录、修改）
	ChatMessageAfter func(ctx context.Context, input ChatMessageInput, output *ChatMessageOutput) error

	// 权限请求拦截（当 policy="ask" 时，可自动处理权限决策）
	PermissionAsk func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error)
	// 工具定义拦截（工具注册时，可修改描述和参数 schema）
	ToolDefinitionHook func(ctx context.Context, input *ToolDefinitionInput) error
	// Shell 环境变量注入（Bash 工具执行命令前，可注入额外环境变量）
	ShellEnv func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error)

	// 会话开始（新会话创建时触发）
	SessionStart func(ctx context.Context, input *SessionStartInput) error
	// 会话结束（会话关闭时触发）
	SessionEnd func(ctx context.Context, input *SessionEndInput) error
	// 压缩前（上下文压缩开始前触发）
	PreCompact func(ctx context.Context, input *CompactInput) error
	// 压缩后（上下文压缩完成后触发）
	PostCompact func(ctx context.Context, input *CompactInput) error
	// 任务创建（并行任务组中单个任务创建时触发）
	TaskCreated func(ctx context.Context, input *TaskEventInput) error
	// 任务完成（并行任务组中单个任务完成时触发）
	TaskCompleted func(ctx context.Context, input *TaskEventInput) error
	// 配置变更（Store 配置项变更时触发）
	ConfigChange func(ctx context.Context, input *ConfigChangeInput) error
	// 文件变更（工具写入/编辑文件后触发）
	FileChanged func(ctx context.Context, input *FileChangedInput) error
	// Agent 创建（动态 Agent 注册时触发）
	AgentSpawned func(ctx context.Context, input *AgentLifecycleInput) error
	// Agent 销毁（动态 Agent 注销时触发）
	AgentDestroyed func(ctx context.Context, input *AgentLifecycleInput) error
	// 日志条目写入（Journal 写入新条目时触发）
	JournalEntry func(ctx context.Context, input *JournalEntryInput) error

	// 自定义工具定义
	Tools map[string]ToolDefinition
}

// ToolExecuteInput 工具执行输入
type ToolExecuteInput struct {
	ToolName  string          `json:"tool_name"`
	SessionID string          `json:"session_id"`
	Args      json.RawMessage `json:"args"`
	Blocked   bool            `json:"blocked"` // 设为 true 则阻止执行
	Reason    string          `json:"reason"`  // 阻止原因
}

// ToolExecuteOutput 工具执行输出
type ToolExecuteOutput struct {
	Title  string `json:"title"`
	Output string `json:"output"`
}

// ChatMessageInput Chat 消息输入
type ChatMessageInput struct {
	SessionID    string
	SystemPrompt string
	Messages     []llm.MessageWithTools
	Agent        string
}

// ChatMessageOutput Chat 消息输出
type ChatMessageOutput struct {
	Content string
	Model   string
}

// ToolDefinition 插件自定义工具
type ToolDefinition struct {
	Name        string                                                          `json:"name"`
	Description string                                                          `json:"description"`
	ArgsSchema  json.RawMessage                                                 `json:"args_schema"`
	Execute     func(ctx context.Context, args json.RawMessage) (string, error) `json:"-"`
}

// --- 新增 Hook 类型 ---

// PermissionAskInput 权限请求输入
type PermissionAskInput struct {
	ToolName string          `json:"tool_name"` // 请求权限的工具名
	Args     json.RawMessage `json:"args"`      // 工具参数
	Policy   string          `json:"policy"`    // 当前权限策略（如 "ask"）
	Rules    []string        `json:"rules"`     // 当前生效的权限规则描述
}

// PermissionAskOutput 权限请求输出
type PermissionAskOutput struct {
	// Decision 权限决策: "allow" 允许, "deny" 拒绝, "" 空表示继续走默认流程让用户决定
	Decision string `json:"decision"`
	// Reason 决策原因（用于日志和审计）
	Reason string `json:"reason,omitempty"`
}

// ToolDefinitionInput 工具定义拦截输入
type ToolDefinitionInput struct {
	Name        string          `json:"name"`        // 工具名
	Description string          `json:"description"` // 工具描述（可修改）
	ArgsSchema  json.RawMessage `json:"args_schema"` // 参数 schema（可修改）
}

// ShellEnvInput Shell 环境变量注入输入
type ShellEnvInput struct {
	Command string `json:"command"` // 即将执行的命令
	WorkDir string `json:"workdir"` // 当前工作目录
}

// ShellEnvOutput Shell 环境变量注入输出
type ShellEnvOutput struct {
	// Env 额外的环境变量，会合并到命令执行环境中
	Env map[string]string `json:"env"`
}

// SessionStartInput 会话开始输入
type SessionStartInput struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id,omitempty"`
}

// SessionEndInput 会话结束输入
type SessionEndInput struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id,omitempty"`
}

// CompactInput 压缩事件输入（PreCompact / PostCompact 共用）
type CompactInput struct {
	SessionID    string `json:"session_id"`
	MessageCount int    `json:"message_count"` // 压缩前消息数
}

// TaskEventInput 任务事件输入（TaskCreated / TaskCompleted 共用）
type TaskEventInput struct {
	TaskID    string `json:"task_id"`
	AgentID   string `json:"agent_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status,omitempty"` // "pending", "running", "completed", "failed"
	Error     string `json:"error,omitempty"`
}

// ConfigChangeInput 配置变更输入
type ConfigChangeInput struct {
	Key      string `json:"key"`
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

// FileChangedInput 文件变更输入
type FileChangedInput struct {
	Path      string `json:"path"`       // 变更的文件路径
	Operation string `json:"operation"`  // "write", "edit", "delete"
	SessionID string `json:"session_id,omitempty"`
}

// AgentLifecycleInput Agent 生命周期输入（AgentSpawned / AgentDestroyed 共用）
type AgentLifecycleInput struct {
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name,omitempty"`
	Description string `json:"description,omitempty"`
}

// JournalEntryInput 日志条目写入输入
type JournalEntryInput struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// Hook 类型名常量（用于日志、事件总线等）
const (
	HookTypeToolExecuteBefore = "tool.execute.before"
	HookTypeToolExecuteAfter  = "tool.execute.after"
	HookTypeChatMessageBefore = "chat.message.before"
	HookTypeChatMessageAfter  = "chat.message.after"
	HookTypePermissionAsk     = "permission.ask"
	HookTypeToolDefinition    = "tool.definition"
	HookTypeShellEnv          = "shell.env"
	HookTypeSessionStart      = "session.start"
	HookTypeSessionEnd        = "session.end"
	HookTypePreCompact        = "compact.pre"
	HookTypePostCompact       = "compact.post"
	HookTypeTaskCreated       = "task.created"
	HookTypeTaskCompleted     = "task.completed"
	HookTypeConfigChange      = "config.change"
	HookTypeFileChanged       = "file.changed"
	HookTypeAgentSpawned      = "agent.spawned"
	HookTypeAgentDestroyed    = "agent.destroyed"
	HookTypeJournalEntry      = "journal.entry"
)
