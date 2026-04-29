package wecom

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"go.uber.org/zap"
)

// TestWeComPluginBasic 测试企业微信 Plugin 基本功能
func TestWeComPluginBasic(t *testing.T) {
	logger := zap.NewNop()

	cfg := config.WeComConfig{
		CorpID:         "test_corp",
		AgentID:        123,
		Secret:         "test_secret",
		Token:          "test_token",
		EncodingAESKey: "test_aes_key_32_characters_long",
	}

	plugin := New(cfg, nil, logger)

	// 测试 Platform 方法
	if plugin.Platform() != channel.PlatformWeCom {
		t.Errorf("预期平台为 wecom，实际: %s", plugin.Platform())
	}
}

// TestWeComMessageChunking 测试企业微信消息分块
func TestWeComMessageChunking(t *testing.T) {
	// 测试分块逻辑
	longMessage := strings.Repeat("测试", 1000) // 约 6KB

	// 使用 chunk 函数分块
	chunks := channel.ChunkText(longMessage, 2048)

	if len(chunks) < 2 {
		t.Errorf("预期消息被分块，实际块数: %d", len(chunks))
	}

	// 验证每块不超过限制
	for i, chunk := range chunks {
		if len(chunk) > 2048 {
			t.Errorf("第 %d 块超过 2048 字节: %d", i, len(chunk))
		}
	}

	// 验证合并后等于原始消息
	combined := strings.Join(chunks, "")
	if combined != longMessage {
		t.Error("分块合并后与原始消息不一致")
	}
}

// TestWeComConfigValidation 测试配置验证
func TestWeComConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.WeComConfig
		wantErr bool
	}{
		{
			name: "有效配置",
			cfg: config.WeComConfig{
				CorpID:         "test_corp",
				AgentID:        123,
				Secret:         "test_secret",
				Token:          "test_token",
				EncodingAESKey: "test_aes_key_32_characters_long",
			},
			wantErr: false,
		},
		{
			name: "缺少 CorpID",
			cfg: config.WeComConfig{
				AgentID: 123,
				Secret:  "test_secret",
			},
			wantErr: false, // Plugin 初始化不会失败，但使用时会有问题
		},
	}

	logger := zap.NewNop()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New(tt.cfg, nil, logger)
			if plugin == nil {
				t.Error("插件创建失败")
			}
		})
	}
}

// TestWeComOutboundMessage 测试消息发送结构
func TestWeComOutboundMessage(t *testing.T) {
	msg := channel.OutboundMessage{
		Platform: channel.PlatformWeCom,
		ChatID:   "test_chat_id",
		Content:  "测试消息",
		ReplyTo:  "msg_123",
	}

	// 验证消息字段
	if msg.Platform != channel.PlatformWeCom {
		t.Errorf("预期平台 wecom，实际: %s", msg.Platform)
	}

	if msg.ChatID != "test_chat_id" {
		t.Errorf("预期 chat_id=test_chat_id，实际: %s", msg.ChatID)
	}

	if msg.Content != "测试消息" {
		t.Errorf("预期内容='测试消息'，实际: %s", msg.Content)
	}

	// 验证 JSON 序列化
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var decoded channel.OutboundMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if decoded.Content != msg.Content {
		t.Error("序列化/反序列化后消息内容不一致")
	}
}
