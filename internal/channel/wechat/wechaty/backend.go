package wechaty

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	pb "github.com/chef-guo/agents-hive/internal/channel/wechat/wechaty/proto"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// Backend go-wechaty gRPC 桥接后端
// 通过 gRPC 连接 Wechaty Puppet Service，使用 Event() 服务端流式 RPC 接收消息
type Backend struct {
	cfg      config.WechatyInstanceConfig
	conn     *grpc.ClientConn
	client   pb.PuppetClient
	handler  wechat.MessageHandler
	logger   *zap.Logger
	loggedIn atomic.Bool
	cancel   context.CancelFunc
	mu       sync.Mutex
}

// New 创建 Wechaty 后端
func New(cfg config.WechatyInstanceConfig, logger *zap.Logger) *Backend {
	return &Backend{
		cfg:    cfg,
		logger: logger,
	}
}

// Name 返回协议名称
func (b *Backend) Name() string {
	return "wechaty"
}

// Start 连接 Wechaty gRPC gateway 并启动事件流
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	endpoint := b.cfg.Endpoint
	if endpoint == "" {
		endpoint = "localhost:8788"
	}

	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return errs.Wrap(errs.CodeWeChatLoginFailed, "连接 Wechaty gateway 失败", err)
	}

	client := pb.NewPuppetClient(conn)

	// 调用 Start RPC 启动 Puppet
	startCtx, startCancel := context.WithTimeout(ctx, 30*time.Second)
	defer startCancel()
	if _, err := client.Start(startCtx, &pb.StartRequest{}); err != nil {
		conn.Close()
		return errs.Wrap(errs.CodeWeChatLoginFailed, "启动 Wechaty Puppet 失败", err)
	}

	b.conn = conn
	b.client = client

	childCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel

	// 启动事件监听
	go b.eventLoop(childCtx)

	b.logger.Info("Wechaty gRPC 已连接", zap.String("endpoint", endpoint))
	return nil
}

// Stop 断开 gRPC 连接
func (b *Backend) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.loggedIn.Store(false)
	if b.cancel != nil {
		b.cancel()
	}

	// 调用 Stop RPC
	if b.client != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if _, err := b.client.Stop(stopCtx, &pb.StopRequest{}); err != nil {
			b.logger.Warn("停止 Wechaty Puppet 失败", zap.Error(err))
		}
	}

	if b.conn != nil {
		b.conn.Close()
		b.conn = nil
		b.client = nil
	}
	return nil
}

// SendText 通过 Wechaty gRPC 发送文本消息
func (b *Backend) SendText(ctx context.Context, chatID, content string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "Wechaty 未连接")
	}

	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return errs.New(errs.CodeWeChatProtocolError, "Wechaty gRPC 客户端为空")
	}

	if _, err := client.MessageSendText(ctx, &pb.MessageSendTextRequest{
		ConversationId: chatID,
		Text:           content,
	}); err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "Wechaty 发送消息失败", err)
	}

	return nil
}

// SetMessageHandler 设置消息回调
func (b *Backend) SetMessageHandler(handler wechat.MessageHandler) {
	b.handler = handler
}

// IsLoggedIn 返回登录状态
func (b *Backend) IsLoggedIn() bool {
	return b.loggedIn.Load()
}

// eventPayload Event RPC payload 中 JSON 编码的事件数据
type eventPayload struct {
	ContactID string `json:"contactId,omitempty"` // 登录/登出事件的用户 ID
	MessageID string `json:"messageId,omitempty"` // 消息事件的消息 ID
}

// eventLoop 监听 Wechaty gRPC 事件流
func (b *Backend) eventLoop(ctx context.Context) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return
	}

	stream, err := client.Event(ctx, &pb.EventRequest{})
	if err != nil {
		b.logger.Error("创建 Wechaty 事件流失败", zap.Error(err))
		return
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil || err == io.EOF {
				b.logger.Info("Wechaty 事件流结束")
				return
			}
			b.logger.Error("Wechaty 事件流接收失败", zap.Error(err))
			b.loggedIn.Store(false)
			return
		}

		switch event.Type {
		case pb.EventType_EVENT_TYPE_LOGIN:
			b.loggedIn.Store(true)
			b.logger.Info("Wechaty 登录成功", zap.String("payload", event.Payload))

		case pb.EventType_EVENT_TYPE_LOGOUT:
			b.loggedIn.Store(false)
			b.logger.Info("Wechaty 已登出", zap.String("payload", event.Payload))

		case pb.EventType_EVENT_TYPE_MESSAGE:
			b.handleMessageEvent(ctx, event.Payload)

		case pb.EventType_EVENT_TYPE_HEARTBEAT:
			// 心跳，忽略

		case pb.EventType_EVENT_TYPE_ERROR:
			b.logger.Warn("Wechaty 事件错误", zap.String("payload", event.Payload))

		case pb.EventType_EVENT_TYPE_SCAN:
			b.logger.Info("Wechaty 扫码事件", zap.String("payload", event.Payload))
		}
	}
}

// handleMessageEvent 处理消息事件
func (b *Backend) handleMessageEvent(ctx context.Context, payload string) {
	if b.handler == nil {
		return
	}

	var ep eventPayload
	if err := json.Unmarshal([]byte(payload), &ep); err != nil {
		b.logger.Warn("解析消息事件失败", zap.Error(err))
		return
	}

	if ep.MessageID == "" {
		return
	}

	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return
	}

	// 获取消息详情
	msgCtx, msgCancel := context.WithTimeout(ctx, 10*time.Second)
	defer msgCancel()

	msgResp, err := client.MessagePayload(msgCtx, &pb.MessagePayloadRequest{
		Id: ep.MessageID,
	})
	if err != nil {
		b.logger.Warn("获取 Wechaty 消息详情失败",
			zap.String("msg_id", ep.MessageID),
			zap.Error(err))
		return
	}

	// 仅处理文本消息
	if msgResp.Type != pb.MessageType_MESSAGE_TYPE_TEXT {
		return
	}

	msg := wechat.IncomingMessage{
		MsgID:     msgResp.Id,
		MsgType:   wechat.MsgText,
		FromUser:  msgResp.ContactId,
		Content:   msgResp.Text,
		Timestamp: time.Unix(int64(msgResp.Timestamp), 0),
	}

	// 群聊消息
	if msgResp.RoomId != "" {
		msg.FromGroup = msgResp.RoomId
	}

	// 尝试获取发送者昵称
	if msgResp.ContactId != "" {
		contactCtx, contactCancel := context.WithTimeout(ctx, 5*time.Second)
		defer contactCancel()
		contactResp, err := client.ContactPayload(contactCtx, &pb.ContactPayloadRequest{
			Id: msgResp.ContactId,
		})
		if err == nil {
			msg.SenderName = contactResp.Name
		}
	}

	b.handler(msg)
}
