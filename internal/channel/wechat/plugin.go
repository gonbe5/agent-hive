package wechat

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// Plugin 个人微信 ChannelPlugin 实现
// 策略模式：委托给 Protocol 接口的具体实现
type Plugin struct {
	protocolName string // "openwechat"|"gewechat"|"wcferry"|"wechaty"
	protocol     Protocol
	router       *channel.Router
	logger       *zap.Logger
}

// New 创建微信插件
func New(protocolName string, protocol Protocol, router *channel.Router, logger *zap.Logger) *Plugin {
	p := &Plugin{
		protocolName: protocolName,
		protocol:     protocol,
		router:       router,
		logger:       logger,
	}

	// 设置协议消息回调 → 桥接到 Router
	protocol.SetMessageHandler(p.onMessage)

	return p
}

// Platform 返回平台标识
func (p *Plugin) Platform() channel.Platform {
	return channel.Platform("wechat-" + p.protocolName)
}

// Send 发送消息到微信
func (p *Plugin) Send(ctx context.Context, msg channel.OutboundMessage) error {
	start := time.Now()
	if !p.protocol.IsLoggedIn() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录，无法发送消息")
	}

	// 大消息分块发送（微信单条消息限制约 2KB）
	chunks := channel.ChunkText(msg.Content, 2048)
	for _, chunk := range chunks {
		if err := p.protocol.SendText(ctx, msg.ChatID, chunk); err != nil {
			p.logger.Warn("微信消息发送失败",
				zap.Duration("duration", time.Since(start)),
				zap.String("chat_id", msg.ChatID),
				zap.Error(err),
			)
			return errs.Wrap(errs.CodeChannelSendFailed, "微信消息发送失败", err)
		}
	}
	p.logger.Info("微信消息发送完成",
		zap.Duration("duration", time.Since(start)),
		zap.String("chat_id", msg.ChatID),
		zap.Int("chunks", len(chunks)),
	)
	return nil
}

// WebhookHandler 返回 HTTP handler
// 仅 GeweChat 协议需要 webhook，其余协议返回 405
func (p *Plugin) WebhookHandler() http.HandlerFunc {
	if wp, ok := p.protocol.(WebhookProvider); ok {
		return wp.WebhookHandler()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "该微信协议不支持 webhook 回调", http.StatusMethodNotAllowed)
	}
}

// Verify 验证请求签名（微信个人协议无统一签名机制）
func (p *Plugin) Verify(_ *http.Request) bool {
	return true
}

// Start 启动协议连接
func (p *Plugin) Start(ctx context.Context) error {
	p.logger.Info("启动微信协议", zap.String("protocol", p.protocol.Name()))
	return p.protocol.Start(ctx)
}

// Stop 停止协议连接
func (p *Plugin) Stop() error {
	p.logger.Info("停止微信协议", zap.String("protocol", p.protocol.Name()))
	return p.protocol.Stop()
}

// onMessage 协议消息回调 → 转换为 InboundMessage → 路由到 Router
func (p *Plugin) onMessage(msg IncomingMessage) {
	// 仅处理文本消息
	if msg.MsgType != MsgText {
		return
	}

	chatType := channel.ChatDirect
	if msg.IsGroup() {
		chatType = channel.ChatGroup
	}

	inbound := channel.InboundMessage{
		MessageID:  msg.MsgID,
		Platform:   p.Platform(), // 使用具体协议的 Platform 标识
		ChatType:   chatType,
		ChatID:     msg.ChatID(),
		SenderID:   msg.FromUser,
		SenderName: msg.SenderName,
		Content:    msg.Content,
		Timestamp:  time.Now(),
	}

	// 异步处理，不阻塞协议层消息循环
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := p.router.HandleMessage(bgCtx, inbound); err != nil {
			p.logger.Error("处理微信消息失败",
				zap.String("msg_id", msg.MsgID),
				zap.Error(err))
		}
	}()
}
