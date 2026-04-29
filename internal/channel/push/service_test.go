package push

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"go.uber.org/zap"
)

func TestServicePush_SendsMarkdownViaRouterPlugin(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.Push(context.Background(), Request{
		Platform: channel.PlatformFeishu,
		ChatID:   "oc_test_chat",
		MsgType:  channel.MsgTypeMarkdown,
		Content:  "## hello",
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(plugin.sent))
	}
	if plugin.sent[0].MsgType != channel.MsgTypeMarkdown {
		t.Fatalf("msgType = %q, want markdown", plugin.sent[0].MsgType)
	}
}

func TestServicePush_IdempotencySuppressesDuplicate(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	req := Request{
		Platform:       channel.PlatformFeishu,
		ChatID:         "oc_test_chat",
		MsgType:        channel.MsgTypeText,
		Content:        "hello",
		IdempotencyKey: "dup-1",
	}
	if err := svc.Push(context.Background(), req); err != nil {
		t.Fatalf("first Push() error = %v", err)
	}
	if err := svc.Push(context.Background(), req); err != nil {
		t.Fatalf("second Push() error = %v", err)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(plugin.sent))
	}
}

// TestServicePush_OpenIDFallbacksToOpenID 验证 Phase 6 缺口 12 修复:
// push.Service 不再拼非法的 "p2p:ou_xxx" 前缀,而是直接传 OpenID(ou_xxx)
// 给下游。feishu/client.SendMessage 通过 inferReceiveIDType 看到 ou_ 前缀
// 自动切 receive_id_type=open_id,飞书 IM API 接受。
//
// 旧期望 "p2p:ou_user_1" 是缺口 12 — 那个字符串既不是 chat_id 也不是 open_id
// 格式,生产 SDK 会拒。
func TestServicePush_OpenIDFallbacksToOpenID(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.Push(context.Background(), Request{
		Platform: channel.PlatformFeishu,
		OpenID:   "ou_user_1",
		MsgType:  channel.MsgTypeText,
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if plugin.sent[0].ChatID != "ou_user_1" {
		t.Fatalf("chatID = %q, want ou_user_1 (no p2p: prefix)", plugin.sent[0].ChatID)
	}
}

func TestServicePush_SendFailureEnqueuesRetry(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	queue := channel.NewMemoryRetryQueue(0, zap.NewNop())
	router.SetRetryQueue(queue)
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu, err: errors.New("send failed")}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.Push(context.Background(), Request{
		Platform:       channel.PlatformFeishu,
		ChatID:         "oc_retry_chat",
		MsgType:        channel.MsgTypeText,
		Content:        "hello",
		IdempotencyKey: "push-1",
	})
	if err == nil {
		t.Fatal("expected Push() to return error")
	}
	if queue.Len() != 1 {
		t.Fatalf("retry queue len = %d, want 1", queue.Len())
	}
	item := queue.Snapshot()[0]
	if item.Reason != channel.RetryReasonPushSend {
		t.Fatalf("reason = %q, want %q", item.Reason, channel.RetryReasonPushSend)
	}
	if item.ChatID != "oc_retry_chat" {
		t.Fatalf("chatID = %q, want oc_retry_chat", item.ChatID)
	}
}

func TestServicePush_RendersBuiltInTemplateBeforeSend(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.Push(context.Background(), Request{
		Platform: channel.PlatformFeishu,
		ChatID:   "oc_tpl_chat",
		Template: "task_done",
		Vars: map[string]any{
			"title":   "日报生成完成",
			"summary": "请查收最新结果",
		},
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(plugin.sent))
	}
	if plugin.sent[0].MsgType != channel.MsgTypeMarkdown {
		t.Fatalf("msgType = %q, want markdown", plugin.sent[0].MsgType)
	}
	if plugin.sent[0].Content != "## 日报生成完成\n请查收最新结果" {
		t.Fatalf("content = %q, want rendered markdown", plugin.sent[0].Content)
	}
}

func TestParseScheduledPrompt_MapsReservedFieldsAndVars(t *testing.T) {
	req, matched, err := ParseScheduledPrompt("scheduled_push:task_done:chat_id=oc_chat_1:msg_type=markdown:idempotency_key=job-1:title=日报生成完成:summary=请查收")
	if err != nil {
		t.Fatalf("ParseScheduledPrompt() error = %v", err)
	}
	if !matched {
		t.Fatal("ParseScheduledPrompt() matched = false, want true")
	}
	if req.Platform != channel.PlatformFeishu {
		t.Fatalf("platform = %q, want feishu", req.Platform)
	}
	if req.ChatID != "oc_chat_1" {
		t.Fatalf("chatID = %q, want oc_chat_1", req.ChatID)
	}
	if req.MsgType != channel.MsgTypeMarkdown {
		t.Fatalf("msgType = %q, want markdown", req.MsgType)
	}
	if req.Template != "task_done" {
		t.Fatalf("template = %q, want task_done", req.Template)
	}
	if req.IdempotencyKey != "job-1" {
		t.Fatalf("idempotencyKey = %q, want job-1", req.IdempotencyKey)
	}
	if got := req.Vars["title"]; got != "日报生成完成" {
		t.Fatalf("vars[title] = %v, want 日报生成完成", got)
	}
	if got := req.Vars["summary"]; got != "请查收" {
		t.Fatalf("vars[summary] = %v, want 请查收", got)
	}
}

func TestServiceDispatchScheduledPrompt_RendersAndSends(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.DispatchScheduledPrompt(context.Background(), "scheduled_push:task_done:chat_id=oc_sched_1:title=日报生成完成:summary=请查收")
	if err != nil {
		t.Fatalf("DispatchScheduledPrompt() error = %v", err)
	}
	if len(plugin.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(plugin.sent))
	}
	if plugin.sent[0].ChatID != "oc_sched_1" {
		t.Fatalf("chatID = %q, want oc_sched_1", plugin.sent[0].ChatID)
	}
	if plugin.sent[0].Content != "## 日报生成完成\n请查收" {
		t.Fatalf("content = %q, want rendered markdown", plugin.sent[0].Content)
	}
}

func TestServicePush_TemplateRetryPayloadOmitsRenderedContent(t *testing.T) {
	router := channel.NewRouter(nil, zap.NewNop())
	queue := channel.NewMemoryRetryQueue(0, zap.NewNop())
	router.SetRetryQueue(queue)
	plugin := &stubPushPlugin{platform: channel.PlatformFeishu, err: errors.New("send failed")}
	router.RegisterPlugin(plugin)

	svc := NewService(router, Config{Enabled: true}, zap.NewNop())
	err := svc.Push(context.Background(), Request{
		Platform: channel.PlatformFeishu,
		ChatID:   "oc_retry_tpl",
		Template: "task_done",
		Vars: map[string]any{
			"title":   "日报生成完成",
			"summary": "请查收",
		},
	})
	if err == nil {
		t.Fatal("expected Push() to return error")
	}

	var payload Request
	if unmarshalErr := json.Unmarshal(queue.Snapshot()[0].Payload, &payload); unmarshalErr != nil {
		t.Fatalf("retry payload unmarshal error = %v", unmarshalErr)
	}
	if payload.Template != "task_done" {
		t.Fatalf("payload.Template = %q, want task_done", payload.Template)
	}
	if payload.Content != "" {
		t.Fatalf("payload.Content = %q, want empty for template retry payload", payload.Content)
	}
}

func TestServiceReloadFromConfig_UpdatesRateLimitAndIdempotencyTTL(t *testing.T) {
	svc := NewService(nil, Config{
		Enabled:          true,
		PerChatPerMinute: 10,
		IdempotencyTTL:   5 * time.Minute,
	}, zap.NewNop())

	if err := svc.ReloadFromConfig(config.FeishuConfig{
		Push: config.FeishuPushConfig{
			Enabled:           true,
			PerChatPerMinute:  3,
			IdempotencyTTLSec: 42,
		},
	}); err != nil {
		t.Fatalf("ReloadFromConfig() error = %v", err)
	}

	if svc.perChatLimit != 3 {
		t.Fatalf("perChatLimit = %d, want 3", svc.perChatLimit)
	}
	if svc.idempotencyTTL != 42*time.Second {
		t.Fatalf("idempotencyTTL = %s, want 42s", svc.idempotencyTTL)
	}
}

func TestServiceDispatchScheduledPrompt_NonScheduledPromptReturnsMatchedError(t *testing.T) {
	svc := NewService(nil, Config{Enabled: true}, zap.NewNop())
	err := svc.DispatchScheduledPrompt(context.Background(), "plain prompt")
	if err == nil {
		t.Fatal("expected DispatchScheduledPrompt() to fail for non-scheduled prompt")
	}
	if err.Error() != "scheduled push prompt not matched" {
		t.Fatalf("error = %v, want not matched", err)
	}
}

type stubPushPlugin struct {
	platform channel.Platform
	sent     []channel.OutboundMessage
	err      error
}

func (s *stubPushPlugin) Platform() channel.Platform { return s.platform }
func (s *stubPushPlugin) Send(_ context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return s.err
}
func (s *stubPushPlugin) WebhookHandler() http.HandlerFunc { return nil }
func (s *stubPushPlugin) Verify(_ *http.Request) bool      { return true }
