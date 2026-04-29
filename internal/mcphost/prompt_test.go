package mcphost

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterPrompt(t *testing.T) {
	tests := []struct {
		name    string
		defs    []PromptDefinition
		wantLen int
	}{
		{
			name:    "注册单个提示",
			defs:    []PromptDefinition{{Name: "greeting"}},
			wantLen: 1,
		},
		{
			name: "注册多个提示",
			defs: []PromptDefinition{
				{Name: "greeting"},
				{Name: "summary"},
			},
			wantLen: 2,
		},
		{
			name: "重复名称覆盖注册",
			defs: []PromptDefinition{
				{Name: "greeting", Description: "旧版"},
				{Name: "greeting", Description: "新版"},
			},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			executor := func(_ context.Context, _ map[string]string) ([]PromptMessage, error) {
				return nil, nil
			}
			for _, d := range tt.defs {
				h.RegisterPrompt(d, executor)
			}
			assert.Len(t, h.ListPrompts(), tt.wantLen)
		})
	}
}

func TestGetPrompt(t *testing.T) {
	tests := []struct {
		name        string
		promptName  string
		register    bool
		def         PromptDefinition
		args        map[string]string
		execErr     error
		wantErr     bool
		errContains string
		wantMsgs    int
	}{
		{
			name:       "执行已注册提示",
			promptName: "greeting",
			register:   true,
			def:        PromptDefinition{Name: "greeting"},
			args:       map[string]string{"name": "世界"},
			wantErr:    false,
			wantMsgs:   1,
		},
		{
			name:        "执行未注册提示",
			promptName:  "missing",
			register:    false,
			def:         PromptDefinition{Name: "missing"},
			args:        nil,
			wantErr:     true,
			errContains: "未找到",
		},
		{
			name:       "缺少必需参数",
			promptName: "greeting",
			register:   true,
			def: PromptDefinition{
				Name: "greeting",
				Arguments: []PromptArgument{
					{Name: "name", Required: true},
				},
			},
			args:        map[string]string{},
			wantErr:     true,
			errContains: "缺少必需参数",
		},
		{
			name:       "可选参数不报错",
			promptName: "greeting",
			register:   true,
			def: PromptDefinition{
				Name: "greeting",
				Arguments: []PromptArgument{
					{Name: "name", Required: false},
				},
			},
			args:     map[string]string{},
			wantErr:  false,
			wantMsgs: 1,
		},
		{
			name:       "执行器返回错误",
			promptName: "fail",
			register:   true,
			def:        PromptDefinition{Name: "fail"},
			args:       nil,
			execErr:    errors.New("执行失败"),
			wantErr:    true,
			errContains: "执行提示",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			if tt.register {
				h.RegisterPrompt(tt.def, func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
					if tt.execErr != nil {
						return nil, tt.execErr
					}
					return []PromptMessage{{Role: "assistant", Content: "你好"}}, nil
				})
			}

			msgs, err := h.GetPrompt(context.Background(), tt.promptName, tt.args)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, msgs)
			} else {
				require.NoError(t, err)
				assert.Len(t, msgs, tt.wantMsgs)
			}
		})
	}
}

func TestListPrompts(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		wantLen int
	}{
		{
			name:    "空列表",
			names:   nil,
			wantLen: 0,
		},
		{
			name:    "多个提示",
			names:   []string{"a", "b", "c"},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHost(testLogger())
			executor := func(_ context.Context, _ map[string]string) ([]PromptMessage, error) {
				return nil, nil
			}
			for _, n := range tt.names {
				h.RegisterPrompt(PromptDefinition{Name: n}, executor)
			}
			defs := h.ListPrompts()
			assert.Len(t, defs, tt.wantLen)
		})
	}
}
