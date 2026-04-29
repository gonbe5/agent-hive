package master

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/subagent"
)

// subagent-session-scoping spec contract test:
// 验证 CreateAgentProgressCallback 返回的回调把 ProgressEvent.SessionID 落到
// BroadcastMessage.SessionID envelope —— 这是跨租户隔离的根本保护。
// 反向验证：手工 revert master.go:573-587 的 BroadcastSessionMessage 改回 Broadcast，
// 此测试必红。
func TestMaster_CreateAgentProgressCallback_PropagatesSessionID(t *testing.T) {
	logger := zap.NewNop()
	eventBus := NewEventBus(logger)
	defer eventBus.Close()

	m := &Master{
		eventBus: eventBus,
		logger:   logger,
	}

	// 订阅 EventBus 拿广播
	subID, ch := eventBus.Subscribe()
	defer eventBus.Unsubscribe(subID)

	cb := m.CreateAgentProgressCallback()
	require.NotNil(t, cb)

	// 触发回调，模拟 subagent 进度事件
	cb(subagent.ProgressEvent{
		AgentID:   "test-agent",
		SessionID: "session-A",
		Turn:      0,
		MaxTurns:  25,
		ToolName:  "fs_read",
		Status:    "tool_start",
	})

	// 验证 BroadcastMessage envelope 携带 SessionID
	select {
	case msg := <-ch:
		assert.Equal(t, EventTypeAgentProgress, msg.Type)
		assert.Equal(t, "session-A", msg.SessionID,
			"BroadcastMessage.SessionID 必须等于 ProgressEvent.SessionID（跨租户隔离根契约）")
	case <-time.After(time.Second):
		t.Fatal("等待 BroadcastMessage 超时（回调可能未执行 Broadcast 或路径错误）")
	}
}

// subagent-session-scoping spec contract test:
// 验证 CreateAgentStreamCallback 返回的回调把 sessionID 参数落到
// BroadcastMessage.SessionID envelope，并写入 payload.session_id。
func TestMaster_CreateAgentStreamCallback_PropagatesSessionID(t *testing.T) {
	logger := zap.NewNop()
	eventBus := NewEventBus(logger)
	defer eventBus.Close()

	m := &Master{
		eventBus: eventBus,
		logger:   logger,
	}

	subID, ch := eventBus.Subscribe()
	defer eventBus.Unsubscribe(subID)

	cb := m.CreateAgentStreamCallback()
	require.NotNil(t, cb)

	cb("test-agent", "session-B", "hello world", "thinking...")

	select {
	case msg := <-ch:
		assert.Equal(t, EventTypeAgentProgress, msg.Type)
		assert.Equal(t, "session-B", msg.SessionID,
			"BroadcastMessage.SessionID 必须等于 sessionID 参数")

		payload, ok := msg.Payload.(map[string]interface{})
		require.True(t, ok, "payload 应为 map[string]interface{}")
		assert.Equal(t, "test-agent", payload["agent_id"])
		assert.Equal(t, "session-B", payload["session_id"],
			"payload 也应携带 session_id 给前端兜底")
		assert.Equal(t, "hello world", payload["content"])
		assert.Equal(t, "thinking...", payload["reasoning_content"])
		assert.Equal(t, "streaming", payload["status"])
	case <-time.After(time.Second):
		t.Fatal("等待 stream BroadcastMessage 超时")
	}
}

// subagent-session-scoping spec contract test:
// 跨租户零渗透 —— session A 触发的 progress 事件，session B 的订阅者必须能在
// SessionID 字段上识别出"非我"并 drop（具体 drop 由 IM EventRenderer / WS writeLoop
// 实现，本测试只验证 envelope 字段被正确填充以支持下游过滤）。
func TestMaster_CreateAgentProgressCallback_NoCrossTenantLeak(t *testing.T) {
	logger := zap.NewNop()
	eventBus := NewEventBus(logger)
	defer eventBus.Close()

	m := &Master{
		eventBus: eventBus,
		logger:   logger,
	}

	subID, ch := eventBus.Subscribe()
	defer eventBus.Unsubscribe(subID)

	cb := m.CreateAgentProgressCallback()

	// session A 触发
	cb(subagent.ProgressEvent{AgentID: "agent-A", SessionID: "sA", Status: "tool_start"})
	// session B 触发
	cb(subagent.ProgressEvent{AgentID: "agent-B", SessionID: "sB", Status: "tool_start"})

	got := make(map[string]string) // sessionID -> agentID
	for i := 0; i < 2; i++ {
		select {
		case msg := <-ch:
			payload, _ := msg.Payload.(AgentProgressEvent)
			got[msg.SessionID] = payload.AgentID
		case <-time.After(time.Second):
			t.Fatalf("等待第 %d 条 BroadcastMessage 超时", i+1)
		}
	}

	assert.Equal(t, "agent-A", got["sA"], "sA envelope 应包含 agent-A")
	assert.Equal(t, "agent-B", got["sB"], "sB envelope 应包含 agent-B")
	assert.NotEqual(t, got["sA"], got["sB"], "两个 session 的 envelope 严禁混用")
}
