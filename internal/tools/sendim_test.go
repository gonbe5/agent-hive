package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// MockIMRouter 模拟 IM 路由器
type MockIMRouter struct {
	sendFunc func(ctx context.Context, platform, chatID, content string) error
	sentMsgs []MockSentMessage
}

// MockSentMessage 记录发送的消息
type MockSentMessage struct {
	Platform string
	ChatID   string
	Content  string
}

func (m *MockIMRouter) SendMessage(ctx context.Context, platform, chatID, content string) error {
	// 记录发送的消息
	m.sentMsgs = append(m.sentMsgs, MockSentMessage{
		Platform: platform,
		ChatID:   chatID,
		Content:  content,
	})

	// 如果有自定义发送逻辑，使用它
	if m.sendFunc != nil {
		return m.sendFunc(ctx, platform, chatID, content)
	}

	return nil
}

// TestSendIMMessageSuccess 测试成功发送消息
func TestSendIMMessageSuccess(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	mockRouter := &MockIMRouter{}

	// 注册工具
	RegisterSendIMMessage(host, logger, mockRouter)

	// 构造输入参数
	input, _ := json.Marshal(map[string]any{
		"platform": "dingtalk",
		"chat_id":  "chat123",
		"content":  "测试消息",
	})

	// 调用工具
	result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
	if err != nil {
		t.Fatalf("工具调用失败: %v", err)
	}

	if result.IsError {
		t.Errorf("预期成功，但返回错误: %v", result.Content)
	}

	// 验证消息已发送
	if len(mockRouter.sentMsgs) != 1 {
		t.Fatalf("预期发送1条消息，实际发送: %d", len(mockRouter.sentMsgs))
	}

	sent := mockRouter.sentMsgs[0]
	if sent.Platform != "dingtalk" {
		t.Errorf("预期平台 dingtalk，实际: %s", sent.Platform)
	}
	if sent.ChatID != "chat123" {
		t.Errorf("预期 chat_id chat123，实际: %s", sent.ChatID)
	}
	if sent.Content != "测试消息" {
		t.Errorf("预期内容 '测试消息'，实际: %s", sent.Content)
	}
}

// TestSendIMMessageAllPlatforms 测试所有支持的平台
func TestSendIMMessageAllPlatforms(t *testing.T) {
	platforms := []string{
		"dingtalk",
		"feishu",
		"wecom",
		"wechat-wechaty",
		"wechat-wechatpadpro",
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			logger := zap.NewNop()
			host := mcphost.NewHost(logger)
			mockRouter := &MockIMRouter{}

			RegisterSendIMMessage(host, logger, mockRouter)

			input, _ := json.Marshal(map[string]any{
				"platform": platform,
				"chat_id":  "chat456",
				"content":  "测试消息",
			})

			result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
			if err != nil {
				t.Fatalf("工具调用失败: %v", err)
			}

			if result.IsError {
				t.Errorf("预期成功，但返回错误: %v", result.Content)
			}

			if len(mockRouter.sentMsgs) != 1 {
				t.Fatalf("预期发送1条消息，实际: %d", len(mockRouter.sentMsgs))
			}

			if mockRouter.sentMsgs[0].Platform != platform {
				t.Errorf("预期平台 %s，实际: %s", platform, mockRouter.sentMsgs[0].Platform)
			}
		})
	}
}

// TestSendIMMessageMissingParams 测试缺少参数
func TestSendIMMessageMissingParams(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		errMsg string
	}{
		{
			name:   "缺少platform",
			input:  map[string]any{"chat_id": "chat123", "content": "测试"},
			errMsg: "platform 参数不能为空",
		},
		{
			name:   "缺少chat_id",
			input:  map[string]any{"platform": "dingtalk", "content": "测试"},
			errMsg: "chat_id 参数不能为空",
		},
		{
			name:   "缺少content",
			input:  map[string]any{"platform": "dingtalk", "chat_id": "chat123"},
			errMsg: "content 参数不能为空",
		},
		{
			name:   "platform为空字符串",
			input:  map[string]any{"platform": "", "chat_id": "chat123", "content": "测试"},
			errMsg: "platform 参数不能为空",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			host := mcphost.NewHost(logger)
			mockRouter := &MockIMRouter{}

			RegisterSendIMMessage(host, logger, mockRouter)

			input, _ := json.Marshal(tt.input)

			result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
			if err != nil {
				t.Fatalf("工具调用失败: %v", err)
			}

			if !result.IsError {
				t.Error("预期返回错误，但成功了")
			}

			var errMsg string
			_ = json.Unmarshal(result.Content, &errMsg)
			if errMsg != tt.errMsg {
				t.Errorf("预期错误信息 '%s'，实际: '%s'", tt.errMsg, errMsg)
			}
		})
	}
}

// TestSendIMMessageSendError 测试发送失败
func TestSendIMMessageSendError(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)

	// Mock 路由器返回错误
	mockRouter := &MockIMRouter{
		sendFunc: func(ctx context.Context, platform, chatID, content string) error {
			return errors.New("网络错误")
		},
	}

	RegisterSendIMMessage(host, logger, mockRouter)

	input, _ := json.Marshal(map[string]any{
		"platform": "dingtalk",
		"chat_id":  "chat123",
		"content":  "测试消息",
	})

	result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
	if err != nil {
		t.Fatalf("工具调用失败: %v", err)
	}

	if !result.IsError {
		t.Error("预期返回错误，但成功了")
	}

	var errMsg string
	_ = json.Unmarshal(result.Content, &errMsg)
	if errMsg != "发送失败: 网络错误" {
		t.Errorf("预期错误信息包含 '网络错误'，实际: %s", errMsg)
	}
}

// TestSendIMMessageInvalidJSON 测试无效的 JSON 输入
func TestSendIMMessageInvalidJSON(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	mockRouter := &MockIMRouter{}

	RegisterSendIMMessage(host, logger, mockRouter)

	// 无效的 JSON
	input := json.RawMessage(`{invalid json}`)

	result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
	if err != nil {
		t.Fatalf("工具调用失败: %v", err)
	}

	if !result.IsError {
		t.Error("预期返回错误，但成功了")
	}

	var errMsg string
	_ = json.Unmarshal(result.Content, &errMsg)
	if errMsg == "" {
		t.Error("预期有错误信息，但为空")
	}
}

// TestSendIMMessageLongContent 测试发送长消息
func TestSendIMMessageLongContent(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	mockRouter := &MockIMRouter{}

	RegisterSendIMMessage(host, logger, mockRouter)

	// 创建一个很长的消息（5000个字符）
	longContent := string(make([]byte, 5000))
	for i := range longContent {
		longContent = longContent[:i] + "测"
	}

	input, _ := json.Marshal(map[string]any{
		"platform": "feishu",
		"chat_id":  "chat789",
		"content":  longContent[:5000], // 取前5000个字节
	})

	result, err := host.ExecuteTool(context.Background(), "send_im_message", input)
	if err != nil {
		t.Fatalf("工具调用失败: %v", err)
	}

	if result.IsError {
		t.Errorf("预期成功，但返回错误: %v", result.Content)
	}

	// 验证消息已发送
	if len(mockRouter.sentMsgs) != 1 {
		t.Fatalf("预期发送1条消息，实际: %d", len(mockRouter.sentMsgs))
	}
}
