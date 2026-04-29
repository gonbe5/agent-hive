package imctx

import "strings"

// ReferenceType 标识飞书文档资源类型。
type ReferenceType string

const (
	RefDocx     ReferenceType = "docx"
	RefDoc      ReferenceType = "doc"
	RefSheet    ReferenceType = "sheet"
	RefBitable  ReferenceType = "bitable"
	RefWiki     ReferenceType = "wiki"
	RefMindnote ReferenceType = "mindnote"
	RefFile     ReferenceType = "file"
	RefUnknown  ReferenceType = "unknown"
)

// DocRef 是消息中引用的飞书文档资源。
type DocRef struct {
	Token  string        `json:"token"`
	Type   ReferenceType `json:"type"`
	URL    string        `json:"url,omitempty"`
	Title  string        `json:"title,omitempty"`
	Source string        `json:"source,omitempty"` // "url" | "card" | "parent" | "share_*"
}

// Mention 是消息中 @ 的用户。
type Mention struct {
	Name   string `json:"name"`
	OpenID string `json:"open_id,omitempty"`
	IsBot  bool   `json:"is_bot"`
}

// NormalizeDocType 将 URL 路径段 / 卡片 schema 值 / SDK 返回的 obj_type 映射为 ReferenceType。
func NormalizeDocType(s string) ReferenceType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "docx", "doc":
		return RefDocx
	case "docs":
		return RefDoc
	case "sheets", "sheet":
		return RefSheet
	case "base", "bitable":
		return RefBitable
	case "wiki":
		return RefWiki
	case "mindnotes", "mindnote":
		return RefMindnote
	case "file":
		return RefFile
	default:
		return RefUnknown
	}
}
