package i18n

import (
	"embed"
	"strings"
)

//go:embed prompts
var embeddedPrompts embed.FS

// loadEmbedded 从 go:embed 的 prompts 目录加载 prompt（第三层 fallback）
// relPath 格式如 "system/base"，自动补 .md 后缀
func loadEmbedded(relPath string) string {
	// 规范化路径：去掉前导 /，补 .md
	relPath = strings.TrimPrefix(relPath, "/")
	data, err := embeddedPrompts.ReadFile("prompts/" + relPath + ".md")
	if err != nil {
		return ""
	}
	return string(data)
}
