package wechatpadpro

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// ReconnectConfig WebSocket 重连配置
type ReconnectConfig struct {
	MaxRetries   int           // 最大重试次数（-1表示无限重试）
	InitialDelay time.Duration // 初始延迟
	MaxDelay     time.Duration // 最大延迟
	Multiplier   float64       // 指数退避乘数
}

// DefaultReconnectConfig 默认重连配置
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		MaxRetries:   -1,              // 无限重试
		InitialDelay: 1 * time.Second, // 初始1秒
		MaxDelay:     5 * time.Minute, // 最大5分钟
		Multiplier:   2.0,             // 指数退避
	}
}

// WebSocketClient WeChatPadPro WebSocket 客户端
// 负责连接 WebSocket 端点，接收实时消息推送
type WebSocketClient struct {
	baseURL       string                                  // HTTP(S) 基础 URL
	conn          *websocket.Conn                         // WebSocket 连接
	handler       func(msg *wechat.IncomingMessage) error // 消息处理回调
	logger        *zap.Logger                             // 日志记录器
	stopCh        chan struct{}                           // 停止信号
	mu            sync.Mutex                              // 保护 conn 和 stopCh
	wg            sync.WaitGroup                          // 等待 goroutine 退出
	reconnectCfg  ReconnectConfig                         // 重连配置
	lastHeartbeat time.Time                               // 最后心跳时间
}

// NewWebSocketClient 创建 WebSocket 客户端
func NewWebSocketClient(
	baseURL string,
	handler func(msg *wechat.IncomingMessage) error,
	logger *zap.Logger,
	reconnectCfg ReconnectConfig,
) *WebSocketClient {
	return &WebSocketClient{
		baseURL:       baseURL,
		handler:       handler,
		logger:        logger,
		stopCh:        make(chan struct{}),
		reconnectCfg:  reconnectCfg,
		lastHeartbeat: time.Now(),
	}
}

// Connect 连接 WebSocket 端点
// 自动将 HTTP → ws://, HTTPS → wss://
func (c *WebSocketClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 构造 WebSocket URL
	wsURL, err := c.buildWebSocketURL()
	if err != nil {
		return errs.New(errs.CodeWeChatPadProConnectFailed, fmt.Sprintf("无法构造 WebSocket URL: %v", err))
	}

	// 连接 WebSocket
	c.logger.Info("正在连接 WeChatPadPro WebSocket",
		zap.String("url", wsURL))

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return errs.New(errs.CodeWeChatPadProConnectFailed, fmt.Sprintf("连接失败: %v", err))
	}

	c.conn = conn
	c.lastHeartbeat = time.Now()
	c.logger.Info("WebSocket 连接成功")

	// 启动接收循环（带重连）
	c.wg.Add(1)
	go c.receiveLoopWithReconnect()

	return nil
}

// buildWebSocketURL 构造 WebSocket URL
// HTTP → ws://, HTTPS → wss://
// 路径: /api/message/ws
func (c *WebSocketClient) buildWebSocketURL() (string, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}

	// 转换协议
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	// 构造 WebSocket URL
	wsURL := &url.URL{
		Scheme: scheme,
		Host:   u.Host,
		Path:   "/api/message/ws",
	}

	return wsURL.String(), nil
}

// receiveLoopWithReconnect 带重连的消息接收循环
// 自动处理连接断开并重连
func (c *WebSocketClient) receiveLoopWithReconnect() {
	defer c.wg.Done()
	c.logger.Info("WebSocket 接收循环已启动（带自动重连）")

	retryCount := 0
	delay := c.reconnectCfg.InitialDelay

	for {
		// 检查停止信号
		select {
		case <-c.stopCh:
			c.logger.Info("收到停止信号，退出接收循环")
			return
		default:
		}

		// 执行消息接收循环
		needReconnect := c.receiveLoop()

		// 如果不需要重连（正常停止），退出
		if !needReconnect {
			return
		}

		// 检查最大重试次数
		if c.reconnectCfg.MaxRetries >= 0 && retryCount >= c.reconnectCfg.MaxRetries {
			c.logger.Error("达到最大重试次数，停止重连",
				zap.Int("max_retries", c.reconnectCfg.MaxRetries))
			return
		}

		// 等待重连延迟
		c.logger.Info("准备重连 WebSocket",
			zap.Int("retry_count", retryCount+1),
			zap.Duration("delay", delay))

		select {
		case <-c.stopCh:
			c.logger.Info("收到停止信号，取消重连")
			return
		case <-time.After(delay):
		}

		// 尝试重连
		if err := c.reconnect(); err != nil {
			c.logger.Error("重连失败",
				zap.Error(err),
				zap.Int("retry_count", retryCount+1))

			// 增加重试计数和延迟（指数退避）
			retryCount++
			delay = time.Duration(float64(delay) * c.reconnectCfg.Multiplier)
			if delay > c.reconnectCfg.MaxDelay {
				delay = c.reconnectCfg.MaxDelay
			}
		} else {
			// 重连成功，重置计数器
			c.logger.Info("WebSocket 重连成功")
			retryCount = 0
			delay = c.reconnectCfg.InitialDelay
		}
	}
}

// reconnect 重新建立 WebSocket 连接
func (c *WebSocketClient) reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭旧连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// 构造 WebSocket URL
	wsURL, err := c.buildWebSocketURL()
	if err != nil {
		return errs.New(errs.CodeWeChatPadProConnectFailed, fmt.Sprintf("无法构造 WebSocket URL: %v", err))
	}

	// 建立新连接
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return errs.New(errs.CodeWeChatPadProConnectFailed, fmt.Sprintf("连接失败: %v", err))
	}

	c.conn = conn
	c.lastHeartbeat = time.Now()

	return nil
}

// receiveLoop 消息接收循环
// 返回 true 表示需要重连，false 表示正常退出
func (c *WebSocketClient) receiveLoop() bool {
	c.logger.Debug("开始消息接收循环")
	heartbeatTimeout := 30 * time.Second // 30秒无消息视为心跳超时

	for {
		select {
		case <-c.stopCh:
			c.logger.Info("收到停止信号，退出接收循环")
			return false // 正常退出，不需要重连
		default:
		}

		// 检查心跳超时
		c.mu.Lock()
		if time.Since(c.lastHeartbeat) > heartbeatTimeout {
			c.logger.Warn("心跳超时，需要重连",
				zap.Duration("timeout", heartbeatTimeout))
			c.mu.Unlock()
			return true // 需要重连
		}
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			c.logger.Warn("WebSocket 连接已关闭")
			return true // 需要重连
		}

		// 读取消息（设置读超时，避免阻塞停止信号）
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			// 检查是否为超时错误（正常，继续下一轮）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				continue
			}

			// 检查是否为正常关闭
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				c.logger.Info("WebSocket 连接已正常关闭")
				return false // 正常退出，不需要重连
			}

			c.logger.Error("读取 WebSocket 消息失败", zap.Error(err))
			return true // 需要重连
		}

		// 更新心跳时间
		c.mu.Lock()
		c.lastHeartbeat = time.Now()
		c.mu.Unlock()

		// 解析消息
		var wsMsg WebSocketMessage
		if err := json.Unmarshal(messageBytes, &wsMsg); err != nil {
			c.logger.Warn("解析 WebSocket 消息失败",
				zap.Error(err),
				zap.String("raw", string(messageBytes)))
			continue
		}

		// 过滤：只处理 type="message" 的消息
		if wsMsg.Type != "message" {
			c.logger.Debug("跳过非消息类型",
				zap.String("type", wsMsg.Type))
			continue
		}

		// 检查数据是否存在
		if wsMsg.Data == nil {
			c.logger.Warn("WebSocket 消息缺少 data 字段")
			continue
		}

		// 支持多种消息类型：文本(1)、图片(3)、语音(34)、表情(47)、链接/文件(49)、系统(10000)
		switch wsMsg.Data.MsgType {
		case 1, 3, 34, 47, 49, 10000:
			// 支持的消息类型，继续处理
		default:
			c.logger.Debug("跳过不支持的消息类型",
				zap.Int("msg_type", wsMsg.Data.MsgType))
			continue
		}

		// 转换为 IncomingMessage
		incomingMsg := c.convertToIncomingMessage(wsMsg.Data)

		// 调用处理回调
		if err := c.handler(&incomingMsg); err != nil {
			c.logger.Error("处理消息失败",
				zap.Error(err),
				zap.String("msg_id", incomingMsg.MsgID))
		}
	}
}

// convertToIncomingMessage 将 WSMsgData 转换为 IncomingMessage
func (c *WebSocketClient) convertToIncomingMessage(data *WSMsgData) wechat.IncomingMessage {
	return wechat.IncomingMessage{
		MsgID:      data.MsgID,
		MsgType:    wechat.MsgType(data.MsgType),
		FromUser:   data.FromWxID,
		FromGroup:  data.RoomWxID,
		Content:    strings.TrimSpace(data.Content),
		SenderName: data.FromName,
		Timestamp:  time.Unix(data.CreateTime, 0),
	}
}

// Close 关闭 WebSocket 连接
// 优雅停止接收循环，防止重复关闭
func (c *WebSocketClient) Close() error {
	c.mu.Lock()

	// 防止重复关闭
	select {
	case <-c.stopCh:
		c.mu.Unlock()
		return nil
	default:
	}

	// 发送停止信号
	close(c.stopCh)

	// 关闭连接
	var closeErr error
	if c.conn != nil {
		closeErr = c.conn.Close()
		c.conn = nil
	}

	c.mu.Unlock()

	// 等待接收循环退出
	c.wg.Wait()

	c.logger.Info("WebSocket 客户端已关闭")
	return closeErr
}
