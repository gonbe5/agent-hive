package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/airouter"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// RegisterVideoGen 注册 generate_video 工具，支持 Sora、即梦等视频生成模型。
// 若 router 为 nil，则跳过注册。
func RegisterVideoGen(host *mcphost.Host, router *airouter.Router, logger *zap.Logger) {
	if router == nil {
		return
	}

	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "视频内容的详细描述",
			},
			"duration": map[string]any{
				"type":        "string",
				"description": "视频时长，如 5s、10s，默认 5s",
			},
			"resolution": map[string]any{
				"type":        "string",
				"enum":        []string{"720p", "1080p"},
				"description": "视频分辨率，默认 1080p",
			},
		},
		"required": []string{"prompt"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "generate_video",
			Description: "根据文字描述生成视频。支持 Sora、即梦等视频生成模型。",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			var params struct {
				Prompt     string `json:"prompt"`
				Duration   string `json:"duration"`
				Resolution string `json:"resolution"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return errorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
			}

			if params.Prompt == "" {
				return errorResult("prompt 不能为空"), nil
			}

			resp, err := router.Execute(ctx, airouter.ServiceRequest{
				Type: airouter.ServiceVideoGen,
				Params: map[string]any{
					"prompt":     params.Prompt,
					"duration":   params.Duration,
					"resolution": params.Resolution,
				},
			})
			if err != nil {
				return errorResult(fmt.Sprintf("视频生成失败: %v", err)), nil
			}

			result := map[string]any{
				"url":     resp.URL,
				"message": "视频已生成",
			}
			resultJSON, _ := json.Marshal(result)
			return &mcphost.ToolResult{Content: resultJSON}, nil
		},
	)

	logger.Info("generate_video 工具已注册")
}
