package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBotAddedWelcomeSender_SendWelcome(t *testing.T) {
	client := &stubWelcomeMessageClient{}
	sender := NewBotAddedWelcomeSender(client, zap.NewNop())

	err := sender.SendWelcome(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		TenantKey: "tenant-1",
		ChatID:    "chat-1",
	})

	require.NoError(t, err)
	require.Len(t, client.calls, 1)
	require.Equal(t, "chat-1", client.calls[0].chatID)
	require.Equal(t, "interactive", client.calls[0].msgType)
	require.Contains(t, client.calls[0].content, "Agents Hive")
	require.Contains(t, client.calls[0].content, "/reset")
}

func TestBotAddedWelcomeSender_PropagatesSendError(t *testing.T) {
	client := &stubWelcomeMessageClient{err: errors.New("send failed")}
	sender := NewBotAddedWelcomeSender(client, zap.NewNop())

	err := sender.SendWelcome(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		TenantKey: "tenant-1",
		ChatID:    "chat-1",
	})

	require.EqualError(t, err, "send failed")
}

func TestBotAddedWelcomeSender_SendErrorEnqueuesRetry(t *testing.T) {
	client := &stubWelcomeMessageClient{err: errors.New("send failed")}
	queue := channel.NewMemoryRetryQueue(0, zap.NewNop())
	sender := NewBotAddedWelcomeSender(client, zap.NewNop()).WithRetryQueue(queue)

	err := sender.SendWelcome(context.Background(), LifecycleEvent{
		EventID:   "evt-2",
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})

	require.EqualError(t, err, "send failed")
	require.Equal(t, 1, queue.Len())
	item := queue.Snapshot()[0]
	require.Equal(t, channel.RetryReasonWelcomeSend, item.Reason)
	require.Equal(t, "tenant-2", item.TenantKey)
	require.Equal(t, "chat-2", item.ChatID)
	var event LifecycleEvent
	require.NoError(t, json.Unmarshal(item.Payload, &event))
	require.Equal(t, "evt-2", event.EventID)
}

func TestNewLifecycleHandlerWithDefaults(t *testing.T) {
	repo := &stubChatStateRepo{}
	client := &stubWelcomeMessageClient{}

	handler := NewLifecycleHandlerWithDefaults(repo, &stubSessionTerminator{}, nil, client, nil, zap.NewNop())

	require.NotNil(t, handler)
	require.NotNil(t, handler.welcome)
}

type stubWelcomeMessageClient struct {
	calls []struct {
		chatID  string
		msgType string
		content string
	}
	err error
}

func (s *stubWelcomeMessageClient) SendMessage(ctx context.Context, chatID, msgType, content string) error {
	s.calls = append(s.calls, struct {
		chatID  string
		msgType string
		content string
	}{
		chatID:  chatID,
		msgType: msgType,
		content: content,
	})
	return s.err
}
