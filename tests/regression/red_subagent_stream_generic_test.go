// Package regression — session-scope-regression-matrix Phase 1.3 (R-2)
//
// Envelope invariant 红测：驱动 Master.CreateAgentStreamCallback，验证
// eventBus 广播的 BroadcastMessage.SessionID + payload["session_id"] 都等于
// callback 传入的 sessionID。若未来 PR 把 stream 路径从 BroadcastSessionMessage
// 改成 BroadcastGenericMessage，envelope SessionID 会变空，本测试必红。
package regression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
)

func TestRedR2_SubagentStream_EnvelopeAndPayloadSessionIDPreserved(t *testing.T) {
	logger := zap.NewNop()
	eb := master.NewEventBus(logger)
	defer eb.Close()

	m := master.NewForRegressionTest(logger, eb)

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	cb := m.CreateAgentStreamCallback()
	require.NotNil(t, cb)

	cb("agent-Y", "sY", "streamed chunk", "thinking...")

	select {
	case msg := <-ch:
		assert.Equal(t, master.EventTypeAgentProgress, msg.Type)
		assert.Equal(t, "sY", msg.SessionID,
			"R-2 regression: envelope SessionID 必须等于 sessionID 参数 — 若空，说明 stream 路径被改回 BroadcastGenericMessage")

		payload, ok := msg.Payload.(map[string]interface{})
		require.True(t, ok, "payload 应为 map[string]interface{}")
		assert.Equal(t, "sY", payload["session_id"],
			"R-2 regression: payload.session_id 必须同步携带，前端兜底依赖此字段")
		assert.Equal(t, "agent-Y", payload["agent_id"])
		assert.Equal(t, "streamed chunk", payload["content"])
	case <-time.After(time.Second):
		t.Fatal("等待 stream BroadcastMessage 超时")
	}
}
