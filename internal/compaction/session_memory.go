package compaction

import (
	"context"
	"strings"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// SessionMemoryCompactor 从消息历史中提取关键信息，生成 session memory 摘要。
// 与 TruncateCompactor 不同，它不删除消息，而是在消息列表头部插入一条
// system 消息作为"会话记忆"，帮助 LLM 在后续压缩后仍能保持上下文连贯。
type SessionMemoryCompactor struct {
	// MaxExtractMessages 最多从前 N 条消息中提取记忆（默认 20）
	MaxExtractMessages int
}

func (c *SessionMemoryCompactor) Name() string { return "session_memory" }

func (c *SessionMemoryCompactor) Compact(_ context.Context, messages []llm.MessageWithTools, _ int) ([]llm.MessageWithTools, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	maxExtract := c.MaxExtractMessages
	if maxExtract <= 0 {
		maxExtract = 20
	}

	// 如果消息太少，不需要提取记忆
	if len(messages) < 6 {
		return messages, nil
	}

	// 从前 N 条消息中提取关键信息
	extractEnd := min(len(messages), maxExtract)
	toExtract := messages[:extractEnd]

	memory := extractSessionMemory(toExtract)
	if memory == "" {
		return messages, nil
	}

	// 检查是否已有 session memory（避免重复插入）
	if len(messages) > 0 && messages[0].Role == "system" &&
		strings.HasPrefix(messages[0].Content.Text(), "[会话记忆]") {
		// 更新已有的 session memory
		result := make([]llm.MessageWithTools, len(messages))
		copy(result, messages)
		result[0].Content = llm.NewTextContent(memory)
		return result, nil
	}

	// 在头部插入 session memory
	result := make([]llm.MessageWithTools, 0, 1+len(messages))
	result = append(result, llm.MessageWithTools{
		Role:    "system",
		Content: llm.NewTextContent(memory),
	})
	result = append(result, messages...)
	return result, nil
}

// extractSessionMemory 从消息中提取关键信息生成会话记忆
func extractSessionMemory(messages []llm.MessageWithTools) string {
	var goals []string
	var fileChanges []string
	var decisions []string

	for _, msg := range messages {
		text := msg.Content.Text()
		if text == "" {
			continue
		}

		switch msg.Role {
		case "user":
			// 提取用户目标（取前两条用户消息的摘要）
			if len(goals) < 2 {
				goals = append(goals, truncateRunes(text, 80))
			}
		case "tool":
			// 提取文件变更信息
			if len(fileChanges) < 5 && msg.ToolName != "" {
				if isFileModifyTool(msg.ToolName) {
					fileChanges = append(fileChanges, msg.ToolName+": "+truncateRunes(text, 40))
				}
			}
		case "assistant":
			// 提取关键决策
			if len(decisions) < 3 && len(text) > 20 {
				decisions = append(decisions, truncateRunes(text, 60))
			}
		}
	}

	if len(goals) == 0 && len(fileChanges) == 0 && len(decisions) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("[会话记忆]\n")

	if len(goals) > 0 {
		buf.WriteString("用户目标：")
		buf.WriteString(strings.Join(goals, "；"))
		buf.WriteString("\n")
	}
	if len(fileChanges) > 0 {
		buf.WriteString("文件操作：")
		buf.WriteString(strings.Join(fileChanges, "；"))
		buf.WriteString("\n")
	}
	if len(decisions) > 0 {
		buf.WriteString("关键决策：")
		buf.WriteString(strings.Join(decisions, "；"))
		buf.WriteString("\n")
	}

	return buf.String()
}

// isFileModifyTool 判断工具是否涉及文件修改
func isFileModifyTool(toolName string) bool {
	switch toolName {
	case "write_file", "edit_file", "create_file", "delete_file",
		"bash", "shell", "multiedit":
		return true
	}
	return false
}
