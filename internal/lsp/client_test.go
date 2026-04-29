package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockPipe 模拟 LSP 服务器的 stdin/stdout
type mockPipe struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func newMockPipe() *mockPipe {
	r, w := io.Pipe()
	return &mockPipe{reader: r, writer: w}
}

func (m *mockPipe) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

func (m *mockPipe) Write(p []byte) (n int, err error) {
	return m.writer.Write(p)
}

func (m *mockPipe) Close() error {
	m.reader.Close()
	return m.writer.Close()
}

func TestClient_CallAndResponse(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟管道
	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	// 启动一个 goroutine 模拟 LSP 服务器响应
	go func() {
		// 读取请求
		buf := make([]byte, 4096)
		n, err := stdinPipe.reader.Read(buf)
		if err != nil {
			return
		}

		// 解析请求
		requestData := buf[:n]
		// 查找 JSON 部分（跳过 Content-Length header）
		jsonStart := bytes.Index(requestData, []byte("\r\n\r\n"))
		if jsonStart < 0 {
			return
		}
		jsonData := requestData[jsonStart+4:]

		var req jsonrpcRequest
		if err := json.Unmarshal(jsonData, &req); err != nil {
			return
		}

		// 构造响应
		result := map[string]string{"message": "hello from server"}
		resultJSON, _ := json.Marshal(result)

		resp := jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultJSON,
		}

		respJSON, _ := json.Marshal(resp)
		// 使用 fmt.Sprintf 正确格式化 Content-Length 数值
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(respJSON))
		respMsg := append([]byte(header), respJSON...)

		stdoutPipe.writer.Write(respMsg)
	}()

	// 发送请求
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var result map[string]string
	err := client.Call(ctx, "test/method", map[string]string{"key": "value"}, &result)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if result["message"] != "hello from server" {
		t.Errorf("expected 'hello from server', got %q", result["message"])
	}
}

func TestClient_Timeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	// 排空 stdin，防止 io.Pipe 写入阻塞
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdinPipe.reader.Read(buf); err != nil {
				return
			}
		}
	}()

	// 不发送响应，应该超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var result any
	err := client.Call(ctx, "test/timeout", nil, &result)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestClient_Notify(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	// 在后台读取 Notify 发送的数据（防止 io.Pipe 阻塞）
	dataCh := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := stdinPipe.reader.Read(buf)
		if err != nil {
			dataCh <- nil
			return
		}
		dataCh <- buf[:n]
	}()

	// 发送通知（不等待响应）
	err := client.Notify("test/notify", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	// 等待读取数据
	select {
	case data := <-dataCh:
		if data == nil {
			t.Fatal("failed to read notification data")
		}
		// 验证是通知格式（无 ID）
		if !bytes.Contains(data, []byte("test/notify")) {
			t.Error("expected notification to contain method name")
		}
		if bytes.Contains(data, []byte(`"id"`)) {
			t.Error("notification should not contain id field")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification data")
	}
}

func TestSplitContentLength(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantAdv int
		wantTok string
		wantErr bool
	}{
		{
			name:    "valid message",
			input:   "Content-Length: 13\r\n\r\n{\"key\":\"val\"}",
			wantAdv: 35,
			wantTok: `{"key":"val"}`,
			wantErr: false,
		},
		{
			name:    "incomplete header",
			input:   "Content-Length: 13\r\n",
			wantAdv: 0,
			wantTok: "",
			wantErr: false, // 等待更多数据
		},
		{
			name:    "incomplete body",
			input:   "Content-Length: 20\r\n\r\n{\"key\":",
			wantAdv: 0,
			wantTok: "",
			wantErr: false, // 等待更多数据
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(tt.input)
			adv, tok, err := splitContentLength(data, false)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if adv != tt.wantAdv {
				t.Errorf("advance: expected %d, got %d", tt.wantAdv, adv)
			}

			if string(tok) != tt.wantTok {
				t.Errorf("token: expected %q, got %q", tt.wantTok, string(tok))
			}
		})
	}
}

// Agent B: LSP 通知处理机制测试

func TestClient_NotificationHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 创建模拟管道
	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	// 排空 stdin
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdinPipe.reader.Read(buf); err != nil {
				return
			}
		}
	}()

	// 注册通知处理器
	receivedCh := make(chan string, 1)
	client.RegisterNotificationHandler("test/notification", func(method string, params json.RawMessage) {
		receivedCh <- string(params)
	})

	// 模拟服务器发送通知
	go func() {
		notif := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "test/notification",
			"params":  map[string]string{"message": "hello"},
		}
		notifJSON, _ := json.Marshal(notif)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(notifJSON))
		msg := append([]byte(header), notifJSON...)
		stdoutPipe.writer.Write(msg)
	}()

	// 等待通知处理
	select {
	case params := <-receivedCh:
		if !bytes.Contains([]byte(params), []byte("hello")) {
			t.Errorf("expected params to contain 'hello', got %q", params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

func TestClient_PublishDiagnostics(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdinPipe.reader.Read(buf); err != nil {
				return
			}
		}
	}()

	// 注册诊断通知处理器
	diagCh := make(chan PublishDiagnosticsParams, 1)
	client.RegisterNotificationHandler("textDocument/publishDiagnostics", func(method string, params json.RawMessage) {
		var diagParams PublishDiagnosticsParams
		if err := json.Unmarshal(params, &diagParams); err != nil {
			t.Errorf("unmarshal error: %v", err)
			return
		}
		diagCh <- diagParams
	})

	// 模拟服务器发送诊断通知
	go func() {
		diag := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "textDocument/publishDiagnostics",
			"params": map[string]interface{}{
				"uri": "file:///test.go",
				"diagnostics": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"start": map[string]int{"line": 0, "character": 0},
							"end":   map[string]int{"line": 0, "character": 5},
						},
						"severity": 1,
						"message":  "test error",
					},
				},
			},
		}
		diagJSON, _ := json.Marshal(diag)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(diagJSON))
		msg := append([]byte(header), diagJSON...)
		stdoutPipe.writer.Write(msg)
	}()

	// 等待诊断通知
	select {
	case diagParams := <-diagCh:
		if diagParams.URI != "file:///test.go" {
			t.Errorf("expected URI 'file:///test.go', got %q", diagParams.URI)
		}
		if len(diagParams.Diagnostics) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diagParams.Diagnostics))
		}
		if diagParams.Diagnostics[0].Message != "test error" {
			t.Errorf("expected message 'test error', got %q", diagParams.Diagnostics[0].Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for diagnostics")
	}
}

func TestClient_UnregisterNotificationHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	stdinPipe := newMockPipe()
	stdoutPipe := newMockPipe()
	stderrPipe := newMockPipe()

	client := NewClient(stdinPipe, stdoutPipe.reader, stderrPipe.reader, logger)
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := stdinPipe.reader.Read(buf); err != nil {
				return
			}
		}
	}()

	// 注册并取消注册处理器
	receivedCh := make(chan bool, 1)
	client.RegisterNotificationHandler("test/notification", func(method string, params json.RawMessage) {
		receivedCh <- true
	})
	client.UnregisterNotificationHandler("test/notification")

	// 发送通知
	go func() {
		notif := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "test/notification",
			"params":  map[string]string{"message": "hello"},
		}
		notifJSON, _ := json.Marshal(notif)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(notifJSON))
		msg := append([]byte(header), notifJSON...)
		stdoutPipe.writer.Write(msg)
	}()

	// 不应该收到通知
	select {
	case <-receivedCh:
		t.Error("should not receive notification after unregister")
	case <-time.After(500 * time.Millisecond):
		// 预期超时
	}
}
