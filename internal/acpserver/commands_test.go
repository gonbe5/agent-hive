package acpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/command"
)

func TestHandleSlashCommand(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		wantHandled  bool
		wantNonEmpty bool // 如果被处理，响应不应为空
	}{
		{
			name:         "help 命令",
			command:      "/help",
			wantHandled:  true,
			wantNonEmpty: true,
		},
		{
			name:         "session 命令",
			command:      "/session",
			wantHandled:  true,
			wantNonEmpty: true,
		},
		{
			name:         "model 命令",
			command:      "/model",
			wantHandled:  true,
			wantNonEmpty: true,
		},
		{
			name:         "空命令不处理",
			command:      "",
			wantHandled:  false,
			wantNonEmpty: false,
		},
		{
			name:         "普通文本不处理",
			command:      "hello world",
			wantHandled:  false,
			wantNonEmpty: false,
		},
		{
			name:         "未知 slash 命令不处理",
			command:      "/unknown_cmd",
			wantHandled:  false,
			wantNonEmpty: false,
		},
		{
			name:         "大写 HELP 命令",
			command:      "/HELP",
			wantHandled:  true,
			wantNonEmpty: true,
		},
		{
			name:         "help 命令带参数",
			command:      "/help some args",
			wantHandled:  true,
			wantNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handled, response := handleSlashCommand(tt.command, nil)
			assert.Equal(t, tt.wantHandled, handled)
			if tt.wantNonEmpty {
				assert.NotEmpty(t, response, "被处理的命令应返回非空响应")
			}
		})
	}
}

func TestBuildSlashCommands_NilRegistry(t *testing.T) {
	commands := buildSlashCommands(nil)
	assert.NotEmpty(t, commands)

	// 应包含内置命令
	names := make(map[string]bool)
	for _, cmd := range commands {
		names[cmd.Name] = true
	}
	assert.True(t, names["/help"])
	assert.True(t, names["/session"])
	assert.True(t, names["/model"])
}

func TestBuildSlashCommands_WithRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	reg := command.NewRegistry(logger)
	reg.Register(&command.Info{
		Name:        "review",
		Description: "代码审查",
		Source:      command.SourceBuiltin,
	})
	reg.Register(&command.Info{
		Name:        "test",
		Description: "运行测试",
		Source:      command.SourceBuiltin,
	})

	commands := buildSlashCommands(reg)

	names := make(map[string]bool)
	for _, cmd := range commands {
		names[cmd.Name] = true
	}
	// 应包含 Registry 中的命令和内置命令
	assert.True(t, names["/review"])
	assert.True(t, names["/test"])
	assert.True(t, names["/help"])
	assert.True(t, names["/session"])
}

func TestBuildHelpText(t *testing.T) {
	help := buildHelpText(nil)
	assert.NotEmpty(t, help)
	assert.Contains(t, help, "/help")
	assert.Contains(t, help, "/session")
	assert.Contains(t, help, "/model")
}

func TestBuildHelpText_WithRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)
	reg := command.NewRegistry(logger)
	reg.Register(&command.Info{
		Name:        "deploy",
		Description: "部署应用",
		Source:      command.SourceBuiltin,
	})

	help := buildHelpText(reg)
	assert.Contains(t, help, "/deploy")
	assert.Contains(t, help, "部署应用")
}

func TestToolKindFromName(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"bash 工具", "bash", "execute"},
		{"read_file 工具", "read_file", "read"},
		{"write_file 工具", "write_file", "edit"},
		{"edit 工具", "edit", "edit"},
		{"glob 工具", "glob", "read"},
		{"grep 工具", "grep", "read"},
		{"未知工具", "custom_tool", "other"},
		{"search 工具", "search_code", "search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind := toolKindFromName(tt.toolName)
			assert.Equal(t, tt.want, string(kind))
		})
	}
}
