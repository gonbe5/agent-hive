package compaction

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// LLMSummaryCompactor 使用 LLM 生成智能摘要来压缩早期消息。
// 需要注入 LLMClient；若为 nil 则跳过（不报错）。
type LLMSummaryCompactor struct {
	LLMClient    *llm.Client
	TokenCounter *llm.TokenCounter
	UseTiktoken  bool
	Timeout      time.Duration // LLM 调用超时，默认 30s
	Logger       *zap.Logger
}

func (c *LLMSummaryCompactor) Name() string { return "llm_summary" }

func (c *LLMSummaryCompactor) Compact(ctx context.Context, messages []llm.MessageWithTools, budget int) ([]llm.MessageWithTools, error) {
	if len(messages) == 0 || budget <= 0 || c.LLMClient == nil {
		return messages, nil
	}

	// 摘要预留 token 数（LLM 摘要 MaxTokens=500，预留 300 足够容纳实际输出）
	summaryReserve := 300
	effectiveBudget := budget - summaryReserve
	if effectiveBudget <= 0 {
		effectiveBudget = budget / 2
	}

	// 从后往前找截断边界
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

	if cutoffIndex == 0 {
		return messages, nil
	}

	// 确保 cutoffIndex 不落在 tool 消息上
	for cutoffIndex < len(messages) && messages[cutoffIndex].Role == "tool" {
		cutoffIndex++
	}

	// 超大消息降级：如果所有消息都超 budget，至少保留最后一条非 tool 消息
	if cutoffIndex >= len(messages) {
		lastNonTool := len(messages) - 1
		for lastNonTool >= 0 && messages[lastNonTool].Role == "tool" {
			lastNonTool--
		}
		if lastNonTool <= 0 {
			return messages, nil
		}
		cutoffIndex = lastNonTool
	}

	olderMessages := messages[:cutoffIndex]
	recentMessages := messages[cutoffIndex:]

	summaryText, err := c.generateSummary(ctx, olderMessages)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Warn("LLM 摘要失败，降级到简单截断", zap.Error(err))
		}
		// 降级：使用简单文本摘要
		summaryText = generateSimpleSummary(olderMessages)
	}

	summaryMsg := llm.MessageWithTools{
		Role:    "system",
		Content: llm.NewTextContent(summaryText),
	}

	result := make([]llm.MessageWithTools, 0, 1+len(recentMessages))
	result = append(result, summaryMsg)
	result = append(result, recentMessages...)
	return result, nil
}

func (c *LLMSummaryCompactor) generateSummary(ctx context.Context, messages []llm.MessageWithTools) (string, error) {
	// 构建对话历史文本
	var hb strings.Builder
	hb.WriteString("对话历史：\n\n")
	for i, msg := range messages {
		hb.WriteString(strconv.Itoa(i + 1))
		hb.WriteString(". [")
		hb.WriteString(msg.Role)
		hb.WriteString("]: ")
		hb.WriteString(msg.Content.Text())
		hb.WriteString("\n\n")
	}

	systemPrompt := `你是一个对话历史压缩助手。你的任务是将长对话历史压缩为简洁的结构化摘要。

请仔细阅读对话历史，提取以下关键信息：
1. 用户的核心目标和需求
2. 已完成的关键操作和决策
3. 重要的文件变更和代码修改
4. 待解决的问题或待办事项

输出格式必须是有效的 JSON，结构如下：
{
  "goal": "用户的核心目标（1-2句话）",
  "completed": ["已完成项1", "已完成项2", ...],
  "file_changes": ["文件1", "文件2", ...],
  "pending": ["待办项1", "待办项2", ...],
  "message_count": 压缩的消息数量
}

注意：
- 保持简洁，每项不超过 50 字
- 只提取最重要的信息
- completed 和 pending 数组各不超过 5 项
- file_changes 只列出修改过的文件名（不包括路径）
- 如果某个字段无内容，使用空数组或空字符串`

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.LLMClient.Chat(callCtx, llm.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: llm.NewTextContent(hb.String())},
		},
		Temperature: 0.3,
		MaxTokens:   500,
		JSONMode:    true,
	})
	if err != nil {
		return "", err
	}

	summary, err := llm.ParseSummaryJSON(resp.Content)
	if err != nil {
		if c.Logger != nil {
			c.Logger.Warn("解析 LLM 摘要 JSON 失败，使用原始文本", zap.Error(err))
		}
		return "# 会话摘要\n\n" + resp.Content, nil
	}

	summary.MessageCount = len(messages)
	return llm.FormatSummary(summary), nil
}
