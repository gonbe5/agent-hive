package wecom

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
)

// Plugin 企业微信 ChannelPlugin 实现
type Plugin struct {
	cfg      config.WeComConfig
	client   *Client
	callback *CallbackHandler
	logger   *zap.Logger
}

// New 创建企业微信插件
func New(cfg config.WeComConfig, router *channel.Router, logger *zap.Logger) *Plugin {
	client := NewClient(cfg.CorpID, cfg.Secret, cfg.AgentID, logger)
	p := &Plugin{
		cfg:    cfg,
		client: client,
		logger: logger,
	}
	p.callback = NewCallbackHandler(cfg.Token, cfg.EncodingAESKey, cfg.CorpID, router, logger)
	return p
}

// Platform 返回平台标识
func (p *Plugin) Platform() channel.Platform {
	return channel.PlatformWeCom
}

// Send 发送消息到企业微信
func (p *Plugin) Send(ctx context.Context, msg channel.OutboundMessage) error {
	start := time.Now()
	// 大消息分块发送
	chunks := channel.ChunkText(msg.Content, 2048) // 企业微信文本消息限制2048字节
	for _, chunk := range chunks {
		if err := p.client.SendMessage(ctx, msg.ChatID, chunk); err != nil {
			p.logger.Warn("企业微信消息发送失败",
				zap.Duration("duration", time.Since(start)),
				zap.String("chat_id", msg.ChatID),
				zap.Error(err),
			)
			return err
		}
	}
	p.logger.Info("企业微信消息发送完成",
		zap.Duration("duration", time.Since(start)),
		zap.String("chat_id", msg.ChatID),
		zap.Int("chunks", len(chunks)),
	)
	return nil
}

// WebhookHandler 返回企业微信回调处理器
func (p *Plugin) WebhookHandler() http.HandlerFunc {
	return p.callback.ServeHTTP
}

// Verify 验证企业微信回调签名
func (p *Plugin) Verify(r *http.Request) bool {
	return true // 签名验证在 CallbackHandler 内部处理
}
