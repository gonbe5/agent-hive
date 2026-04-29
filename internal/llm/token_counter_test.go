package llm

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTokenCounter(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)
	assert.NotNil(t, tc)
	assert.NotNil(t, tc.encoding)
}

func TestCountText(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	tests := []struct {
		name     string
		text     string
		minCount int
		maxCount int
	}{
		{"empty", "", 0, 0},
		{"short english", "Hello", 1, 2},
		{"medium english", "Hello, how are you today?", 5, 8},
		{"chinese", "你好世界", 2, 6},
		{"mixed", "Hello 世界 你好 world", 4, 10},
		{"code", "func main() { fmt.Println(\"hello\") }", 8, 15},
		{"markdown", "# Title\n\n- Item 1\n- Item 2", 8, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := tc.CountText(tt.text)
			assert.GreaterOrEqual(t, count, tt.minCount, "token count should be >= min")
			assert.LessOrEqual(t, count, tt.maxCount, "token count should be <= max")
		})
	}
}

func TestCountContent_Text(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	content := NewTextContent("Hello world")
	count := tc.CountContent(content)
	assert.Greater(t, count, 0)
	assert.Less(t, count, 10)
}

func TestCountContent_Multimodal(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	content := NewMultiContent(
		TextPart("Describe this image"),
		ImageURLPart("https://example.com/image.jpg"),
	)

	count := tc.CountContent(content)
	// 文本 ~4 tokens + 图片 765 tokens (auto)
	assert.Greater(t, count, 750)
	assert.Less(t, count, 800)
}

func TestCountContent_ImageDetail(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	tests := []struct {
		name     string
		detail   string
		expected int
	}{
		{"low detail", "low", 85},
		{"auto detail", "auto", 765},
		{"high detail", "high", 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := NewMultiContent(ContentPart{
				Type:     ContentImage,
				ImageURL: "https://example.com/image.jpg",
				Detail:   tt.detail,
			})
			count := tc.CountContent(content)
			// 允许 ±10 token 误差（文本部分）
			assert.InDelta(t, tt.expected, count, 10)
		})
	}
}

func TestCountMessage(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Hello, how are you?"),
	}

	count := tc.CountMessage(msg)
	// 角色开销 4 + 文本 ~5 tokens
	assert.Greater(t, count, 5)
	assert.Less(t, count, 15)
}

func TestCountMessage_WithToolCalls(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg := MessageWithTools{
		Role:    "assistant",
		Content: NewTextContent("Let me search for that."),
		ToolCalls: []ToolCall{
			{
				ID:        "call_123",
				Name:      "search",
				Arguments: json.RawMessage(`{"query": "test"}`),
			},
		},
	}

	count := tc.CountMessage(msg)
	// 角色开销 + 文本 + 工具调用
	assert.Greater(t, count, 10)
	assert.Less(t, count, 30)
}

func TestCountMessages(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	messages := []MessageWithTools{
		{Role: "user", Content: NewTextContent("Hello")},
		{Role: "assistant", Content: NewTextContent("Hi there!")},
		{Role: "user", Content: NewTextContent("How are you?")},
	}

	count := tc.CountMessages(messages)
	// 固定开销 3 + 每条消息 ~4-10 tokens
	assert.Greater(t, count, 15)
	assert.Less(t, count, 50)
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty", "", 0},
		{"short", "Hello", 1},         // 5 / 4 = 1
		{"medium", "Hello World", 2},  // 11 / 4 = 2
		{"long", "Hello, how are you today?", 6}, // 26 / 4 = 6
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := EstimateTokens(tt.text)
			assert.Equal(t, tt.expected, count)
		})
	}
}

func TestEstimateContentTokens(t *testing.T) {
	tests := []struct {
		name     string
		content  Content
		expected int
	}{
		{
			name:     "text",
			content:  NewTextContent("Hello world"),
			expected: 2, // 11 / 4 = 2
		},
		{
			name: "image",
			content: NewMultiContent(
				TextPart("Look"),
				ImageURLPart("https://example.com/img.jpg"),
			),
			expected: 765 + 1, // 4/4 + 765
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := EstimateContentTokens(tt.content)
			assert.InDelta(t, tt.expected, count, 5)
		})
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Hello"),
	}

	count := EstimateMessageTokens(msg)
	// 角色开销 4 + 文本 1 = 5
	assert.Equal(t, 5, count)
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []MessageWithTools{
		{Role: "user", Content: NewTextContent("Hello")},      // 4 + 1 = 5
		{Role: "assistant", Content: NewTextContent("Hi!")},   // 4 + 1 = 5
	}

	count := EstimateMessagesTokens(messages)
	// 固定开销 3 + 5 + 5 = 13
	// 注意："Hi!" 是 3 个字符，3/4 = 0（整数除法），所以实际是 12
	assert.Equal(t, 12, count)
}

func TestFormatSummary(t *testing.T) {
	summary := CompactSummary{
		Goal:         "创建一个新功能",
		Completed:    []string{"实现核心逻辑", "添加测试"},
		FileChanges:  []string{"main.go", "main_test.go"},
		Pending:      []string{"添加文档"},
		MessageCount: 10,
	}

	formatted := FormatSummary(summary)

	assert.Contains(t, formatted, "# 会话摘要")
	assert.Contains(t, formatted, "已压缩 10 条早期消息")
	assert.Contains(t, formatted, "## 目标")
	assert.Contains(t, formatted, "创建一个新功能")
	assert.Contains(t, formatted, "## 已完成")
	assert.Contains(t, formatted, "实现核心逻辑")
	assert.Contains(t, formatted, "## 文件变更")
	assert.Contains(t, formatted, "main.go")
	assert.Contains(t, formatted, "## 待处理")
	assert.Contains(t, formatted, "添加文档")
}

func TestParseSummaryJSON(t *testing.T) {
	jsonText := `{
		"goal": "实现新功能",
		"completed": ["步骤1", "步骤2"],
		"file_changes": ["file1.go", "file2.go"],
		"pending": ["待办1"],
		"message_count": 5
	}`

	summary, err := ParseSummaryJSON(jsonText)
	require.NoError(t, err)

	assert.Equal(t, "实现新功能", summary.Goal)
	assert.Equal(t, []string{"步骤1", "步骤2"}, summary.Completed)
	assert.Equal(t, []string{"file1.go", "file2.go"}, summary.FileChanges)
	assert.Equal(t, []string{"待办1"}, summary.Pending)
	assert.Equal(t, 5, summary.MessageCount)
}

func TestParseSummaryJSON_InvalidJSON(t *testing.T) {
	_, err := ParseSummaryJSON("invalid json")
	assert.Error(t, err)
}

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{1000, "1000"},
		{-5, "-5"},
		{-123, "-123"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// 基准测试：对比 tiktoken 和启发式估算的性能
func BenchmarkCountText_Tiktoken(b *testing.B) {
	tc, _ := NewTokenCounter()
	text := "Hello, this is a long message with multiple words and sentences. It should be representative of typical chat content."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CountText(text)
	}
}

func BenchmarkEstimateTokens_Heuristic(b *testing.B) {
	text := "Hello, this is a long message with multiple words and sentences. It should be representative of typical chat content."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(text)
	}
}

func BenchmarkCountMessages_Tiktoken(b *testing.B) {
	tc, _ := NewTokenCounter()
	messages := make([]MessageWithTools, 50)
	for i := range messages {
		messages[i] = MessageWithTools{
			Role:    "user",
			Content: NewTextContent("This is a test message with some content."),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CountMessages(messages)
	}
}

func BenchmarkEstimateMessagesTokens_Heuristic(b *testing.B) {
	messages := make([]MessageWithTools, 50)
	for i := range messages {
		messages[i] = MessageWithTools{
			Role:    "user",
			Content: NewTextContent("This is a test message with some content."),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateMessagesTokens(messages)
	}
}

// --- 缓存功能测试 ---

func TestCountMessageWithCache(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Hello, this is a test message."),
	}

	// 第一次调用应该缓存未命中
	count1 := tc.CountMessageWithCache(msg)
	stats1 := tc.GetCacheStats()
	assert.Equal(t, int64(0), stats1.Hits)
	assert.Equal(t, int64(1), stats1.Misses)
	assert.Equal(t, 1, stats1.Size)

	// 第二次调用相同消息应该缓存命中
	count2 := tc.CountMessageWithCache(msg)
	stats2 := tc.GetCacheStats()
	assert.Equal(t, count1, count2)
	assert.Equal(t, int64(1), stats2.Hits)
	assert.Equal(t, int64(1), stats2.Misses)
	assert.Equal(t, 1, stats2.Size)
}

func TestCountMessageWithCache_DifferentMessages(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg1 := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("First message"),
	}

	msg2 := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Second message"),
	}

	// 两条不同消息
	tc.CountMessageWithCache(msg1)
	tc.CountMessageWithCache(msg2)
	tc.CountMessageWithCache(msg1) // 重复第一条
	tc.CountMessageWithCache(msg2) // 重复第二条

	stats := tc.GetCacheStats()
	assert.Equal(t, int64(2), stats.Hits)   // msg1 和 msg2 各命中一次
	assert.Equal(t, int64(2), stats.Misses) // msg1 和 msg2 各未命中一次
	assert.Equal(t, 2, stats.Size)
}

func TestCacheStats_Reset(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Test"),
	}

	tc.CountMessageWithCache(msg)
	tc.CountMessageWithCache(msg)

	stats := tc.GetCacheStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)

	// 重置统计
	tc.ResetCacheStats()
	stats = tc.GetCacheStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 1, stats.Size) // 缓存内容仍然存在

	// 再次访问应该命中（因为缓存未清空）
	tc.CountMessageWithCache(msg)
	stats = tc.GetCacheStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
}

func TestClearCache(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Test"),
	}

	tc.CountMessageWithCache(msg)
	tc.ClearCache()

	stats := tc.GetCacheStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0, stats.Size)

	// 清空后再访问应该未命中
	tc.CountMessageWithCache(msg)
	stats = tc.GetCacheStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestHashMessage_Consistency(t *testing.T) {
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Hello world"),
	}

	// 相同消息应该生成相同哈希
	hash1 := hashMessage(msg)
	hash2 := hashMessage(msg)
	assert.Equal(t, hash1, hash2)

	// 不同内容应该生成不同哈希
	msg2 := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("Different content"),
	}
	hash3 := hashMessage(msg2)
	assert.NotEqual(t, hash1, hash3)

	// 不同角色应该生成不同哈希
	msg3 := MessageWithTools{
		Role:    "assistant",
		Content: NewTextContent("Hello world"),
	}
	hash4 := hashMessage(msg3)
	assert.NotEqual(t, hash1, hash4)
}

func TestHashMessage_WithToolCalls(t *testing.T) {
	msg1 := MessageWithTools{
		Role:    "assistant",
		Content: NewTextContent("Let me search."),
		ToolCalls: []ToolCall{
			{
				ID:        "call_123",
				Name:      "search",
				Arguments: json.RawMessage(`{"query": "test"}`),
			},
		},
	}

	msg2 := MessageWithTools{
		Role:    "assistant",
		Content: NewTextContent("Let me search."),
		ToolCalls: []ToolCall{
			{
				ID:        "call_456", // 不同 ID
				Name:      "search",
				Arguments: json.RawMessage(`{"query": "test"}`),
			},
		},
	}

	hash1 := hashMessage(msg1)
	hash2 := hashMessage(msg2)
	assert.NotEqual(t, hash1, hash2, "不同的 tool call ID 应该生成不同哈希")
}

func TestCacheSize_Custom(t *testing.T) {
	tc, err := NewTokenCounterWithCacheSize(2)
	require.NoError(t, err)

	msg1 := MessageWithTools{Role: "user", Content: NewTextContent("Message 1")}
	msg2 := MessageWithTools{Role: "user", Content: NewTextContent("Message 2")}
	msg3 := MessageWithTools{Role: "user", Content: NewTextContent("Message 3")}

	tc.CountMessageWithCache(msg1)
	tc.CountMessageWithCache(msg2)
	tc.CountMessageWithCache(msg3) // 应该驱逐 msg1

	stats := tc.GetCacheStats()
	assert.Equal(t, 2, stats.Size, "缓存大小应该受限")

	// 再次访问 msg1 应该未命中（已被驱逐）
	tc.ResetCacheStats()
	tc.CountMessageWithCache(msg1)
	stats = tc.GetCacheStats()
	assert.Equal(t, int64(1), stats.Misses, "被驱逐的消息应该重新未命中")
}

func TestCacheHitRate_HighRepetition(t *testing.T) {
	tc, err := NewTokenCounter()
	require.NoError(t, err)

	messages := []MessageWithTools{
		{Role: "user", Content: NewTextContent("Hello")},
		{Role: "assistant", Content: NewTextContent("Hi there!")},
		{Role: "user", Content: NewTextContent("How are you?")},
	}

	// 模拟重复会话场景
	for round := 0; round < 10; round++ {
		for _, msg := range messages {
			tc.CountMessageWithCache(msg)
		}
	}

	stats := tc.GetCacheStats()
	total := stats.Hits + stats.Misses
	hitRate := float64(stats.Hits) / float64(total) * 100

	t.Logf("缓存命中率: %.2f%% (命中: %d, 未命中: %d)",
		hitRate, stats.Hits, stats.Misses)

	// 预期命中率应该很高（前 3 次未命中，后 27 次全命中）
	// 命中率 = 27 / 30 = 90%
	assert.Greater(t, hitRate, 85.0, "高重复场景命中率应该 > 85%")
}

// --- 缓存性能基准测试 ---

func BenchmarkCountMessageWithCache_ColdCache(b *testing.B) {
	tc, _ := NewTokenCounter()
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("This is a benchmark test message with reasonable length."),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次清空缓存，模拟冷启动
		tc.ClearCache()
		tc.CountMessageWithCache(msg)
	}
}

func BenchmarkCountMessageWithCache_HotCache(b *testing.B) {
	tc, _ := NewTokenCounter()
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("This is a benchmark test message with reasonable length."),
	}

	// 预热缓存
	tc.CountMessageWithCache(msg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CountMessageWithCache(msg)
	}
}

func BenchmarkCountMessage_NoCache(b *testing.B) {
	tc, _ := NewTokenCounter()
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("This is a benchmark test message with reasonable length."),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CountMessage(msg)
	}
}

func BenchmarkCountMessagesWithCache_RepeatedMessages(b *testing.B) {
	tc, _ := NewTokenCounter()

	// 模拟真实场景：部分消息重复
	baseMessages := []MessageWithTools{
		{Role: "system", Content: NewTextContent("You are a helpful assistant.")},
		{Role: "user", Content: NewTextContent("Hello")},
		{Role: "assistant", Content: NewTextContent("Hi! How can I help you today?")},
	}

	messages := make([]MessageWithTools, 0, 50)
	for i := 0; i < 10; i++ {
		messages = append(messages, baseMessages...)
		messages = append(messages, MessageWithTools{
			Role:    "user",
			Content: NewTextContent(fmt.Sprintf("Question %d", i)),
		})
		messages = append(messages, MessageWithTools{
			Role:    "assistant",
			Content: NewTextContent(fmt.Sprintf("Answer %d", i)),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tc.CountMessages(messages)
	}
}

func BenchmarkHashMessage(b *testing.B) {
	msg := MessageWithTools{
		Role:    "user",
		Content: NewTextContent("This is a test message for hashing benchmark."),
		ToolCalls: []ToolCall{
			{
				ID:        "call_123",
				Name:      "search",
				Arguments: json.RawMessage(`{"query": "test query with some parameters"}`),
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashMessage(msg)
	}
}
