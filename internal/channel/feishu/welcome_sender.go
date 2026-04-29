package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chef-guo/agents-hive/internal/channel"
	"go.uber.org/zap"
)

type welcomeMessageClient interface {
	SendMessage(ctx context.Context, chatID, msgType, content string) error
}

type BotAddedWelcomeSender struct {
	client     welcomeMessageClient
	retryQueue channel.RetryQueue
	logger     *zap.Logger
}

func NewBotAddedWelcomeSender(client welcomeMessageClient, logger *zap.Logger) *BotAddedWelcomeSender {
	if client == nil {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BotAddedWelcomeSender{
		client: client,
		logger: logger,
	}
}

func (s *BotAddedWelcomeSender) WithRetryQueue(retryQueue channel.RetryQueue) *BotAddedWelcomeSender {
	if s == nil {
		return nil
	}
	s.retryQueue = retryQueue
	return s
}

func (s *BotAddedWelcomeSender) SendWelcome(ctx context.Context, event LifecycleEvent) error {
	card := BuildMarkdownCard("已接入 Agents Hive。\n\n直接发送消息即可开始，也可以使用 `/reset` 重置当前会话。")
	if err := s.client.SendMessage(ctx, event.ChatID, "interactive", card); err != nil {
		s.logger.Warn("飞书 bot_added 欢迎消息发送失败",
			zap.String("tenant_key", event.TenantKey),
			zap.String("chat_id", event.ChatID),
			zap.String("event_id", event.EventID),
			zap.Error(err))
		s.enqueueRetry(event, err)
		return err
	}
	s.logger.Info("飞书 bot_added 欢迎消息发送成功",
		zap.String("tenant_key", event.TenantKey),
		zap.String("chat_id", event.ChatID),
		zap.String("event_id", event.EventID))
	return nil
}

func (s *BotAddedWelcomeSender) enqueueRetry(event LifecycleEvent, cause error) {
	if s == nil || s.retryQueue == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		s.logger.Warn("飞书欢迎消息重试载荷序列化失败",
			zap.String("tenant_key", event.TenantKey),
			zap.String("chat_id", event.ChatID),
			zap.String("event_id", event.EventID),
			zap.Error(err))
	}
	messageID := event.EventID
	if messageID == "" {
		messageID = fmt.Sprintf("welcome:%s:%s", event.TenantKey, event.ChatID)
	}
	if err := s.retryQueue.Enqueue(channel.RetryItem{
		MessageID: messageID,
		Platform:  string(channel.PlatformFeishu),
		TenantKey: event.TenantKey,
		ChatID:    event.ChatID,
		Reason:    channel.RetryReasonWelcomeSend,
		ErrorMsg:  truncateRetryError(cause),
		Payload:   payload,
	}); err != nil {
		s.logger.Error("飞书欢迎消息入 retry_queue 失败",
			zap.String("tenant_key", event.TenantKey),
			zap.String("chat_id", event.ChatID),
			zap.String("event_id", event.EventID),
			zap.Error(err))
	}
}

func NewWelcomeRetryHandler(sender WelcomeSender) func(context.Context, channel.RetryItem) error {
	if sender == nil {
		return nil
	}
	return func(ctx context.Context, item channel.RetryItem) error {
		event := LifecycleEvent{
			EventID:   item.MessageID,
			TenantKey: item.TenantKey,
			ChatID:    item.ChatID,
		}
		if len(item.Payload) > 0 {
			if err := json.Unmarshal(item.Payload, &event); err != nil {
				return err
			}
		}
		return sender.SendWelcome(ctx, event)
	}
}

func NewLifecycleHandlerWithDefaults(
	repo ChatStateRepo,
	terminator SessionTerminator,
	welcome WelcomeSender,
	client welcomeMessageClient,
	retryQueue channel.RetryQueue,
	logger *zap.Logger,
) *LifecycleHandler {
	if repo == nil {
		return nil
	}
	if welcome == nil {
		welcome = NewBotAddedWelcomeSender(client, logger).WithRetryQueue(retryQueue)
	}
	return NewLifecycleHandler(repo, terminator, welcome, logger)
}
