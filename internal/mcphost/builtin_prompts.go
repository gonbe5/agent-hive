package mcphost

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// RegisterBuiltinPrompts 注册内置提示模板到 MCP Host
func RegisterBuiltinPrompts(host *Host, logger *zap.Logger) {
	// 1. summarize - 总结提示
	host.RegisterPrompt(
		PromptDefinition{
			Name:        "summarize",
			Description: "总结给定文本的要点",
			Arguments: []PromptArgument{
				{Name: "text", Description: "要总结的文本", Required: true},
			},
		},
		func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
			text, ok := args["text"]
			if !ok || text == "" {
				return nil, errs.New(errs.CodeInvalidInput, "缺少必需参数 text")
			}
			return []PromptMessage{
				{Role: "user", Content: fmt.Sprintf("请简洁地总结以下内容：\n\n%s", text)},
			}, nil
		},
	)

	// 2. translate - 翻译提示
	host.RegisterPrompt(
		PromptDefinition{
			Name:        "translate",
			Description: "将文本翻译为指定语言",
			Arguments: []PromptArgument{
				{Name: "text", Description: "要翻译的文本", Required: true},
				{Name: "language", Description: "目标语言", Required: true},
			},
		},
		func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
			text, ok := args["text"]
			if !ok || text == "" {
				return nil, errs.New(errs.CodeInvalidInput, "缺少必需参数 text")
			}
			language, ok := args["language"]
			if !ok || language == "" {
				return nil, errs.New(errs.CodeInvalidInput, "缺少必需参数 language")
			}
			return []PromptMessage{
				{Role: "user", Content: fmt.Sprintf("请将以下内容翻译为%s：\n\n%s", language, text)},
			}, nil
		},
	)

	// 3. code-review - 代码审查提示
	host.RegisterPrompt(
		PromptDefinition{
			Name:        "code-review",
			Description: "审查代码并给出改进建议",
			Arguments: []PromptArgument{
				{Name: "code", Description: "要审查的代码", Required: true},
				{Name: "language", Description: "编程语言", Required: false},
			},
		},
		func(_ context.Context, args map[string]string) ([]PromptMessage, error) {
			code, ok := args["code"]
			if !ok || code == "" {
				return nil, errs.New(errs.CodeInvalidInput, "缺少必需参数 code")
			}
			language := args["language"]
			if language == "" {
				return []PromptMessage{
					{Role: "user", Content: fmt.Sprintf("请审查以下代码，指出潜在问题和改进建议：\n\n```\n%s\n```", code)},
				}, nil
			}
			return []PromptMessage{
				{Role: "user", Content: fmt.Sprintf("请审查以下%s代码，指出潜在问题和改进建议：\n\n```%s\n%s\n```", language, language, code)},
			}, nil
		},
	)

	logger.Info("已注册内置提示模板", zap.Int("数量", 3))
}
