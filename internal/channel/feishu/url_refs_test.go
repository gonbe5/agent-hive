package feishu

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

func TestParseDocURL(t *testing.T) {
	tests := []struct {
		url       string
		wantOK    bool
		wantToken string
		wantType  imctx.ReferenceType
	}{
		{"https://abc.feishu.cn/docx/ABC123", true, "ABC123", imctx.RefDocx},
		{"https://abc.feishu.cn/docs/DEF456", true, "DEF456", imctx.RefDoc},
		{"https://abc.feishu.cn/sheets/GHI789", true, "GHI789", imctx.RefSheet},
		{"https://abc.feishu.cn/base/JKL012", true, "JKL012", imctx.RefBitable},
		{"https://abc.feishu.cn/wiki/MNO345", true, "MNO345", imctx.RefWiki},
		{"https://abc.feishu.cn/file/PQR678", true, "PQR678", imctx.RefFile},
		{"https://abc.feishu.cn/mindnotes/STU901", true, "STU901", imctx.RefMindnote},
		{"https://abc.feishu.net/docx/Token1", true, "Token1", imctx.RefDocx},
		{"https://abc.feishu.us/sheets/Token2", true, "Token2", imctx.RefSheet},
		{"https://example.com/docx/ABC123", false, "", ""},
		{"not a url", false, "", ""},
		{"", false, "", ""},
	}
	for _, tt := range tests {
		ref, ok := parseDocURL(tt.url)
		if ok != tt.wantOK {
			t.Errorf("parseDocURL(%q) ok=%v, want %v", tt.url, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if ref.Token != tt.wantToken {
			t.Errorf("parseDocURL(%q) token=%q, want %q", tt.url, ref.Token, tt.wantToken)
		}
		if ref.Type != tt.wantType {
			t.Errorf("parseDocURL(%q) type=%q, want %q", tt.url, ref.Type, tt.wantType)
		}
		if ref.Source != "url" {
			t.Errorf("parseDocURL(%q) source=%q, want 'url'", tt.url, ref.Source)
		}
	}
}

func TestExtractRefsFromText(t *testing.T) {
	text := "请看这个文档 https://abc.feishu.cn/docx/ABC123 和这个表格 https://abc.feishu.cn/sheets/DEF456"
	refs := extractRefsFromText(text)
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d", len(refs))
	}
	if refs[0].Token != "ABC123" || refs[0].Type != imctx.RefDocx {
		t.Errorf("ref[0]=%+v", refs[0])
	}
	if refs[1].Token != "DEF456" || refs[1].Type != imctx.RefSheet {
		t.Errorf("ref[1]=%+v", refs[1])
	}
}

func TestExtractRefsFromText_NoDuplicates(t *testing.T) {
	text := "https://abc.feishu.cn/docx/ABC123 重复 https://abc.feishu.cn/docx/ABC123"
	refs := extractRefsFromText(text)
	if len(refs) != 1 {
		t.Fatalf("want 1 ref (deduped), got %d", len(refs))
	}
}

func TestExtractRefsFromText_NoURLs(t *testing.T) {
	refs := extractRefsFromText("普通文本没有链接")
	if len(refs) != 0 {
		t.Fatalf("want 0 refs, got %d", len(refs))
	}
}

func TestExtractRefsFromPost(t *testing.T) {
	content := mustMarshal(FeishuPostWrapper{
		ZhCN: &FeishuPostContent{
			Title: "测试",
			Content: [][]FeishuPostEntry{
				{
					{Tag: "text", Text: "看看 https://abc.feishu.cn/wiki/WikiToken1"},
					{Tag: "a", Text: "链接", Href: "https://abc.feishu.cn/docx/DocToken2"},
				},
			},
		},
	})
	refs := extractRefsFromPost(content)
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
}

func TestExtractRefsFromAnyJSON_DocTokens(t *testing.T) {
	content := mustMarshal(map[string]any{
		"target": map[string]any{
			"docs_token": "doccnToken123",
			"docs_type":  "docx",
		},
	})
	refs := extractRefsFromAnyJSON(content, "card")
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Token != "doccnToken123" || refs[0].Type != imctx.RefDocx {
		t.Fatalf("ref=%+v", refs[0])
	}
}

func TestExtractRefsFromAnyJSON_ObjTokens(t *testing.T) {
	content := mustMarshal(map[string]any{
		"node": map[string]any{
			"obj_token": "sheetToken456",
			"obj_type":  "sheet",
		},
	})
	refs := extractRefsFromAnyJSON(content, "message")
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Token != "sheetToken456" || refs[0].Type != imctx.RefSheet {
		t.Fatalf("ref=%+v", refs[0])
	}
}

func TestParseInboundMessage_TextWithRefs(t *testing.T) {
	content := mustMarshal(FeishuTextContent{Text: "请看 https://abc.feishu.cn/docx/Token123"})
	p := ParseInboundMessage("text", content)
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "Token123" || p.References[0].Type != imctx.RefDocx {
		t.Errorf("ref=%+v", p.References[0])
	}
}

func TestParseInboundMessage_PostWithRefs(t *testing.T) {
	content := mustMarshal(FeishuPostWrapper{
		ZhCN: &FeishuPostContent{
			Content: [][]FeishuPostEntry{
				{{Tag: "a", Text: "文档", Href: "https://abc.feishu.cn/sheets/SheetTok"}},
			},
		},
	})
	p := ParseInboundMessage("post", content)
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "SheetTok" || p.References[0].Type != imctx.RefSheet {
		t.Errorf("ref=%+v", p.References[0])
	}
}
