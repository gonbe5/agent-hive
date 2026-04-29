package command

import (
	"fmt"
	"regexp"
	"strings"
)

// Source 命令来源
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceConfig  Source = "config"
	SourceMCP     Source = "mcp"
	SourceSkill   Source = "skill"
)

// sourcePriority 返回来源的优先级数字（越小越高）
func sourcePriority(s Source) int {
	switch s {
	case SourceBuiltin:
		return 0
	case SourceConfig:
		return 1
	case SourceMCP:
		return 2
	case SourceSkill:
		return 3
	default:
		return 99
	}
}

// Info 描述一个可调用的命令
type Info struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	Source      Source   `json:"source"`
	Template    string   `json:"template"`
	Subtask     bool     `json:"subtask,omitempty"`
	Hints       []string `json:"hints,omitempty"`
}

// Render 将模板中的占位符替换为实际参数
// 支持: $1, $2, ..., $ARGUMENTS（全部参数空格拼接）
func (c *Info) Render(args []string) string {
	result := c.Template
	// 替换 $ARGUMENTS
	result = strings.ReplaceAll(result, "$ARGUMENTS", strings.Join(args, " "))
	// 替换 $1, $2, ...（从后往前替换，避免 $1 匹配 $10 的前缀）
	for i := len(args); i >= 1; i-- {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i), args[i-1])
	}
	return result
}

// reHintPattern 匹配模板中的占位符: $1, $2, ..., $ARGUMENTS
var reHintPattern = regexp.MustCompile(`\$(?:ARGUMENTS|\d+)`)

// ExtractHints 从模板中提取占位符提示（$1, $2, $ARGUMENTS）
func ExtractHints(template string) []string {
	matches := reHintPattern.FindAllString(template, -1)
	if len(matches) == 0 {
		return nil
	}
	// 去重并保持顺序
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}
