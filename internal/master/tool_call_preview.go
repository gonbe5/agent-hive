package master

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/llm"
)

const maxToolCallPreviewArgumentBytes = 512

func buildToolCallPreviewPayload(sessionID string, chunk llm.StreamChunk, now time.Time) (map[string]any, bool) {
	if chunk.Done || len(chunk.ToolCalls) == 0 {
		return nil, false
	}
	toolCalls := make([]map[string]any, 0, len(chunk.ToolCalls))
	for _, tc := range chunk.ToolCalls {
		toolCalls = append(toolCalls, map[string]any{
			"id":        tc.ID,
			"name":      tc.Name,
			"arguments": truncateToolCallPreviewArgs(tc.Arguments),
		})
	}
	return map[string]any{
		"content":           "",
		"session_id":        sessionID,
		"partial":           true,
		"tool_call_preview": true,
		"tool_calls":        toolCalls,
		"timestamp":         now.UTC().Format(time.RFC3339),
	}, true
}

func toolCallPreviewFingerprint(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, call := range calls {
		b.WriteString(call.ID)
		b.WriteByte('\x00')
		b.WriteString(call.Name)
		b.WriteByte('\x00')
		b.Write(call.Arguments)
		b.WriteByte('\x1e')
	}
	return b.String()
}

func truncateToolCallPreviewArgs(args json.RawMessage) string {
	if len(args) <= maxToolCallPreviewArgumentBytes {
		return string(args)
	}
	return string(args[:maxToolCallPreviewArgumentBytes]) + "...<truncated>"
}
