package i18n

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestPromptBuilder_Build_Default(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop())

	// 不注入任何上下文，应返回原始提示词
	result := builder.Build(PromptCodeReview)
	if result == "" {
		t.Fatal("Build() 返回空字符串")
	}
	if !containsChinese(result) {
		t.Error("期望中文提示词")
	}
}

func TestPromptBuilder_WithProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderKey
		key      PromptKey
		contains string // 预期包含的关键标记
	}{
		{
			name:     "Claude 使用 XML 标签",
			provider: ProviderClaude,
			key:      PromptCodeReview,
			contains: "<rules>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := NewPromptManager("en-US")
			builder := NewPromptBuilder(pm, zap.NewNop()).
				WithProvider(tt.provider)

			result := builder.Build(tt.key)
			if result == "" {
				t.Fatal("Build() 返回空字符串")
			}
			if !strings.Contains(result, tt.contains) {
				t.Errorf("期望包含 %q，实际内容前 200 字符: %s", tt.contains, truncate(result, 200))
			}
		})
	}
}

func TestPromptBuilder_ProviderFallback(t *testing.T) {
	pm := NewPromptManager("en-US")

	// DeepSeek 没有注册 PromptCodeReview，应降级到默认模板
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithProvider(ProviderDeepSeek)

	result := builder.Build(PromptCodeReview)
	if result == "" {
		t.Fatal("降级失败，Build() 返回空字符串")
	}

	// 应该返回默认的代码审查提示词
	defaultPrompt := pm.GetPrompt(PromptCodeReview)
	if result != defaultPrompt {
		t.Error("降级后的提示词应与默认模板一致")
	}
}

func TestPromptBuilder_WithGitInfo(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithGitInfo("feature/test", true)

	result := builder.Build(PromptCodeReview)
	if !strings.Contains(result, "feature/test") {
		t.Error("期望包含 Git 分支名")
	}
	if !strings.Contains(result, "是") {
		t.Error("期望包含未提交修改标记")
	}
}

func TestPromptBuilder_WithGitInfo_Clean(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithGitInfo("main", false)

	result := builder.Build(PromptCodeReview)
	if !strings.Contains(result, "main") {
		t.Error("期望包含 Git 分支名 main")
	}
	if !strings.Contains(result, "否") {
		t.Error("期望未提交修改为否")
	}
}

func TestPromptBuilder_WithDate(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithDate("2026-03-18")

	result := builder.Build(PromptCodeReview)
	if !strings.Contains(result, "2026-03-18") {
		t.Error("期望包含日期")
	}
}

func TestPromptBuilder_WithWorkDir(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithWorkDir("/home/user/project")

	result := builder.Build(PromptCodeReview)
	if !strings.Contains(result, "/home/user/project") {
		t.Error("期望包含工作目录路径")
	}
}

func TestPromptBuilder_WithOSInfo(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithOSInfo()

	result := builder.Build(PromptCodeReview)
	// 应包含 os/arch 格式
	if !strings.Contains(result, "/") {
		t.Error("期望包含操作系统信息（os/arch 格式）")
	}
}

func TestPromptBuilder_WithExtra(t *testing.T) {
	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithExtra("project_name", "agents-hive").
		WithExtra("version", "1.0.0")

	result := builder.Build(PromptCodeReview)
	if !strings.Contains(result, "agents-hive") {
		t.Error("期望包含自定义上下文 project_name")
	}
	if !strings.Contains(result, "1.0.0") {
		t.Error("期望包含自定义上下文 version")
	}
}

func TestPromptBuilder_ChainedCall(t *testing.T) {
	pm := NewPromptManager("en-US")
	result := NewPromptBuilder(pm, zap.NewNop()).
		WithProvider(ProviderClaude).
		WithGitInfo("develop", true).
		WithDate("2026-03-18").
		WithWorkDir("/workspace").
		WithOSInfo().
		WithExtra("user", "test-user").
		Build(PromptCodeReview)

	if result == "" {
		t.Fatal("链式调用 Build() 返回空字符串")
	}

	// Claude 格式应使用 XML 上下文
	if !strings.Contains(result, "<context>") {
		t.Error("Claude provider 应使用 XML 格式上下文")
	}
	if !strings.Contains(result, "<current_date>2026-03-18</current_date>") {
		t.Error("期望包含日期的 XML 标签")
	}
	if !strings.Contains(result, "<work_dir>/workspace</work_dir>") {
		t.Error("期望包含工作目录的 XML 标签")
	}
	if !strings.Contains(result, "<git_branch>develop</git_branch>") {
		t.Error("期望包含 Git 分支的 XML 标签")
	}
}

func TestPromptBuilder_ContextFormat_GPT(t *testing.T) {
	pm := NewPromptManager("en-US")
	result := NewPromptBuilder(pm, zap.NewNop()).
		WithProvider(ProviderGPT).
		WithDate("2026-03-18").
		WithWorkDir("/workspace").
		Build(PromptCodeReview)

	// GPT 格式应使用 Markdown
	if !strings.Contains(result, "## Context") {
		t.Error("GPT provider 应使用 Markdown 格式上下文")
	}
	if !strings.Contains(result, "**当前日期**") {
		t.Error("期望包含 Markdown 粗体标签")
	}
}

func TestPromptBuilder_ContextFormat_Plain(t *testing.T) {
	pm := NewPromptManager("en-US")
	result := NewPromptBuilder(pm, zap.NewNop()).
		WithProvider(ProviderDefault).
		WithDate("2026-03-18").
		Build(PromptCodeReview)

	// 默认格式应使用纯文本
	if !strings.Contains(result, "[Context]") {
		t.Error("Default provider 应使用纯文本格式上下文")
	}
}

func TestPromptBuilder_NoContext(t *testing.T) {
	pm := NewPromptManager("en-US")
	builder := NewPromptBuilder(pm, zap.NewNop())

	// 不注入任何上下文，应返回纯基础提示词（无额外换行）
	result := builder.Build(PromptCodeReview)
	basePrompt := pm.GetPrompt(PromptCodeReview)
	if result != basePrompt {
		t.Error("无上下文时应返回与 GetPrompt 相同的结果")
	}
}

func TestPromptBuilder_UnknownKey(t *testing.T) {
	pm := NewPromptManager("en-US")
	builder := NewPromptBuilder(pm, zap.NewNop())

	result := builder.Build("nonexistent.key")
	if result != "" {
		t.Error("未知 key 应返回空字符串")
	}
}

func TestPromptBuilder_NilLogger(t *testing.T) {
	pm := NewPromptManager("en-US")
	// 传入 nil logger 不应 panic
	builder := NewPromptBuilder(pm, nil)
	result := builder.Build(PromptCodeReview)
	if result == "" {
		t.Error("nil logger 不应影响 Build 结果")
	}
}

func TestPromptBuilder_WithGitStatus_MockSuccess(t *testing.T) {
	// 保存原始函数
	origExec := execCommand
	defer func() { execCommand = origExec }()

	callCount := 0
	execCommand = func(name string, args ...string) (string, error) {
		callCount++
		if len(args) > 0 && args[0] == "rev-parse" {
			return "mock-branch\n", nil
		}
		if len(args) > 0 && args[0] == "status" {
			return " M file.go\n", nil
		}
		return "", nil
	}

	pm := NewPromptManager("zh-CN")
	builder := NewPromptBuilder(pm, zap.NewNop()).
		WithGitStatus()

	if builder.gitBranch != "mock-branch" {
		t.Errorf("期望 gitBranch=mock-branch，实际=%s", builder.gitBranch)
	}
	if !builder.gitDirty {
		t.Error("期望 gitDirty=true")
	}
}

func TestGetPromptForProvider_Direct(t *testing.T) {
	pm := NewPromptManager("en-US")

	// Claude 应返回带 XML 标签的 CodeReview 版本
	claudePrompt := pm.GetPromptForProvider(PromptCodeReview, ProviderClaude)
	if !strings.Contains(claudePrompt, "<rules>") {
		t.Error("Claude 提示词应包含 XML 标签")
	}

	// 未注册的 provider key 应降级到默认
	defaultPrompt := pm.GetPromptForProvider(PromptCodeReview, "unknown-provider")
	basePrompt := pm.GetPrompt(PromptCodeReview)
	if defaultPrompt != basePrompt {
		t.Error("未知 provider 应降级到默认提示词")
	}
}

func TestRegisterProviderPrompt(t *testing.T) {
	pm := NewPromptManager("en-US")

	// 动态注册自定义 provider 提示词
	pm.RegisterProviderPrompt("custom", PromptCodeReview, map[string]string{
		"en-US": "Custom review prompt",
		"zh-CN": "自定义审查提示词",
	})

	result := pm.GetPromptForProvider(PromptCodeReview, "custom")
	if result != "Custom review prompt" {
		t.Errorf("期望 Custom review prompt，实际=%s", result)
	}

	// 切换语言
	pm.SetLanguage("zh-CN")
	result = pm.GetPromptForProvider(PromptCodeReview, "custom")
	if result != "自定义审查提示词" {
		t.Errorf("期望自定义审查提示词，实际=%s", result)
	}
}

// truncate 截断字符串用于错误消息展示
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
