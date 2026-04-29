// Package acpserver 实现 ACP (Agent Client Protocol) 协议服务器
package acpserver

import (
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"

	"github.com/chef-guo/agents-hive/internal/command"
)

// buildSlashCommands 从 command.Registry 动态构建 ACP 可用命令列表
// 如果 registry 为 nil 则返回内置命令
func buildSlashCommands(registry *command.Registry) []acp.AvailableCommand {
	// 内置 ACP 命令（始终包含）
	builtins := []acp.AvailableCommand{
		{Name: "/help", Description: "显示可用命令帮助信息"},
		{Name: "/session", Description: "显示当前会话信息"},
		{Name: "/model", Description: "显示当前使用的模型信息"},
	}

	if registry == nil {
		return builtins
	}

	// 从 command.Registry 加载注册的命令
	commands := registry.List()
	result := make([]acp.AvailableCommand, 0, len(commands)+len(builtins))

	for _, cmd := range commands {
		result = append(result, acp.AvailableCommand{
			Name:        "/" + cmd.Name,
			Description: cmd.Description,
		})
	}

	// 追加内置命令（去重：如果 Registry 中已有同名命令则跳过内置的）
	registered := make(map[string]bool, len(result))
	for _, cmd := range result {
		registered[cmd.Name] = true
	}
	for _, b := range builtins {
		if !registered[b.Name] {
			result = append(result, b)
		}
	}

	return result
}

// handleSlashCommand 处理 slash 命令（在 Prompt 中检测并路由）
// 返回 (handled bool, response string)
// 如果命令已被处理，handled 为 true，response 为对应的回复内容
func handleSlashCommand(input string, registry *command.Registry) (bool, string) {
	if input == "" || !strings.HasPrefix(input, "/") {
		return false, ""
	}

	// 提取命令名（去掉前缀 / 并截取到第一个空格）
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), " ", 2)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "help":
		return true, buildHelpText(registry)
	case "session":
		return true, fmt.Sprintf("当前会话信息：\n- 协议版本：%d", acp.ProtocolVersionNumber)
	case "model":
		return true, "当前使用的模型信息请通过配置文件中 llm.model 字段查看"
	default:
		// 未知 slash 命令，不拦截，让 Master 处理
		return false, ""
	}
}

// buildHelpText 构建 /help 命令的帮助文本
func buildHelpText(registry *command.Registry) string {
	var sb strings.Builder
	sb.WriteString("可用命令：\n")

	// 内置命令
	sb.WriteString("/help    - 显示此帮助信息\n")
	sb.WriteString("/session - 显示当前会话信息\n")
	sb.WriteString("/model   - 显示当前使用的模型信息\n")

	// 动态加载 Registry 中的命令
	if registry != nil {
		commands := registry.List()
		if len(commands) > 0 {
			sb.WriteString("\n注册命令：\n")
			for _, cmd := range commands {
				desc := cmd.Description
				if desc == "" {
					desc = "无描述"
				}
				sb.WriteString(fmt.Sprintf("/%s - %s\n", cmd.Name, desc))
			}
		}
	}

	return sb.String()
}

// toolKindFromName 根据工具名推断 ACP ToolKind
func toolKindFromName(name string) acp.ToolKind {
	switch {
	case strings.HasPrefix(name, "read") || name == "glob" || name == "grep" || name == "lsp":
		return acp.ToolKindRead
	case strings.HasPrefix(name, "write") || strings.HasPrefix(name, "edit") || name == "applypatch" || name == "multiedit":
		return acp.ToolKindEdit
	case name == "bash" || name == "shell":
		return acp.ToolKindExecute
	case strings.Contains(name, "search") || strings.Contains(name, "find"):
		return acp.ToolKindSearch
	case strings.Contains(name, "fetch") || strings.Contains(name, "http"):
		return acp.ToolKindFetch
	default:
		return acp.ToolKindOther
	}
}
