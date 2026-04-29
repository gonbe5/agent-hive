package wecom

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
)

// CallbackHandler 处理企业微信回调
type CallbackHandler struct {
	token  string
	aesKey string
	corpID string
	router *channel.Router
	logger *zap.Logger
}

// NewCallbackHandler 创建企业微信回调处理器
func NewCallbackHandler(token, aesKey, corpID string, router *channel.Router, logger *zap.Logger) *CallbackHandler {
	return &CallbackHandler{
		token:  token,
		aesKey: aesKey,
		corpID: corpID,
		router: router,
		logger: logger,
	}
}

// ServeHTTP 处理企业微信回调 HTTP 请求
func (h *CallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	msgSignature := query.Get("msg_signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")

	// GET 请求为 URL 验证
	if r.Method == http.MethodGet {
		echoStr := query.Get("echostr")
		if !VerifySignature(h.token, timestamp, nonce, echoStr, msgSignature) {
			h.logger.Warn("企业微信 URL 验证签名失败")
			http.Error(w, "签名验证失败", http.StatusForbidden)
			return
		}
		decrypted, err := DecryptMessage(h.aesKey, echoStr)
		if err != nil {
			h.logger.Warn("企业微信 URL 验证解密失败", zap.Error(err))
			http.Error(w, "解密失败", http.StatusInternalServerError)
			return
		}
		w.Write(decrypted)
		return
	}

	// POST 请求为消息回调
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Warn("读取企业微信回调体失败", zap.Error(err))
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}

	var callback WeComCallback
	if err := xml.Unmarshal(body, &callback); err != nil {
		h.logger.Warn("解析企业微信回调 XML 失败", zap.Error(err))
		http.Error(w, "请求无效", http.StatusBadRequest)
		return
	}

	// 验证签名
	if !VerifySignature(h.token, timestamp, nonce, callback.Encrypt, msgSignature) {
		h.logger.Warn("企业微信消息签名验证失败")
		http.Error(w, "签名验证失败", http.StatusForbidden)
		return
	}

	// 解密消息
	decrypted, err := DecryptMessage(h.aesKey, callback.Encrypt)
	if err != nil {
		h.logger.Warn("企业微信消息解密失败", zap.Error(err))
		http.Error(w, "解密失败", http.StatusInternalServerError)
		return
	}

	var msg WeComMessage
	if err := xml.Unmarshal(decrypted, &msg); err != nil {
		h.logger.Warn("解析企业微信消息 XML 失败", zap.Error(err))
		http.Error(w, "消息解析失败", http.StatusBadRequest)
		return
	}

	// 只处理文本消息
	if msg.MsgType != "text" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
		return
	}

	inbound := channel.InboundMessage{
		MessageID:  fmt.Sprintf("%d", msg.MsgID),
		Platform:   channel.PlatformWeCom,
		ChatType:   channel.ChatDirect,
		ChatID:     msg.FromUserName,
		SenderID:   msg.FromUserName,
		SenderName: msg.FromUserName,
		Content:    msg.Content,
		Timestamp:  time.Now(),
	}

	// 异步处理消息，立即返回
	// 使用独立上下文，避免 HTTP 响应返回后 r.Context() 被取消
	go func() {
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		defer cancel()
		if err := h.router.HandleMessage(bgCtx, inbound); err != nil {
			h.logger.Error("处理企业微信消息失败", zap.Error(err))
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}
