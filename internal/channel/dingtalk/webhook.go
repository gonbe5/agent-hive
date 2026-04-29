package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
)

// WebhookHandler 处理钉钉机器人回调
type WebhookHandler struct {
	router    *channel.Router
	appSecret string // 用于签名验证
	logger    *zap.Logger
}

func NewWebhookHandler(router *channel.Router, appSecret string, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		router:    router,
		appSecret: appSecret,
		logger:    logger,
	}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 签名验证：防止伪造请求
	if h.appSecret != "" {
		timestamp := r.Header.Get("timestamp")
		sign := r.Header.Get("sign")
		if !VerifySignature(timestamp, sign, h.appSecret) {
			h.logger.Warn("钉钉回调签名验证失败",
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("timestamp", timestamp))
			http.Error(w, "signature verification failed", http.StatusForbidden)
			return
		}
	}

	var cb DingTalkCallback
	if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
		h.logger.Warn("钉钉回调请求无效", zap.Error(err))
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	content := ""
	if cb.Text != nil {
		content = cb.Text.Content
	}

	// conversationType: "1"=私聊, "2"=群聊；未知时默认群聊（保守策略）
	chatType := channel.ChatGroup
	if cb.ConversationType == "1" {
		chatType = channel.ChatDirect
	}

	msg := channel.InboundMessage{
		MessageID:  cb.MsgID,
		Platform:   channel.PlatformDingTalk,
		ChatType:   chatType,
		ChatID:     cb.ConversationID,
		SenderID:   cb.SenderID,
		SenderName: cb.SenderNick,
		Content:    content,
		Timestamp:  time.Now(),
	}

	// 异步处理消息，立即返回 200
	// 使用独立上下文，避免 HTTP 响应返回后 r.Context() 被取消
	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		defer cancel()
		if err := h.router.HandleMessage(bgCtx, msg); err != nil {
			h.logger.Error("处理钉钉消息失败", zap.Error(err))
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
