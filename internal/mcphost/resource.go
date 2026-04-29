package mcphost

import "context"

// ResourceDefinition MCP 资源定义
type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContent MCP 资源内容
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64 编码
}

// ResourceProvider 资源提供者函数
type ResourceProvider func(ctx context.Context, uri string) (*ResourceContent, error)

type resourceEntry struct {
	def      ResourceDefinition
	provider ResourceProvider
}
