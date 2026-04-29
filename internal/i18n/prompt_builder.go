package i18n

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PromptBuilder 动态提示词组装器，支持链式调用
type PromptBuilder struct {
	logger   *zap.Logger
	pm       *PromptManager
	provider ProviderKey

	// 动态上下文
	gitBranch    string
	gitDirty     bool
	currentDate  string
	workDir      string
	osInfo       string
	extraContext map[string]string
}

// NewPromptBuilder 创建新的 PromptBuilder
func NewPromptBuilder(pm *PromptManager, logger *zap.Logger) *PromptBuilder {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PromptBuilder{
		logger:       logger,
		pm:           pm,
		provider:     ProviderDefault,
		extraContext:  make(map[string]string),
	}
}

// WithProvider 设置 LLM 提供商
func (b *PromptBuilder) WithProvider(provider ProviderKey) *PromptBuilder {
	b.provider = provider
	return b
}

// WithGitStatus 注入 Git 状态信息（自动检测）
func (b *PromptBuilder) WithGitStatus() *PromptBuilder {
	branch, err := execCommand("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		b.logger.Debug("获取 Git 分支失败", zap.Error(err))
		b.gitBranch = ""
	} else {
		b.gitBranch = strings.TrimSpace(branch)
	}

	status, err := execCommand("git", "status", "--porcelain")
	if err != nil {
		b.logger.Debug("获取 Git 状态失败", zap.Error(err))
		b.gitDirty = false
	} else {
		b.gitDirty = strings.TrimSpace(status) != ""
	}

	return b
}

// WithGitInfo 手动设置 Git 信息（用于测试或已知场景）
func (b *PromptBuilder) WithGitInfo(branch string, dirty bool) *PromptBuilder {
	b.gitBranch = branch
	b.gitDirty = dirty
	return b
}

// WithCurrentDate 注入当前日期
func (b *PromptBuilder) WithCurrentDate() *PromptBuilder {
	b.currentDate = time.Now().Format("2006-01-02")
	return b
}

// WithDate 手动设置日期（用于测试）
func (b *PromptBuilder) WithDate(date string) *PromptBuilder {
	b.currentDate = date
	return b
}

// WithWorkDir 设置工作目录路径
func (b *PromptBuilder) WithWorkDir(dir string) *PromptBuilder {
	b.workDir = dir
	return b
}

// WithOSInfo 注入操作系统/平台信息（自动检测）
func (b *PromptBuilder) WithOSInfo() *PromptBuilder {
	b.osInfo = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	return b
}

// WithExtra 注入自定义上下文键值对
func (b *PromptBuilder) WithExtra(key, value string) *PromptBuilder {
	b.extraContext[key] = value
	return b
}

// BuildContext 只生成格式化后的动态上下文块（不含基础提示词）
// 适用于已有基础提示词、只需追加上下文的场景
func (b *PromptBuilder) BuildContext() string {
	contextParts := b.buildContextParts()
	if len(contextParts) == 0 {
		return ""
	}
	return b.formatContext(contextParts)
}

// Build 组装最终的系统提示词
func (b *PromptBuilder) Build(key PromptKey) string {
	// 获取基础提示词
	var basePrompt string
	if b.provider != ProviderDefault {
		basePrompt = b.pm.GetPromptForProvider(key, b.provider)
	} else {
		basePrompt = b.pm.GetPrompt(key)
	}

	if basePrompt == "" {
		b.logger.Warn("未找到提示词", zap.String("key", string(key)), zap.String("provider", string(b.provider)))
		return ""
	}

	// 组装动态上下文
	contextParts := b.buildContextParts()
	if len(contextParts) == 0 {
		return basePrompt
	}

	// 根据 provider 选择上下文格式
	contextBlock := b.formatContext(contextParts)
	return basePrompt + "\n\n" + contextBlock
}

// buildContextParts 构建所有动态上下文条目
func (b *PromptBuilder) buildContextParts() []contextEntry {
	var parts []contextEntry

	if b.currentDate != "" {
		parts = append(parts, contextEntry{
			key:   "current_date",
			label: "当前日期",
			value: b.currentDate,
		})
	}

	if b.workDir != "" {
		parts = append(parts, contextEntry{
			key:   "work_dir",
			label: "工作目录",
			value: b.workDir,
		})
	}

	if b.osInfo != "" {
		parts = append(parts, contextEntry{
			key:   "os",
			label: "操作系统",
			value: b.osInfo,
		})
	}

	if b.gitBranch != "" {
		dirtyFlag := "否"
		if b.gitDirty {
			dirtyFlag = "是"
		}
		parts = append(parts, contextEntry{
			key:   "git_branch",
			label: "Git 分支",
			value: b.gitBranch,
		})
		parts = append(parts, contextEntry{
			key:   "git_dirty",
			label: "未提交修改",
			value: dirtyFlag,
		})
	}

	for k, v := range b.extraContext {
		parts = append(parts, contextEntry{
			key:   k,
			label: k,
			value: v,
		})
	}

	return parts
}

// contextEntry 上下文条目
type contextEntry struct {
	key   string
	label string
	value string
}

// formatContext 根据 provider 偏好格式化上下文块
func (b *PromptBuilder) formatContext(parts []contextEntry) string {
	switch b.provider {
	case ProviderClaude:
		return b.formatContextXML(parts)
	case ProviderGPT:
		return b.formatContextMarkdown(parts)
	default:
		return b.formatContextPlain(parts)
	}
}

// formatContextXML XML 标签格式（Claude 偏好）
func (b *PromptBuilder) formatContextXML(parts []contextEntry) string {
	var sb strings.Builder
	sb.WriteString("<context>\n")
	for _, p := range parts {
		sb.WriteString(fmt.Sprintf("<%s>%s</%s>\n", p.key, p.value, p.key))
	}
	sb.WriteString("</context>")
	return sb.String()
}

// formatContextMarkdown Markdown 格式（GPT 偏好）
func (b *PromptBuilder) formatContextMarkdown(parts []contextEntry) string {
	var sb strings.Builder
	sb.WriteString("## Context\n")
	for _, p := range parts {
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", p.label, p.value))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// formatContextPlain 纯文本格式（通用）
func (b *PromptBuilder) formatContextPlain(parts []contextEntry) string {
	var sb strings.Builder
	sb.WriteString("[Context]\n")
	for _, p := range parts {
		sb.WriteString(fmt.Sprintf("%s: %s\n", p.label, p.value))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// execCommand 执行外部命令并返回输出（可被测试替换）
var execCommand = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
