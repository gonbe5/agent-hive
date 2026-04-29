package master

import (
	"encoding/json"
	"testing"

	"github.com/chef-guo/agents-hive/internal/llm"
)

func TestClassifyStreamChunk_ToolCallOnlyCountsAsStreamEvent(t *testing.T) {
	got := classifyStreamChunk(llm.StreamChunk{
		ToolCalls: []llm.ToolCall{
			{
				ID:        "call_1",
				Name:      "feishu_api",
				Arguments: json.RawMessage(`{"action":"get_doc_content"`),
			},
		},
	})

	if !got.CountsAsStreamEvent {
		t.Fatal("只有 tool_calls 的非终态 chunk 应计为流式事件")
	}
	if !got.HasToolCalls {
		t.Fatal("应识别 tool_calls")
	}
	if got.HasText {
		t.Fatal("没有文本/推理内容时不应识别为文本 chunk")
	}
}

func TestClassifyStreamChunk_DoneToolCallDoesNotCountAsStreamEvent(t *testing.T) {
	got := classifyStreamChunk(llm.StreamChunk{
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Name: "feishu_api", Arguments: json.RawMessage(`{"action":"read_sheet"}`)},
		},
		FinishReason: "tool_calls",
		Done:         true,
	})

	if got.CountsAsStreamEvent {
		t.Fatal("Done 终态 chunk 不应计为 provider 的非终态流式事件")
	}
	if !got.HasToolCalls {
		t.Fatal("Done chunk 仍应保留 tool_calls 诊断信息")
	}
}

func TestClassifyStreamChunk_TextCountsAsStreamEvent(t *testing.T) {
	got := classifyStreamChunk(llm.StreamChunk{
		ContentDelta: "你",
		ContentSoFar: "你",
	})

	if !got.CountsAsStreamEvent {
		t.Fatal("文本 chunk 应计为流式事件")
	}
	if !got.HasText {
		t.Fatal("应识别文本 chunk")
	}
	if got.HasToolCalls {
		t.Fatal("没有 tool_calls 时不应识别为工具 chunk")
	}
}

func TestClassifyStreamChunk_EmptyNonDoneIgnored(t *testing.T) {
	got := classifyStreamChunk(llm.StreamChunk{})

	if got.CountsAsStreamEvent {
		t.Fatal("空的非终态 chunk 不应计为有效流式事件")
	}
	if got.HasText || got.HasToolCalls {
		t.Fatalf("空 chunk 不应包含文本或工具标记: %+v", got)
	}
}
