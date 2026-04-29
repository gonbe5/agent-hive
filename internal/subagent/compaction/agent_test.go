package compaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// ============================================================
// 辅助函数
// ============================================================

// waitForRunning 等待 agent 进入运行状态
func waitForRunning(t *testing.T, agent *Agent) {
	t.Helper()
	deadline := time.Now().Add(600 * time.Second)
	for time.Now().Before(deadline) {
		if agent.Status() == subagent.StatusRunning {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("等待 agent 启动超时")
}

// 创建默认测试配置（启发式计数，速度更快）
func defaultTestCfg() config.CompactionConfig {
	return config.CompactionConfig{
		Enabled:       true,
		Strategy:      config.StrategyTruncate,
		MaxTokens:     100,
		ReserveTokens: 50,
		UseTiktoken:   false,
		LazyMode:      true,
		LazyThreshold: 200,
	}
}

// 创建 N 条具有指定内容的消息
func makeMessages(n int, content string) []llm.MessageWithTools {
	msgs := make([]llm.MessageWithTools, n)
	for i := range msgs {
		msgs[i] = llm.MessageWithTools{
			Role:    "user",
			Content: llm.NewTextContent(content),
		}
	}
	return msgs
}

// ============================================================
// TestNew - 验证 agent 创建
// ============================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.CompactionConfig
		llmClient *llm.Client
	}{
		{
			name: "基本创建_无tiktoken",
			cfg: config.CompactionConfig{
				Enabled:     true,
				Strategy:    config.StrategyTruncate,
				MaxTokens:   1000,
				UseTiktoken: false,
			},
			llmClient: nil,
		},
		{
			name: "启用tiktoken",
			cfg: config.CompactionConfig{
				Enabled:     true,
				Strategy:    config.StrategyTruncate,
				MaxTokens:   1000,
				UseTiktoken: true,
			},
			llmClient: nil,
		},
		{
			name: "LLM摘要策略_无llmClient",
			cfg: config.CompactionConfig{
				Enabled:  true,
				Strategy: config.StrategyLLMSummary,
			},
			llmClient: nil,
		},
		{
			name: "禁用压缩",
			cfg: config.CompactionConfig{
				Enabled: false,
			},
			llmClient: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			agent := New(tt.llmClient, tt.cfg, logger)

			assert.NotNil(t, agent, "agent 不应为 nil")
			assert.Equal(t, "compaction", agent.ID(), "agent ID 应为 compaction")
			assert.Equal(t, tt.cfg, agent.cfg, "配置应匹配")
			assert.Equal(t, tt.llmClient, agent.llm, "llmClient 应匹配")
		})
	}
}

// ============================================================
// TestCompact_EmptyMessages - 空输入返回空输出
// ============================================================

func TestCompact_EmptyMessages(t *testing.T) {
	logger := zaptest.NewLogger(t)
	agent := New(nil, defaultTestCfg(), logger)

	result, stats := agent.compact(context.Background(), nil)

	assert.Empty(t, result, "空输入应返回空结果")
	assert.NotNil(t, stats, "stats 不应为 nil")
	assert.Equal(t, 0, stats.Original)
	assert.Equal(t, 0, stats.Remaining)
}

// ============================================================
// TestCompact_Disabled - 禁用压缩时消息原样通过
// ============================================================

func TestCompact_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := defaultTestCfg()
	cfg.Enabled = false
	agent := New(nil, cfg, logger)

	messages := makeMessages(5, "Hello world")

	result, stats := agent.compact(context.Background(), messages)

	assert.Equal(t, len(messages), len(result), "消息数量不应改变")
	assert.Equal(t, messages, result, "消息内容应完全相同")
	assert.Equal(t, 5, stats.Original)
	assert.Equal(t, 5, stats.Remaining)
	assert.Equal(t, 0, stats.Compressed)
}

// ============================================================
// TestCompact_LazySkipped - 懒惰模式下 token 低于阈值时跳过
// ============================================================

func TestCompact_LazySkipped(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := defaultTestCfg()
	cfg.LazyMode = true
	cfg.LazyThreshold = 99999 // 非常高的阈值，确保跳过
	agent := New(nil, cfg, logger)

	messages := makeMessages(3, "Short")

	result, stats := agent.compact(context.Background(), messages)

	assert.Equal(t, len(messages), len(result), "消息数量不应改变")
	assert.True(t, stats.LazySkipped, "应标记为懒惰跳过")
	assert.Equal(t, 3, stats.Original)
	assert.Equal(t, 3, stats.Remaining)
	assert.Greater(t, stats.OriginalToken, 0, "原始 token 数应大于 0")
}

// ============================================================
// TestCompact_UnderMaxTokens - token 未超过 MaxTokens 时不压缩
// ============================================================

func TestCompact_UnderMaxTokens(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := defaultTestCfg()
	cfg.LazyMode = false  // 禁用懒惰模式
	cfg.MaxTokens = 99999 // 非常高的上限
	agent := New(nil, cfg, logger)

	messages := makeMessages(5, "Hello")

	result, stats := agent.compact(context.Background(), messages)

	assert.Equal(t, len(messages), len(result), "消息数量不应改变")
	assert.Equal(t, 5, stats.Original)
	assert.Equal(t, 5, stats.Remaining)
	assert.Equal(t, 0, stats.Compressed)
	assert.False(t, stats.LazySkipped, "非懒惰模式不应标记 LazySkipped")
}

// ============================================================
// TestCompactTruncate - 截断策略验证
// ============================================================

func TestCompactTruncate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := defaultTestCfg()
	cfg.LazyMode = false
	cfg.MaxTokens = 200 // 较低的上限以触发截断
	agent := New(nil, cfg, logger)

	// 每条消息约 ~90+ token（启发式估算），20 条会远超 200
	messages := makeMessages(20, strings.Repeat("Hello world, this is a test message. ", 10))

	result, stats := agent.compact(context.Background(), messages)

	assert.Less(t, len(result), len(messages), "消息应被压缩")
	assert.Greater(t, stats.Compressed, 0, "应有消息被压缩")
	assert.Equal(t, string(config.StrategyTruncate), stats.Strategy)

	// 第一条应为系统摘要
	assert.Equal(t, "system", result[0].Role, "第一条应为系统摘要")
	assert.Contains(t, result[0].Content.Text(), "会话摘要", "摘要应包含标识文本")
}

// ============================================================
// TestHandleTask - 通过 SendTask 测试 handleTask
// ============================================================

func TestHandleTask(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := defaultTestCfg()
	cfg.Enabled = false // 禁用压缩以简化验证
	agent := New(nil, cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		agent.Run(ctx)
		close(done)
	}()
	// 确保测试结束前 agent.Run 完全退出，避免 zaptest logger 竞态
	defer func() { cancel(); <-done }()

	waitForRunning(t, agent)

	// 构造 CompactionRequest
	messages := makeMessages(3, "Test message")
	msgsJSON, err := json.Marshal(messages)
	require.NoError(t, err)

	compReq := CompactionRequest{Messages: msgsJSON}
	payload, err := json.Marshal(compReq)
	require.NoError(t, err)

	taskReq := subagent.TaskRequest{
		ID:      "test-task-1",
		Type:    "compact",
		Payload: payload,
	}

	resp, err := agent.SendTask(ctx, taskReq)
	require.NoError(t, err)

	assert.Equal(t, "completed", resp.Status, "任务应成功完成")
	assert.Empty(t, resp.Error, "不应有错误")

	// 解析结果
	var compResult CompactionResult
	err = json.Unmarshal(resp.Result, &compResult)
	require.NoError(t, err)

	assert.NotNil(t, compResult.Stats, "stats 不应为 nil")
	assert.Equal(t, 3, compResult.Stats.Original, "原始消息数应为 3")
	assert.Equal(t, 3, compResult.Stats.Remaining, "保留消息数应为 3（未压缩）")

	// 解析压缩后的消息
	var resultMsgs []llm.MessageWithTools
	err = json.Unmarshal(compResult.Messages, &resultMsgs)
	require.NoError(t, err)
	assert.Equal(t, 3, len(resultMsgs), "消息数量不应改变")
}

// ============================================================
// TestHandleTask_InvalidPayload - 无效 JSON 错误处理
// ============================================================

func TestHandleTask_InvalidPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		errMsg  string
	}{
		{
			name:    "无效JSON",
			payload: `{invalid json`,
			errMsg:  "解析压缩请求失败",
		},
		{
			name:    "messages字段为无效JSON",
			payload: `{"messages": "not_an_array"}`,
			errMsg:  "反序列化消息失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			agent := New(nil, defaultTestCfg(), logger)

			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan struct{})
			go func() {
				agent.Run(ctx)
				close(done)
			}()
			// 确保测试结束前 agent.Run 完全退出，避免 zaptest logger 竞态
			defer func() { cancel(); <-done }()

			waitForRunning(t, agent)

			taskReq := subagent.TaskRequest{
				ID:      "test-invalid",
				Type:    "compact",
				Payload: json.RawMessage(tt.payload),
			}

			resp, err := agent.SendTask(ctx, taskReq)
			require.NoError(t, err, "SendTask 本身不应返回错误")

			assert.Equal(t, "failed", resp.Status, "状态应为 failed")
			assert.Contains(t, resp.Error, tt.errMsg, "错误信息应包含预期文本")
		})
	}
}

// ============================================================
// TestGenerateSimpleSummary - 简单摘要生成测试
// ============================================================

func TestGenerateSimpleSummary(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.MessageWithTools
		checks   []string // 期望包含的子字符串
	}{
		{
			name:     "空消息列表",
			messages: nil,
			checks:   []string{"[会话摘要]", "无早期消息"},
		},
		{
			name: "少量消息",
			messages: []llm.MessageWithTools{
				{Role: "user", Content: llm.NewTextContent("你好")},
				{Role: "assistant", Content: llm.NewTextContent("你好！有什么帮助？")},
			},
			checks: []string{"[会话摘要]", "已压缩 2 条早期消息", "[user]", "[assistant]"},
		},
		{
			name:     "超过10条消息时应显示省略提示",
			messages: makeMessages(15, "消息内容"),
			checks:   []string{"[会话摘要]", "已压缩 15 条早期消息", "还有 5 条消息已省略"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateSimpleSummary(tt.messages)
			for _, check := range tt.checks {
				assert.Contains(t, summary, check, "摘要应包含: %s", check)
			}
		})
	}
}

// ============================================================
// TestTruncateContent - UTF-8 安全截断测试
// ============================================================

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxLen   int
		expected string
	}{
		{
			name:     "短内容_不截断",
			content:  "Hello",
			maxLen:   10,
			expected: "Hello",
		},
		{
			name:     "刚好长度_不截断",
			content:  "Hello",
			maxLen:   5,
			expected: "Hello",
		},
		{
			name:     "超长英文_截断",
			content:  "Hello World!",
			maxLen:   5,
			expected: "Hello...",
		},
		{
			name:     "中文内容_按rune截断",
			content:  "你好世界测试内容",
			maxLen:   4,
			expected: "你好世界...",
		},
		{
			name:     "空字符串",
			content:  "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "混合中英文",
			content:  "Hello你好World",
			maxLen:   7,
			expected: "Hello你好...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.content, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNew_WithCallbacks 验证构造函数正确设置 LLMComplete 回调
func TestNew_WithCallbacks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.CompactionConfig{Enabled: true}

	t.Run("no callbacks", func(t *testing.T) {
		agent := New(nil, cfg, logger)
		assert.Nil(t, agent.llmCompleteFn)
	})

	t.Run("with LLMCompleteFn", func(t *testing.T) {
		cb := subagent.AgentCallbacks{
			LLMCompleteFn: func(agentID, sessionID, userID, model string, usage llm.Usage) {},
		}
		agent := New(nil, cfg, logger, cb)
		assert.NotNil(t, agent.llmCompleteFn)
	})
}

// TestHandleTask_SessionIDPassthrough 验证 handleTask 从 TaskRequest 提取 sessionID
func TestHandleTask_SessionIDPassthrough(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := config.CompactionConfig{Enabled: false} // disabled 避免触发 LLM 调用
	agent := New(nil, cfg, logger)
	// 不需要 Start/waitForRunning，直接调用 handleTask

	msgs := []llm.MessageWithTools{
		{Role: "user", Content: llm.NewTextContent("hello")},
	}
	msgsJSON, _ := json.Marshal(msgs)
	payload, _ := json.Marshal(CompactionRequest{SessionID: "sess-compact-123", Messages: msgsJSON})

	agent.handleTask(context.Background(), subagent.TaskRequest{
		ID:        "req-1",
		SessionID: "sess-compact-123",
		Payload:   payload,
	})

	assert.Equal(t, "sess-compact-123", agent.sessionID, "sessionID should be extracted from TaskRequest")
}
