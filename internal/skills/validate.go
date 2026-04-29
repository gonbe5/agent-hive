package skills

import (
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// forbiddenFrontmatterKeys 禁止用户在 DB skill 中设置的 frontmatter 字段
// 这些字段涉及 hook/脚本执行，不允许通过 DB 配置（安全边界）
var forbiddenFrontmatterKeys = []string{
	"hooks",
	"scripts",
	"context",  // context=fork 可启动子 agent，禁止 DB 覆盖
	"agent",
}

// ValidateTemplateSkill 验证 DB skill 内容的安全性。
// 禁止包含 hooks、scripts、context=fork 等可执行配置。
// 仅做静态文本检查，不解析完整 frontmatter（避免循环依赖）。
func ValidateTemplateSkill(content string) error {
	// 提取 frontmatter 区域（--- ... ---）
	fm := extractFrontmatter(content)
	lower := strings.ToLower(fm)
	for _, key := range forbiddenFrontmatterKeys {
		// 检查 key: 或 key : 形式
		if strings.Contains(lower, key+":") || strings.Contains(lower, key+" :") {
			return errs.New(errs.CodeInvalidInput,
				"DB skill 禁止包含 "+key+" 配置（安全限制）")
		}
	}
	return nil
}

// extractFrontmatter 提取 YAML frontmatter 内容（--- ... --- 之间）
func extractFrontmatter(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	// 跳过第一个 ---
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
