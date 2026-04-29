package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// QuestionBridge 连接到 Master 的提问机制
type QuestionBridge interface {
	AskQuestion(ctx context.Context, question string, options []string, timeout time.Duration) (string, error)
}

// questionInput 工具输入参数
type questionInput struct {
	Question string   `json:"question"`          // 要问的问题
	Options  []string `json:"options,omitempty"` // 预设选项（可选）
	Timeout  int      `json:"timeout,omitempty"` // 超时秒数（默认60s）
}

// registerQuestion 注册 question 工具
func registerQuestion(host *mcphost.Host, logger *zap.Logger, questionBridge QuestionBridge) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "要向用户提问的问题",
			},
			"options": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "预设的选项列表（可选，例如 [\"是\", \"否\"]）",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "等待用户回答的超时时间（秒，默认60，最大300）",
				"minimum":     1,
				"maximum":     300,
			},
		},
		"required": []string{"question"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "question",
			Description: "向用户主动提问并等待回答（用于收集必要信息或澄清需求）",
			InputSchema: schema,
			Core:        true,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params questionInput
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult("输入无效: " + err.Error()), nil
			}

			if params.Question == "" {
				return errorResult("问题内容不能为空"), nil
			}

			// 设置默认超时（5分钟），最大60分钟
			timeout := 300 * time.Second
			if params.Timeout > 0 {
				if params.Timeout > 3600 {
					params.Timeout = 3600 // 最大60分钟
				}
				timeout = time.Duration(params.Timeout) * time.Second
			}

			logger.Info("Agent 主动提问",
				zap.String("question", params.Question),
				zap.Strings("options", params.Options),
				zap.Duration("timeout", timeout))

			// 发送问题并等待回答
			answer, err := questionBridge.AskQuestion(ctx, params.Question, params.Options, timeout)
			if err != nil {
				if err == context.DeadlineExceeded {
					logger.Warn("等待用户回答超时", zap.String("question", params.Question))
					return errorResult("等待用户回答超时"), nil
				}
				logger.Error("提问失败", zap.Error(err))
				return errorResult("提问失败: " + err.Error()), nil
			}

			logger.Info("用户回答问题",
				zap.String("question", params.Question),
				zap.String("answer", answer))

			return textResult(fmt.Sprintf("用户回答: %s", answer)), nil
		},
	)
}
