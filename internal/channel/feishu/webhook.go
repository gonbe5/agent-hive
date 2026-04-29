package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
)

// acceptableTimestamp 解析飞书 X-Lark-Request-Timestamp（秒级 unix 时间戳）
// 并判断是否落在 [now-window, now+window]。
//
// 不可解析 / 负值 / 偏差 > window → false（fail-closed）。
//
// 飞书生产路径上时间戳是字符串 unix 秒；测试可注入 now 函数固定时间。
func (h *WebhookHandler) acceptableTimestamp(ts string, now time.Time) bool {
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sec <= 0 {
		return false
	}
	delta := now.Sub(time.Unix(sec, 0))
	if delta < 0 {
		delta = -delta
	}
	return delta <= webhookReplayWindow
}

func (h *WebhookHandler) now() time.Time {
	if h.nowFn != nil {
		return h.nowFn()
	}
	return time.Now()
}

// WebhookHandler 处理飞书事件回调（webhook ingress）。
//
// Phase 0 P0-#4 重写要点：
//   - 不再手撸 JSON 解析与签名验证，整段委托给 SDK 的 dispatcher（与 longconn 共享同一实例）。
//   - URL 验证 / encrypt 解密 / signature 验证全部由 SDK 完成；本地仅留薄壳便于注入 HITL bridge
//     与未来的 P0-#5 / P0-#6 wrapper（signature-missing 401 与 timestamp ±5min 防回放）。
//   - SDK 的 OnP2MessageReceiveV1 handler 复用 longconn 的业务路径，避免双通道行为漂移。
type WebhookHandler struct {
	verificationToken   string
	encryptKey          string
	eventEncryptEnabled bool
	router              *channel.Router
	logger              *zap.Logger

	hitlBridge    *FeishuHITLBridge
	lifecycle     *LifecycleHandler
	metricsWriter observability.MetricsWriter

	nowFn func() time.Time // 测试可注入；nil 时退化为 time.Now

	once sync.Once
	sdk  http.HandlerFunc

	botOpenIDMu sync.Mutex
	botOpenID   string
	client      *Client // 选传：用于 GetBotOpenID 群聊 @ 过滤
}

// NewWebhookHandler 创建飞书 webhook 处理器。签名向后兼容：旧调用点不需改动即可继续编译。
func NewWebhookHandler(verificationToken, encryptKey string, router *channel.Router, logger *zap.Logger) *WebhookHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &WebhookHandler{
		verificationToken: verificationToken,
		encryptKey:        encryptKey,
		router:            router,
		logger:            logger,
	}
}

func (h *WebhookHandler) WithEventEncryptEnabled(enabled bool) *WebhookHandler {
	h.eventEncryptEnabled = enabled
	return h
}

func (h *WebhookHandler) ReloadFromConfig(cfg config.FeishuConfig) error {
	if h == nil {
		return nil
	}
	h.verificationToken = cfg.VerificationToken
	h.encryptKey = cfg.EncryptKey
	h.eventEncryptEnabled = cfg.EventEncryptEnabled
	h.once = sync.Once{}
	h.sdk = nil
	return nil
}

// WithHITLBridge 注入 HITL 桥接。返回 self 便于链式构造。
func (h *WebhookHandler) WithHITLBridge(bridge *FeishuHITLBridge) *WebhookHandler {
	h.hitlBridge = bridge
	return h
}

// WithLifecycleHandler 注入机器人进群/退群生命周期处理器。
func (h *WebhookHandler) WithLifecycleHandler(handler *LifecycleHandler) *WebhookHandler {
	h.lifecycle = handler
	return h
}

// WithClient 注入飞书 OpenAPI 客户端，用于群聊 @机器人过滤（getBotOpenID）。
// 不注入时降级为"任意 mention 即处理"，与 longconn 兜底策略对齐。
func (h *WebhookHandler) WithClient(c *Client) *WebhookHandler {
	h.client = c
	return h
}

// WithNowFunc 注入时钟，仅用于单测稳定 timestamp window 判断。
func (h *WebhookHandler) WithNowFunc(now func() time.Time) *WebhookHandler {
	h.nowFn = now
	return h
}

func (h *WebhookHandler) SetMetricsWriter(w observability.MetricsWriter) {
	if h == nil {
		return
	}
	h.metricsWriter = w
}

// 飞书 webhook 签名头名（与 SDK larkevent 包内常量一致；为避免外部依赖直接复制字符串）。
// 若飞书未来变更头名，SDK 会一起换；这里也需同步。
const (
	headerLarkSignature = "X-Lark-Signature"
	headerLarkTimestamp = "X-Lark-Request-Timestamp"
	headerLarkNonce     = "X-Lark-Request-Nonce"
)

// 飞书 webhook timestamp 防回放窗口。
// 飞书自己签名时间戳粒度为秒；±5 分钟窗口与社区共识 / 大多数 IM webhook 实现一致。
//
// 触发拒绝的场景：
//   - 攻击者重放 1 小时前的合法签名包（哪怕签名仍校验通过，timestamp 已过期）
//   - 客户机器或飞书服务器时钟严重偏差（超过 5 分钟视为不可信）
//
// 不超过窗口但 timestamp 不可解析（非数字、空、负）一律拒绝，故障于安全侧。
const webhookReplayWindow = 5 * time.Minute

// ServeHTTP 是 webhook 入口。所有协议层细节交给 SDK：
//  1. 读 body
//  2. 用 EncryptKey 解 AES（若配置）
//  3. 用 EncryptKey 验签（若配置）
//  4. 解析 JSON
//  5. 路由到注册的 OnXxx handler
//  6. handler 返回 nil → 200; 返回 err → 500（这就是为何 P0-#7 要求 handler 永返 nil）
//
// Phase 0 P0-#5：在委托 SDK 前做"签名头存在性"前置守卫。
//   - SDK 的 VerifySign 在缺签名头时会返回签名失败错误并经 processError 写 500，
//     500 会让飞书无限重试（红队链 A 的另一面：5xx 循环）。
//   - 我们提前对未带签名头的请求返回 401，让飞书停止重试并触发可观察告警。
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	if h.eventEncryptEnabled {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.logger.Warn("飞书 webhook 读取请求体失败",
				zap.String("remote", r.RemoteAddr),
				zap.Error(err))
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var probe struct {
			Encrypt string `json:"encrypt"`
		}
		if err := json.Unmarshal(body, &probe); err != nil || probe.Encrypt == "" {
			h.logger.Warn("飞书 webhook 开启严格加密模式但请求非加密体，返回 400",
				zap.String("remote", r.RemoteAddr),
				zap.Error(err))
			h.emitSecurityRejectMetric("plaintext_body_when_encrypt_required")
			http.Error(w, "encryption required", http.StatusBadRequest)
			return
		}
	}
	if h.encryptKey != "" {
		ts := r.Header.Get(headerLarkTimestamp)
		if r.Header.Get(headerLarkSignature) == "" ||
			ts == "" ||
			r.Header.Get(headerLarkNonce) == "" {
			h.logger.Warn("飞书 webhook 收到缺签名头请求，返回 401",
				zap.String("remote", r.RemoteAddr),
				zap.Bool("has_sig", r.Header.Get(headerLarkSignature) != ""),
				zap.Bool("has_ts", ts != ""),
				zap.Bool("has_nonce", r.Header.Get(headerLarkNonce) != ""))
			h.emitSecurityRejectMetric("missing_signature_header")
			http.Error(w, "missing signature header", http.StatusUnauthorized)
			return
		}
		if !h.acceptableTimestamp(ts, h.now()) {
			h.logger.Warn("飞书 webhook timestamp 超出 ±5min 窗口或不可解析，返回 401",
				zap.String("ts", ts),
				zap.String("remote", r.RemoteAddr))
			h.emitSecurityRejectMetric("stale_or_invalid_timestamp")
			http.Error(w, "stale or invalid timestamp", http.StatusUnauthorized)
			return
		}
	}
	h.once.Do(h.initSDKHandler)
	h.sdk(w, r)
}

func (h *WebhookHandler) emitSecurityRejectMetric(reason string) {
	if h == nil || h.metricsWriter == nil {
		return
	}
	_ = h.metricsWriter.Record(context.Background(), observability.Metric{
		Name:  MetricWebhookSecurityReject,
		Value: 1,
		Labels: map[string]any{
			"reason": reason,
		},
		Ts: time.Now(),
	})
}

func (h *WebhookHandler) initSDKHandler() {
	d := BuildEventDispatcher(DispatcherDeps{
		VerificationToken: h.verificationToken,
		EncryptKey:        h.encryptKey,
		MessageReceived:   h.handleMessageReceive,
		HITLBridge:        h.hitlBridge,
		LifecycleHandler:  h.lifecycle,
		Logger:            h.logger,
	})

	// 用 SDK logger 把内部 [Error] 桥接到 zap，避免双 logger 漂移。
	h.sdk = httpserverext.NewEventHandlerFunc(d, larkevent.WithLogger(&sdkZapLogger{l: h.logger}))
}

// handleMessageReceive 是 P0-#4 webhook 业务入口。
// Phase 0 P0-#7 不变量：
//   - 永远返回 nil（即使内部 panic）。返回非 nil 会让 SDK 写 5xx 触发飞书无限重试风暴（红队链 A）
//   - 业务失败 / router nil / panic recover 三条路径都必须把消息塞进 retry_queue（兜底持久化）
//   - panic 被 recover 后日志 + 入队，不再向上冒
func (h *WebhookHandler) handleMessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
	// P0-#7 panic recover：handler 顶层守卫，任何 panic 都不能冒到 SDK 触发 5xx
	defer func() {
		if rec := recover(); rec != nil {
			h.logger.Error("飞书 webhook handler 顶层 panic recovered",
				zap.Any("panic", rec))
			h.enqueueRetry(channel.RetryItem{
				Platform: string(channel.PlatformFeishu),
				Reason:   channel.RetryReasonHandlerPanic,
				ErrorMsg: fmt.Sprintf("handler panic: %v", rec),
			})
		}
	}()

	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	msgEvent := event.Event
	msg := msgEvent.Message

	messageID := strDeref(msg.MessageId)
	chatID := strDeref(msg.ChatId)
	chatType := channel.ChatGroup
	if msg.ChatType != nil && *msg.ChatType == "p2p" {
		chatType = channel.ChatDirect
	}

	// 群聊：@机器人时才处理（与 longconn 行为对齐）
	if chatType == channel.ChatGroup && !h.isBotMentioned(msgEvent.Message.Mentions) {
		return nil
	}

	senderID := ""
	if msgEvent.Sender != nil && msgEvent.Sender.SenderId != nil && msgEvent.Sender.SenderId.OpenId != nil {
		senderID = *msgEvent.Sender.SenderId.OpenId
	}

	messageType := ""
	content := ""
	var attachments []channel.Attachment
	var refs []imctx.DocRef
	if msg.MessageType != nil && msg.Content != nil {
		messageType = *msg.MessageType
		parsed := ParseInboundMessage(messageType, *msg.Content)
		content = parsed.TextContent
		if messageType == "text" {
			content = resolveMentions(content, msg.Mentions)
		}
		for _, att := range parsed.Attachments {
			attachments = append(attachments, channel.Attachment{
				Type:     att.Type,
				Key:      att.Key,
				FileName: att.FileName,
			})
		}
		refs = parsed.References
	}

	// 提取父消息 ID（回复/引用场景）。RootId 作为 ParentId 兜底，详见 longconn.go 同名注释。
	parentID := ""
	rootID := ""
	if msg.ParentId != nil {
		parentID = *msg.ParentId
	}
	if msg.RootId != nil {
		rootID = *msg.RootId
	}
	if parentID == "" && rootID != "" {
		parentID = rootID
	}

	// 提取 mentions 结构化信息
	mentions, botMentioned := extractMentions(msg.Mentions, h.cachedBotOpenID())

	// 从事件头提取 tenantKey，填入 InboundMessage 用于多租户 session_id 构造。
	tenantKey := ""
	if event.EventV2Base != nil {
		tenantKey = event.EventV2Base.TenantKey()
	}

	inbound := channel.InboundMessage{
		MessageID:    messageID,
		Platform:     channel.PlatformFeishu,
		TenantKey:    tenantKey,
		ChatType:     chatType,
		ChatID:       chatID,
		SenderID:     senderID,
		SenderName:   "",
		Content:      content,
		MessageType:  messageType,
		Attachments:  attachments,
		References:   refs,
		ParentID:     parentID,
		RootID:       rootID,
		Mentions:     mentions,
		BotMentioned: botMentioned,
		Timestamp:    time.Now(),
	}

	h.logger.Info("收到飞书 webhook 消息",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		zap.String("chat_type", string(chatType)),
		zap.String("safe_sender_id", SafeSenderID(senderID)),
		zap.String("message_type", messageType),
		zap.Int("content_len", len(content)),
		zap.Int("refs_count", len(refs)),
		zap.Strings("refs_summary", formatRefsForLog(refs)),
		zap.String("parent_id", parentID),
		zap.String("root_id", rootID))

	if h.router == nil {
		// 编排错误（或测试场景）。logger 记录 + 入 retry_queue，绝不返回 err 让飞书重试。
		h.logger.Warn("webhook router 未配置，落 retry_queue 兜底",
			zap.String("message_id", messageID))
		h.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonRouterNil, "router not configured"))
		return nil
	}

	// P0-#8：从事件头提取 eventID，用于两阶段 claim dedup。
	var eventID string
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}

	// 异步处理；HTTP 响应立即 200。
	// 异步 goroutine 自身也要 panic recover + retry_queue 兜底——否则崩溃只剩 stderr 痕迹。
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				h.logger.Error("飞书 webhook 异步处理 goroutine panic recovered",
					zap.String("message_id", messageID),
					zap.Any("panic", rec))
				h.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonHandlerPanic,
					fmt.Sprintf("async goroutine panic: %v", rec)))
			}
		}()

		// P0-#8：两阶段 event claim。claimer 未注入时跳过（兼容老测试）。
		claimer := h.router.EventClaimer()
		var claimToken master.ClaimToken
		if claimer != nil && eventID != "" {
			tok, ok := claimer.ClaimEvent(eventID, master.DefaultClaimLease)
			if !ok {
				h.logger.Info("event claim 被拒（已被其他 worker 占住或已完成），跳过处理",
					zap.String("event_id", eventID),
					zap.String("message_id", messageID))
				return
			}
			claimToken = tok
		}

		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := h.router.HandleMessage(bgCtx, inbound); err != nil {
			h.logger.Error("处理飞书 webhook 消息失败，落 retry_queue",
				zap.String("message_id", messageID),
				zap.Error(err))
			h.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonHandlerError, err.Error()))
			h.router.NotifyError(bgCtx, inbound, err)
			return
		}

		// P0-#8：业务成功 → 标记 completed，dedup 正式生效。
		if claimer != nil && claimToken.EventID != "" {
			if err := claimer.CompleteEvent(claimToken); err != nil {
				h.logger.Warn("CompleteEvent 失败（可能 lease 已过期被 reclaim）",
					zap.String("event_id", eventID),
					zap.Error(err))
			}
		}
	}()

	return nil
}

// enqueueRetry 把一条 RetryItem 推到 router.retryQueue。
// router 为 nil 或 retryQueue 未注入时降级为日志，仍然保证 handler 永返 nil。
func (h *WebhookHandler) enqueueRetry(item channel.RetryItem) {
	if h.router == nil {
		h.logger.Warn("retry_queue 未注入（router 为 nil），仅记录日志兜底",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.String("error", item.ErrorMsg))
		return
	}
	q := h.router.RetryQueue()
	if q == nil {
		h.logger.Warn("retry_queue 未注入，仅记录日志兜底",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.String("error", item.ErrorMsg))
		return
	}
	if err := q.Enqueue(item); err != nil {
		h.logger.Error("retry_queue Enqueue 失败（消息进入 best-effort 日志兜底）",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.Error(err))
	}
}

// retryItemFromInbound 把 InboundMessage 序列化进 RetryItem。
// 序列化失败时退化为不带 payload 的 item，仍然带上 ID/原因/错误字符串便于运维定位。
func retryItemFromInbound(msg channel.InboundMessage, reason channel.RetryReason, errMsg string) channel.RetryItem {
	item := channel.RetryItem{
		MessageID: msg.MessageID,
		Platform:  string(msg.Platform),
		TenantKey: msg.TenantKey,
		ChatID:    msg.ChatID,
		SenderID:  msg.SenderID,
		Reason:    reason,
		ErrorMsg:  errMsg,
	}
	if data, err := json.Marshal(msg); err == nil {
		item.Payload = data
	}
	return item
}

// isBotMentioned 与 longconn 同算法：未知 botOpenID 时降级为"有任意 mention 即处理"。
func (h *WebhookHandler) isBotMentioned(mentions []*larkim.MentionEvent) bool {
	if len(mentions) == 0 {
		return false
	}
	openID := h.cachedBotOpenID()
	if openID == "" {
		return true
	}
	for _, m := range mentions {
		if m == nil || m.Id == nil || m.Id.OpenId == nil {
			continue
		}
		if *m.Id.OpenId == openID {
			return true
		}
	}
	return false
}

// cachedBotOpenID 懒加载 bot open_id（client 未注入时返回空，触发降级策略）。
func (h *WebhookHandler) cachedBotOpenID() string {
	h.botOpenIDMu.Lock()
	defer h.botOpenIDMu.Unlock()
	if h.botOpenID != "" || h.client == nil {
		return h.botOpenID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = ctx
	id := h.client.BotOpenID()
	if id == "" {
		h.logger.Warn("webhook 获取机器人 open_id 失败，降级为 mention-any")
		return ""
	}
	h.botOpenID = id
	return id
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// sdkZapLogger 把 SDK 的 larkcore.Logger 桥接到 zap。
type sdkZapLogger struct {
	l *zap.Logger
}

func (s *sdkZapLogger) Debug(_ context.Context, args ...any) { s.l.Sugar().Debug(args...) }
func (s *sdkZapLogger) Info(_ context.Context, args ...any)  { s.l.Sugar().Info(args...) }
func (s *sdkZapLogger) Warn(_ context.Context, args ...any)  { s.l.Sugar().Warn(args...) }
func (s *sdkZapLogger) Error(_ context.Context, args ...any) { s.l.Sugar().Error(args...) }
