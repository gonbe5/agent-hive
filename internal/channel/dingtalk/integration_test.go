package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"go.uber.org/zap"
)

// TestDingTalkIntegrationWebhookFlow 测试钉钉 Webhook 完整流程
func TestDingTalkIntegrationWebhookFlow(t *testing.T) {
	logger := zap.NewNop()

	// 创建模拟消息处理器
	var receivedContent string
	var wg sync.WaitGroup
	wg.Add(1)
	processor := &mockProcessor{
		processFunc: func(ctx context.Context, sessionID, content string) (master.TaskResponse, error) {
			defer wg.Done()
			receivedContent = content
			return master.TaskResponse{
				Content: "收到：" + content,
			}, nil
		},
	}

	router := channel.NewRouter(processor, logger)

	// 创建 DingTalk Plugin
	cfg := config.DingTalkConfig{
		AppKey:    "test_key",
		AppSecret: "test_secret",
		AgentID:   123456,
	}

	plugin := New(cfg, router, logger)
	router.RegisterPlugin(plugin)

	// 绑定会话
	router.Bind(channel.Binding{
		Platform:  channel.PlatformDingTalk,
		ChatID:    "test_chat_id",
		SessionID: "test_session",
	})

	// 模拟 Webhook 请求
	webhookBody := map[string]any{
		"msgtype": "text",
		"text": map[string]any{
			"content": "测试消息",
		},
		"conversationId": "test_chat_id",
		"senderNick":     "测试用户",
	}

	bodyBytes, _ := json.Marshal(webhookBody)
	req := httptest.NewRequest(http.MethodPost, "/webhook/dingtalk", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")

	// 添加签名头
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	stringToSign := fmt.Sprintf("%s\n%s", timestamp, cfg.AppSecret)
	mac := hmac.New(sha256.New, []byte(cfg.AppSecret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("sign", sign)

	rec := httptest.NewRecorder()
	handler := router.WebhookHandler(channel.PlatformDingTalk)
	handler(rec, req)

	// 验证响应
	if rec.Code != http.StatusOK {
		t.Errorf("预期状态码 200，实际: %d", rec.Code)
	}

	// 等待异步消息处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("等待消息处理超时")
	}

	// 验证消息被正确处理
	if receivedContent != "测试消息" {
		t.Errorf("预期接收消息 '测试消息'，实际: %s", receivedContent)
	}
}

// TestDingTalkIntegrationMessageChunking 测试消息分块发送
func TestDingTalkIntegrationMessageChunking(t *testing.T) {
	logger := zap.NewNop()

	var sentMessages []string

	// 创建模拟 HTTP 服务器（钉钉 API）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var msg map[string]any
		json.Unmarshal(body, &msg)

		// 实际发送的格式为 {"msgtype":"text","text":{"content":"..."}}
		if textContent, ok := msg["text"].(map[string]any); ok {
			if content, ok := textContent["content"].(string); ok {
				sentMessages = append(sentMessages, content)
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"errcode": 0,
			"errmsg":  "ok",
		})
	}))
	defer server.Close()

	// 创建 Plugin
	cfg := config.DingTalkConfig{
		AppKey:    "test_key",
		AppSecret: "test_secret",
		AgentID:   123456,
	}

	plugin := New(cfg, nil, logger)

	// 缓存 Webhook URL
	plugin.CacheWebhook("test_chat", server.URL)

	// 发送超长消息（超过 18000 字节）
	longMessage := strings.Repeat("测试消息", 2000) // 约 24KB

	err := plugin.Send(context.Background(), channel.OutboundMessage{
		Platform: channel.PlatformDingTalk,
		ChatID:   "test_chat",
		Content:  longMessage,
	})

	if err != nil {
		t.Fatalf("发送消息失败: %v", err)
	}

	// 验证消息被分块
	if len(sentMessages) < 2 {
		t.Errorf("预期消息被分块（至少2块），实际发送: %d", len(sentMessages))
	}

	// 验证所有分块内容合并后等于原始消息
	combined := strings.Join(sentMessages, "")
	if combined != longMessage {
		t.Error("分块消息合并后与原始消息不一致")
	}
}

// TestDingTalkIntegrationSignatureValidation 测试签名验证
func TestDingTalkIntegrationSignatureValidation(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		appSecret   string
		timestamp   string
		sign        string
		expectValid bool
	}{
		{
			name:        "无 AppSecret（应跳过验证）",
			appSecret:   "",
			timestamp:   "",
			sign:        "",
			expectValid: true,
		},
		{
			name:        "有 AppSecret 但签名错误",
			appSecret:   "test_secret",
			timestamp:   "1234567890000",
			sign:        "invalid_signature",
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DingTalkConfig{
				AppKey:    "test_key",
				AppSecret: tt.appSecret,
				AgentID:   123456,
			}
			plugin := New(cfg, nil, logger)

			req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("{}"))
			if tt.timestamp != "" {
				req.Header.Set("timestamp", tt.timestamp)
				req.Header.Set("sign", tt.sign)
			}

			valid := plugin.Verify(req)
			if valid != tt.expectValid {
				t.Errorf("预期 valid=%v，实际: %v", tt.expectValid, valid)
			}
		})
	}
}

// mockProcessor 模拟消息处理器
type mockProcessor struct {
	processFunc func(ctx context.Context, sessionID, content string) (master.TaskResponse, error)
}

func (m *mockProcessor) ProcessMessage(ctx context.Context, sessionID, content string) (master.TaskResponse, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, sessionID, content)
	}
	return master.TaskResponse{Content: "ok"}, nil
}
