package feishu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLifecycleHandler_HandleBotRemoved_TerminatesOnlyOnRealTransition(t *testing.T) {
	repo := &stubChatStateRepo{
		markEvictedRecord: &ChatStateRecord{
			SessionID:        "sess-1",
			State:            ChatStateEvicted,
			SuppressOutbound: true,
		},
		markEvictedChanged: true,
	}
	terminator := &stubSessionTerminator{}
	h := NewLifecycleHandler(repo, terminator, nil, zap.NewNop())

	err := h.HandleBotRemoved(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		EventTime: 1700000001,
		TenantKey: "tenant-1",
		ChatID:    "chat-1",
	})

	require.NoError(t, err)
	require.Len(t, repo.markEvictedCalls, 1)
	require.Equal(t, "tenant-1", repo.markEvictedCalls[0].tenantKey)
	require.Equal(t, "chat-1", repo.markEvictedCalls[0].chatID)
	require.Equal(t, "evt-1", repo.markEvictedCalls[0].eventID)
	require.True(t, repo.markEvictedRecord.SuppressOutbound)
	require.Len(t, terminator.calls, 1)
	require.Equal(t, "sess-1", terminator.calls[0].sessionID)
	require.Equal(t, "bot_removed", terminator.calls[0].reason)
}

func TestLifecycleHandler_HandleBotRemoved_IdempotentAndMonotonicNoTerminate(t *testing.T) {
	tests := []struct {
		name    string
		record  *ChatStateRecord
		changed bool
	}{
		{
			name: "repo reports no state change",
			record: &ChatStateRecord{
				SessionID: "sess-1",
				State:     ChatStateEvicted,
			},
			changed: false,
		},
		{
			name: "state changed but no session bound",
			record: &ChatStateRecord{
				SessionID: "",
				State:     ChatStateEvicted,
			},
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubChatStateRepo{
				markEvictedRecord:  tt.record,
				markEvictedChanged: tt.changed,
			}
			terminator := &stubSessionTerminator{}
			h := NewLifecycleHandler(repo, terminator, nil, zap.NewNop())

			err := h.HandleBotRemoved(context.Background(), LifecycleEvent{
				EventID:   "evt-1",
				EventTime: 1700000001,
				TenantKey: "tenant-1",
				ChatID:    "chat-1",
			})

			require.NoError(t, err)
			require.Empty(t, terminator.calls)
		})
	}
}

func TestLifecycleHandler_HandleBotAdded_MarksActiveAndOptionalWelcome(t *testing.T) {
	repo := &stubChatStateRepo{
		markActiveRecord:  &ChatStateRecord{State: ChatStateActive},
		markActiveChanged: true,
	}
	welcome := &stubWelcomeSender{}
	h := NewLifecycleHandler(repo, &stubSessionTerminator{}, welcome, zap.NewNop())

	err := h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-2",
		EventTime: 1700000002,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})

	require.NoError(t, err)
	require.Len(t, repo.markActiveCalls, 1)
	require.Len(t, welcome.calls, 1)
	require.Equal(t, "tenant-2", welcome.calls[0].TenantKey)
	require.Equal(t, "chat-2", welcome.calls[0].ChatID)
	require.Empty(t, repo.setSessionIDCalls)
}

func TestLifecycleHandler_HandleBotAdded_DuplicateLifecycleSkipsWelcome(t *testing.T) {
	repo := &stubChatStateRepo{
		markActiveRecord:  &ChatStateRecord{State: ChatStateActive},
		markActiveChanged: false,
	}
	welcome := &stubWelcomeSender{}
	h := NewLifecycleHandler(repo, &stubSessionTerminator{}, welcome, zap.NewNop())

	err := h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-dup",
		EventTime: 1700000003,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})

	require.NoError(t, err)
	require.Len(t, repo.markActiveCalls, 1)
	require.Empty(t, welcome.calls)
}

func TestLifecycleHandler_EmitsLifecycleMetrics(t *testing.T) {
	repo := &stubChatStateRepo{
		markActiveRecord:   &ChatStateRecord{State: ChatStateActive},
		markActiveChanged:  true,
		markEvictedRecord:  &ChatStateRecord{SessionID: "sess-1", State: ChatStateEvicted},
		markEvictedChanged: true,
	}
	welcome := &stubWelcomeSender{}
	writer := &feishuMetricCaptureWriter{}
	h := NewLifecycleHandler(repo, &stubSessionTerminator{}, welcome, zap.NewNop()).
		WithMetricsWriter(writer)

	err := h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-added",
		EventTime: 1700000002,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})
	require.NoError(t, err)

	err = h.HandleBotRemoved(context.Background(), LifecycleEvent{
		EventID:   "evt-removed",
		EventTime: 1700000003,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})
	require.NoError(t, err)

	if metric := writer.find(MetricLifecycleEventCount); metric == nil {
		t.Fatalf("expected %s metric", MetricLifecycleEventCount)
	} else if got := metric.Labels["event_type"]; got != "bot_added" {
		t.Fatalf("metric event_type = %v, want bot_added", got)
	}

	if metric := writer.findWithLabel(MetricLifecycleEventCount, "event_type", "bot_removed"); metric == nil {
		t.Fatalf("expected %s metric for bot_removed", MetricLifecycleEventCount)
	}

	if metric := writer.find(MetricLifecycleWelcomeSent); metric == nil {
		t.Fatalf("expected %s metric", MetricLifecycleWelcomeSent)
	} else if got := metric.Labels["tenant_key_hash"]; got == "" {
		t.Fatal("expected tenant_key_hash label")
	}
}

func TestLifecycleHandler_HandleBotAdded_WithoutWelcomeHookIsNoop(t *testing.T) {
	repo := &stubChatStateRepo{
		markActiveRecord:  &ChatStateRecord{State: ChatStateActive},
		markActiveChanged: true,
	}
	h := NewLifecycleHandler(repo, &stubSessionTerminator{}, nil, zap.NewNop())

	err := h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-2",
		EventTime: 1700000002,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})

	require.NoError(t, err)
	require.Len(t, repo.markActiveCalls, 1)
}

func TestLifecycleHandler_RequiresTenantKey(t *testing.T) {
	h := NewLifecycleHandler(&stubChatStateRepo{}, &stubSessionTerminator{}, nil, zap.NewNop())

	err := h.HandleBotRemoved(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		EventTime: 1700000001,
		ChatID:    "chat-1",
	})

	require.ErrorIs(t, err, ErrTenantKeyRequired)
}

func TestLifecycleHandler_WithoutRepoFailsClosed(t *testing.T) {
	h := NewLifecycleHandler(nil, &stubSessionTerminator{}, nil, zap.NewNop())

	err := h.HandleBotRemoved(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		EventTime: 1700000001,
		TenantKey: "tenant-1",
		ChatID:    "chat-1",
	})
	require.ErrorIs(t, err, ErrChatStateRepoNotImplemented)

	err = h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-2",
		EventTime: 1700000002,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})
	require.ErrorIs(t, err, ErrChatStateRepoNotImplemented)
}

func TestLifecycleHandler_PropagatesRepoAndTerminatorErrors(t *testing.T) {
	repoErr := errors.New("repo failed")
	termErr := errors.New("terminate failed")

	repo := &stubChatStateRepo{markActiveErr: repoErr}
	h := NewLifecycleHandler(repo, &stubSessionTerminator{}, nil, zap.NewNop())
	err := h.HandleBotAdded(context.Background(), LifecycleEvent{
		EventID:   "evt-2",
		EventTime: 1700000002,
		TenantKey: "tenant-2",
		ChatID:    "chat-2",
	})
	require.ErrorIs(t, err, repoErr)

	repo = &stubChatStateRepo{
		markEvictedRecord:  &ChatStateRecord{SessionID: "sess-1"},
		markEvictedChanged: true,
	}
	terminator := &stubSessionTerminator{err: termErr}
	h = NewLifecycleHandler(repo, terminator, nil, zap.NewNop())
	err = h.HandleBotRemoved(context.Background(), LifecycleEvent{
		EventID:   "evt-1",
		EventTime: 1700000001,
		TenantKey: "tenant-1",
		ChatID:    "chat-1",
	})
	require.ErrorIs(t, err, termErr)
}

func TestLifecycleEventFromBotEvents(t *testing.T) {
	added := &larkim.P2ChatMemberBotAddedV1{
		EventV2Base: &larkevent.EventV2Base{
			Header: &larkevent.EventHeader{
				EventID:    "evt-added",
				CreateTime: "1700000003",
				TenantKey:  "tenant-3",
			},
		},
		Event: &larkim.P2ChatMemberBotAddedV1Data{
			ChatId: strPtr("chat-3"),
		},
	}
	gotAdded := lifecycleEventFromBotAdded(added)
	require.Equal(t, LifecycleEvent{
		EventID:   "evt-added",
		EventTime: 1700000003,
		TenantKey: "tenant-3",
		ChatID:    "chat-3",
	}, gotAdded)

	deleted := &larkim.P2ChatMemberBotDeletedV1{
		EventV2Base: &larkevent.EventV2Base{
			Header: &larkevent.EventHeader{
				EventID:    "evt-deleted",
				CreateTime: "1700000004",
				TenantKey:  "tenant-4",
			},
		},
		Event: &larkim.P2ChatMemberBotDeletedV1Data{
			ChatId: strPtr("chat-4"),
		},
	}
	gotDeleted := lifecycleEventFromBotDeleted(deleted)
	require.Equal(t, LifecycleEvent{
		EventID:   "evt-deleted",
		EventTime: 1700000004,
		TenantKey: "tenant-4",
		ChatID:    "chat-4",
	}, gotDeleted)
}

type stubChatStateRepo struct {
	getRecord         *ChatStateRecord
	getErr            error
	listActiveRecords []ChatStateRecord
	listActiveErr     error

	markEvictedRecord  *ChatStateRecord
	markEvictedChanged bool
	markEvictedErr     error
	markEvictedCalls   []lifecycleRepoCall

	markActiveRecord  *ChatStateRecord
	markActiveChanged bool
	markActiveErr     error
	markActiveCalls   []lifecycleRepoCall

	setSessionIDCalls      []string
	setMuteUntilCalled     bool
	lastMuteUntil          *time.Time
	setModelOverrideCalled bool
	lastModelOverride      string
	setAgentProfileCalled  bool
	lastAgentProfile       string
}

type lifecycleRepoCall struct {
	platform  string
	tenantKey string
	chatID    string
	eventID   string
	eventTime int64
	updatedBy string
}

func (s *stubChatStateRepo) Get(ctx context.Context, platform, tenantKey, chatID string) (*ChatStateRecord, error) {
	return s.getRecord, s.getErr
}

func (s *stubChatStateRepo) ListActive(ctx context.Context, platform, tenantKey string) ([]ChatStateRecord, error) {
	return s.listActiveRecords, s.listActiveErr
}

func (s *stubChatStateRepo) Upsert(ctx context.Context, record ChatStateRecord) error {
	return nil
}

func (s *stubChatStateRepo) MarkEvicted(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error) {
	s.markEvictedCalls = append(s.markEvictedCalls, lifecycleRepoCall{
		platform:  platform,
		tenantKey: tenantKey,
		chatID:    chatID,
		eventID:   eventID,
		eventTime: eventTime,
		updatedBy: updatedBy,
	})
	return s.markEvictedRecord, s.markEvictedChanged, s.markEvictedErr
}

func (s *stubChatStateRepo) MarkActive(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error) {
	s.markActiveCalls = append(s.markActiveCalls, lifecycleRepoCall{
		platform:  platform,
		tenantKey: tenantKey,
		chatID:    chatID,
		eventID:   eventID,
		eventTime: eventTime,
		updatedBy: updatedBy,
	})
	return s.markActiveRecord, s.markActiveChanged, s.markActiveErr
}

func (s *stubChatStateRepo) SetSessionID(ctx context.Context, platform, tenantKey, chatID, sessionID, updatedBy string) error {
	s.setSessionIDCalls = append(s.setSessionIDCalls, sessionID)
	return nil
}

func (s *stubChatStateRepo) SetMuteUntil(ctx context.Context, platform, tenantKey, chatID string, muteUntil *time.Time, updatedBy string) error {
	s.setMuteUntilCalled = true
	s.lastMuteUntil = muteUntil
	return nil
}

func (s *stubChatStateRepo) SetRolloutMode(ctx context.Context, platform, tenantKey, chatID string, mode GovernanceRolloutMode, updatedBy string) error {
	return nil
}

func (s *stubChatStateRepo) SetModelOverride(ctx context.Context, platform, tenantKey, chatID, modelOverride, updatedBy string) error {
	s.setModelOverrideCalled = true
	s.lastModelOverride = modelOverride
	return nil
}

type feishuMetricCaptureWriter struct {
	items []observability.Metric
}

func (w *feishuMetricCaptureWriter) Record(_ context.Context, metric observability.Metric) error {
	w.items = append(w.items, metric)
	return nil
}

func (w *feishuMetricCaptureWriter) find(name string) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name {
			return &w.items[i]
		}
	}
	return nil
}

func (w *feishuMetricCaptureWriter) findWithLabel(name, key string, value any) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name && w.items[i].Labels[key] == value {
			return &w.items[i]
		}
	}
	return nil
}

func (s *stubChatStateRepo) SetAgentProfile(ctx context.Context, platform, tenantKey, chatID, agentProfile, updatedBy string) error {
	s.setAgentProfileCalled = true
	s.lastAgentProfile = agentProfile
	return nil
}

type stubSessionTerminator struct {
	calls []terminateCall
	err   error
}

type terminateCall struct {
	sessionID string
	reason    string
}

func (s *stubSessionTerminator) TerminateSession(sessionID, reason string) error {
	s.calls = append(s.calls, terminateCall{sessionID: sessionID, reason: reason})
	return s.err
}

type stubWelcomeSender struct {
	calls []LifecycleEvent
	err   error
}

func (s *stubWelcomeSender) SendWelcome(ctx context.Context, event LifecycleEvent) error {
	s.calls = append(s.calls, event)
	return s.err
}

func strPtr(v string) *string {
	return &v
}
