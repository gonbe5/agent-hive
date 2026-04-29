package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentToSDKParts_Text(t *testing.T) {
	parts := contentToSDKParts([]ContentPart{TextPart("hello")})
	require.Len(t, parts, 1)
	// 验证序列化结果包含 text
	data, err := json.Marshal(parts[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"text":"hello"`)
	assert.Contains(t, string(data), `"type":"text"`)
}

func TestContentToSDKParts_Image(t *testing.T) {
	parts := contentToSDKParts([]ContentPart{ImageURLPart("https://example.com/img.png")})
	require.Len(t, parts, 1)
	data, err := json.Marshal(parts[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"image_url"`)
	assert.Contains(t, string(data), `"url":"https://example.com/img.png"`)
}

func TestContentToSDKParts_Audio(t *testing.T) {
	parts := contentToSDKParts([]ContentPart{AudioPart("base64data", "wav")})
	require.Len(t, parts, 1)
	data, err := json.Marshal(parts[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"input_audio"`)
	assert.Contains(t, string(data), `"data":"base64data"`)
}

func TestContentToSDKParts_File(t *testing.T) {
	parts := contentToSDKParts([]ContentPart{FilePart("file-123")})
	require.Len(t, parts, 1)
	data, err := json.Marshal(parts[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"file"`)
	assert.Contains(t, string(data), `"file_id":"file-123"`)
}

func TestToSDKUserMessage_Text(t *testing.T) {
	msg := toSDKUserMessage(NewTextContent("hello"))
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"role":"user"`)
	assert.Contains(t, string(data), `"content":"hello"`)
}

func TestToSDKUserMessage_Multi(t *testing.T) {
	msg := toSDKUserMessage(NewMultiContent(
		TextPart("describe this"),
		ImageURLPart("https://example.com/img.png"),
	))
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"role":"user"`)
	assert.Contains(t, string(data), `"type":"text"`)
	assert.Contains(t, string(data), `"type":"image_url"`)
}

func TestContentToSDKString(t *testing.T) {
	assert.Equal(t, "hello", contentToSDKString(NewTextContent("hello")))
	assert.Equal(t, "hello", contentToSDKString(NewMultiContent(TextPart("hello"))))
}
