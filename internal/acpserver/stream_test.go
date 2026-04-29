package acpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chef-guo/agents-hive/internal/master"
)

func TestConvertToACPUpdates(t *testing.T) {
	tests := []struct {
		name      string
		msg       master.BroadcastMessage
		wantEmpty bool
	}{
		{
			name:      "空消息类型不产生更新",
			msg:       master.BroadcastMessage{Type: "unknown"},
			wantEmpty: true,
		},
		{
			name: "文本消息产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeMessage,
				Payload: "hello world",
			},
			wantEmpty: false,
		},
		{
			name: "空文本消息不产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeMessage,
				Payload: "",
			},
			wantEmpty: true,
		},
		{
			name: "工具调用消息产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeToolCall,
				Payload: map[string]interface{}{"tool_name": "bash"},
			},
			wantEmpty: false,
		},
		{
			name: "Agent 启动事件产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeAgentStart,
				Payload: map[string]interface{}{"agent_name": "explore"},
			},
			wantEmpty: false,
		},
		{
			name: "Skill 执行事件产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeSkillExec,
				Payload: map[string]interface{}{"skill_name": "review"},
			},
			wantEmpty: false,
		},
		{
			name: "错误事件产生更新",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeError,
				Payload: "执行失败",
			},
			wantEmpty: false,
		},
		{
			name: "Agent 启动事件使用结构体类型",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeAgentStart,
				Payload: master.AgentStartEvent{AgentName: "plan", TaskDesc: "制定计划"},
			},
			wantEmpty: false,
		},
		{
			name: "Skill 执行事件使用结构体类型",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeSkillExec,
				Payload: master.SkillExecEvent{SkillName: "deploy", Args: "--prod"},
			},
			wantEmpty: false,
		},
		{
			name: "工具调用事件使用结构体类型",
			msg: master.BroadcastMessage{
				Type:    master.EventTypeToolCall,
				Payload: master.ToolCallEvent{ToolName: "read_file", Status: "start"},
			},
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := convertToACPUpdates(tt.msg)
			if tt.wantEmpty {
				assert.Empty(t, updates)
			} else {
				assert.NotEmpty(t, updates)
			}
		})
	}
}

func TestExtractMessageText(t *testing.T) {
	tests := []struct {
		name     string
		payload  interface{}
		wantText string
	}{
		{"字符串载荷", "hello", "hello"},
		{"map 含 text 字段", map[string]interface{}{"text": "world"}, "world"},
		{"map 含 message 字段", map[string]interface{}{"message": "msg"}, "msg"},
		{"nil 载荷", nil, ""},
		{"空字符串", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMessageText(tt.payload)
			assert.Equal(t, tt.wantText, got)
		})
	}
}
