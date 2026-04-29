package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// mockQuestionBridge 模拟 QuestionBridge
type mockQuestionBridge struct {
	answerFunc func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error)
}

func (m *mockQuestionBridge) AskQuestion(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
	if m.answerFunc != nil {
		return m.answerFunc(ctx, question, options, timeout)
	}
	return "默认回答", nil
}

func TestQuestionTool(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]any
		mockAnswer   func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error)
		wantError    bool
		wantContent  string
		wantTimeout  time.Duration
		wantQuestion string
		wantOptions  []string
	}{
		{
			name: "基本提问",
			input: map[string]any{
				"question": "你想要什么颜色？",
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "蓝色", nil
			},
			wantError:    false,
			wantContent:  "用户回答: 蓝色",
			wantTimeout:  300 * time.Second, // 默认超时改为 300 秒（5 分钟）
			wantQuestion: "你想要什么颜色？",
			wantOptions:  nil,
		},
		{
			name: "带预设选项的提问",
			input: map[string]any{
				"question": "是否继续？",
				"options":  []string{"是", "否"},
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "是", nil
			},
			wantError:    false,
			wantContent:  "用户回答: 是",
			wantTimeout:  300 * time.Second, // 默认超时改为 300 秒（5 分钟）
			wantQuestion: "是否继续？",
			wantOptions:  []string{"是", "否"},
		},
		{
			name: "自定义超时",
			input: map[string]any{
				"question": "请输入密码",
				"timeout":  120,
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "secret123", nil
			},
			wantError:    false,
			wantContent:  "用户回答: secret123",
			wantTimeout:  120 * time.Second,
			wantQuestion: "请输入密码",
		},
		{
			name: "超时限制（最大3600秒）",
			input: map[string]any{
				"question": "需要很长时间思考",
				"timeout":  5000, // 超过最大值
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "想好了", nil
			},
			wantError:    false,
			wantContent:  "用户回答: 想好了",
			wantTimeout:  3600 * time.Second, // 应该被限制为 3600（60 分钟）
			wantQuestion: "需要很长时间思考",
		},
		{
			name: "等待回答超时",
			input: map[string]any{
				"question": "这个问题没人回答",
				"timeout":  30,
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "", context.DeadlineExceeded
			},
			wantError:   true,
			wantContent: "等待用户回答超时",
		},
		{
			name: "提问失败",
			input: map[string]any{
				"question": "这个问题会出错",
			},
			mockAnswer: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
				return "", errors.New("网络错误")
			},
			wantError:   true,
			wantContent: "提问失败: 网络错误",
		},
		{
			name: "问题为空",
			input: map[string]any{
				"question": "",
			},
			wantError:   true,
			wantContent: "问题内容不能为空",
		},
		{
			name: "无效输入",
			input: map[string]any{
				// 缺少 required 字段 question
				"timeout": 60,
			},
			wantError:   true,
			wantContent: "问题内容不能为空",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			host := mcphost.NewHost(logger)

			// 创建 mock bridge
			mockBridge := &mockQuestionBridge{
				answerFunc: tt.mockAnswer,
			}

			// 注册工具
			registerQuestion(host, logger, mockBridge)

			// 序列化输入
			inputJSON, err := json.Marshal(tt.input)
			require.NoError(t, err)

			// 调用工具
			ctx := context.Background()
			result, err := host.ExecuteTool(ctx, "question", inputJSON)
			require.NoError(t, err)
			require.NotNil(t, result)

			// 验证结果
			if tt.wantError {
				assert.True(t, result.IsError, "期望工具返回错误")
			} else {
				assert.False(t, result.IsError, "期望工具成功")
			}

			// 检查内容
			var content string
			err = json.Unmarshal(result.Content, &content)
			require.NoError(t, err)
			if tt.wantContent != "" {
				assert.Contains(t, content, tt.wantContent)
			}

			// 验证传递给 mock 的参数（仅当有 mockAnswer 时）
			if tt.mockAnswer != nil && !tt.wantError {
				// 重新调用一次来验证参数
				var capturedTimeout time.Duration
				var capturedQuestion string
				var capturedOptions []string

				mockBridge.answerFunc = func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
					capturedQuestion = question
					capturedOptions = options
					capturedTimeout = timeout
					return "验证用", nil
				}

				_, _ = host.ExecuteTool(ctx, "question", inputJSON)

				if tt.wantQuestion != "" {
					assert.Equal(t, tt.wantQuestion, capturedQuestion)
				}
				if tt.wantOptions != nil {
					assert.Equal(t, tt.wantOptions, capturedOptions)
				}
				if tt.wantTimeout > 0 {
					assert.Equal(t, tt.wantTimeout, capturedTimeout)
				}
			}
		})
	}
}

func TestQuestionToolIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	host := mcphost.NewHost(logger)

	// 模拟用户回答的通道
	answerCh := make(chan string, 1)

	mockBridge := &mockQuestionBridge{
		answerFunc: func(ctx context.Context, question string, options []string, timeout time.Duration) (string, error) {
			select {
			case answer := <-answerCh:
				return answer, nil
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(timeout):
				return "", context.DeadlineExceeded
			}
		},
	}

	registerQuestion(host, logger, mockBridge)

	t.Run("异步回答", func(t *testing.T) {
		input := map[string]any{
			"question": "你的名字是？",
			"timeout":  5,
		}
		inputJSON, _ := json.Marshal(input)

		// 在另一个 goroutine 中模拟用户回答
		go func() {
			time.Sleep(100 * time.Millisecond)
			answerCh <- "Alice"
		}()

		ctx := context.Background()
		result, err := host.ExecuteTool(ctx, "question", inputJSON)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		var content string
		_ = json.Unmarshal(result.Content, &content)
		assert.Contains(t, content, "Alice")
	})

	t.Run("上下文取消", func(t *testing.T) {
		input := map[string]any{
			"question": "这个问题会被取消",
			"timeout":  60,
		}
		inputJSON, _ := json.Marshal(input)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result, err := host.ExecuteTool(ctx, "question", inputJSON)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)

		var content string
		_ = json.Unmarshal(result.Content, &content)
		assert.Contains(t, content, "提问失败")
	})
}
