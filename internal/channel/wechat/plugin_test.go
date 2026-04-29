package wechat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/master"
)

// mockProtocol 模拟 Protocol 接口，用于单元测试
type mockProtocol struct {
	name     string
	started  bool
	stopped  bool
	loggedIn bool
	handler  MessageHandler
	sentMsgs []sentMsg
	startErr error
	sendErr  error
	mu       sync.Mutex
}

type sentMsg struct {
	chatID  string
	content string
}

func (m *mockProtocol) Name() string { return m.name }

func (m *mockProtocol) Start(ctx context.Context) error {
	m.started = true
	return m.startErr
}

func (m *mockProtocol) Stop() error {
	m.stopped = true
	return nil
}

func (m *mockProtocol) SendText(ctx context.Context, chatID, content string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	m.sentMsgs = append(m.sentMsgs, sentMsg{chatID: chatID, content: content})
	m.mu.Unlock()
	return nil
}

func (m *mockProtocol) SetMessageHandler(handler MessageHandler) {
	m.handler = handler
}

func (m *mockProtocol) IsLoggedIn() bool {
	return m.loggedIn
}

// mockProcessor 模拟 MessageProcessor
type mockProcessor struct {
	response master.TaskResponse
	err      error
}

func (p *mockProcessor) ProcessMessage(ctx context.Context, sessionID, input string) (master.TaskResponse, error) {
	return p.response, p.err
}

func newTestLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func TestPluginPlatform(t *testing.T) {
	mp := &mockProtocol{name: "test"}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("wechatpadpro", mp, router, logger)

	assert.Equal(t, channel.PlatformWeChatPadPro, p.Platform())
}

func TestPluginSend_LoggedIn(t *testing.T) {
	mp := &mockProtocol{name: "test", loggedIn: true}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	err := p.Send(context.Background(), channel.OutboundMessage{
		Platform: channel.PlatformWeChat,
		ChatID:   "user123",
		Content:  "你好",
	})

	require.NoError(t, err)
	assert.Len(t, mp.sentMsgs, 1)
	assert.Equal(t, "user123", mp.sentMsgs[0].chatID)
	assert.Equal(t, "你好", mp.sentMsgs[0].content)
}

func TestPluginSend_NotLoggedIn(t *testing.T) {
	mp := &mockProtocol{name: "test", loggedIn: false}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	err := p.Send(context.Background(), channel.OutboundMessage{
		Platform: channel.PlatformWeChat,
		ChatID:   "user123",
		Content:  "你好",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "未登录")
}

func TestPluginSend_LargeMessage(t *testing.T) {
	mp := &mockProtocol{name: "test", loggedIn: true}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	// 生成超过 2048 字节的消息
	longContent := make([]byte, 3000)
	for i := range longContent {
		longContent[i] = 'A'
	}

	err := p.Send(context.Background(), channel.OutboundMessage{
		Platform: channel.PlatformWeChat,
		ChatID:   "user123",
		Content:  string(longContent),
	})

	require.NoError(t, err)
	assert.Greater(t, len(mp.sentMsgs), 1, "大消息应分块发送")
}

func TestPluginWebhookHandler_NoWebhookProvider(t *testing.T) {
	mp := &mockProtocol{name: "test"}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	handler := p.WebhookHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/webhook", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// mockWebhookProtocol 模拟实现 WebhookProvider 的协议
type mockWebhookProtocol struct {
	mockProtocol
	webhookCalled bool
}

func (m *mockWebhookProtocol) WebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.webhookCalled = true
		w.WriteHeader(http.StatusOK)
	}
}

func TestPluginWebhookHandler_WithWebhookProvider(t *testing.T) {
	mp := &mockWebhookProtocol{
		mockProtocol: mockProtocol{name: "gewechat"},
	}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("gewechat", mp, router, logger)

	handler := p.WebhookHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/webhook", nil)
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, mp.webhookCalled)
}

func TestPluginStart(t *testing.T) {
	mp := &mockProtocol{name: "test"}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	err := p.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, mp.started)
}

func TestPluginStop(t *testing.T) {
	mp := &mockProtocol{name: "test"}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	err := p.Stop()
	require.NoError(t, err)
	assert.True(t, mp.stopped)
}

func TestPluginVerify(t *testing.T) {
	mp := &mockProtocol{name: "test"}
	logger := newTestLogger()
	router := channel.NewRouter(&mockProcessor{}, logger)
	p := New("openwechat", mp, router, logger)

	req := httptest.NewRequest("POST", "/webhook", nil)
	assert.True(t, p.Verify(req))
}

func TestPluginOnMessage_TextMessage(t *testing.T) {
	mp := &mockProtocol{name: "test", loggedIn: true}
	logger := newTestLogger()

	processor := &mockProcessor{
		response: master.TaskResponse{Content: "回复内容"},
	}
	router := channel.NewRouter(processor, logger)
	p := New("wechatpadpro", mp, router, logger)
	router.RegisterPlugin(p)

	// 绑定一个会话
	router.Bind(channel.Binding{
		Platform:  channel.PlatformWeChatPadPro,
		ChatID:    "user123",
		SessionID: "session-1",
	})

	// 触发消息回调
	require.NotNil(t, mp.handler, "handler 应在 New() 中设置")

	// 发送非文本消息 → 应忽略
	mp.handler(IncomingMessage{
		MsgID:    "msg-1",
		MsgType:  MsgImage,
		FromUser: "user123",
		Content:  "image data",
	})

	// 给异步处理一些时间
	// 非文本消息不会触发处理
}

func TestIncomingMessage_ChatID(t *testing.T) {
	tests := []struct {
		name     string
		msg      IncomingMessage
		expected string
	}{
		{
			name:     "私聊消息",
			msg:      IncomingMessage{FromUser: "user123"},
			expected: "user123",
		},
		{
			name:     "群聊消息",
			msg:      IncomingMessage{FromUser: "sender1", FromGroup: "group123@chatroom"},
			expected: "group123@chatroom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.msg.ChatID())
		})
	}
}

func TestIncomingMessage_IsGroup(t *testing.T) {
	assert.False(t, IncomingMessage{FromUser: "user1"}.IsGroup())
	assert.True(t, IncomingMessage{FromUser: "user1", FromGroup: "group1"}.IsGroup())
}
