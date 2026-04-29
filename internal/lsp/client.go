package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// NotificationHandler 通知处理函数
type NotificationHandler func(method string, params json.RawMessage)

// Client 是 LSP JSON-RPC 2.0 客户端
// 通过 stdin/stdout 与 LSP 服务器通信
type Client struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	nextID    atomic.Int64
	pending   map[int64]chan *jsonrpcResponse
	pendingMu sync.RWMutex

	// 通知处理器
	notificationHandlers   map[string]NotificationHandler
	notificationHandlersMu sync.RWMutex

	// goroutine 生命周期跟踪
	wg sync.WaitGroup

	logger *zap.Logger
	closed atomic.Bool
}

// jsonrpcRequest 表示 JSON-RPC 请求
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcResponse 表示 JSON-RPC 响应
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcNotification 表示 JSON-RPC 通知（无 ID）
type jsonrpcNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcError 表示 JSON-RPC 错误
type jsonrpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewClient 创建新的 LSP 客户端
func NewClient(stdin io.WriteCloser, stdout, stderr io.ReadCloser, logger *zap.Logger) *Client {
	c := &Client{
		stdin:                stdin,
		stdout:               stdout,
		stderr:               stderr,
		pending:              make(map[int64]chan *jsonrpcResponse),
		notificationHandlers: make(map[string]NotificationHandler),
		logger:               logger,
	}

	// 启动响应读取协程
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.readLoop()
	}()

	// 启动 stderr 读取协程（用于调试）
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.readStderr()
	}()

	return c
}

// Call 发送 JSON-RPC 请求并等待响应
func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	if c.closed.Load() {
		return errs.New(errs.CodeInternal, "LSP client is closed")
	}

	// 生成请求 ID
	id := c.nextID.Add(1)

	// 创建响应通道
	respChan := make(chan *jsonrpcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respChan
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// 构造请求
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// 序列化请求
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "序列化请求失败", err)
	}

	// 发送请求（Content-Length header + JSON body）
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(reqBytes), reqBytes)
	if _, err := c.stdin.Write([]byte(msg)); err != nil {
		return errs.Wrap(errs.CodeInternal, "发送请求失败", err)
	}

	c.logger.Debug("LSP 请求已发送",
		zap.String("method", method),
		zap.Int64("id", id))

	// 等待响应或超时
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return errs.New(errs.CodeInternal, fmt.Sprintf("LSP 错误: %s", resp.Error.Message))
		}

		// 反序列化结果
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return errs.Wrap(errs.CodeInternal, "反序列化响应失败", err)
			}
		}

		return nil

	case <-ctx.Done():
		return errs.New(errs.CodeTimeout, "LSP 请求超时")
	}
}

// Notify 发送 JSON-RPC 通知（不等待响应）
func (c *Client) Notify(method string, params interface{}) error {
	if c.closed.Load() {
		return errs.New(errs.CodeInternal, "LSP client is closed")
	}

	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	notifBytes, err := json.Marshal(notif)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "序列化通知失败", err)
	}

	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(notifBytes), notifBytes)
	if _, err := c.stdin.Write([]byte(msg)); err != nil {
		return errs.Wrap(errs.CodeInternal, "发送通知失败", err)
	}

	c.logger.Debug("LSP 通知已发送", zap.String("method", method))
	return nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil // 已经关闭
	}

	c.logger.Debug("关闭 LSP 客户端")

	// 关闭输入输出流
	if err := c.stdin.Close(); err != nil {
		c.logger.Warn("关闭 stdin 失败", zap.Error(err))
	}
	if err := c.stdout.Close(); err != nil {
		c.logger.Warn("关闭 stdout 失败", zap.Error(err))
	}
	if err := c.stderr.Close(); err != nil {
		c.logger.Warn("关闭 stderr 失败", zap.Error(err))
	}

	// 等待所有读取协程退出，防止 goroutine 泄漏
	c.wg.Wait()

	// 清理待处理请求
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	// 清理通知处理器
	c.notificationHandlersMu.Lock()
	c.notificationHandlers = make(map[string]NotificationHandler)
	c.notificationHandlersMu.Unlock()

	return nil
}

// RegisterNotificationHandler 注册通知处理器
func (c *Client) RegisterNotificationHandler(method string, handler NotificationHandler) {
	c.notificationHandlersMu.Lock()
	defer c.notificationHandlersMu.Unlock()
	c.notificationHandlers[method] = handler
}

// UnregisterNotificationHandler 取消注册通知处理器
func (c *Client) UnregisterNotificationHandler(method string) {
	c.notificationHandlersMu.Lock()
	defer c.notificationHandlersMu.Unlock()
	delete(c.notificationHandlers, method)
}

// getNotificationHandler 获取已注册的通知处理器（用于临时替换后恢复）
func (c *Client) getNotificationHandler(method string) NotificationHandler {
	c.notificationHandlersMu.RLock()
	defer c.notificationHandlersMu.RUnlock()
	return c.notificationHandlers[method]
}

// readLoop 读取响应和通知
func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.stdout)
	scanner.Split(splitContentLength)

	for scanner.Scan() {
		if c.closed.Load() {
			return
		}

		data := scanner.Bytes()
		if len(data) == 0 {
			continue
		}

		// 尝试解析为响应
		var resp jsonrpcResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			c.logger.Warn("解析 LSP 响应失败",
				zap.Error(err),
				zap.String("data", string(data)))
			continue
		}

		// 如果有 ID，说明是响应
		if resp.ID > 0 {
			c.pendingMu.RLock()
			ch, ok := c.pending[resp.ID]
			c.pendingMu.RUnlock()

			if ok {
				ch <- &resp
			} else {
				c.logger.Warn("收到未知 ID 的响应", zap.Int64("id", resp.ID))
			}
		} else {
			// 尝试解析为通知
			var notif jsonrpcNotification
			if err := json.Unmarshal(data, &notif); err != nil {
				c.logger.Warn("解析 LSP 通知失败",
					zap.Error(err),
					zap.String("data", string(data)))
				continue
			}

			// 分发通知
			c.handleNotification(notif.Method, notif.Params)
		}
	}

	if err := scanner.Err(); err != nil && !c.closed.Load() {
		c.logger.Error("LSP 读取循环错误", zap.Error(err))
	}
}

// handleNotification 处理通知
func (c *Client) handleNotification(method string, params interface{}) {
	c.logger.Debug("收到 LSP 通知", zap.String("method", method))

	// 将 params 转换为 json.RawMessage
	var rawParams json.RawMessage
	if params != nil {
		paramBytes, err := json.Marshal(params)
		if err != nil {
			c.logger.Warn("序列化通知参数失败",
				zap.String("method", method),
				zap.Error(err))
			return
		}
		rawParams = paramBytes
	}

	// 查找并调用对应的处理器
	c.notificationHandlersMu.RLock()
	handler, ok := c.notificationHandlers[method]
	c.notificationHandlersMu.RUnlock()

	if ok && handler != nil {
		// 在新的 goroutine 中执行处理器，避免阻塞读取循环
		go func() {
			defer func() {
				if r := recover(); r != nil {
					c.logger.Error("通知处理器 panic",
						zap.String("method", method),
						zap.Any("panic", r))
				}
			}()
			handler(method, rawParams)
		}()
	} else {
		c.logger.Debug("未找到通知处理器，忽略",
			zap.String("method", method))
	}
}

// readStderr 读取 stderr 输出（用于调试）
func (c *Client) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		if c.closed.Load() {
			return
		}
		c.logger.Debug("LSP stderr", zap.String("line", scanner.Text()))
	}
}

// splitContentLength 按 Content-Length header 分割消息
func splitContentLength(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// 查找 Content-Length header
	headerEnd := strings.Index(string(data), "\r\n\r\n")
	if headerEnd < 0 {
		if atEOF {
			return 0, nil, errs.New(errs.CodeInternal, "未找到 header 分隔符")
		}
		return 0, nil, nil // 等待更多数据
	}

	// 解析 Content-Length
	header := string(data[:headerEnd])
	var contentLength int
	for _, line := range strings.Split(header, "\r\n") {
		if strings.HasPrefix(line, "Content-Length:") {
			lenStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(lenStr)
			if err != nil {
				return 0, nil, errs.Wrap(errs.CodeInternal, "解析 Content-Length 失败", err)
			}
			break
		}
	}

	if contentLength == 0 {
		return 0, nil, errs.New(errs.CodeInternal, "Content-Length 为 0")
	}

	// 检查是否有足够的数据
	totalLen := headerEnd + 4 + contentLength
	if len(data) < totalLen {
		if atEOF {
			return 0, nil, errs.New(errs.CodeInternal, "数据不完整")
		}
		return 0, nil, nil // 等待更多数据
	}

	// 返回消息体
	return totalLen, data[headerEnd+4 : totalLen], nil
}
