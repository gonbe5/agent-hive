package compaction

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// HistorySnipCompactor 移除中间轮次，保留首尾消息。
// 对于超长会话，这是一种有效的渐进式压缩策略：
// - 保留第一条 user 消息（建立任务上下文）
// - 保留最近的 N 条消息（保持最新状态）
// - 中间消息用摘要替代
type HistorySnipCompactor struct {
	// KeepFirst 保留第一条消息。零值 false 表示不保留，需显式设为 true。
	KeepFirst bool
	// KeepLast 保留最近消息数量（默认 4）
	KeepLast int
}

func (c *HistorySnipCompactor) Name() string { return "history_snip" }

func (c *HistorySnipCompactor) Compact(_ context.Context, messages []llm.MessageWithTools, _ int) ([]llm.MessageWithTools, error) {
	if len(messages) <= 4 {
		return messages, nil
	}

	keepLast := c.KeepLast
	if keepLast <= 0 {
		keepLast = 4
	}

	var result []llm.MessageWithTools

	middleStart := 0
	if c.KeepFirst {
		result = append(result, messages[0])
		middleStart = 1
	}

	middleEnd := len(messages) - keepLast
	if middleEnd <= middleStart {
		// 没有中间消息可压缩
		return messages, nil
	}

	// 生成中间消息摘要
	middleMessages := messages[middleStart:middleEnd]
	summary := generateSimpleSummary(middleMessages)

	result = append(result, llm.MessageWithTools{
		Role:    "system",
		Content: llm.NewTextContent("[中间消息已压缩] " + summary),
	})

	// 保留最近消息
	result = append(result, messages[len(messages)-keepLast:]...)

	return result, nil
}
