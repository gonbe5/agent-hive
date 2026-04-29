package mcphost

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// SSETransportConfig SSE 传输配置
type SSETransportConfig struct {
	URL          string                 // SSE 端点 URL
	Headers      map[string]string      // 自定义 HTTP 头
	MaxRetries   int                    // 最大重试次数，默认 3
	Timeout      time.Duration          // HTTP 请求超时，默认 30s
	AuthProvider func() (string, error) // 可选，返回 Authorization header 值
}

// SSETransport 通过 SSE 协议与 MCP 服务端通信的传输层
// 使用 HTTP GET 接收服务端推送消息，HTTP POST 发送客户端消息
type SSETransport struct {
	cfg    SSETransportConfig
	logger *zap.Logger

	mu        sync.Mutex
	postURL   string        // 从 SSE endpoint 事件中获取的消息发送 URL
	postURLCh chan struct{} // postURL 设置后关闭的信号 channel
	msgCh     chan json.RawMessage
	errCh     chan error
	client    *http.Client
	cancel    context.CancelFunc
	closed    bool
	connected bool
}

// NewSSETransport 创建 SSE 传输实例
func NewSSETransport(cfg SSETransportConfig, logger *zap.Logger) *SSETransport {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &SSETransport{
		cfg:       cfg,
		logger:    logger,
		postURLCh: make(chan struct{}),
		msgCh:     make(chan json.RawMessage, 64),
		errCh:     make(chan error, 1),
		client:    &http.Client{Timeout: cfg.Timeout},
	}
}

// Connect 建立 SSE 连接
func (t *SSETransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errs.New(errs.CodeMCPTransportClosed, "SSE 传输已关闭")
	}
	if t.connected {
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < t.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避: 1s, 2s, 4s ...
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			t.logger.Info("SSE 重连中", zap.Int("尝试次数", attempt+1), zap.Duration("退避时间", backoff))
			select {
			case <-ctx.Done():
				return errs.Wrap(errs.CodeMCPTransportFailed, "SSE 连接被取消", ctx.Err())
			case <-time.After(backoff):
			}
		}

		err := t.doConnect(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		t.logger.Warn("SSE 连接失败", zap.Int("尝试次数", attempt+1), zap.Error(err))
	}

	return errs.Wrap(errs.CodeMCPTransportFailed, fmt.Sprintf("SSE 连接失败（已重试 %d 次）", t.cfg.MaxRetries), lastErr)
}

// doConnect 执行单次 SSE 连接
func (t *SSETransport) doConnect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.cfg.URL, nil)
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "创建 SSE 请求失败", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if err := t.applyAuth(req); err != nil {
		return err
	}

	// SSE 使用长连接，不应用超时
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "SSE HTTP 请求失败", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return errs.New(errs.CodeMCPTransportFailed, fmt.Sprintf("SSE 服务端返回 %d", resp.StatusCode))
	}

	connCtx, cancel := context.WithCancel(ctx)
	t.mu.Lock()
	t.cancel = cancel
	t.connected = true
	t.mu.Unlock()

	// 在后台协程中读取 SSE 事件流
	go t.readLoop(connCtx, resp.Body)

	// 等待从 SSE 流中获取 endpoint 事件（包含 POST URL）
	select {
	case <-ctx.Done():
		cancel()
		return errs.Wrap(errs.CodeMCPTransportFailed, "等待 SSE endpoint 超时", ctx.Err())
	case err := <-t.errCh:
		cancel()
		return err
	case <-t.waitForPostURL():
		t.logger.Info("SSE 连接已建立", zap.String("postURL", t.getPostURL()))
		return nil
	}
}

// waitForPostURL 返回一个在 postURL 设置后关闭的 channel
func (t *SSETransport) waitForPostURL() <-chan struct{} {
	return t.postURLCh
}

// getPostURL 线程安全地获取 postURL
func (t *SSETransport) getPostURL() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.postURL
}

// readLoop 读取 SSE 事件流
func (t *SSETransport) readLoop(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	scanner := bufio.NewScanner(body)

	var eventType string
	var dataBuf strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()

		// 空行表示事件结束
		if line == "" {
			if dataBuf.Len() > 0 {
				t.handleEvent(eventType, dataBuf.String())
				eventType = ""
				dataBuf.Reset()
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataBuf.Len() > 0 {
				dataBuf.WriteString("\n")
			}
			dataBuf.WriteString(data)
		}
		// 忽略注释行（以 : 开头）和其他字段
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-ctx.Done():
		default:
			t.logger.Error("SSE 读取流错误", zap.Error(err))
			select {
			case t.errCh <- errs.Wrap(errs.CodeMCPSSEParseFailed, "SSE 读取流错误", err):
			default:
			}
		}
	}
}

// handleEvent 处理解析后的 SSE 事件
func (t *SSETransport) handleEvent(eventType, data string) {
	switch eventType {
	case "endpoint":
		// MCP SSE 协议：endpoint 事件包含用于发送消息的 POST URL
		postURL := data
		if !strings.HasPrefix(postURL, "http") {
			// 相对路径，拼接基础 URL
			base := t.cfg.URL
			if idx := strings.LastIndex(base, "/"); idx > 0 {
				base = base[:idx]
			}
			postURL = base + "/" + strings.TrimPrefix(postURL, "/")
		}
		t.mu.Lock()
		t.postURL = postURL
		select {
		case <-t.postURLCh:
			// 已关闭，无需再次关闭
		default:
			close(t.postURLCh)
		}
		t.mu.Unlock()
		t.logger.Debug("收到 SSE endpoint 事件", zap.String("postURL", postURL))
	case "message", "":
		// 服务端推送的 JSON-RPC 消息
		select {
		case t.msgCh <- json.RawMessage(data):
		default:
			t.logger.Warn("SSE 消息队列已满，丢弃消息")
		}
	case "notifications/tools/list_changed":
		// 工具列表变更通知，转发到消息 channel 供 Host 处理
		t.logger.Info("收到工具列表变更通知")
		notification := fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed","params":%s}`, data)
		if data == "" {
			notification = `{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`
		}
		select {
		case t.msgCh <- json.RawMessage(notification):
		default:
			t.logger.Warn("SSE 消息队列已满，丢弃工具列表变更通知")
		}
	default:
		t.logger.Debug("忽略未知 SSE 事件", zap.String("事件类型", eventType))
	}
}

// Send 通过 HTTP POST 发送 JSON-RPC 消息
func (t *SSETransport) Send(ctx context.Context, msg json.RawMessage) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errs.New(errs.CodeMCPTransportClosed, "SSE 传输已关闭")
	}
	postURL := t.postURL
	t.mu.Unlock()

	if postURL == "" {
		return errs.New(errs.CodeMCPTransportFailed, "SSE 尚未获取到 POST 端点")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, postURL, bytes.NewReader(msg))
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "创建 POST 请求失败", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if err := t.applyAuth(req); err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "发送 POST 请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return errs.New(errs.CodeMCPTransportFailed, fmt.Sprintf("POST 请求返回 %d: %s", resp.StatusCode, string(body)))
	}

	return nil
}

// Receive 接收服务端推送的 JSON-RPC 消息（阻塞）
func (t *SSETransport) Receive(ctx context.Context) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		return nil, errs.Wrap(errs.CodeMCPTransportFailed, "接收消息被取消", ctx.Err())
	case err := <-t.errCh:
		return nil, err
	case msg := <-t.msgCh:
		return msg, nil
	}
}

// applyAuth 如果配置了 AuthProvider，则设置 Authorization 头
func (t *SSETransport) applyAuth(req *http.Request) error {
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

// Close 关闭 SSE 连接
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true
	t.connected = false

	if t.cancel != nil {
		t.cancel()
	}

	t.logger.Info("SSE 传输已关闭")
	return nil
}
