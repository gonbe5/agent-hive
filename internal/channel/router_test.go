package channel

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// mockProcessor 实现 MessageProcessor 接口用于测试（线程安全）
type mockProcessor struct {
	mu            sync.Mutex
	lastSessionID string
	lastInput     string
	response      master.TaskResponse
	err           error
}

type imCaptureProcessor struct {
	mu                sync.Mutex
	lastSessionID     string
	lastInput         string
	lastMessageID     string
	lastModelOverride string
	lastIMContext     *imctx.IMMessageContext
	response          master.TaskResponse
}

func (p *imCaptureProcessor) ProcessMessage(_ context.Context, sessionID string, input string) (master.TaskResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSessionID = sessionID
	p.lastInput = input
	return p.response, nil
}

func (p *imCaptureProcessor) ProcessMessageFromIM(_ context.Context, sessionID, input, channelMessageID, modelOverride string, _ bool, imCtx *imctx.IMMessageContext) (master.TaskResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSessionID = sessionID
	p.lastInput = input
	p.lastMessageID = channelMessageID
	p.lastModelOverride = modelOverride
	p.lastIMContext = imCtx
	return p.response, nil
}

func (p *imCaptureProcessor) snapshot() (string, string, string, string, *imctx.IMMessageContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSessionID, p.lastInput, p.lastMessageID, p.lastModelOverride, p.lastIMContext
}

type staticResolver struct {
	out *imctx.IMMessageContext
	err error
}

func (r *staticResolver) Resolve(_ context.Context, _ *InboundMessage) (*imctx.IMMessageContext, error) {
	return r.out, r.err
}

type metricCaptureWriter struct {
	mu      sync.Mutex
	metrics []observability.Metric
}

func (w *metricCaptureWriter) Record(_ context.Context, metric observability.Metric) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.metrics = append(w.metrics, metric)
	return nil
}

func (w *metricCaptureWriter) waitMetric(t *testing.T, name string) observability.Metric {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		for _, metric := range w.metrics {
			if metric.Name == name {
				w.mu.Unlock()
				return metric
			}
		}
		w.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("metric %q not found within timeout", name)
	return observability.Metric{}
}

func (m *mockProcessor) ProcessMessage(_ context.Context, sessionID string, input string) (master.TaskResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastSessionID = sessionID
	m.lastInput = input
	return m.response, m.err
}

func (m *mockProcessor) getLastInput() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastInput
}

func (m *mockProcessor) getLastSessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSessionID
}

// mockPlugin 实现 ChannelPlugin 接口用于测试（线程安全）
type mockPlugin struct {
	mu       sync.Mutex
	platform Platform
	lastMsg  OutboundMessage
}

func (m *mockPlugin) Platform() Platform { return m.platform }
func (m *mockPlugin) Send(_ context.Context, msg OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastMsg = msg
	return nil
}
func (m *mockPlugin) getLastMsg() OutboundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastMsg
}
func (m *mockPlugin) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {}
}
func (m *mockPlugin) Verify(_ *http.Request) bool { return true }

func TestRouterBindAndLookup(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{}
	router := NewRouter(proc, logger)

	// 绑定
	router.Bind(Binding{
		Platform:  PlatformDingTalk,
		ChatID:    "chat-001",
		SessionID: "session-abc",
	})

	// 查找
	sid := router.LookupSession(PlatformDingTalk, "chat-001")
	assert.Equal(t, "session-abc", sid)

	// 未绑定的返回空
	sid = router.LookupSession(PlatformFeishu, "chat-001")
	assert.Equal(t, "", sid)

	// 解绑
	router.Unbind(PlatformDingTalk, "chat-001")
	sid = router.LookupSession(PlatformDingTalk, "chat-001")
	assert.Equal(t, "", sid)
}

func TestRouterRegisterPlugin(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{}
	router := NewRouter(proc, logger)

	_, ok := router.GetPlugin(PlatformDingTalk)
	assert.False(t, ok)

	router.RegisterPlugin(&mockPlugin{platform: PlatformDingTalk})
	_, ok = router.GetPlugin(PlatformDingTalk)
	assert.True(t, ok)
}

func TestRouterHandleMessage(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{
		response: master.TaskResponse{
			Content:   "你好",
			Completed: true,
		},
	}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformDingTalk}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformDingTalk,
		ChatID:    "chat-001",
		SessionID: "session-abc",
	})

	// SenderID 为空，不走 debounce，直接同步处理
	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform:   PlatformDingTalk,
		TenantKey:  "tenant-a",
		ChatID:     "chat-001",
		SenderName: "test-user",
		Content:    "hello",
	})
	assert.NoError(t, err)
	assert.Equal(t, "session-abc", proc.getLastSessionID())
	assert.Equal(t, "hello", proc.getLastInput())
	assert.Equal(t, "你好", plugin.getLastMsg().Content)
	assert.Equal(t, "tenant-a", plugin.getLastMsg().TenantKey)
}

type mockInboundControllerPlugin struct {
	mockPlugin
	result InboundControlResult
	err    error
}

func (m *mockInboundControllerPlugin) ControlInbound(ctx context.Context, msg InboundMessage, currentSessionID string) (InboundControlResult, error) {
	return m.result, m.err
}

func TestRouterInboundController_ResetRebindsSession(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{}
	router := NewRouter(proc, logger)

	plugin := &mockInboundControllerPlugin{
		mockPlugin: mockPlugin{platform: PlatformFeishu},
		result: InboundControlResult{
			Handled:           true,
			Response:          "会话已重置: new-session",
			SessionIDOverride: "new-session",
		},
	}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "chat-001",
		SessionID: "old-session",
	})

	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform:   PlatformFeishu,
		TenantKey:  "tenant-a",
		ChatID:     "chat-001",
		SenderName: "test-user",
		Content:    "/reset",
	})
	assert.NoError(t, err)
	assert.Equal(t, "new-session", router.LookupSession(PlatformFeishu, "chat-001"))
	assert.Equal(t, "会话已重置: new-session", plugin.getLastMsg().Content)
}

func TestRouterInboundController_DropSkipsProcessingAndReply(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{
		response: master.TaskResponse{
			Content:   "should not be sent",
			Completed: true,
		},
	}
	router := NewRouter(proc, logger)

	plugin := &mockInboundControllerPlugin{
		mockPlugin: mockPlugin{platform: PlatformFeishu},
		result: InboundControlResult{
			Drop: true,
		},
	}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "chat-evicted",
		SessionID: "sess-evicted",
	})

	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform:   PlatformFeishu,
		TenantKey:  "tenant-a",
		ChatID:     "chat-evicted",
		SenderName: "test-user",
		Content:    "hello after bot removed",
	})

	assert.NoError(t, err)
	assert.Equal(t, "", proc.getLastSessionID())
	assert.Equal(t, "", proc.getLastInput())
	assert.Equal(t, OutboundMessage{}, plugin.getLastMsg())
}

func TestRouterInboundController_ModelOverridePassesToIMProcessor(t *testing.T) {
	logger := zap.NewNop()
	proc := &imCaptureProcessor{
		response: master.TaskResponse{Content: "ok", Completed: true},
	}
	router := NewRouter(proc, logger)

	plugin := &mockInboundControllerPlugin{
		mockPlugin: mockPlugin{platform: PlatformFeishu},
		result: InboundControlResult{
			ModelOverride: "gpt-5.2",
		},
	}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "chat-model",
		SessionID: "sess-model",
	})

	err := router.HandleMessage(context.Background(), InboundMessage{
		MessageID:  "msg-model-1",
		Platform:   PlatformFeishu,
		TenantKey:  "tenant-a",
		ChatID:     "chat-model",
		SenderName: "test-user",
		Content:    "hello",
	})
	assert.NoError(t, err)

	sessionID, input, messageID, modelOverride, _ := proc.snapshot()
	assert.Equal(t, "sess-model", sessionID)
	assert.Equal(t, "hello", input)
	assert.Equal(t, "msg-model-1", messageID)
	assert.Equal(t, "gpt-5.2", modelOverride)
}

func TestRouterHandleMessage_Debounce(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{
		response: master.TaskResponse{
			Content:   "ok",
			Completed: true,
		},
	}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformDingTalk}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformDingTalk,
		ChatID:    "chat-001",
		SessionID: "session-abc",
	})

	// 连续发送两条来自同一发送者的消息（SenderID 非空）
	msg1 := InboundMessage{
		MessageID:  "msg-1",
		Platform:   PlatformDingTalk,
		ChatID:     "chat-001",
		SenderID:   "user-001",
		SenderName: "Alice",
		Content:    "hello",
	}
	msg2 := InboundMessage{
		MessageID:  "msg-2",
		Platform:   PlatformDingTalk,
		ChatID:     "chat-001",
		SenderID:   "user-001",
		SenderName: "Alice",
		Content:    "world",
	}

	// HandleMessage 立即返回（消息被缓冲）
	assert.NoError(t, router.HandleMessage(context.Background(), msg1))
	assert.NoError(t, router.HandleMessage(context.Background(), msg2))

	// 立即检查：消息还未 flush
	assert.Equal(t, "", proc.getLastInput(), "消息应在 debounce 窗口期内被缓冲")

	// 等待 debounce 窗口到期
	time.Sleep(2500 * time.Millisecond)

	// 窗口到期后，两条消息应被合并为一条
	assert.Equal(t, "hello\nworld", proc.getLastInput())
	assert.Equal(t, "msg-2", plugin.getLastMsg().ReplyTo) // 保留最后一条的 MessageID

	// 清理
	router.Stop()
}

func TestRouterHandleMessage_NoDebounceBypassesBatchMerge(t *testing.T) {
	logger := zap.NewNop()
	proc := &mockProcessor{
		response: master.TaskResponse{
			Content:   "ok",
			Completed: true,
		},
	}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "chat-001",
		SessionID: "session-abc",
	})

	msg1 := InboundMessage{
		MessageID:  "msg-1",
		Platform:   PlatformFeishu,
		ChatID:     "chat-001",
		SenderID:   "user-001",
		SenderName: "Alice",
		Content:    "hello",
		NoDebounce: true,
	}
	msg2 := InboundMessage{
		MessageID:  "msg-2",
		Platform:   PlatformFeishu,
		ChatID:     "chat-001",
		SenderID:   "user-001",
		SenderName: "Alice",
		Content:    "world",
		NoDebounce: true,
	}

	assert.NoError(t, router.HandleMessage(context.Background(), msg1))
	assert.Equal(t, "hello", proc.getLastInput())

	assert.NoError(t, router.HandleMessage(context.Background(), msg2))
	assert.Equal(t, "world", proc.getLastInput())
	assert.Equal(t, "msg-2", plugin.getLastMsg().ReplyTo)

	router.Stop()
}

// ctxCaptureProcessor 捕获 ProcessMessage 调用时的 context，用于验证 enrichCtx 注入
type ctxCaptureProcessor struct {
	mu       sync.Mutex
	lastCtx  context.Context
	response master.TaskResponse
}

func (c *ctxCaptureProcessor) ProcessMessage(ctx context.Context, _ string, _ string) (master.TaskResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastCtx = ctx
	return c.response, nil
}

func (c *ctxCaptureProcessor) getLastCtx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCtx
}

// TestHandleMessage_UserAssociation 验证私聊消息且 enrichCtx 找到用户时，context 中注入了 user
func TestHandleMessage_UserAssociation(t *testing.T) {
	logger := zap.NewNop()
	proc := &ctxCaptureProcessor{response: master.TaskResponse{Content: "ok", Completed: true}}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)

	wantUser := &auth.User{ID: "user-42", ExternalID: "feishu-uid-001", AuthProvider: "feishu"}
	router.SetContextEnricher(func(ctx context.Context, externalID, provider string) context.Context {
		if externalID == "feishu-uid-001" && provider == "feishu" {
			return auth.WithUser(ctx, wantUser)
		}
		return ctx
	})

	// SenderID 为空，绕过 debounce，同步处理
	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform: PlatformFeishu,
		ChatID:   "chat-direct-001",
		SenderID: "feishu-uid-001",
		ChatType: ChatDirect,
		Content:  "hello",
	})
	assert.NoError(t, err)

	// SenderID 非空走 debounce，等待 flush
	time.Sleep(2500 * time.Millisecond)

	gotCtx := proc.getLastCtx()
	assert.NotNil(t, gotCtx)
	gotUser := auth.UserFrom(gotCtx)
	assert.NotNil(t, gotUser)
	assert.Equal(t, "user-42", gotUser.ID)
	imValue, ok := IMContextFrom(gotCtx)
	assert.True(t, ok)
	assert.Equal(t, "feishu-uid-001", imValue.SenderOpenID)
	assert.Equal(t, string(PlatformFeishu), imValue.Platform)
	assert.Equal(t, ChatDirect, imValue.ChatType)
	assert.Equal(t, "user-42", imValue.InternalUserID)
}

// TestHandleMessage_UnknownUser 验证 enrichCtx 找不到用户时，返回原 ctx，消息正常处理不阻塞
func TestHandleMessage_UnknownUser(t *testing.T) {
	logger := zap.NewNop()
	proc := &ctxCaptureProcessor{response: master.TaskResponse{Content: "ok", Completed: true}}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)

	router.SetContextEnricher(func(ctx context.Context, _, _ string) context.Context {
		return ctx // 模拟未找到用户，直接返回原 ctx
	})

	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform: PlatformFeishu,
		ChatID:   "chat-direct-002",
		SenderID: "unknown-uid",
		ChatType: ChatDirect,
		Content:  "hi",
	})
	assert.NoError(t, err)

	// SenderID 非空走 debounce，等待 flush
	time.Sleep(2500 * time.Millisecond)

	// context 中不应有 user
	gotCtx := proc.getLastCtx()
	assert.NotNil(t, gotCtx)
	gotUser := auth.UserFrom(gotCtx)
	assert.Nil(t, gotUser)
}

func TestRouterHandleMessage_FeishuResolverPassesIMContextAndMetrics(t *testing.T) {
	logger := zap.NewNop()
	proc := &imCaptureProcessor{
		response: master.TaskResponse{Content: "ok", Completed: true},
	}
	router := NewRouter(proc, logger)
	writer := &metricCaptureWriter{}
	router.SetMetricsWriter(writer)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "oc_feishu_chat",
		SessionID: "session-feishu",
	})

	wantCtx := &imctx.IMMessageContext{
		ParentMessageID:    "om_parent",
		ParentContent:      "上一条父消息",
		SystemPromptPrefix: "<im_context>prefix</im_context>",
		References: []imctx.DocRef{
			{Type: imctx.RefDocx, Token: "doccn123", Source: "url"},
			{Type: imctx.RefSheet, Token: "shtcn456", Source: "parent"},
		},
	}
	router.SetInboundContextResolver(PlatformFeishu, &staticResolver{out: wantCtx})

	err := router.HandleMessage(context.Background(), InboundMessage{
		MessageID:  "om_current",
		Platform:   PlatformFeishu,
		ChatID:     "oc_feishu_chat",
		SenderName: "tester",
		ChatType:   ChatDirect,
		Content:    "请总结这个链接",
	})
	assert.NoError(t, err)

	sessionID, input, messageID, _, gotCtx := proc.snapshot()
	assert.Equal(t, "session-feishu", sessionID)
	assert.Equal(t, "请总结这个链接", input)
	assert.Equal(t, "om_current", messageID)
	if assert.NotNil(t, gotCtx) {
		assert.Equal(t, "上一条父消息", gotCtx.ParentContent)
		assert.Equal(t, "<im_context>prefix</im_context>", gotCtx.SystemPromptPrefix)
		assert.Len(t, gotCtx.References, 2)
	}

	durationMetric := writer.waitMetric(t, "feishu.resolver.duration_ms")
	assert.Equal(t, "feishu", durationMetric.Labels["platform"])
	assert.Equal(t, "ok", durationMetric.Labels["status"])
	assert.Equal(t, "default", durationMetric.Labels["tenant_key_hash"])

	refsMetric := writer.waitMetric(t, "feishu.inbound.refs_count")
	assert.Equal(t, string(ChatDirect), refsMetric.Labels["chat_type"])
	assert.Equal(t, float64(2), refsMetric.Value)
	assert.Equal(t, "default", refsMetric.Labels["tenant_key_hash"])
}

func TestRouterHandleMessage_FeishuResolverErrorStillEmitsDurationMetric(t *testing.T) {
	logger := zap.NewNop()
	proc := &imCaptureProcessor{
		response: master.TaskResponse{Content: "ok", Completed: true},
	}
	router := NewRouter(proc, logger)
	writer := &metricCaptureWriter{}
	router.SetMetricsWriter(writer)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)
	router.Bind(Binding{
		Platform:  PlatformFeishu,
		ChatID:    "oc_feishu_chat",
		SessionID: "session-feishu",
	})
	router.SetInboundContextResolver(PlatformFeishu, &staticResolver{err: assert.AnError})

	err := router.HandleMessage(context.Background(), InboundMessage{
		MessageID:  "om_current",
		Platform:   PlatformFeishu,
		ChatID:     "oc_feishu_chat",
		SenderName: "tester",
		ChatType:   ChatDirect,
		Content:    "hello",
	})
	assert.NoError(t, err)

	_, _, _, _, gotCtx := proc.snapshot()
	assert.Nil(t, gotCtx)

	durationMetric := writer.waitMetric(t, "feishu.resolver.duration_ms")
	assert.Equal(t, "error", durationMetric.Labels["status"])
	assert.Equal(t, "default", durationMetric.Labels["tenant_key_hash"])
}

// TestHandleMessage_NoAuthEngine 验证未设置 enrichCtx 时，消息正常处理，context 中无 user
func TestHandleMessage_NoAuthEngine(t *testing.T) {
	logger := zap.NewNop()
	proc := &ctxCaptureProcessor{response: master.TaskResponse{Content: "ok", Completed: true}}
	router := NewRouter(proc, logger) // 不调用 SetContextEnricher

	plugin := &mockPlugin{platform: PlatformDingTalk}
	router.RegisterPlugin(plugin)

	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform: PlatformDingTalk,
		ChatID:   "chat-direct-003",
		SenderID: "dt-uid-001",
		ChatType: ChatDirect,
		Content:  "test",
	})
	assert.NoError(t, err)

	// SenderID 非空走 debounce，等待 flush
	time.Sleep(2500 * time.Millisecond)

	gotCtx := proc.getLastCtx()
	assert.NotNil(t, gotCtx)
	gotUser := auth.UserFrom(gotCtx)
	assert.Nil(t, gotUser)
}

// TestIMSession_NotBlockedByAuthCheck 验证群聊消息也触发 enrichCtx（Phase 3 群聊用户归属）
func TestIMSession_NotBlockedByAuthCheck(t *testing.T) {
	logger := zap.NewNop()
	proc := &ctxCaptureProcessor{response: master.TaskResponse{Content: "ok", Completed: true}}
	router := NewRouter(proc, logger)

	plugin := &mockPlugin{platform: PlatformFeishu}
	router.RegisterPlugin(plugin)

	var enrichCalled sync.Mutex
	enrichCalledFlag := false
	router.SetContextEnricher(func(ctx context.Context, _, _ string) context.Context {
		enrichCalled.Lock()
		enrichCalledFlag = true
		enrichCalled.Unlock()
		return ctx
	})

	err := router.HandleMessage(context.Background(), InboundMessage{
		Platform: PlatformFeishu,
		ChatID:   "chat-group-001",
		SenderID: "feishu-uid-002",
		ChatType: ChatGroup, // 群聊，不应触发 enrichCtx
		Content:  "group message",
	})
	assert.NoError(t, err)

	// 等待 debounce 窗口，确认 enrichCtx 始终未被调用
	time.Sleep(2500 * time.Millisecond)

	enrichCalled.Lock()
	called := enrichCalledFlag
	enrichCalled.Unlock()
	assert.True(t, called, "群聊消息也应触发 ContextEnricher")

	gotCtx := proc.getLastCtx()
	imValue, ok := IMContextFrom(gotCtx)
	assert.True(t, ok)
	assert.Equal(t, "feishu-uid-002", imValue.SenderOpenID)
	assert.Equal(t, ChatGroup, imValue.ChatType)
}
