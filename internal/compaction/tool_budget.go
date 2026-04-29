package compaction

import (
	"context"
	"strconv"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// ToolResultBudgetCompactor 对旧工具输出施加 token/字节 budget，
// 保护最近 N 轮对话不被裁剪，超阈值的旧工具输出截断为占位符。
type ToolResultBudgetCompactor struct {
	// ProtectedTurns 保护的最近用户轮数（默认 2）
	ProtectedTurns int
	// OutputThreshold 单条工具输出超过此字节数时裁剪（默认 20KB）
	OutputThreshold int
	// ContextBudget 累积保护上下文总量（字节），超出后强制裁剪（默认 40KB）
	ContextBudget int
}

func (c *ToolResultBudgetCompactor) Name() string { return "tool_budget" }

func (c *ToolResultBudgetCompactor) Compact(_ context.Context, messages []llm.MessageWithTools, _ int) ([]llm.MessageWithTools, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	protectedTurns := c.ProtectedTurns
	if protectedTurns <= 0 {
		protectedTurns = 2
	}
	threshold := c.OutputThreshold
	if threshold <= 0 {
		threshold = 20 * 1024
	}
	budgetBytes := c.ContextBudget
	if budgetBytes <= 0 {
		budgetBytes = 40 * 1024
	}

	// 复制消息列表避免修改原始数据
	result := make([]llm.MessageWithTools, len(messages))
	copy(result, messages)

	// 找到保护区域的起始位置
	protectedStart := findProtectedStart(messages, protectedTurns)

	// 从前往后扫描非保护区域的工具消息
	cumulativeSize := 0
	for i := 0; i < protectedStart; i++ {
		if result[i].Role != "tool" {
			continue
		}

		contentSize := len(result[i].Content.Text())
		cumulativeSize += contentSize

		if contentSize > threshold || cumulativeSize > budgetBytes {
			sizeKB := float64(contentSize) / 1024.0
			result[i].Content = llm.NewTextContent(
				"[输出已裁剪，原始大小: " + strconv.FormatFloat(sizeKB, 'f', 1, 64) + " KB]",
			)
		}
	}

	return result, nil
}

// findProtectedStart 计算保护区域的起始索引。
// 从消息末尾往前数 turns 轮用户消息，返回最后一轮保护区域的起始位置。
func findProtectedStart(messages []llm.MessageWithTools, turns int) int {
	userCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			userCount++
			if userCount >= turns {
				return i
			}
		}
	}
	return 0
}
