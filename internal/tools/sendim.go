package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// IMPlatform IM 平台类型
type IMPlatform string

// IMMessage IM 消息（避免循环依赖）
type IMMessage struct {
	Platform IMPlatform
	ChatID   string
	Content  string
}

// IMRouter IM 路由器接口（避免直接依赖 channel 包）
type IMRouter interface {
	SendMessage(ctx context.Context, platform, chatID, content string) error
}

// sendIMMessageInput send_im_message 工具的输入参数
type sendIMMessageInput struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
	Content  string `json:"content"`
}

// RegisterSendIMMessage 注册 send_im_message 工具（导出函数，供 bootstrap 延迟调用）
// 允许 Agent 主动发送 IM 消息
func RegisterSendIMMessage(host *mcphost.Host, logger *zap.Logger, router IMRouter) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform": map[string]any{
				"type": "string",
				"enum": []string{
					"dingtalk",
					"feishu",
					"wecom",
					"wechat-wechaty",
					"wechat-wechatpadpro",
				},
				"description": "IM 平台名称",
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "聊天 ID（群 ID 或用户 ID，从 webhook 消息中获取）",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "消息内容（纯文本）",
			},
		},
		"required": []string{"platform", "chat_id", "content"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "send_im_message",
			Description: "发送消息到 IM 平台（钉钉/飞书/企业微信/个人微信）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params sendIMMessageInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("解析参数失败: " + err.Error()), nil
			}

			// 验证参数
			if params.Platform == "" {
				return errorResult("platform 参数不能为空"), nil
			}
			if params.ChatID == "" {
				return errorResult("chat_id 参数不能为空"), nil
			}
			if params.Content == "" {
				return errorResult("content 参数不能为空"), nil
			}

			// 发送消息
			if err := router.SendMessage(ctx, params.Platform, params.ChatID, params.Content); err != nil {
				logger.Error("发送 IM 消息失败",
					zap.String("platform", params.Platform),
					zap.String("chat_id", params.ChatID),
					zap.Error(err))

				return errorResult(fmt.Sprintf("发送失败: %v", err)), nil
			}

			logger.Info("IM 消息发送成功",
				zap.String("platform", params.Platform),
				zap.String("chat_id", params.ChatID),
				zap.Int("content_len", len(params.Content)))

			return textResult(fmt.Sprintf("✅ 消息已发送到 %s (chat: %s)", params.Platform, params.ChatID)), nil
		},
	)
}
