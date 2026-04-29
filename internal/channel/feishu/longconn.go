package feishu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/imctx"
)

// LongConnClient 飞书长连接客户端，通过 WebSocket 接收飞书事件
type LongConnClient struct {
	appID     string
	appSecret string
	client    *Client
	router    *channel.Router
	logger    *zap.Logger

	botOpenID string // 机器人自身的 OpenID，用于群聊 @过滤

	hitlBridge *FeishuHITLBridge // 卡片回调桥接，nil 表示未配置 HITL（仅文本通路）

	cancel    context.CancelFunc
	ctx       context.Context // 长连接生命周期上下文
	started   bool            // 防重入：避免多次调用 Start() 建立多条连接
	mu        sync.Mutex
	startHook func(context.Context) error

	watchdogMu                  sync.RWMutex
	watchdogCfg                 longConnWatchdogConfig
	watchdogStateMu             sync.RWMutex
	lastEventAt                 time.Time
	lastTenantKey               string
	reconnecting                bool
	reliabilityLeader           bool
	gapFetchEnabled             bool
	gapFetchMaxWindow           time.Duration
	pendingGapFetch             *GapFetchWindow
	pendingGapFetchWindowCapped bool
	gapFetchRunner              *gapFetchRunner
	lastGapFetchAt              time.Time
	lastGapFetchChatIDs         []string
	lastGapFetchError           string
}

// NewLongConnClient 创建飞书长连接客户端
func NewLongConnClient(appID, appSecret string, client *Client, router *channel.Router, logger *zap.Logger) *LongConnClient {
	c := &LongConnClient{
		appID:     appID,
		appSecret: appSecret,
		client:    client,
		router:    router,
		logger:    logger,
	}
	c.gapFetchRunner = newGapFetchRunner(router, client, logger)
	c.initWatchdog(newLongConnWatchdogConfig(logger))
	return c
}

// WithHITLBridge 注入 HITL 桥接，使长连接 dispatcher 注册 card.action.trigger 处理器。
// nil 表示禁用 HITL 通路；保留无桥接构造便于纯文本压测。
func (c *LongConnClient) WithHITLBridge(bridge *FeishuHITLBridge) *LongConnClient {
	c.hitlBridge = bridge
	return c
}

// Start 启动长连接。首次握手成功后返回；失败则返回错误。
func (c *LongConnClient) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		c.logger.Warn("飞书长连接已在运行，忽略重复的 Start 调用")
		return nil
	}
	c.mu.Unlock()

	if c.startHook != nil {
		if err := c.startHook(ctx); err != nil {
			return err
		}
		c.mu.Lock()
		c.started = true
		c.ctx = ctx
		c.mu.Unlock()
		c.markEventObserved(time.Now())
		c.startWatchdog(ctx)
		return nil
	}

	// 启动前同步预热机器人 OpenID（用于群聊 @ 过滤）
	if c.client != nil {
		if openID := c.client.BotOpenID(); openID != "" {
			c.botOpenID = openID
			c.logger.Info("机器人 OpenID 已获取", zap.String("safe_bot_id", imctx.SafeSenderID(openID)))
		} else {
			c.logger.Warn("获取机器人 OpenID 失败，群聊消息将不过滤 @ 状态")
		}
	}

	innerCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.ctx = innerCtx

	// 共享 factory：与 webhook 走同一段事件注册逻辑，避免双通道行为漂移。
	eventDispatcher := BuildEventDispatcher(DispatcherDeps{
		MessageReceived: c.handleMessageReceive,
		HITLBridge:      c.hitlBridge,
		Logger:          c.logger,
	})

	// 创建 WebSocket 客户端
	wsClient := larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithAutoReconnect(true),
	)

	readyCtx, readyCancel := context.WithCancel(innerCtx)
	readyLogger := &longConnReadyLogger{
		inner:   c.logger,
		readyCh: make(chan struct{}, 1),
	}
	wsClient = larkws.NewClient(c.appID, c.appSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithAutoReconnect(true),
		larkws.WithLogger(readyLogger),
	)

	c.logger.Info("飞书长连接启动中", zap.String("app_id", c.appID))

	startErrCh := make(chan error, 1)
	go func() {
		if err := wsClient.Start(readyCtx); err != nil {
			select {
			case startErrCh <- err:
			default:
			}
			if readyCtx.Err() == nil {
				c.logger.Error("飞书长连接异常退出", zap.Error(err))
			}
		}
	}()

	select {
	case <-readyLogger.readyCh:
		readyCancel()
		c.mu.Lock()
		c.started = true
		c.cancel = cancel
		c.ctx = innerCtx
		c.mu.Unlock()
		c.markEventObserved(time.Now())
		c.startWatchdog(innerCtx)
		c.logger.Info("飞书长连接首次握手成功", zap.String("app_id", c.appID))
		return nil
	case err := <-startErrCh:
		readyCancel()
		cancel()
		return err
	case <-ctx.Done():
		readyCancel()
		cancel()
		return ctx.Err()
	case <-time.After(15 * time.Second):
		readyCancel()
		cancel()
		return fmt.Errorf("feishu longconn startup timeout")
	}
}

// Stop 停止长连接
func (c *LongConnClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.logger.Info("飞书长连接正在关闭")
	}
	c.cancel = nil
	c.ctx = nil
	c.started = false
	return nil
}

// getContext 返回长连接生命周期上下文，用于派生异步任务的上下文
func (c *LongConnClient) getContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

type longConnReadyLogger struct {
	inner   *zap.Logger
	readyCh chan struct{}
	once    sync.Once
}

func (l *longConnReadyLogger) Debug(context.Context, ...interface{}) {}

func (l *longConnReadyLogger) Info(_ context.Context, args ...interface{}) {
	if l.detectConnected(args...) {
		l.once.Do(func() {
			l.readyCh <- struct{}{}
		})
	}
}

func (l *longConnReadyLogger) Warn(context.Context, ...interface{}) {}

func (l *longConnReadyLogger) Error(context.Context, ...interface{}) {}

func (l *longConnReadyLogger) detectConnected(args ...interface{}) bool {
	for _, arg := range args {
		s, ok := arg.(string)
		if ok && strings.Contains(s, "connected to") {
			return true
		}
	}
	return false
}

// handleMessageReceive 处理 im.message.receive_v1 事件
//
// P0-#7 不变量：永远返回 nil；任何 panic 走顶层 recover + retry_queue 兜底。
// 即使长连接没有 webhook 那种"5xx → 飞书重试风暴"，业务 panic 不入队同样会让消息永久丢失。
func (c *LongConnClient) handleMessageReceive(_ context.Context, event *larkim.P2MessageReceiveV1) error {
	defer func() {
		if rec := recover(); rec != nil {
			c.logger.Error("飞书长连接 handler 顶层 panic recovered",
				zap.Any("panic", rec))
			c.enqueueRetry(channel.RetryItem{
				Platform: string(channel.PlatformFeishu),
				Reason:   channel.RetryReasonHandlerPanic,
				ErrorMsg: fmt.Sprintf("longconn handler panic: %v", rec),
			})
		}
	}()
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}

	msgEvent := event.Event
	msg := msgEvent.Message

	// 提取消息 ID
	messageID := ""
	if msg.MessageId != nil {
		messageID = *msg.MessageId
	}

	// 提取会话 ID
	chatID := ""
	if msg.ChatId != nil {
		chatID = *msg.ChatId
	}

	// 提取聊天类型
	chatType := channel.ChatGroup
	if msg.ChatType != nil && *msg.ChatType == "p2p" {
		chatType = channel.ChatDirect
	}

	// 群聊消息：只有 @机器人 时才处理
	if chatType == channel.ChatGroup {
		if !c.isBotMentioned(msg.Mentions) {
			return nil
		}
	}

	// 提取发送者 OpenID
	senderID := ""
	if msgEvent.Sender != nil && msgEvent.Sender.SenderId != nil && msgEvent.Sender.SenderId.OpenId != nil {
		senderID = *msgEvent.Sender.SenderId.OpenId
	}

	// 提取消息内容（支持所有消息类型）
	messageType := ""
	content := ""
	var attachments []channel.Attachment
	var refs []imctx.DocRef
	if msg.MessageType != nil && msg.Content != nil {
		messageType = *msg.MessageType
		parsed := ParseInboundMessage(messageType, *msg.Content)
		content = parsed.TextContent
		// text 类型需要额外解析 mention 占位符
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

	// 提取父消息 ID（回复/引用场景）。
	// 飞书 SDK 同时暴露 ParentId 和 RootId：
	//   - 直接回复: ParentId = 被回复消息
	//   - 话题/线程内回复: RootId = 线程根, ParentId = 线程内被回复消息
	//   - 用户引用某文档分享卡片：通常只填 ParentId，但少数线程嵌套场景仅填 RootId
	// 我们把 RootId 作为 ParentId 兜底，避免 resolver 拿不到父消息上下文（截图实测：
	// 用户引用 Frank 发的"Untitled" wiki 卡片但 resolver 静默走过 → ref 抽不到 → agent 脑补"需要 space_id"）。
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
	mentions, botMentioned := extractMentions(msg.Mentions, c.botOpenID)

	// 从事件头提取 tenantKey，填入 InboundMessage 用于多租户 session_id 构造。
	tenantKey := ""
	if event.EventV2Base != nil {
		tenantKey = event.EventV2Base.TenantKey()
	}
	if tenantKey != "" {
		c.setLastTenantKey(tenantKey)
	}
	c.markEventObserved(time.Now())

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

	c.logger.Info("收到飞书长连接消息",
		zap.String("message_id", messageID),
		zap.String("chat_id", chatID),
		// P0-#12: raw open_id 禁止直接落日志（监控聚合 / 三方 SaaS 会泄露用户身份）。
		zap.String("safe_sender_id", imctx.SafeSenderID(senderID)),
		zap.String("chat_type", string(chatType)),
		// 诊断观测：消息体 + 父/线程 + ref 抽取数。父消息为空就别指望 resolver 能拉到上下文；
		// refs_count=0 + parent_id 空 → agent 看不到任何文档线索（哪怕用户引用了 wiki 卡片）。
		zap.String("message_type", messageType),
		zap.Int("content_len", len(content)),
		zap.Int("refs_count", len(refs)),
		zap.Strings("refs_summary", formatRefsForLog(refs)),
		zap.String("parent_id", parentID),
		zap.String("root_id", rootID))

	// ack 表情（"已收到，处理中"）现由 feishu renderer 订阅 input_received 事件触发，
	// 不再在长连接层私下调 AddReaction——消除 webhook / longconn 不对称，统一入 harness 事件流。
	// 详见 openspec/changes/im-streaming-reply design.md D3。

	// 异步处理，避免阻塞事件分发器
	// 使用 innerCtx 的派生上下文，确保长连接关闭时能取消处理
	//
	// P0-#7：goroutine 内 panic 必须 recover + 入 retry_queue；router nil / HandleMessage err 都要落队。
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				c.logger.Error("飞书长连接异步处理 goroutine panic recovered",
					zap.String("message_id", messageID),
					zap.Any("panic", rec))
				c.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonHandlerPanic,
					fmt.Sprintf("longconn async goroutine panic: %v", rec)))
			}
		}()
		if c.router == nil {
			c.logger.Warn("longconn router 未配置，落 retry_queue 兜底",
				zap.String("message_id", messageID))
			c.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonRouterNil, "router not configured"))
			return
		}
		bgCtx, cancel := context.WithTimeout(context.WithoutCancel(c.getContext()), 5*time.Minute)
		defer cancel()
		if err := c.router.HandleMessage(bgCtx, inbound); err != nil {
			c.logger.Error("处理飞书长连接消息失败，落 retry_queue",
				zap.String("message_id", messageID),
				zap.Error(err))
			c.enqueueRetry(retryItemFromInbound(inbound, channel.RetryReasonHandlerError, err.Error()))
			// 处理失败时尝试通知用户
			c.router.NotifyError(bgCtx, inbound, err)
		}
	}()

	return nil
}

// enqueueRetry 与 webhook 端语义对齐：router 未注入或 retry_queue 缺失时降级为日志，永不阻断。
func (c *LongConnClient) enqueueRetry(item channel.RetryItem) {
	if c.router == nil {
		c.logger.Warn("longconn retry_queue 未注入（router 为 nil），仅记录日志",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.String("error", item.ErrorMsg))
		return
	}
	q := c.router.RetryQueue()
	if q == nil {
		c.logger.Warn("longconn retry_queue 未注入，仅记录日志",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.String("error", item.ErrorMsg))
		return
	}
	if err := q.Enqueue(item); err != nil {
		c.logger.Error("longconn retry_queue Enqueue 失败",
			zap.String("message_id", item.MessageID),
			zap.String("reason", string(item.Reason)),
			zap.Error(err))
	}
}

// isBotMentioned 检查 mentions 列表中是否包含机器人自身
func (c *LongConnClient) isBotMentioned(mentions []*larkim.MentionEvent) bool {
	if len(mentions) == 0 {
		return false
	}
	// 如果未能获取 botOpenID，降级为：有任意 mention 就处理（兜底策略）
	if c.botOpenID == "" {
		return true
	}
	for _, m := range mentions {
		if m == nil || m.Id == nil || m.Id.OpenId == nil {
			continue
		}
		if *m.Id.OpenId == c.botOpenID {
			return true
		}
	}
	return false
}

// resolveMentions 将飞书消息文本中的 @_user_N 占位符替换为真实姓名
// 例如 "@_user_1 你好" → "@AgentsClaw 你好"
func resolveMentions(text string, mentions []*larkim.MentionEvent) string {
	for _, m := range mentions {
		if m == nil || m.Key == nil || m.Name == nil {
			continue
		}
		text = strings.ReplaceAll(text, *m.Key, "@"+*m.Name)
	}
	return text
}
