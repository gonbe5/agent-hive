package mcphost

import (
	"context"
	"encoding/json"
)

// Transport MCP 传输层接口
type Transport interface {
	// Connect 建立连接
	Connect(ctx context.Context) error
	// Send 发送 JSON-RPC 请求
	Send(ctx context.Context, msg json.RawMessage) error
	// Receive 接收 JSON-RPC 响应（阻塞）
	Receive(ctx context.Context) (json.RawMessage, error)
	// Close 关闭连接
	Close() error
}
