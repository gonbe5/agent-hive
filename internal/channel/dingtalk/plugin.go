package dingtalk

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// Plugin 钉钉 ChannelPlugin 实现
type Plugin struct {
	cfg     config.DingTalkConfig
	client  *Client
	webhook *WebhookHandler
	logger  *zap.Logger
	mu      sync.RWMutex
	// sessionWebhook 缓存最近的 session webhook URL
	sessionWebhooks map[string]string // chatID → webhookURL
}

func New(cfg config.DingTalkConfig, router *channel.Router, logger *zap.Logger) *Plugin {
	client := NewClient(logger)
	p := &Plugin{
		cfg:             cfg,
		client:          client,
		logger:          logger,
		sessionWebhooks: make(map[string]string),
	}
	p.webhook = NewWebhookHandler(router, cfg.AppSecret, logger)
	return p
}

func (p *Plugin) Platform() channel.Platform {
	return channel.PlatformDingTalk
}

func (p *Plugin) Send(ctx context.Context, msg channel.OutboundMessage) error {
	start := time.Now()
	p.mu.RLock()
	webhookURL, ok := p.sessionWebhooks[msg.ChatID]
	p.mu.RUnlock()

	if !ok || webhookURL == "" {
		return errs.New(errs.CodeChannelSendFailed, "未找到该聊天的 webhook URL: "+msg.ChatID)
	}

	// 大消息分块发送
	chunks := channel.ChunkText(msg.Content, 18000)
	for _, chunk := range chunks {
		if err := p.client.SendByWebhook(ctx, webhookURL, chunk); err != nil {
			p.logger.Warn("钉钉消息发送失败",
				zap.Duration("duration", time.Since(start)),
				zap.String("chat_id", msg.ChatID),
				zap.Error(err),
			)
			return err
		}
	}
	p.logger.Info("钉钉消息发送完成",
		zap.Duration("duration", time.Since(start)),
		zap.String("chat_id", msg.ChatID),
		zap.Int("chunks", len(chunks)),
	)
	return nil
}

func (p *Plugin) WebhookHandler() http.HandlerFunc {
	return p.webhook.ServeHTTP
}

func (p *Plugin) Verify(r *http.Request) bool {
	// 未配置 AppSecret 时跳过签名验证
	if p.cfg.AppSecret == "" {
		return true
	}
	timestamp := r.Header.Get("timestamp")
	sign := r.Header.Get("sign")
	return VerifySignature(timestamp, sign, p.cfg.AppSecret)
}

// CacheWebhook 缓存 session webhook URL（由 webhook handler 调用）
func (p *Plugin) CacheWebhook(chatID, webhookURL string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionWebhooks[chatID] = webhookURL
}
