package feishu

import (
	"context"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"go.uber.org/zap"
)

// MessageReceivedHandler 是飞书 P2MessageReceiveV1 业务回调签名。
// Phase 0 P0-#7 不变量：实现方必须保证返回 nil error，业务失败由内部落 retry_queue 表达。
//   - 返回非 nil 会让 SDK processError 写 5xx → 飞书后端无限重试 → 消息永久不消费。
type MessageReceivedHandler func(ctx context.Context, event *larkim.P2MessageReceiveV1) error

// DispatcherDeps 是 BuildEventDispatcher 的入参集合。
// 用 struct 而非位置参数，便于 webhook / longconn 扩字段时不破坏调用点。
type DispatcherDeps struct {
	VerificationToken string                 // 飞书后台「Verification Token」；webhook 验证 token；longconn 留空
	EncryptKey        string                 // 飞书后台「Encrypt Key」；webhook 解密；longconn 留空
	MessageReceived   MessageReceivedHandler // im.message.receive_v1 业务入口（必填）
	HITLBridge        *FeishuHITLBridge      // nil 表示不启用 card.action.trigger 通路
	LifecycleHandler  *LifecycleHandler      // Phase 2 机器人进群/退群；nil 表示暂不注册 bot_added/bot_deleted
	Logger            *zap.Logger            // 用于注册附属事件的告警日志
}

// BuildEventDispatcher 是 webhook 与 longconn 共享的事件分发器构造函数。
//
// 注册的事件集合（与 longconn 历史行为一致，避免 webhook / longconn 路径不对称）：
//   - im.message.receive_v1                            — 业务消息入口（必注册）
//   - im.chat.access_event.bot_p2p_chat_entered_v1     — 私聊进入（空实现，避免 SDK error log）
//   - im.chat.created_v1                               — 私聊创建（空实现）
//   - im.message.message_read_v1                       — 已读回执（空实现）
//   - im.message.reaction.created_v1 / deleted_v1      — 表情回执（空实现）
//   - card.action.trigger                              — HITL 卡片回调（HITLBridge 非 nil 时注册）
//
// 注：未注册的事件会触发 SDK [Error] handler not found 日志，但不会让飞书重试，
// 因此空实现"占位"是安全做法。
func BuildEventDispatcher(deps DispatcherDeps) *dispatcher.EventDispatcher {
	if deps.MessageReceived == nil {
		// MessageReceived 是必填项；nil 视为编程错误。fail-fast 优于运行时静默丢消息。
		panic("feishu.BuildEventDispatcher: MessageReceived handler is required")
	}
	if deps.Logger == nil {
		deps.Logger = zap.NewNop()
	}

	d := dispatcher.NewEventDispatcher(deps.VerificationToken, deps.EncryptKey)

	// 业务消息入口
	d.OnP2MessageReceiveV1(deps.MessageReceived)

	// "登记型"事件：仅吞掉，避免 SDK 报 [Error] handler not found 噪音
	d.OnP2ChatAccessEventBotP2pChatEnteredV1(func(_ context.Context, _ *larkim.P2ChatAccessEventBotP2pChatEnteredV1) error {
		return nil
	})
	d.OnP1P2PChatCreatedV1(func(_ context.Context, _ *larkim.P1P2PChatCreatedV1) error {
		return nil
	})
	d.OnP2MessageReadV1(func(_ context.Context, _ *larkim.P2MessageReadV1) error {
		return nil
	})
	d.OnP2MessageReactionCreatedV1(func(_ context.Context, _ *larkim.P2MessageReactionCreatedV1) error {
		return nil
	})
	d.OnP2MessageReactionDeletedV1(func(_ context.Context, _ *larkim.P2MessageReactionDeletedV1) error {
		return nil
	})

	// HITL 卡片回调（可选）
	if deps.HITLBridge != nil {
		d.OnP2CardActionTrigger(deps.HITLBridge.HandleCardActionTrigger)
		deps.Logger.Info("飞书 dispatcher 已注册 card.action.trigger 处理器")
	}
	if deps.LifecycleHandler != nil {
		d.OnP2ChatMemberBotAddedV1(func(ctx context.Context, event *larkim.P2ChatMemberBotAddedV1) error {
			return deps.LifecycleHandler.HandleBotAdded(ctx, lifecycleEventFromBotAdded(event))
		})
		d.OnP2ChatMemberBotDeletedV1(func(ctx context.Context, event *larkim.P2ChatMemberBotDeletedV1) error {
			return deps.LifecycleHandler.HandleBotRemoved(ctx, lifecycleEventFromBotDeleted(event))
		})
		deps.Logger.Info("飞书 dispatcher 已注册 bot 生命周期处理器")
	}

	return d
}
