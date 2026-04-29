package feishu

import (
	"encoding/json"
	"testing"
)

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestParseInboundMessage_Text(t *testing.T) {
	content := mustMarshal(FeishuTextContent{Text: "hello world"})
	p := ParseInboundMessage("text", content)
	if p.Type != ParsedText {
		t.Fatalf("type=%s, want text", p.Type)
	}
	if p.TextContent != "hello world" {
		t.Fatalf("text=%q, want 'hello world'", p.TextContent)
	}
	if len(p.Attachments) != 0 {
		t.Fatalf("text should have no attachments")
	}
}

func TestParseInboundMessage_Image(t *testing.T) {
	content := mustMarshal(FeishuImageContent{ImageKey: "img_v2_abc123"})
	p := ParseInboundMessage("image", content)
	if p.Type != ParsedImage {
		t.Fatalf("type=%s, want image", p.Type)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("image should have 1 attachment, got %d", len(p.Attachments))
	}
	att := p.Attachments[0]
	if att.Type != "image" || att.Key != "img_v2_abc123" {
		t.Fatalf("attachment=%+v, want type=image key=img_v2_abc123", att)
	}
}

func TestParseInboundMessage_File(t *testing.T) {
	content := mustMarshal(FeishuFileContent{FileKey: "file_v2_xyz", FileName: "report.pdf"})
	p := ParseInboundMessage("file", content)
	if p.Type != ParsedFile {
		t.Fatalf("type=%s, want file", p.Type)
	}
	if len(p.Attachments) != 1 {
		t.Fatalf("file should have 1 attachment, got %d", len(p.Attachments))
	}
	att := p.Attachments[0]
	if att.Key != "file_v2_xyz" || att.FileName != "report.pdf" {
		t.Fatalf("attachment=%+v", att)
	}
}

func TestParseInboundMessage_Audio(t *testing.T) {
	content := mustMarshal(map[string]any{"file_key": "audio_key_1", "duration": 5000})
	p := ParseInboundMessage("audio", content)
	if p.Type != ParsedAudio {
		t.Fatalf("type=%s, want audio", p.Type)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Key != "audio_key_1" {
		t.Fatalf("audio attachment=%+v", p.Attachments)
	}
}

func TestParseInboundMessage_Video(t *testing.T) {
	content := mustMarshal(map[string]any{"file_key": "video_key_1", "image_key": "thumb_1"})
	p := ParseInboundMessage("video", content)
	if p.Type != ParsedVideo {
		t.Fatalf("type=%s, want video", p.Type)
	}
	if len(p.Attachments) != 2 {
		t.Fatalf("video should have 2 attachments (media + thumb), got %d", len(p.Attachments))
	}
}

func TestParseInboundMessage_Post(t *testing.T) {
	post := FeishuPostWrapper{
		ZhCN: &FeishuPostContent{
			Title: "测试标题",
			Content: [][]FeishuPostEntry{
				{
					{Tag: "text", Text: "第一行"},
					{Tag: "img", ImageKey: "img_post_1"},
				},
				{
					{Tag: "a", Text: "链接", Href: "https://example.com"},
				},
			},
		},
	}
	content := mustMarshal(post)
	p := ParseInboundMessage("post", content)
	if p.Type != ParsedPost {
		t.Fatalf("type=%s, want post", p.Type)
	}
	if len(p.Attachments) != 1 || p.Attachments[0].Key != "img_post_1" {
		t.Fatalf("post should extract 1 image attachment, got %+v", p.Attachments)
	}
	if p.TextContent == "" {
		t.Fatal("post text should not be empty")
	}
}

func TestParseInboundMessage_Location(t *testing.T) {
	content := mustMarshal(map[string]any{"name": "北京市朝阳区", "longitude": "116.4", "latitude": "39.9"})
	p := ParseInboundMessage("location", content)
	if p.Type != ParsedLocation {
		t.Fatalf("type=%s, want location", p.Type)
	}
	if p.TextContent != "[位置: 北京市朝阳区]" {
		t.Fatalf("text=%q", p.TextContent)
	}
}

func TestParseInboundMessage_ShareChat(t *testing.T) {
	content := mustMarshal(map[string]any{"chat_id": "oc_xxx"})
	p := ParseInboundMessage("share_chat", content)
	if p.Type != ParsedShareChat {
		t.Fatalf("type=%s, want share_chat", p.Type)
	}
}

func TestParseInboundMessage_InteractiveWithRefs(t *testing.T) {
	content := mustMarshal(map[string]any{
		"elements": []any{
			map[string]any{
				"tag":  "markdown",
				"text": "请看 https://abc.feishu.cn/wiki/WikiToken1",
			},
		},
	})
	p := ParseInboundMessage("interactive", content)
	if p.Type != ParsedInteractive {
		t.Fatalf("type=%s, want interactive", p.Type)
	}
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "WikiToken1" {
		t.Fatalf("token=%q, want WikiToken1", p.References[0].Token)
	}
}

func TestParseInboundMessage_ShareChatWithRefs(t *testing.T) {
	content := mustMarshal(map[string]any{
		"chat_id": "oc_xxx",
		"link":    "https://abc.feishu.cn/docx/DocTokenABC",
	})
	p := ParseInboundMessage("share_chat", content)
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "DocTokenABC" {
		t.Fatalf("token=%q, want DocTokenABC", p.References[0].Token)
	}
}

func TestParseInboundMessage_MergeForward(t *testing.T) {
	content := mustMarshal(map[string]any{"title": "聊天记录"})
	p := ParseInboundMessage("merge_forward", content)
	if p.Type != ParsedMergeForward {
		t.Fatalf("type=%s, want merge_forward", p.Type)
	}
	if p.TextContent != "[合并转发: 聊天记录]" {
		t.Fatalf("text=%q", p.TextContent)
	}
}

func TestParseInboundMessage_MergeForwardWithRefs(t *testing.T) {
	content := mustMarshal(map[string]any{
		"title": "聊天记录",
		"preview": map[string]any{
			"text": "转发一个表格 https://abc.feishu.cn/sheets/SheetTokenXYZ",
		},
	})
	p := ParseInboundMessage("merge_forward", content)
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "SheetTokenXYZ" {
		t.Fatalf("token=%q, want SheetTokenXYZ", p.References[0].Token)
	}
}

func TestParseInboundMessage_UnknownType(t *testing.T) {
	p := ParseInboundMessage("future_type", "{}")
	if p.Type != ParsedUnknown {
		t.Fatalf("type=%s, want unknown", p.Type)
	}
	if p.TextContent != "[future_type 消息]" {
		t.Fatalf("text=%q", p.TextContent)
	}
}

func TestParseInboundMessage_UnknownTypeWithRefsFallback(t *testing.T) {
	content := mustMarshal(map[string]any{
		"payload": map[string]any{
			"url": "https://abc.feishu.cn/docx/FallbackDocToken",
		},
	})
	p := ParseInboundMessage("future_type", content)
	if len(p.References) != 1 {
		t.Fatalf("want 1 ref, got %d", len(p.References))
	}
	if p.References[0].Token != "FallbackDocToken" {
		t.Fatalf("token=%q, want FallbackDocToken", p.References[0].Token)
	}
}

func TestParseInboundMessage_EmptyType(t *testing.T) {
	p := ParseInboundMessage("", "{}")
	if p.Type != ParsedUnknown {
		t.Fatalf("type=%s, want unknown", p.Type)
	}
	if p.TextContent != "" {
		t.Fatalf("empty type should produce empty text, got %q", p.TextContent)
	}
}

func TestParseInboundMessage_Sticker(t *testing.T) {
	p := ParseInboundMessage("sticker", "{}")
	if p.Type != ParsedSticker || p.TextContent != "[表情消息]" {
		t.Fatalf("sticker: type=%s text=%q", p.Type, p.TextContent)
	}
}

func TestParseInboundMessage_System(t *testing.T) {
	content := mustMarshal(map[string]any{"type": "add_bot"})
	p := ParseInboundMessage("system", content)
	if p.Type != ParsedSystem {
		t.Fatalf("type=%s, want system", p.Type)
	}
	if p.TextContent != "[系统消息: add_bot]" {
		t.Fatalf("text=%q", p.TextContent)
	}
}

func TestParseInboundMessage_MalformedJSON(t *testing.T) {
	p := ParseInboundMessage("image", "not json")
	if p.Type != ParsedImage {
		t.Fatalf("type=%s, want image", p.Type)
	}
	if p.TextContent != "[图片消息]" {
		t.Fatalf("malformed image should fallback, got %q", p.TextContent)
	}
}
