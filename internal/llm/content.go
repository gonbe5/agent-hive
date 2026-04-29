package llm

import "encoding/json"

// ContentPartType 表示 Content 中各 part 的类型
type ContentPartType string

const (
	ContentText  ContentPartType = "text"
	ContentImage ContentPartType = "image"
	ContentAudio ContentPartType = "audio"
	ContentFile  ContentPartType = "file"
)

// ContentPart 表示 Content 中的一个组成部分
type ContentPart struct {
	Type        ContentPartType `json:"type"`
	Text        string          `json:"text,omitempty"`
	ImageURL    string          `json:"image_url,omitempty"`    // URL 或 data:image/...;base64,...
	Detail      string          `json:"detail,omitempty"`       // "auto"/"low"/"high"
	AudioData   string          `json:"audio_data,omitempty"`   // base64
	AudioFormat string          `json:"audio_format,omitempty"` // "wav"/"mp3"
	FileData    string          `json:"file_data,omitempty"`    // base64
	FileID      string          `json:"file_id,omitempty"`
	Filename    string          `json:"filename,omitempty"`
}

// Content 统一表示纯文本或多模态内容
// 纯文本走快速路径（text 字段），多模态使用 parts
type Content struct {
	text  string        // 快速路径：纯文本
	parts []ContentPart // 多模态
}

// NewTextContent 创建纯文本 Content
func NewTextContent(text string) Content {
	return Content{text: text}
}

// NewMultiContent 创建多模态 Content
func NewMultiContent(parts ...ContentPart) Content {
	return Content{parts: parts}
}

// IsMultimodal 判断是否包含多模态内容
func (c Content) IsMultimodal() bool {
	return len(c.parts) > 0
}

// Text 返回纯文本内容
// 纯文本直接返回 text 字段；多模态拼接所有 text part
func (c Content) Text() string {
	if !c.IsMultimodal() {
		return c.text
	}
	var sb []byte
	for _, p := range c.parts {
		if p.Type == ContentText && p.Text != "" {
			if len(sb) > 0 {
				sb = append(sb, '\n')
			}
			sb = append(sb, p.Text...)
		}
	}
	return string(sb)
}

// Parts 返回多模态 parts
// 纯文本时返回 nil
func (c Content) Parts() []ContentPart {
	return c.parts
}

// MarshalJSON 实现自定义 JSON 序列化
// 纯文本 → JSON string；多模态 → JSON array
func (c Content) MarshalJSON() ([]byte, error) {
	if c.IsMultimodal() {
		return json.Marshal(c.parts)
	}
	return json.Marshal(c.text)
}

// UnmarshalJSON 实现自定义 JSON 反序列化
// 自动识别 JSON string 或 JSON array，确保向后兼容
func (c *Content) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// 检查第一个非空白字符
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '"':
			// JSON string → 纯文本
			var s string
			if err := json.Unmarshal(data, &s); err != nil {
				return err
			}
			c.text = s
			c.parts = nil
			return nil
		case '[':
			// JSON array → 多模态
			var parts []ContentPart
			if err := json.Unmarshal(data, &parts); err != nil {
				return err
			}
			c.parts = parts
			c.text = ""
			return nil
		default:
			// 意外的类型，尝试 string
			var s string
			if err := json.Unmarshal(data, &s); err != nil {
				return err
			}
			c.text = s
			c.parts = nil
			return nil
		}
	}
	return nil
}

// --- 便利构造函数 ---

// TextPart 创建文本类型的 ContentPart
func TextPart(text string) ContentPart {
	return ContentPart{Type: ContentText, Text: text}
}

// ImageURLPart 创建图片 URL 类型的 ContentPart
func ImageURLPart(url string) ContentPart {
	return ContentPart{Type: ContentImage, ImageURL: url, Detail: "auto"}
}

// ImageBase64Part 创建 base64 图片类型的 ContentPart
func ImageBase64Part(mimeType, base64Data string) ContentPart {
	return ContentPart{
		Type:     ContentImage,
		ImageURL: "data:" + mimeType + ";base64," + base64Data,
		Detail:   "auto",
	}
}

// AudioPart 创建音频类型的 ContentPart
func AudioPart(base64Data, format string) ContentPart {
	return ContentPart{Type: ContentAudio, AudioData: base64Data, AudioFormat: format}
}

// FilePart 创建文件类型的 ContentPart
func FilePart(fileID string) ContentPart {
	return ContentPart{Type: ContentFile, FileID: fileID}
}
