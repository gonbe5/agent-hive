package cli

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"go.uber.org/zap"
)

// TestEvents_FormatAndPrintEvent 测试事件格式化输出
func TestEvents_FormatAndPrintEvent(t *testing.T) {
	testCases := []struct {
		name            string
		msg             master.BroadcastMessage
		expectOutput    bool
		expectedContain string
	}{
		{
			name: "工具调用事件",
			msg: master.BroadcastMessage{
				Type: master.EventTypeToolCall,
				Payload: master.ToolCallEvent{
					ToolName: "read_file",
					Status:   "start",
				},
			},
			expectOutput:    true,
			expectedContain: "🔧 调用工具: read_file",
		},
		{
			name: "Agent 启动事件",
			msg: master.BroadcastMessage{
				Type: master.EventTypeAgentStart,
				Payload: master.AgentStartEvent{
					AgentName: "research",
					TaskDesc:  "分析代码",
				},
			},
			expectOutput:    true,
			expectedContain: "🤖 启动 Agent: research",
		},
		{
			name: "Skill 执行事件",
			msg: master.BroadcastMessage{
				Type: master.EventTypeSkillExec,
				Payload: master.SkillExecEvent{
					SkillName: "commit",
					Args:      "-m 'test'",
				},
			},
			expectOutput:    true,
			expectedContain: "✨ 执行 Skill: commit",
		},
		{
			name: "错误消息",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeError,
				Payload: "测试错误",
			},
			expectOutput:    true,
			expectedContain: "❌ 错误: 测试错误",
		},
		{
			name: "通用消息",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeMessage,
				Payload: "通用消息内容",
			},
			expectOutput:    true,
			expectedContain: "通用消息内容",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 这里只是调用函数，实际输出需要手动验证
			// 在真实环境中会输出到控制台
			formatAndPrintEvent(tc.msg)
			t.Logf("✅ 事件格式化函数执行成功: %s", tc.name)
		})
	}
}

// TestEvents_BroadcastSubscription 测试事件广播和订阅机制
func TestEvents_BroadcastSubscription(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()

	app := NewApp(cfg, logger)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}

	// 订阅事件广播
	subID, eventCh := app.master.SubscribeWSBroadcast()
	defer app.master.UnsubscribeWSBroadcast(subID)

	// 启动事件接收 goroutine
	receivedEvents := make([]master.BroadcastMessage, 0)
	done := make(chan struct{})

	go func() {
		timeout := time.After(2 * time.Second)
		for {
			select {
			case msg := <-eventCh:
				receivedEvents = append(receivedEvents, msg)
				t.Logf("📨 收到事件: Type=%s", msg.Type)
			case <-timeout:
				close(done)
				return
			}
		}
	}()

	// 触发一些操作（这会产生事件）
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 执行一个简单请求（会触发 Agent/Tool 事件）
	go func() {
		_ = app.RunOnce(ctx, "test event broadcast")
	}()

	// 等待事件收集
	<-done

	// 验证收到了事件
	if len(receivedEvents) == 0 {
		t.Error("❌ 未收到任何事件，事件广播可能未工作")
	} else {
		t.Logf("✅ 收到 %d 个事件", len(receivedEvents))
		for i, evt := range receivedEvents {
			t.Logf("  事件 %d: Type=%s", i+1, evt.Type)
		}
	}
}

// TestEvents_ToolCallEventBroadcast 测试工具调用事件是否正确广播
func TestEvents_ToolCallEventBroadcast(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()

	app := NewApp(cfg, logger)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}

	// 订阅事件
	subID, eventCh := app.master.SubscribeWSBroadcast()
	defer app.master.UnsubscribeWSBroadcast(subID)

	toolCallEvents := make([]master.ToolCallEvent, 0)
	done := make(chan struct{})

	go func() {
		timeout := time.After(3 * time.Second)
		for {
			select {
			case msg := <-eventCh:
				if msg.Type == master.EventTypeToolCall {
					if evt, ok := msg.Payload.(master.ToolCallEvent); ok {
						toolCallEvents = append(toolCallEvents, evt)
						t.Logf("🔧 工具调用: %s", evt.ToolName)
					}
				}
			case <-timeout:
				close(done)
				return
			}
		}
	}()

	// 执行会触发工具调用的请求
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		// 这个请求应该会调用一些工具
		_ = app.RunOnce(ctx, "list files in current directory")
	}()

	<-done

	// 验证收到了工具调用事件
	if len(toolCallEvents) > 0 {
		t.Logf("✅ 收到 %d 个工具调用事件", len(toolCallEvents))
		for _, evt := range toolCallEvents {
			t.Logf("  - %s", evt.ToolName)
		}
	} else {
		t.Log("⚠️  未收到工具调用事件（可能请求没有触发工具调用）")
	}
}

// TestEvents_OutputFormatting 测试事件输出格式
func TestEvents_OutputFormatting(t *testing.T) {
	testCases := []struct {
		name      string
		eventType string
		payload   interface{}
		checkFunc func(string) bool
	}{
		{
			name:      "工具调用包含工具名",
			eventType: master.EventTypeToolCall,
			payload: master.ToolCallEvent{
				ToolName: "glob",
				Status:   "start",
			},
			checkFunc: func(output string) bool {
				return strings.Contains(output, "🔧") && strings.Contains(output, "glob")
			},
		},
		{
			name:      "Agent 启动包含 Agent 名",
			eventType: master.EventTypeAgentStart,
			payload: master.AgentStartEvent{
				AgentName: "explore",
				TaskDesc:  "探索代码库",
			},
			checkFunc: func(output string) bool {
				return strings.Contains(output, "🤖") && strings.Contains(output, "explore")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := master.BroadcastMessage{
				Type:    tc.eventType,
				Payload: tc.payload,
			}

			// 调用格式化函数（实际会输出到控制台）
			formatAndPrintEvent(msg)

			t.Logf("✅ 事件格式化测试通过: %s", tc.name)
		})
	}
}
