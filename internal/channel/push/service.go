package push

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"go.uber.org/zap"
)

type Config struct {
	Enabled          bool
	PerChatPerMinute int
	IdempotencyTTL   time.Duration
}

type Request struct {
	Platform       channel.Platform `json:"platform"`
	ChatID         string           `json:"chat_id,omitempty"`
	OpenID         string           `json:"open_id,omitempty"`
	MsgType        channel.MsgType  `json:"msg_type"`
	Content        string           `json:"content"`
	Template       string           `json:"template,omitempty"`
	Vars           map[string]any   `json:"vars,omitempty"`
	IdempotencyKey string           `json:"idempotency_key,omitempty"`
}

type Service struct {
	router          *channel.Router
	logger          *zap.Logger
	perChatLimit    int
	idempotencyTTL  time.Duration
	mu              sync.Mutex
	idempotencySeen map[string]time.Time
	chatWindows     map[string][]time.Time
}

func NewService(router *channel.Router, cfg Config, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	limit := cfg.PerChatPerMinute
	if limit <= 0 {
		limit = 10
	}
	ttl := cfg.IdempotencyTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Service{
		router:          router,
		logger:          logger,
		perChatLimit:    limit,
		idempotencyTTL:  ttl,
		idempotencySeen: make(map[string]time.Time),
		chatWindows:     make(map[string][]time.Time),
	}
}

func (s *Service) Push(ctx context.Context, req Request) error {
	if s == nil || s.router == nil {
		return fmt.Errorf("push service not configured")
	}
	if req.Platform == "" {
		return fmt.Errorf("platform is required")
	}
	if req.ChatID == "" && req.OpenID == "" {
		return fmt.Errorf("chat_id or open_id is required")
	}
	msgType, content, err := s.resolveContent(req)
	if err != nil {
		return err
	}
	plugin, ok := s.router.GetPlugin(req.Platform)
	if !ok {
		return fmt.Errorf("platform not registered: %s", req.Platform)
	}
	// Phase 6 缺口 12 修复:不再拼 "p2p:" 前缀。
	// feishu/client.SendMessage 现在按 chatID 前缀(receive_id.go inferReceiveIDType)
	// 自动切 receive_id_type:oc_xxx → chat_id, ou_xxx → open_id (P2P 主路径)。
	// 历史 "p2p:" 前缀也会被 inferReceiveIDType 剥掉,保持向后兼容。
	chatID := req.ChatID
	if chatID == "" {
		chatID = req.OpenID
	}
	if req.IdempotencyKey != "" && s.seenIdempotency(req.IdempotencyKey, time.Now()) {
		return nil
	}
	if err := s.allowChat(chatID, time.Now()); err != nil {
		return err
	}
	err = plugin.Send(ctx, channel.OutboundMessage{
		Platform: req.Platform,
		ChatID:   chatID,
		MsgType:  msgType,
		Content:  content,
	})
	if err != nil {
		s.enqueueRetry(req, chatID, err)
	}
	return err
}

func (s *Service) ReloadFromConfig(cfg config.FeishuConfig) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perChatLimit = cfg.PushPerChatPerMinuteResolved()
	s.idempotencyTTL = cfg.PushIdempotencyTTLResolved()
	return nil
}

func (s *Service) DispatchScheduledPrompt(ctx context.Context, prompt string) error {
	req, matched, err := ParseScheduledPrompt(prompt)
	if err != nil {
		return err
	}
	if !matched {
		return fmt.Errorf("scheduled push prompt not matched")
	}
	return s.Push(ctx, req)
}

func (s *Service) resolveContent(req Request) (channel.MsgType, string, error) {
	if req.Template != "" {
		msgType, rendered, err := renderBuiltInTemplate(req.Template, req.Vars)
		if err != nil {
			return "", "", err
		}
		if req.MsgType != "" {
			msgType = req.MsgType
		}
		return msgType, rendered, nil
	}
	if req.MsgType == "" {
		req.MsgType = channel.MsgTypeText
	}
	if req.Content == "" {
		return "", "", fmt.Errorf("content is required")
	}
	return req.MsgType, req.Content, nil
}

func ParseScheduledPrompt(prompt string) (Request, bool, error) {
	const prefix = "scheduled_push:"
	if !strings.HasPrefix(prompt, prefix) {
		return Request{}, false, nil
	}
	raw := strings.TrimPrefix(prompt, prefix)
	parts := strings.Split(raw, ":")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return Request{}, true, fmt.Errorf("scheduled push template is required")
	}
	req := Request{
		Platform: channel.PlatformFeishu,
		Template: strings.TrimSpace(parts[0]),
		Vars:     make(map[string]any),
	}
	for _, part := range parts[1:] {
		if strings.TrimSpace(part) == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return Request{}, true, fmt.Errorf("scheduled push argument missing '=': %s", part)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "platform":
			req.Platform = channel.Platform(value)
		case "chat_id":
			req.ChatID = value
		case "open_id":
			req.OpenID = value
		case "msg_type":
			req.MsgType = channel.MsgType(value)
		case "idempotency_key":
			req.IdempotencyKey = value
		default:
			req.Vars[key] = value
		}
	}
	if req.ChatID == "" && req.OpenID == "" {
		return Request{}, true, fmt.Errorf("scheduled push requires chat_id or open_id")
	}
	return req, true, nil
}

func (s *Service) seenIdempotency(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, exp := range s.idempotencySeen {
		if now.After(exp) {
			delete(s.idempotencySeen, k)
		}
	}
	if exp, ok := s.idempotencySeen[key]; ok && now.Before(exp) {
		return true
	}
	s.idempotencySeen[key] = now.Add(s.idempotencyTTL)
	return false
}

func (s *Service) allowChat(chatID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	windowStart := now.Add(-time.Minute)
	items := s.chatWindows[chatID][:0]
	for _, ts := range s.chatWindows[chatID] {
		if ts.After(windowStart) {
			items = append(items, ts)
		}
	}
	if len(items) >= s.perChatLimit {
		s.chatWindows[chatID] = items
		return fmt.Errorf("push rate limited for chat %s", chatID)
	}
	s.chatWindows[chatID] = append(items, now)
	return nil
}

func (s *Service) enqueueRetry(req Request, chatID string, cause error) {
	if s == nil || s.router == nil {
		return
	}
	q := s.router.RetryQueue()
	if q == nil {
		return
	}
	payload, err := json.Marshal(req)
	if err != nil {
		s.logger.Warn("push retry payload marshal failed", zap.Error(err))
	}
	if err := q.Enqueue(channel.RetryItem{
		MessageID: req.IdempotencyKey,
		Platform:  string(req.Platform),
		ChatID:    chatID,
		Reason:    channel.RetryReasonPushSend,
		ErrorMsg:  cause.Error(),
		Payload:   payload,
	}); err != nil {
		s.logger.Error("push enqueue retry failed", zap.Error(err))
	}
}

func NewRetryHandler(service *Service) func(context.Context, channel.RetryItem) error {
	if service == nil {
		return nil
	}
	return func(ctx context.Context, item channel.RetryItem) error {
		var req Request
		if err := json.Unmarshal(item.Payload, &req); err != nil {
			return err
		}
		req.IdempotencyKey = ""
		return service.Push(ctx, req)
	}
}
