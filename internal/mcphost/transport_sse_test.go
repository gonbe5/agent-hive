package mcphost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func TestSSETransport_Connect(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr bool
		errMsg  string
	}{
		{
			name: "正常连接_收到endpoint事件",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					flusher, ok := w.(http.Flusher)
					if !ok {
						return
					}
					fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
					flusher.Flush()
					// 保持连接一段时间
					time.Sleep(100 * time.Millisecond)
				}
			},
			wantErr: false,
		},
		{
			name: "服务端返回错误状态码",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
			errMsg:  "SSE 服务端返回 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			transport := NewSSETransport(SSETransportConfig{
				URL:        srv.URL,
				MaxRetries: 1,
				Timeout:    5 * time.Second,
			}, testLogger())
			defer transport.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			err := transport.Connect(ctx)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSSETransport_SendReceive(t *testing.T) {
	// 模拟 MCP SSE 服务端
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		// SSE 端点
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
		flusher.Flush()
		// 模拟服务端推送响应
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[]}}\n\n")
		flusher.Flush()
		time.Sleep(200 * time.Millisecond)
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		// 接收客户端消息
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	transport := NewSSETransport(SSETransportConfig{
		URL:        srv.URL + "/sse",
		MaxRetries: 1,
		Timeout:    5 * time.Second,
	}, testLogger())
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 连接
	err := transport.Connect(ctx)
	require.NoError(t, err)

	// 发送消息
	msg := json.RawMessage(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	err = transport.Send(ctx, msg)
	require.NoError(t, err)

	// 接收响应
	resp, err := transport.Receive(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"result"`)
}

func TestSSETransport_Close(t *testing.T) {
	transport := NewSSETransport(SSETransportConfig{
		URL:        "http://localhost:9999",
		MaxRetries: 1,
	}, testLogger())

	// 关闭
	err := transport.Close()
	require.NoError(t, err)

	// 关闭后发送应报错
	ctx := context.Background()
	err = transport.Send(ctx, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已关闭")

	// 关闭后连接应报错
	err = transport.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已关闭")

	// 重复关闭不报错
	err = transport.Close()
	require.NoError(t, err)
}

func TestSSETransport_RetryBackoff(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	transport := NewSSETransport(SSETransportConfig{
		URL:        srv.URL,
		MaxRetries: 3,
		Timeout:    2 * time.Second,
	}, testLogger())
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已重试 3 次")
	assert.Equal(t, 3, attempts, "应重试指定次数")
}

func TestSSETransport_CustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer srv.Close()

	transport := NewSSETransport(SSETransportConfig{
		URL:        srv.URL,
		MaxRetries: 1,
		Headers: map[string]string{
			"Authorization": "Bearer test-token-123",
		},
	}, testLogger())
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token-123", receivedAuth)
}

func TestSSETransport_DefaultConfig(t *testing.T) {
	transport := NewSSETransport(SSETransportConfig{
		URL: "http://localhost:9999",
	}, testLogger())

	assert.Equal(t, 3, transport.cfg.MaxRetries, "默认最大重试次数应为 3")
	assert.Equal(t, 30*time.Second, transport.cfg.Timeout, "默认超时应为 30s")
}
