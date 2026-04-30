package master

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/llm"
)

func TestBuildToolCallPreviewPayload(t *testing.T) {
	chunk := llm.StreamChunk{
		ToolCalls: []llm.ToolCall{
			{
				ID:        "call_1",
				Name:      "web_search",
				Arguments: json.RawMessage(`{"query":"agents hive quality","long":"` + string(make([]byte, 600)) + `"}`),
			},
		},
	}

	payload, ok := buildToolCallPreviewPayload("session-1", chunk, time.Unix(1700000000, 0))

	if !ok {
		t.Fatal("非终态 tool_calls chunk 应生成预览 payload")
	}
	if payload["content"] != "" {
		t.Fatalf("tool-call 预览不应伪造文本 content，got=%q", payload["content"])
	}
	if payload["session_id"] != "session-1" {
		t.Fatalf("session_id mismatch: %v", payload["session_id"])
	}
	if payload["partial"] != true || payload["tool_call_preview"] != true {
		t.Fatalf("预览 payload 必须是 partial preview: %+v", payload)
	}
	calls, ok := payload["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("tool_calls payload 形态错误: %#v", payload["tool_calls"])
	}
	if calls[0]["id"] != "call_1" || calls[0]["name"] != "web_search" {
		t.Fatalf("tool call 基本字段丢失: %+v", calls[0])
	}
	args, _ := calls[0]["arguments"].(string)
	if len(args) > maxToolCallPreviewArgumentBytes+len("...<truncated>") {
		t.Fatalf("preview arguments 未截断，len=%d", len(args))
	}
	if payload["timestamp"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("timestamp mismatch: %v", payload["timestamp"])
	}
}

func TestBuildToolCallPreviewPayload_IgnoresDoneChunk(t *testing.T) {
	payload, ok := buildToolCallPreviewPayload("session-1", llm.StreamChunk{
		Done: true,
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Name: "web_search", Arguments: json.RawMessage(`{}`)},
		},
	}, time.Unix(1700000000, 0))

	if ok || payload != nil {
		t.Fatalf("Done chunk 不应生成预览 payload: ok=%v payload=%+v", ok, payload)
	}
}

func TestToolCallPreviewFingerprint_AllowsPartialJSONArguments(t *testing.T) {
	fp := toolCallPreviewFingerprint([]llm.ToolCall{
		{
			ID:        "call_1",
			Name:      "web_search",
			Arguments: json.RawMessage(`{"query":"half`),
		},
	})

	if fp == "" {
		t.Fatal("流式 tool call 参数可能是半截 JSON，fingerprint 仍必须可用，否则预览会被跳过")
	}
}
