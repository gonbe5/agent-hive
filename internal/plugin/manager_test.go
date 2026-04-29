package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func TestManager_RegisterHooks(t *testing.T) {
	tests := []struct {
		name      string
		hookCount int
	}{
		{"注册零个 hooks", 0},
		{"注册一个 hooks", 1},
		{"注册三个 hooks", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			for i := 0; i < tt.hookCount; i++ {
				mgr.RegisterHooks(Hooks{})
			}
			assert.Equal(t, tt.hookCount, mgr.HookCount())
		})
	}
}

func TestManager_TriggerToolBefore(t *testing.T) {
	tests := []struct {
		name     string
		hook     func(ctx context.Context, input *ToolExecuteInput) error
		wantArgs string
	}{
		{
			name: "hook 修改参数",
			hook: func(ctx context.Context, input *ToolExecuteInput) error {
				input.Args = json.RawMessage(`{"modified":true}`)
				return nil
			},
			wantArgs: `{"modified":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{ToolExecuteBefore: tt.hook})

			input := &ToolExecuteInput{
				ToolName: "bash",
				Args:     json.RawMessage(`{"command":"ls"}`),
			}
			err := mgr.TriggerToolBefore(context.Background(), input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantArgs, string(input.Args))
		})
	}
}

func TestManager_TriggerToolBefore_Block(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		wantErrMsg string
	}{
		{
			name:       "hook 阻止执行",
			reason:     "危险操作",
			wantErrMsg: "危险操作",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{
				ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
					input.Blocked = true
					input.Reason = tt.reason
					return nil
				},
			})

			input := &ToolExecuteInput{ToolName: "bash"}
			err := mgr.TriggerToolBefore(context.Background(), input)
			require.Error(t, err)
			assert.True(t, errs.IsCode(err, errs.CodeSkillToolBlocked))
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestManager_TriggerToolAfter(t *testing.T) {
	tests := []struct {
		name       string
		hook       func(ctx context.Context, input ToolExecuteInput, output *ToolExecuteOutput) error
		wantOutput string
	}{
		{
			name: "hook 修改输出",
			hook: func(ctx context.Context, input ToolExecuteInput, output *ToolExecuteOutput) error {
				output.Output = "modified: " + output.Output
				return nil
			},
			wantOutput: "modified: original",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{ToolExecuteAfter: tt.hook})

			output := &ToolExecuteOutput{Output: "original"}
			err := mgr.TriggerToolAfter(context.Background(), ToolExecuteInput{ToolName: "bash"}, output)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOutput, output.Output)
		})
	}
}

func TestManager_TriggerChatBefore(t *testing.T) {
	tests := []struct {
		name       string
		hook       func(ctx context.Context, input *ChatMessageInput) error
		wantPrompt string
	}{
		{
			name: "hook 修改 system prompt",
			hook: func(ctx context.Context, input *ChatMessageInput) error {
				input.SystemPrompt = "injected: " + input.SystemPrompt
				return nil
			},
			wantPrompt: "injected: original prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{ChatMessageBefore: tt.hook})

			input := &ChatMessageInput{SystemPrompt: "original prompt"}
			err := mgr.TriggerChatBefore(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPrompt, input.SystemPrompt)
		})
	}
}

func TestManager_TriggerChatAfter(t *testing.T) {
	tests := []struct {
		name        string
		hook        func(ctx context.Context, input ChatMessageInput, output *ChatMessageOutput) error
		wantContent string
	}{
		{
			name: "hook 修改 content",
			hook: func(ctx context.Context, input ChatMessageInput, output *ChatMessageOutput) error {
				output.Content = "filtered: " + output.Content
				return nil
			},
			wantContent: "filtered: hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{ChatMessageAfter: tt.hook})

			output := &ChatMessageOutput{Content: "hello"}
			err := mgr.TriggerChatAfter(context.Background(), ChatMessageInput{}, output)
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, output.Content)
		})
	}
}

func TestManager_CustomTools(t *testing.T) {
	tests := []struct {
		name      string
		tools     map[string]ToolDefinition
		wantCount int
	}{
		{
			name:      "无自定义工具",
			tools:     nil,
			wantCount: 0,
		},
		{
			name: "注册自定义工具",
			tools: map[string]ToolDefinition{
				"my_tool": {Name: "my_tool", Description: "测试工具"},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			mgr.RegisterHooks(Hooks{Tools: tt.tools})

			result := mgr.CustomTools()
			assert.Len(t, result, tt.wantCount)
			if tt.wantCount > 0 {
				_, ok := result["my_tool"]
				assert.True(t, ok)
			}
		})
	}
}

func TestManager_MultiplePlugins(t *testing.T) {
	// 验证多个 hook 按注册顺序执行
	var order []int

	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			order = append(order, 1)
			return nil
		},
	})
	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			order = append(order, 2)
			return nil
		},
	})
	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			order = append(order, 3)
			return nil
		},
	})

	input := &ToolExecuteInput{ToolName: "test"}
	err := mgr.TriggerToolBefore(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, order)
	assert.Equal(t, 3, mgr.HookCount())
}

func TestManager_LoadFromDir_Empty(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"目录不存在", "/nonexistent/plugin/dir"},
		{"空目录", t.TempDir()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			err := mgr.LoadFromDir(tt.dir, PluginInput{})
			assert.NoError(t, err)
			assert.Equal(t, 0, mgr.HookCount())
		})
	}
}

func TestManager_NilHooks(t *testing.T) {
	// 所有 hook 函数为 nil 时不应 panic
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{}) // 所有字段为 nil

	ctx := context.Background()

	err := mgr.TriggerToolBefore(ctx, &ToolExecuteInput{ToolName: "test"})
	assert.NoError(t, err)

	err = mgr.TriggerToolAfter(ctx, ToolExecuteInput{ToolName: "test"}, &ToolExecuteOutput{})
	assert.NoError(t, err)

	err = mgr.TriggerChatBefore(ctx, &ChatMessageInput{})
	assert.NoError(t, err)

	err = mgr.TriggerChatAfter(ctx, ChatMessageInput{}, &ChatMessageOutput{})
	assert.NoError(t, err)

	assert.Empty(t, mgr.CustomTools())

	// 新增 hook: PermissionAsk
	out, err := mgr.TriggerPermissionAsk(ctx, &PermissionAskInput{ToolName: "bash"})
	assert.NoError(t, err)
	assert.Nil(t, out)

	// 新增 hook: ToolDefinition
	err = mgr.TriggerToolDefinition(ctx, &ToolDefinitionInput{Name: "bash"})
	assert.NoError(t, err)

	// 新增 hook: ShellEnv
	env, err := mgr.TriggerShellEnv(ctx, &ShellEnvInput{Command: "ls"})
	assert.NoError(t, err)
	assert.Empty(t, env)
}

// --- 新增 Hook 测试 ---

func TestManager_TriggerPermissionAsk(t *testing.T) {
	tests := []struct {
		name         string
		hooks        []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error)
		wantDecision string
		wantReason   string
		wantNil      bool
	}{
		{
			name:    "无 hook 注册时返回 nil",
			hooks:   nil,
			wantNil: true,
		},
		{
			name: "hook 返回 allow",
			hooks: []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error){
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					return &PermissionAskOutput{Decision: "allow", Reason: "自动批准安全工具"}, nil
				},
			},
			wantDecision: "allow",
			wantReason:   "自动批准安全工具",
		},
		{
			name: "hook 返回 deny",
			hooks: []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error){
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					return &PermissionAskOutput{Decision: "deny", Reason: "禁止执行危险命令"}, nil
				},
			},
			wantDecision: "deny",
			wantReason:   "禁止执行危险命令",
		},
		{
			name: "hook 返回空 Decision 则走默认流程",
			hooks: []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error){
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					return &PermissionAskOutput{Decision: ""}, nil
				},
			},
			wantNil: true,
		},
		{
			name: "多个 hook 第一个有效决策生效",
			hooks: []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error){
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					// 第一个 hook 不做决策
					return &PermissionAskOutput{Decision: ""}, nil
				},
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					// 第二个 hook 做出决策
					return &PermissionAskOutput{Decision: "allow", Reason: "第二个插件批准"}, nil
				},
			},
			wantDecision: "allow",
			wantReason:   "第二个插件批准",
		},
		{
			name: "hook 返回 nil output 时继续下一个",
			hooks: []func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error){
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					return nil, nil
				},
				func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
					return &PermissionAskOutput{Decision: "deny", Reason: "兜底拒绝"}, nil
				},
			},
			wantDecision: "deny",
			wantReason:   "兜底拒绝",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			for _, hook := range tt.hooks {
				mgr.RegisterHooks(Hooks{PermissionAsk: hook})
			}

			input := &PermissionAskInput{
				ToolName: "bash",
				Args:     json.RawMessage(`{"command":"rm -rf /"}`),
				Policy:   "ask",
				Rules:    []string{"bash: ask"},
			}
			out, err := mgr.TriggerPermissionAsk(context.Background(), input)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, out)
			} else {
				require.NotNil(t, out)
				assert.Equal(t, tt.wantDecision, out.Decision)
				assert.Equal(t, tt.wantReason, out.Reason)
			}
		})
	}
}

func TestManager_TriggerPermissionAsk_Error(t *testing.T) {
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{
		PermissionAsk: func(ctx context.Context, input *PermissionAskInput) (*PermissionAskOutput, error) {
			return nil, errs.New(errs.CodePluginExecFailed, "权限检查插件异常")
		},
	})

	input := &PermissionAskInput{ToolName: "bash"}
	out, err := mgr.TriggerPermissionAsk(context.Background(), input)
	require.Error(t, err)
	assert.Nil(t, out)
	assert.True(t, errs.IsCode(err, errs.CodePluginExecFailed))
}

func TestManager_TriggerToolDefinition(t *testing.T) {
	tests := []struct {
		name       string
		hooks      []func(ctx context.Context, input *ToolDefinitionInput) error
		wantDesc   string
		wantSchema string
	}{
		{
			name:       "无 hook 不修改",
			hooks:      nil,
			wantDesc:   "原始描述",
			wantSchema: `{"type":"object"}`,
		},
		{
			name: "单个 hook 修改描述",
			hooks: []func(ctx context.Context, input *ToolDefinitionInput) error{
				func(ctx context.Context, input *ToolDefinitionInput) error {
					input.Description = "插件修改后: " + input.Description
					return nil
				},
			},
			wantDesc:   "插件修改后: 原始描述",
			wantSchema: `{"type":"object"}`,
		},
		{
			name: "hook 修改 schema",
			hooks: []func(ctx context.Context, input *ToolDefinitionInput) error{
				func(ctx context.Context, input *ToolDefinitionInput) error {
					input.ArgsSchema = json.RawMessage(`{"type":"object","required":["cmd"]}`)
					return nil
				},
			},
			wantDesc:   "原始描述",
			wantSchema: `{"type":"object","required":["cmd"]}`,
		},
		{
			name: "多个 hook 链式修改",
			hooks: []func(ctx context.Context, input *ToolDefinitionInput) error{
				func(ctx context.Context, input *ToolDefinitionInput) error {
					input.Description = "[安全] " + input.Description
					return nil
				},
				func(ctx context.Context, input *ToolDefinitionInput) error {
					input.Description = input.Description + " (已审核)"
					return nil
				},
			},
			wantDesc:   "[安全] 原始描述 (已审核)",
			wantSchema: `{"type":"object"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			for _, hook := range tt.hooks {
				mgr.RegisterHooks(Hooks{ToolDefinitionHook: hook})
			}

			input := &ToolDefinitionInput{
				Name:        "bash",
				Description: "原始描述",
				ArgsSchema:  json.RawMessage(`{"type":"object"}`),
			}
			err := mgr.TriggerToolDefinition(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDesc, input.Description)
			assert.JSONEq(t, tt.wantSchema, string(input.ArgsSchema))
		})
	}
}

func TestManager_TriggerToolDefinition_Error(t *testing.T) {
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{
		ToolDefinitionHook: func(ctx context.Context, input *ToolDefinitionInput) error {
			return errs.New(errs.CodePluginExecFailed, "工具定义修改失败")
		},
	})

	input := &ToolDefinitionInput{Name: "bash", Description: "test"}
	err := mgr.TriggerToolDefinition(context.Background(), input)
	require.Error(t, err)
	assert.True(t, errs.IsCode(err, errs.CodePluginExecFailed))
}

func TestManager_TriggerShellEnv(t *testing.T) {
	tests := []struct {
		name    string
		hooks   []func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error)
		wantEnv map[string]string
	}{
		{
			name:    "无 hook 返回空 map",
			hooks:   nil,
			wantEnv: map[string]string{},
		},
		{
			name: "单个 hook 注入环境变量",
			hooks: []func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error){
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					return &ShellEnvOutput{Env: map[string]string{
						"NODE_ENV": "production",
						"DEBUG":    "false",
					}}, nil
				},
			},
			wantEnv: map[string]string{
				"NODE_ENV": "production",
				"DEBUG":    "false",
			},
		},
		{
			name: "多个 hook 合并环境变量",
			hooks: []func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error){
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					return &ShellEnvOutput{Env: map[string]string{
						"PATH_PREFIX": "/opt/bin",
						"DEBUG":       "true",
					}}, nil
				},
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					return &ShellEnvOutput{Env: map[string]string{
						"API_KEY": "secret",
						"DEBUG":   "false", // 覆盖第一个 hook 的值
					}}, nil
				},
			},
			wantEnv: map[string]string{
				"PATH_PREFIX": "/opt/bin",
				"API_KEY":     "secret",
				"DEBUG":       "false",
			},
		},
		{
			name: "hook 返回 nil output 不影响",
			hooks: []func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error){
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					return nil, nil
				},
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					return &ShellEnvOutput{Env: map[string]string{"FOO": "bar"}}, nil
				},
			},
			wantEnv: map[string]string{"FOO": "bar"},
		},
		{
			name: "根据命令条件注入",
			hooks: []func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error){
				func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
					if input.Command == "npm test" {
						return &ShellEnvOutput{Env: map[string]string{"CI": "true"}}, nil
					}
					return nil, nil
				},
			},
			wantEnv: map[string]string{"CI": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			for _, hook := range tt.hooks {
				mgr.RegisterHooks(Hooks{ShellEnv: hook})
			}

			input := &ShellEnvInput{
				Command: "npm test",
				WorkDir: "/workspace/project",
			}
			env, err := mgr.TriggerShellEnv(context.Background(), input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnv, env)
		})
	}
}

func TestManager_TriggerShellEnv_Error(t *testing.T) {
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{
		ShellEnv: func(ctx context.Context, input *ShellEnvInput) (*ShellEnvOutput, error) {
			return nil, errs.New(errs.CodePluginExecFailed, "环境变量注入失败")
		},
	})

	input := &ShellEnvInput{Command: "ls"}
	env, err := mgr.TriggerShellEnv(context.Background(), input)
	require.Error(t, err)
	assert.Nil(t, env)
	assert.True(t, errs.IsCode(err, errs.CodePluginExecFailed))
}

func TestHookTypeConstants(t *testing.T) {
	// 验证 hook 类型常量值正确
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"ToolExecuteBefore", HookTypeToolExecuteBefore, "tool.execute.before"},
		{"ToolExecuteAfter", HookTypeToolExecuteAfter, "tool.execute.after"},
		{"ChatMessageBefore", HookTypeChatMessageBefore, "chat.message.before"},
		{"ChatMessageAfter", HookTypeChatMessageAfter, "chat.message.after"},
		{"PermissionAsk", HookTypePermissionAsk, "permission.ask"},
		{"ToolDefinition", HookTypeToolDefinition, "tool.definition"},
		{"ShellEnv", HookTypeShellEnv, "shell.env"},
		{"SessionStart", HookTypeSessionStart, "session.start"},
		{"SessionEnd", HookTypeSessionEnd, "session.end"},
		{"PreCompact", HookTypePreCompact, "compact.pre"},
		{"PostCompact", HookTypePostCompact, "compact.post"},
		{"TaskCreated", HookTypeTaskCreated, "task.created"},
		{"TaskCompleted", HookTypeTaskCompleted, "task.completed"},
		{"ConfigChange", HookTypeConfigChange, "config.change"},
		{"FileChanged", HookTypeFileChanged, "file.changed"},
		{"AgentSpawned", HookTypeAgentSpawned, "agent.spawned"},
		{"AgentDestroyed", HookTypeAgentDestroyed, "agent.destroyed"},
		{"JournalEntry", HookTypeJournalEntry, "journal.entry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

// TestManager_TriggerNewHooks 验证 11 个新 hook 的触发行为（正常触发、nil 跳过、错误传播）
func TestManager_TriggerNewHooks(t *testing.T) {
	t.Run("SessionStart", func(t *testing.T) {
		called := false
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			SessionStart: func(ctx context.Context, input *SessionStartInput) error {
				called = true
				assert.Equal(t, "sess-1", input.SessionID)
				return nil
			},
		})
		err := mgr.TriggerSessionStart(context.Background(), &SessionStartInput{SessionID: "sess-1"})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("SessionStart_nil_hook_skipped", func(t *testing.T) {
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{}) // no SessionStart
		err := mgr.TriggerSessionStart(context.Background(), &SessionStartInput{SessionID: "s"})
		assert.NoError(t, err)
	})

	t.Run("SessionEnd", func(t *testing.T) {
		called := false
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			SessionEnd: func(ctx context.Context, input *SessionEndInput) error {
				called = true
				return nil
			},
		})
		err := mgr.TriggerSessionEnd(context.Background(), &SessionEndInput{SessionID: "sess-1"})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("PreCompact", func(t *testing.T) {
		var got int
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			PreCompact: func(ctx context.Context, input *CompactInput) error {
				got = input.MessageCount
				return nil
			},
		})
		err := mgr.TriggerPreCompact(context.Background(), &CompactInput{MessageCount: 42})
		assert.NoError(t, err)
		assert.Equal(t, 42, got)
	})

	t.Run("PostCompact", func(t *testing.T) {
		called := false
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			PostCompact: func(ctx context.Context, input *CompactInput) error {
				called = true
				return nil
			},
		})
		err := mgr.TriggerPostCompact(context.Background(), &CompactInput{MessageCount: 10})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("TaskCreated", func(t *testing.T) {
		var gotID string
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			TaskCreated: func(ctx context.Context, input *TaskEventInput) error {
				gotID = input.TaskID
				return nil
			},
		})
		err := mgr.TriggerTaskCreated(context.Background(), &TaskEventInput{TaskID: "task-1", Status: "pending"})
		assert.NoError(t, err)
		assert.Equal(t, "task-1", gotID)
	})

	t.Run("TaskCompleted", func(t *testing.T) {
		called := false
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			TaskCompleted: func(ctx context.Context, input *TaskEventInput) error {
				called = true
				return nil
			},
		})
		err := mgr.TriggerTaskCompleted(context.Background(), &TaskEventInput{TaskID: "task-1", Status: "completed"})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("ConfigChange", func(t *testing.T) {
		var gotKey string
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			ConfigChange: func(ctx context.Context, input *ConfigChangeInput) error {
				gotKey = input.Key
				return nil
			},
		})
		err := mgr.TriggerConfigChange(context.Background(), &ConfigChangeInput{Key: "resource:db"})
		assert.NoError(t, err)
		assert.Equal(t, "resource:db", gotKey)
	})

	t.Run("FileChanged", func(t *testing.T) {
		var gotPath string
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			FileChanged: func(ctx context.Context, input *FileChangedInput) error {
				gotPath = input.Path
				return nil
			},
		})
		err := mgr.TriggerFileChanged(context.Background(), &FileChangedInput{Path: "/tmp/foo.go", Operation: "write"})
		assert.NoError(t, err)
		assert.Equal(t, "/tmp/foo.go", gotPath)
	})

	t.Run("AgentSpawned", func(t *testing.T) {
		var gotID string
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			AgentSpawned: func(ctx context.Context, input *AgentLifecycleInput) error {
				gotID = input.AgentID
				return nil
			},
		})
		err := mgr.TriggerAgentSpawned(context.Background(), &AgentLifecycleInput{AgentID: "agent-1", AgentName: "general"})
		assert.NoError(t, err)
		assert.Equal(t, "agent-1", gotID)
	})

	t.Run("AgentDestroyed", func(t *testing.T) {
		called := false
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			AgentDestroyed: func(ctx context.Context, input *AgentLifecycleInput) error {
				called = true
				return nil
			},
		})
		err := mgr.TriggerAgentDestroyed(context.Background(), &AgentLifecycleInput{AgentID: "agent-1"})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("JournalEntry", func(t *testing.T) {
		var gotTool string
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			JournalEntry: func(ctx context.Context, input *JournalEntryInput) error {
				gotTool = input.ToolName
				return nil
			},
		})
		err := mgr.TriggerJournalEntry(context.Background(), &JournalEntryInput{SessionID: "s1", ToolName: "bash", DurationMs: 100})
		assert.NoError(t, err)
		assert.Equal(t, "bash", gotTool)
	})

	t.Run("error_propagated", func(t *testing.T) {
		mgr := NewManager(zap.NewNop())
		mgr.RegisterHooks(Hooks{
			SessionStart: func(ctx context.Context, input *SessionStartInput) error {
				return errs.New(errs.CodePluginExecFailed, "session start failed")
			},
		})
		err := mgr.TriggerSessionStart(context.Background(), &SessionStartInput{SessionID: "s"})
		assert.Error(t, err)
	})
}

func TestManager_Unload(t *testing.T) {
	tests := []struct {
		name      string
		setupHook bool
		unloadID  string
		wantErr   bool
		errCode   int
	}{
		{
			name:      "卸载已注册的插件",
			setupHook: true,
			unloadID:  "hook-0",
			wantErr:   false,
		},
		{
			name:      "卸载不存在的插件返回错误",
			setupHook: false,
			unloadID:  "nonexistent",
			wantErr:   true,
			errCode:   errs.CodePluginNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			if tt.setupHook {
				mgr.RegisterHooks(Hooks{
					ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
						return nil
					},
				})
				assert.Equal(t, 1, mgr.HookCount())
			}

			err := mgr.Unload(context.Background(), tt.unloadID)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errs.IsCode(err, tt.errCode))
			} else {
				require.NoError(t, err)
				assert.Equal(t, 0, mgr.HookCount())
			}
		})
	}
}

func TestManager_Unload_MultiplePlugins(t *testing.T) {
	// 注册多个插件，卸载其中一个，验证剩余插件仍然正常工作
	mgr := NewManager(zap.NewNop())
	var callRecord []string

	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			callRecord = append(callRecord, "plugin-0")
			return nil
		},
	})
	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			callRecord = append(callRecord, "plugin-1")
			return nil
		},
	})
	mgr.RegisterHooks(Hooks{
		ToolExecuteBefore: func(ctx context.Context, input *ToolExecuteInput) error {
			callRecord = append(callRecord, "plugin-2")
			return nil
		},
	})
	assert.Equal(t, 3, mgr.HookCount())

	// 卸载中间的插件
	err := mgr.Unload(context.Background(), "hook-1")
	require.NoError(t, err)
	assert.Equal(t, 2, mgr.HookCount())

	// 触发 hooks 验证剩余两个插件仍然工作
	callRecord = nil
	err = mgr.TriggerToolBefore(context.Background(), &ToolExecuteInput{ToolName: "test"})
	require.NoError(t, err)
	assert.Len(t, callRecord, 2)
	assert.Contains(t, callRecord, "plugin-0")
	assert.Contains(t, callRecord, "plugin-2")
	assert.NotContains(t, callRecord, "plugin-1")
}

func TestManager_Reload_NoPath(t *testing.T) {
	// 手动注册的插件（无 .so 路径）不支持重载
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{})

	err := mgr.Reload(context.Background(), "hook-0")
	require.Error(t, err)
	assert.True(t, errs.IsCode(err, errs.CodePluginLoadFailed))
	assert.Contains(t, err.Error(), "内置插件不支持重载")
}

func TestManager_Reload_NotFound(t *testing.T) {
	mgr := NewManager(zap.NewNop())

	err := mgr.Reload(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.True(t, errs.IsCode(err, errs.CodePluginNotFound))
}

func TestManager_ListPlugins(t *testing.T) {
	tests := []struct {
		name      string
		hookCount int
	}{
		{"无插件", 0},
		{"一个插件", 1},
		{"多个插件", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(zap.NewNop())
			for i := 0; i < tt.hookCount; i++ {
				mgr.RegisterHooks(Hooks{})
			}

			list := mgr.ListPlugins()
			assert.Len(t, list, tt.hookCount)
			for _, info := range list {
				assert.Equal(t, "loaded", info.Status)
				assert.NotEmpty(t, info.ID)
			}
		})
	}
}

func TestManager_ListPlugins_AfterUnload(t *testing.T) {
	mgr := NewManager(zap.NewNop())
	mgr.RegisterHooks(Hooks{})
	mgr.RegisterHooks(Hooks{})
	assert.Len(t, mgr.ListPlugins(), 2)

	err := mgr.Unload(context.Background(), "hook-0")
	require.NoError(t, err)
	list := mgr.ListPlugins()
	assert.Len(t, list, 1)
	assert.Equal(t, "hook-1", list[0].ID)
}

func TestPluginIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/path/to/myplugin.so", "myplugin"},
		{"/path/to/audit-log.so", "audit-log"},
		{"simple.so", "simple"},
		{"/no/ext/plugin", "plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, pluginIDFromPath(tt.path))
		})
	}
}
