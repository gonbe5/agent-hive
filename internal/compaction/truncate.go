package compaction

import (
	"context"
	"strconv"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// TruncateCompactor 简单截断压缩器：从后往前保留消息直到 budget，
// 被截断的早期消息生成简单文本摘要。
type TruncateCompactor struct {
	TokenCounter *llm.TokenCounter
	UseTiktoken  bool
}

func (c *TruncateCompactor) Name() string { return "truncate" }

func (c *TruncateCompactor) Compact(_ context.Context, messages []llm.MessageWithTools, budget int) ([]llm.MessageWithTools, error) {
	if len(messages) == 0 || budget <= 0 {
		return messages, nil
	}

	// 摘要预留 token 数（估算摘要 system 消息的体积）
	summaryReserve := 200
	effectiveBudget := budget - summaryReserve
	if effectiveBudget <= 0 {
		effectiveBudget = budget / 2
	}

	// 从后往前累计 token
	totalTokens := 0
	cutoffIndex := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := EstimateSingleTokens(messages[i], c.TokenCounter, c.UseTiktoken)
		if totalTokens+msgTokens > effectiveBudget {
			cutoffIndex = i + 1
			break
		}
		totalTokens += msgTokens
	}

	// 未超限
	if cutoffIndex == 0 {
		return messages, nil
	}

	// 确保 cutoffIndex 不落在 tool 消息上（保持 tool_calls/tool 配对完整）
	for cutoffIndex < len(messages) && messages[cutoffIndex].Role == "tool" {
		cutoffIndex++
	}

	// 超大消息降级：如果所有消息都超 budget（包括最后一条），
	// 至少保留最后一条非 tool 消息，把前面全部压缩成摘要
	if cutoffIndex >= len(messages) {
		// 从后往前找到第一条非 tool 消息
		lastNonTool := len(messages) - 1
		for lastNonTool >= 0 && messages[lastNonTool].Role == "tool" {
			lastNonTool--
		}
		if lastNonTool <= 0 {
			return messages, nil
		}
		cutoffIndex = lastNonTool
	}

	// 生成简单摘要 + 保留最近消息
	olderMessages := messages[:cutoffIndex]
	recentMessages := messages[cutoffIndex:]

	summary := generateSimpleSummary(olderMessages)
	summaryMsg := llm.MessageWithTools{
		Role:    "system",
		Content: llm.NewTextContent(summary),
	}

	result := make([]llm.MessageWithTools, 0, 1+len(recentMessages))
	result = append(result, summaryMsg)
	result = append(result, recentMessages...)
	return result, nil
}

// generateSimpleSummary 为早期消息生成简单摘要（不使用 LLM）
func generateSimpleSummary(messages []llm.MessageWithTools) string {
	if len(messages) == 0 {
		return "[会话摘要] 无早期消息"
	}

	buf := make([]byte, 0, 512)
	buf = append(buf, "[会话摘要]\n已压缩 "...)
	buf = strconv.AppendInt(buf, int64(len(messages)), 10)
	buf = append(buf, " 条早期消息，以下是简要内容：\n\n"...)

	limit := min(len(messages), 10)
	for i := 0; i < limit; i++ {
		buf = strconv.AppendInt(buf, int64(i+1), 10)
		buf = append(buf, ". ["...)
		buf = append(buf, messages[i].Role...)
		buf = append(buf, "]: "...)
		buf = append(buf, truncateRunes(messages[i].Content.Text(), 100)...)
		buf = append(buf, '\n')
	}
	if len(messages) > 10 {
		buf = append(buf, "...（还有 "...)
		buf = strconv.AppendInt(buf, int64(len(messages)-10), 10)
		buf = append(buf, " 条消息已省略）\n"...)
	}
	return string(buf)
}

// truncateRunes 截断内容到指定 rune 长度（UTF-8 安全）
func truncateRunes(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}
