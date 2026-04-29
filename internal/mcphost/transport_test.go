package mcphost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTransportInterface 验证 SSE 和 HTTP 传输均实现了 Transport 接口
func TestTransportInterface(t *testing.T) {
	tests := []struct {
		name      string
		transport Transport
	}{
		{
			name:      "SSETransport实现Transport接口",
			transport: NewSSETransport(SSETransportConfig{URL: "http://localhost"}, testLogger()),
		},
		{
			name:      "HTTPTransport实现Transport接口",
			transport: NewHTTPTransport(HTTPTransportConfig{URL: "http://localhost"}, testLogger()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.transport, "传输实例不应为 nil")
			// 编译时检查接口实现，运行时验证实例化
			_ = tt.transport.Close()
		})
	}
}
