package feishu

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

// feishuURLRe 匹配飞书文档 URL，提取资源类型和 token。
var feishuURLRe = regexp.MustCompile(
	`https?://[^\s]*\.feishu\.(?:cn|net|us)/(docx|docs|sheets|base|wiki|file|mindnotes)/([A-Za-z0-9]+)`)

// parseDocURL 从飞书文档 URL 中提取 DocRef。
func parseDocURL(u string) (imctx.DocRef, bool) {
	m := feishuURLRe.FindStringSubmatch(u)
	if len(m) != 3 {
		return imctx.DocRef{}, false
	}
	token := strings.TrimRight(m[2], "?#")
	return imctx.DocRef{
		Token:  token,
		Type:   imctx.NormalizeDocType(m[1]),
		URL:    u,
		Source: "url",
	}, true
}

// extractRefsFromText 从纯文本中提取所有飞书文档引用。
func extractRefsFromText(text string) []imctx.DocRef {
	matches := feishuURLRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var refs []imctx.DocRef
	for _, u := range matches {
		if ref, ok := parseDocURL(u); ok {
			refs = append(refs, ref)
		}
	}
	return deduplicateRefs(refs)
}

// extractRefsFromAnyJSON 递归扫描任意 JSON 里的字符串字段，提取飞书文档链接。
// 用于 interactive/share_* 等消息：这类消息常把真实链接埋在卡片 schema/链接字段里，
// 若只看顶层 message_type 会表现成“无法识别飞书文档”。
func extractRefsFromAnyJSON(contentJSON string, source string) []imctx.DocRef {
	if strings.TrimSpace(contentJSON) == "" {
		return nil
	}
	var raw any
	if err := json.Unmarshal([]byte(contentJSON), &raw); err != nil {
		return nil
	}
	var refs []imctx.DocRef
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, ref := range extractRefsFromText(x) {
				ref.Source = source
				refs = append(refs, ref)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		case map[string]any:
			for _, ref := range extractRefsFromTokenFields(x, source) {
				refs = append(refs, ref)
			}
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(raw)
	return deduplicateRefs(refs)
}

func extractRefsFromTokenFields(obj map[string]any, source string) []imctx.DocRef {
	type pair struct {
		tokenKey string
		typeKey  string
	}
	pairs := []pair{
		{tokenKey: "docs_token", typeKey: "docs_type"},
		{tokenKey: "obj_token", typeKey: "obj_type"},
		{tokenKey: "doc_token", typeKey: "doc_type"},
	}
	var refs []imctx.DocRef
	for _, p := range pairs {
		token, tokOK := obj[p.tokenKey].(string)
		docType, typeOK := obj[p.typeKey].(string)
		token = strings.TrimSpace(token)
		if !tokOK || !typeOK || token == "" {
			continue
		}
		refType := imctx.NormalizeDocType(docType)
		if refType == imctx.RefUnknown {
			continue
		}
		refs = append(refs, imctx.DocRef{
			Token:  token,
			Type:   refType,
			Source: source,
		})
	}
	return refs
}

// extractRefsFromPost 从富文本 post 的 href 链接中提取文档引用。
func extractRefsFromPost(contentJSON string) []imctx.DocRef {
	var wrapper FeishuPostWrapper
	if err := json.Unmarshal([]byte(contentJSON), &wrapper); err != nil {
		return nil
	}
	post := wrapper.ZhCN
	if post == nil {
		post = wrapper.EnUS
	}
	if post == nil {
		return nil
	}
	var refs []imctx.DocRef
	for _, line := range post.Content {
		for _, entry := range line {
			if entry.Tag == "a" && entry.Href != "" {
				if ref, ok := parseDocURL(entry.Href); ok {
					refs = append(refs, ref)
				}
			}
			if entry.Tag == "text" && entry.Text != "" {
				refs = append(refs, extractRefsFromText(entry.Text)...)
			}
		}
	}
	return deduplicateRefs(refs)
}

// deduplicateRefs 按 {Token, Type} 去重，保留首次出现的 Source。
func deduplicateRefs(refs []imctx.DocRef) []imctx.DocRef {
	if len(refs) <= 1 {
		return refs
	}
	type key struct {
		Token string
		Type  imctx.ReferenceType
	}
	seen := make(map[key]struct{}, len(refs))
	out := make([]imctx.DocRef, 0, len(refs))
	for _, r := range refs {
		k := key{r.Token, r.Type}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, r)
	}
	return out
}

func formatRefsForLog(refs []imctx.DocRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, fmt.Sprintf("%s:%s@%s", ref.Type, ref.Token, ref.Source))
	}
	return out
}
