package feishu

import (
	"context"
	"strconv"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/observability"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"go.uber.org/zap"
)

const (
	lifecycleUpdatedBy    = "feishu.lifecycle"
	lifecyclePlatform     = "feishu"
	lifecycleRemoveReason = "bot_removed"
)

type LifecycleEvent struct {
	EventID   string
	EventTime int64
	TenantKey string
	ChatID    string
}

type SessionTerminator interface {
	TerminateSession(sessionID, reason string) error
}

type WelcomeSender interface {
	SendWelcome(ctx context.Context, event LifecycleEvent) error
}

type LifecycleHandler struct {
	repo          ChatStateRepo
	terminator    SessionTerminator
	welcome       WelcomeSender
	logger        *zap.Logger
	metricsWriter observability.MetricsWriter
}

func NewLifecycleHandler(repo ChatStateRepo, terminator SessionTerminator, welcome WelcomeSender, logger *zap.Logger) *LifecycleHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LifecycleHandler{
		repo:       repo,
		terminator: terminator,
		welcome:    welcome,
		logger:     logger,
	}
}

func (h *LifecycleHandler) WithMetricsWriter(w observability.MetricsWriter) *LifecycleHandler {
	if h == nil {
		return nil
	}
	h.metricsWriter = w
	return h
}

func (h *LifecycleHandler) HandleBotRemoved(ctx context.Context, event LifecycleEvent) error {
	if err := validateTenantKey(event.TenantKey); err != nil {
		return err
	}
	if h.repo == nil {
		return ErrChatStateRepoNotImplemented
	}
	record, changed, err := h.repo.MarkEvicted(ctx, lifecyclePlatform, event.TenantKey, event.ChatID, event.EventID, event.EventTime, lifecycleUpdatedBy)
	if err != nil {
		return err
	}
	h.emitLifecycleMetric(MetricLifecycleEventCount, event, map[string]any{
		"event_type": "bot_removed",
		"changed":    changed,
	})
	if record != nil {
		record.SuppressOutbound = true
	}
	if !changed || record == nil || record.SessionID == "" || h.terminator == nil {
		return nil
	}
	return h.terminator.TerminateSession(record.SessionID, lifecycleRemoveReason)
}

func (h *LifecycleHandler) HandleBotAdded(ctx context.Context, event LifecycleEvent) error {
	if err := validateTenantKey(event.TenantKey); err != nil {
		return err
	}
	if h.repo == nil {
		return ErrChatStateRepoNotImplemented
	}
	record, changed, err := h.repo.MarkActive(ctx, lifecyclePlatform, event.TenantKey, event.ChatID, event.EventID, event.EventTime, lifecycleUpdatedBy)
	if err != nil {
		return err
	}
	h.emitLifecycleMetric(MetricLifecycleEventCount, event, map[string]any{
		"event_type": "bot_added",
		"changed":    changed,
	})
	if !changed || record == nil || h.welcome == nil {
		return nil
	}
	h.logger.Info("飞书机器人入群生命周期已生效",
		zap.String("tenant_key", event.TenantKey),
		zap.String("chat_id", event.ChatID),
		zap.String("event_id", event.EventID))
	h.emitLifecycleMetric(MetricLifecycleWelcomeSent, event, map[string]any{
		"event_type": "bot_added",
	})
	return h.welcome.SendWelcome(ctx, event)
}

func (h *LifecycleHandler) emitLifecycleMetric(name string, event LifecycleEvent, extra map[string]any) {
	if h == nil || h.metricsWriter == nil {
		return
	}
	labels := map[string]any{
		"tenant_key_hash": channel.TenantKeyHashLabel(event.TenantKey),
	}
	for k, v := range extra {
		labels[k] = v
	}
	_ = h.metricsWriter.Record(context.Background(), observability.Metric{
		Name:   name,
		Value:  1,
		Labels: labels,
		Ts:     time.Now(),
	})
}

func lifecycleEventFromBotAdded(event *larkim.P2ChatMemberBotAddedV1) LifecycleEvent {
	if event == nil {
		return LifecycleEvent{}
	}
	return LifecycleEvent{
		EventID:   lifecycleEventID(event.EventV2Base),
		EventTime: lifecycleEventTime(event.EventV2Base),
		TenantKey: lifecycleTenantKey(event.EventV2Base),
		ChatID:    lifecycleChatIDFromAdded(event.Event),
	}
}

func lifecycleEventFromBotDeleted(event *larkim.P2ChatMemberBotDeletedV1) LifecycleEvent {
	if event == nil {
		return LifecycleEvent{}
	}
	return LifecycleEvent{
		EventID:   lifecycleEventID(event.EventV2Base),
		EventTime: lifecycleEventTime(event.EventV2Base),
		TenantKey: lifecycleTenantKey(event.EventV2Base),
		ChatID:    lifecycleChatIDFromDeleted(event.Event),
	}
}

func lifecycleEventID(base *larkevent.EventV2Base) string {
	if base == nil || base.Header == nil {
		return ""
	}
	return base.Header.EventID
}

func lifecycleEventTime(base *larkevent.EventV2Base) int64 {
	if base == nil || base.Header == nil {
		return 0
	}
	sec, err := strconv.ParseInt(base.Header.CreateTime, 10, 64)
	if err != nil {
		return 0
	}
	return sec
}

func lifecycleTenantKey(base *larkevent.EventV2Base) string {
	if base == nil {
		return ""
	}
	return base.TenantKey()
}

func lifecycleChatIDFromAdded(data *larkim.P2ChatMemberBotAddedV1Data) string {
	if data == nil {
		return ""
	}
	return strDeref(data.ChatId)
}

func lifecycleChatIDFromDeleted(data *larkim.P2ChatMemberBotDeletedV1Data) string {
	if data == nil {
		return ""
	}
	return strDeref(data.ChatId)
}
