package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTextContent(t *testing.T) {
	c := NewTextContent("hello")
	assert.Equal(t, "hello", c.Text())
	assert.False(t, c.IsMultimodal())
	assert.Nil(t, c.Parts())
}

func TestNewMultiContent(t *testing.T) {
	c := NewMultiContent(
		TextPart("hello"),
		ImageURLPart("https://example.com/img.png"),
	)
	assert.True(t, c.IsMultimodal())
	assert.Equal(t, "hello", c.Text())
	assert.Len(t, c.Parts(), 2)
}

func TestContent_MarshalJSON_Text(t *testing.T) {
	c := NewTextContent("hello world")
	data, err := json.Marshal(c)
	require.NoError(t, err)
	assert.Equal(t, `"hello world"`, string(data))
}

func TestContent_MarshalJSON_Multi(t *testing.T) {
	c := NewMultiContent(
		TextPart("hello"),
		ImageURLPart("https://example.com/img.png"),
	)
	data, err := json.Marshal(c)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"text"`)
	assert.Contains(t, string(data), `"type":"image"`)
}

func TestContent_UnmarshalJSON_String(t *testing.T) {
	var c Content
	err := json.Unmarshal([]byte(`"hello"`), &c)
	require.NoError(t, err)
	assert.Equal(t, "hello", c.Text())
	assert.False(t, c.IsMultimodal())
}

func TestContent_UnmarshalJSON_Array(t *testing.T) {
	var c Content
	raw := `[{"type":"text","text":"hello"},{"type":"image","image_url":"https://example.com/img.png","detail":"auto"}]`
	err := json.Unmarshal([]byte(raw), &c)
	require.NoError(t, err)
	assert.True(t, c.IsMultimodal())
	assert.Len(t, c.Parts(), 2)
	assert.Equal(t, "hello", c.Text())
}

func TestContent_RoundTrip_Text(t *testing.T) {
	original := NewTextContent("round trip")
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored Content
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.Equal(t, original.Text(), restored.Text())
	assert.Equal(t, original.IsMultimodal(), restored.IsMultimodal())
}

func TestContent_RoundTrip_Multi(t *testing.T) {
	original := NewMultiContent(
		TextPart("hello"),
		AudioPart("base64data", "wav"),
	)
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var restored Content
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.True(t, restored.IsMultimodal())
	assert.Len(t, restored.Parts(), 2)
	assert.Equal(t, ContentAudio, restored.Parts()[1].Type)
	assert.Equal(t, "wav", restored.Parts()[1].AudioFormat)
}

func TestContent_EmptyJSON(t *testing.T) {
	var c Content
	err := c.UnmarshalJSON([]byte{})
	assert.NoError(t, err)
	assert.Equal(t, "", c.Text())
}

func TestContent_InStruct(t *testing.T) {
	// 测试 Content 在结构体中的序列化/反序列化
	type Msg struct {
		Role    string  `json:"role"`
		Content Content `json:"content"`
	}

	// 纯文本消息
	msg := Msg{Role: "user", Content: NewTextContent("hello")}
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.Equal(t, `{"role":"user","content":"hello"}`, string(data))

	var restored Msg
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.Equal(t, "user", restored.Role)
	assert.Equal(t, "hello", restored.Content.Text())

	// 向后兼容：旧格式 string content
	oldJSON := `{"role":"user","content":"old format"}`
	var oldMsg Msg
	err = json.Unmarshal([]byte(oldJSON), &oldMsg)
	require.NoError(t, err)
	assert.Equal(t, "old format", oldMsg.Content.Text())
	assert.False(t, oldMsg.Content.IsMultimodal())
}

func TestContentPart_Constructors(t *testing.T) {
	tp := TextPart("text")
	assert.Equal(t, ContentText, tp.Type)
	assert.Equal(t, "text", tp.Text)

	ip := ImageURLPart("https://example.com/img.png")
	assert.Equal(t, ContentImage, ip.Type)
	assert.Equal(t, "https://example.com/img.png", ip.ImageURL)
	assert.Equal(t, "auto", ip.Detail)

	ib := ImageBase64Part("image/png", "abc123")
	assert.Equal(t, ContentImage, ib.Type)
	assert.Equal(t, "data:image/png;base64,abc123", ib.ImageURL)

	ap := AudioPart("audiodata", "mp3")
	assert.Equal(t, ContentAudio, ap.Type)
	assert.Equal(t, "audiodata", ap.AudioData)
	assert.Equal(t, "mp3", ap.AudioFormat)

	fp := FilePart("file-123")
	assert.Equal(t, ContentFile, fp.Type)
	assert.Equal(t, "file-123", fp.FileID)
}
