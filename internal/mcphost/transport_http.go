package mcphost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// HTTPTransportConfig StreamableHTTP 传输配置
type HTTPTransportConfig struct {
	URL          string                 // MCP 服务端 URL
	Headers      map[string]string      // 自定义 HTTP 头
	Timeout      time.Duration          // HTTP 请求超时，默认 30s
	AuthProvider func() (string, error) // 可选，返回 Authorization header 值
}

// HTTPTransport 使用标准 HTTP POST 请求与 MCP 服务端通信的传输层
// 支持 SSE 降级：如果服务端返回 text/event-stream content-type，自动按 SSE 解析响应
type HTTPTransport struct {
	cfg    HTTPTransportConfig
	logger *zap.Logger
	client *http.Client

	mu     sync.Mutex
	msgCh  chan json.RawMessage
	closed bool
}

// NewHTTPTransport 创建 StreamableHTTP 传输实例
func NewHTTPTransport(cfg HTTPTransportConfig, logger *zap.Logger) *HTTPTransport {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &HTTPTransport{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{Timeout: cfg.Timeout},
		msgCh:  make(chan json.RawMessage, 64),
	}
}

// Connect 验证服务端可达性
func (t *HTTPTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errs.New(errs.CodeMCPTransportClosed, "HTTP 传输已关闭")
	}
	t.mu.Unlock()

	// 发送一个空的 JSON-RPC 初始化请求来验证连接
	initMsg := json.RawMessage(`{"jsonrpc":"2.0","method":"initialize","id":0}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(initMsg))
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "创建 HTTP 连接检测请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if err := t.applyAuth(req); err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "HTTP 连接检测失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return errs.New(errs.CodeMCPTransportFailed, fmt.Sprintf("HTTP 连接检测返回 %d: %s", resp.StatusCode, string(body)))
	}

	// 将初始化响应放入消息队列
	contentType := resp.Header.Get("Content-Type")
	if err := t.consumeResponse(ctx, resp.Body, contentType); err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "读取初始化响应失败", err)
	}

	t.logger.Info("HTTP 传输连接成功", zap.String("url", t.cfg.URL))
	return nil
}

// Send 通过 HTTP POST 发送 JSON-RPC 消息并读取响应
func (t *HTTPTransport) Send(ctx context.Context, msg json.RawMessage) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errs.New(errs.CodeMCPTransportClosed, "HTTP 传输已关闭")
	}
	t.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(msg))
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "创建 HTTP 请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if err := t.applyAuth(req); err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "发送 HTTP 请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return errs.New(errs.CodeMCPTransportFailed, fmt.Sprintf("HTTP 请求返回 %d: %s", resp.StatusCode, string(body)))
	}

	contentType := resp.Header.Get("Content-Type")
	return t.consumeResponse(ctx, resp.Body, contentType)
}

// consumeResponse 消费 HTTP 响应体，支持 JSON 和 SSE 两种格式
func (t *HTTPTransport) consumeResponse(ctx context.Context, body io.Reader, contentType string) error {
	if isSSEContentType(contentType) {
		// SSE 降级：服务端返回事件流
		t.logger.Debug("检测到 SSE 响应，使用 SSE 模式解析")
		return t.consumeSSEResponse(ctx, body)
	}

	// 标准 JSON 响应
	data, err := io.ReadAll(io.LimitReader(body, 10*1024*1024)) // 限制 10MB
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "读取 HTTP 响应失败", err)
	}

	if len(data) == 0 {
		return nil
	}

	// 检查是否为合法 JSON
	if !json.Valid(data) {
		return errs.New(errs.CodeMCPResponseInvalid, "HTTP 响应不是有效的 JSON")
	}

	select {
	case t.msgCh <- json.RawMessage(data):
	default:
		t.logger.Warn("HTTP 消息队列已满，丢弃响应")
	}

	return nil
}

// consumeSSEResponse 解析 SSE 格式的响应体
func (t *HTTPTransport) consumeSSEResponse(ctx context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	var dataBuf strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return errs.Wrap(errs.CodeMCPTransportFailed, "读取 SSE 响应被取消", ctx.Err())
		default:
		}

		line := scanner.Text()

		if line == "" {
			// 空行表示事件结束
			if dataBuf.Len() > 0 {
				data := dataBuf.String()
				dataBuf.Reset()

				if json.Valid([]byte(data)) {
					select {
					case t.msgCh <- json.RawMessage(data):
					default:
						t.logger.Warn("HTTP-SSE 消息队列已满，丢弃消息")
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataBuf.Len() > 0 {
				dataBuf.WriteString("\n")
			}
			dataBuf.WriteString(d)
		}
		// 忽略 event: / id: / retry: / 注释行
	}

	// 处理流结尾可能遗留的数据
	if dataBuf.Len() > 0 {
		data := dataBuf.String()
		if json.Valid([]byte(data)) {
			select {
			case t.msgCh <- json.RawMessage(data):
			default:
			}
		}
	}

	return scanner.Err()
}

// Receive 接收 JSON-RPC 响应消息（阻塞）
func (t *HTTPTransport) Receive(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, errs.Wrap(errs.CodeMCPTransportFailed, "接收消息被取消", ctx.Err())
	case msg := <-t.msgCh:
		return msg, nil
	}
}

// applyAuth 如果配置了 AuthProvider，则设置 Authorization 头
func (t *HTTPTransport) applyAuth(req *http.Request) error {
	if t.cfg.AuthProvider != nil {
		authHeader, err := t.cfg.AuthProvider()
		if err != nil {
			return errs.Wrap(errs.CodeMCPOAuthFailed, "获取认证信息失败", err)
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
	}
	return nil
}

// Close 关闭 HTTP 传输
func (t *HTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	t.logger.Info("HTTP 传输已关闭")
	return nil
}

// isSSEContentType 判断 Content-Type 是否为 SSE 事件流
func isSSEContentType(ct string) bool {
	return strings.Contains(ct, "text/event-stream")
}
