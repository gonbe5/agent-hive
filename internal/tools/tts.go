package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/airouter"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// RegisterTTS 注册 text_to_speech 工具
func RegisterTTS(host *mcphost.Host, router *airouter.Router, logger *zap.Logger) {
	if router == nil {
		return
	}

	tool := mcphost.ToolDefinition{
		Name:        "text_to_speech",
		Description: "将文字转换为语音音频。支持 OpenAI TTS 等兼容接口，返回 base64 编码的音频数据。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"text": {
					"type": "string",
					"description": "要转换为语音的文字（最长 4096 字符）"
				},
				"voice": {
					"type": "string",
					"enum": ["alloy", "ash", "ballad", "coral", "echo", "sage", "shimmer", "verse"],
					"description": "语音角色，默认 alloy"
				},
				"format": {
					"type": "string",
					"enum": ["mp3", "opus", "aac", "flac", "wav", "pcm"],
					"description": "音频格式，默认 mp3"
				}
			},
			"required": ["text"]
		}`),
	}

	handler := func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
		var params struct {
			Text   string `json:"text"`
			Voice  string `json:"voice"`
			Format string `json:"format"`
		}
		if err := json.Unmarshal(input, &params); err != nil {
			return errorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
		}

		if params.Text == "" {
			return errorResult("text 不能为空"), nil
		}

		resp, err := router.Execute(ctx, airouter.ServiceRequest{
			Type: airouter.ServiceTTS,
			Params: map[string]any{
				"text":   params.Text,
				"voice":  params.Voice,
				"format": params.Format,
			},
		})
		if err != nil {
			return errorResult(fmt.Sprintf("TTS 转换失败: %v", err)), nil
		}

		result := map[string]any{
			"mime_type":    resp.MimeType,
			"audio_base64": base64.StdEncoding.EncodeToString(resp.Data),
			"size_bytes":   len(resp.Data),
		}
		if resp.Metadata != nil {
			if v, ok := resp.Metadata["voice"].(string); ok {
				result["voice"] = v
			}
			if v, ok := resp.Metadata["model"].(string); ok {
				result["model"] = v
			}
		}

		resultJSON, _ := json.Marshal(result)
		return textResult(string(resultJSON)), nil
	}

	host.RegisterTool(tool, handler)
	logger.Info("text_to_speech 工具已注册")
}
