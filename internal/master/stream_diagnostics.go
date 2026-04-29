package master

import "github.com/chef-guo/agents-hive/internal/llm"

type streamChunkClass struct {
	HasText             bool
	HasToolCalls        bool
	CountsAsStreamEvent bool
}

// classifyStreamChunk 把 LLM 流式回调里的 chunk 分成可见文本、工具调用和有效流事件。
// Done chunk 是终态快照，不代表 provider 在中途真正吐过 SSE 数据，所以不计入流式事件。
func classifyStreamChunk(chunk llm.StreamChunk) streamChunkClass {
	hasText := chunk.ContentDelta != "" || chunk.ContentSoFar != "" || chunk.ReasoningContent != ""
	hasToolCalls := len(chunk.ToolCalls) > 0
	return streamChunkClass{
		HasText:             hasText,
		HasToolCalls:        hasToolCalls,
		CountsAsStreamEvent: !chunk.Done && (hasText || hasToolCalls || chunk.FinishReason != ""),
	}
}
