package master

import (
	"encoding/json"
	"testing"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestRepairOrphanedToolCalls 验证孤立 tool_call 修复逻辑
func TestRepairOrphanedToolCalls(t *testing.T) {
	logger := zaptest.NewLogger(t)

	makeToolCall := func(id, name string) llm.ToolCall {
		return llm.ToolCall{
			ID:        id,
			Name:      name,
			Arguments: json.RawMessage(`{}`),
		}
	}

	t.Run("有孤立的 tool_call 时补全 tool result", func(t *testing.T) {
		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent("帮我执行一下")},
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					makeToolCall("id1", "tool_a"),
					makeToolCall("id2", "tool_b"),
				},
			},
			// 只有 id1 有 result，id2 缺失
			{Role: "tool", ToolCallID: "id1", Content: llm.NewTextContent("结果A")},
		}

		result, _ := repairOrphanedToolCalls(messages, logger)

		// 原来 3 条 + 补全 1 条
		require.Len(t, result, 4)

		// 补全的 id2 紧跟在 assistant 之后（位置 2），id1 result 在位置 3
		assert.Equal(t, "tool", result[2].Role)
		assert.Equal(t, "id2", result[2].ToolCallID)
		assert.Contains(t, result[2].Content.Text(), "中断")
		// id1 的原始 result 在位置 3
		assert.Equal(t, "tool", result[3].Role)
		assert.Equal(t, "id1", result[3].ToolCallID)
	})

	t.Run("所有 tool_call 都有 result 时不修改", func(t *testing.T) {
		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent("请求")},
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					makeToolCall("id1", "tool_a"),
				},
			},
			{Role: "tool", ToolCallID: "id1", Content: llm.NewTextContent("结果A")},
		}

		result, _ := repairOrphanedToolCalls(messages, logger)
		assert.Len(t, result, 3)
	})

	t.Run("无 tool_call 的消息列表不修改", func(t *testing.T) {
		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent("你好")},
			{Role: "assistant", Content: llm.NewTextContent("你好！")},
		}

		result, _ := repairOrphanedToolCalls(messages, logger)
		assert.Len(t, result, 2)
	})

	t.Run("多个 assistant 消息各有孤立 tool_call", func(t *testing.T) {
		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent("请求")},
			{
				Role:      "assistant",
				ToolCalls: []llm.ToolCall{makeToolCall("id1", "tool_a")},
			},
			// id1 缺失
			{
				Role:      "assistant",
				ToolCalls: []llm.ToolCall{makeToolCall("id2", "tool_b")},
			},
			// id2 缺失
		}

		result, _ := repairOrphanedToolCalls(messages, logger)
		// 原 3 条 + 补全 2 条
		require.Len(t, result, 5)

		// 验证顺序：每个 tool result 紧跟在对应 assistant 之后
		assert.Equal(t, "user", result[0].Role)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Equal(t, "tool", result[2].Role)
		assert.Equal(t, "id1", result[2].ToolCallID)
		assert.Equal(t, "assistant", result[3].Role)
		assert.Equal(t, "tool", result[4].Role)
		assert.Equal(t, "id2", result[4].ToolCallID)
	})

	t.Run("修复的 tool result 紧跟在对应 assistant 之后而非末尾", func(t *testing.T) {
		// 消息: [user, assistant(tc=[id1,id2]), tool(id1), user, assistant(tc=[id3])]
		// id2 和 id3 缺失
		messages := []llm.MessageWithTools{
			{Role: "user", Content: llm.NewTextContent("请求1")},
			{
				Role: "assistant",
				ToolCalls: []llm.ToolCall{
					makeToolCall("id1", "tool_a"),
					makeToolCall("id2", "tool_b"),
				},
			},
			{Role: "tool", ToolCallID: "id1", Content: llm.NewTextContent("结果A")},
			{Role: "user", Content: llm.NewTextContent("请求2")},
			{
				Role:      "assistant",
				ToolCalls: []llm.ToolCall{makeToolCall("id3", "tool_c")},
			},
		}

		result, _ := repairOrphanedToolCalls(messages, logger)
		// 原 5 条 + 补全 2 条 = 7 条
		require.Len(t, result, 7)

		// 期望顺序:
		// [0] user
		// [1] assistant(tc=[id1,id2])
		// [2] tool(id2=修复) — 紧跟 assistant
		// [3] tool(id1) — 原始 result
		// [4] user
		// [5] assistant(tc=[id3])
		// [6] tool(id3=修复)
		assert.Equal(t, "user", result[0].Role)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Equal(t, "tool", result[2].Role)
		assert.Equal(t, "id2", result[2].ToolCallID)
		assert.Contains(t, result[2].Content.Text(), "中断")
		assert.Equal(t, "tool", result[3].Role)
		assert.Equal(t, "id1", result[3].ToolCallID)
		assert.Equal(t, "user", result[4].Role)
		assert.Equal(t, "assistant", result[5].Role)
		assert.Equal(t, "tool", result[6].Role)
		assert.Equal(t, "id3", result[6].ToolCallID)
		assert.Contains(t, result[6].Content.Text(), "中断")
	})
}
