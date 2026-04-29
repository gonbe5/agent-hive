package cli

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chef-guo/agents-hive/internal/master"
)

// captureOutput 捕获 stdout 输出
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// captureStderr 捕获 stderr 输出
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestFormatAndPrintEvent_ToolCall(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeToolCall,
		Payload: master.ToolCallEvent{
			ToolName: "read_file",
			Status:   "start",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "🔧 调用工具: read_file")
}

func TestFormatAndPrintEvent_ToolCallNoArgs(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeToolCall,
		Payload: master.ToolCallEvent{
			ToolName: "list_files",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "🔧 工具: list_files")
	assert.NotContains(t, output, "参数:")
}

func TestFormatAndPrintEvent_AgentStart(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeAgentStart,
		Payload: master.AgentStartEvent{
			AgentName: "research",
			TaskDesc:  "分析项目架构",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "🤖 启动 Agent: research")
	assert.Contains(t, output, "任务: 分析项目架构")
}

func TestFormatAndPrintEvent_AgentStartNoTask(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeAgentStart,
		Payload: master.AgentStartEvent{
			AgentName: "explore",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "🤖 启动 Agent: explore")
	assert.NotContains(t, output, "任务:")
}

func TestFormatAndPrintEvent_SkillExec(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeSkillExec,
		Payload: master.SkillExecEvent{
			SkillName: "commit",
			Args:      "-m 'fix bug'",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "✨ 执行 Skill: commit")
	assert.Contains(t, output, "参数: -m 'fix bug'")
}

func TestFormatAndPrintEvent_Error(t *testing.T) {
	msg := master.BroadcastMessage{
		Type:    master.EventTypeError,
		Payload: "连接失败",
	}

	output := captureStderr(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "❌ 错误: 连接失败")
}

func TestFormatAndPrintEvent_Message(t *testing.T) {
	msg := master.BroadcastMessage{
		Type:    master.EventTypeMessage,
		Payload: "操作完成",
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "操作完成")
}

func TestFormatAndPrintEvent_UnknownType(t *testing.T) {
	msg := master.BroadcastMessage{
		Type:    "unknown_type",
		Payload: "some data",
	}

	// 未知类型应该被忽略，不应该有输出
	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Empty(t, output)
}

func TestFormatAndPrintEvent_InvalidPayload(t *testing.T) {
	// 测试 payload 类型不匹配的情况
	msg := master.BroadcastMessage{
		Type:    master.EventTypeToolCall,
		Payload: "invalid payload", // 应该是 ToolCallEvent
	}

	// 不应该 panic，应该优雅地忽略
	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	// 因为类型断言失败，不应该有输出
	assert.Empty(t, output)
}

func TestFormatAndPrintEvent_TaskGroup(t *testing.T) {
	tests := []struct {
		name    string
		evt     master.TaskGroupEvent
		wantOut string
	}{
		{
			name: "任务组开始",
			evt: master.TaskGroupEvent{
				GroupID: "g-1",
				Status:  "started",
				Total:   5,
			},
			wantOut: "🚀 并行任务组 (5 个任务)",
		},
		{
			name: "任务组完成",
			evt: master.TaskGroupEvent{
				GroupID:   "g-1",
				Status:    "completed",
				Total:     5,
				Completed: 5,
			},
			wantOut: "✅ 并行任务组完成 (5/5)",
		},
		{
			name: "任务组失败",
			evt: master.TaskGroupEvent{
				GroupID:   "g-1",
				Status:    "failed",
				Total:     5,
				Completed: 3,
			},
			wantOut: "❌ 并行任务组失败 (3/5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := master.BroadcastMessage{
				Type:    master.EventTypeTaskGroup,
				Payload: tt.evt,
			}

			output := captureOutput(func() {
				formatAndPrintEvent(msg)
			})

			assert.Contains(t, output, tt.wantOut)
		})
	}
}

func TestFormatAndPrintEvent_TaskProgress(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeTaskProgress,
		Payload: map[string]string{
			"task_id": "task-42",
			"status":  "running",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "任务 task-42: running")
}

func TestFormatAndPrintEvent_TaskProgressWithError(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeTaskProgress,
		Payload: map[string]string{
			"task_id": "task-7",
			"status":  "failed",
			"error":   "超时",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "任务 task-7: failed")
	assert.Contains(t, output, "超时")
}

func TestFormatAndPrintEvent_AgentProgress(t *testing.T) {
	msg := master.BroadcastMessage{
		Type: master.EventTypeAgentProgress,
		Payload: map[string]interface{}{
			"tool_name": "write_file",
			"agent_id":  "explore",
			"turn":      float64(3),
			"max_turns": float64(10),
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "write_file")
	assert.Contains(t, output, "explore")
	assert.Contains(t, output, "3/10")
}

func TestFormatAndPrintEvent_AgentProgressNoTurn(t *testing.T) {
	// 不含 turn 信息时应正常显示，不带轮次
	msg := master.BroadcastMessage{
		Type: master.EventTypeAgentProgress,
		Payload: map[string]interface{}{
			"tool_name": "bash",
			"agent_id":  "builder",
		},
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Contains(t, output, "bash")
	assert.Contains(t, output, "builder")
	assert.NotContains(t, output, "/")
}

func TestFormatAndPrintEvent_EventTypeEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload interface{}
		wantOut string
	}{
		{
			name: "带 message 字段的 map payload",
			payload: map[string]interface{}{
				"message": "系统初始化完成",
			},
			wantOut: "📢 事件: 系统初始化完成",
		},
		{
			name: "无 message 字段的 map payload",
			payload: map[string]interface{}{
				"type": "heartbeat",
			},
			wantOut: "📢 收到事件通知",
		},
		{
			name:    "字符串 payload",
			payload: "连接成功",
			wantOut: "📢 事件: 连接成功",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := master.BroadcastMessage{
				Type:    master.EventTypeEvent,
				Payload: tt.payload,
			}

			output := captureOutput(func() {
				formatAndPrintEvent(msg)
			})

			assert.Contains(t, output, tt.wantOut)
		})
	}
}

func TestFormatAndPrintEvent_EventTypeEvent_EmptyStringPayload(t *testing.T) {
	// 空字符串 payload 不应输出任何内容
	msg := master.BroadcastMessage{
		Type:    master.EventTypeEvent,
		Payload: "",
	}

	output := captureOutput(func() {
		formatAndPrintEvent(msg)
	})

	assert.Empty(t, output)
}

func TestFormatAndPrintEvent_ToolListChanged(t *testing.T) {
	tests := []struct {
		name      string
		payload   interface{}
		wantOut   string
		wantCount bool
	}{
		{
			name: "float64 类型的 tool_count（JSON 反序列化路径）",
			payload: map[string]interface{}{
				"tool_count": float64(12),
			},
			wantOut: "🔄 工具列表已更新 (当前共 12 个工具)",
		},
		{
			name: "int 类型的 tool_count（直接传入路径）",
			payload: map[string]interface{}{
				"tool_count": 8,
			},
			wantOut: "🔄 工具列表已更新 (当前共 8 个工具)",
		},
		{
			name:    "非 map payload（兜底分支）",
			payload: "unexpected",
			wantOut: "🔄 工具列表已更新",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := master.BroadcastMessage{
				Type:    master.EventTypeToolListChanged,
				Payload: tt.payload,
			}

			output := captureOutput(func() {
				formatAndPrintEvent(msg)
			})

			assert.Contains(t, output, tt.wantOut)
		})
	}
}
