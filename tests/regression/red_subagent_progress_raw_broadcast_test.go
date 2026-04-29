// Package regression — session-scope-regression-matrix Phase 1.2 (R-1)
//
// Envelope invariant 红测：驱动 Master.CreateAgentProgressCallback，验证
// eventBus 广播的 BroadcastMessage.SessionID == ProgressEvent.SessionID。
// 若未来 PR 把 CreateAgentProgressCallback 从 BroadcastSessionMessage 改回
// 裸 Broadcast，envelope SessionID 会变空，本测试必红。
package regression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

func TestRedR1_SubagentProgress_EnvelopeSessionIDPreserved(t *testing.T) {
	logger := zap.NewNop()
	eb := master.NewEventBus(logger)
	defer eb.Close()

	m := master.NewForRegressionTest(logger, eb)

	subID, ch := m.SubscribeWSBroadcast()
	defer m.UnsubscribeWSBroadcast(subID)

	cb := m.CreateAgentProgressCallback()
	require.NotNil(t, cb)

	cb(subagent.ProgressEvent{
		AgentID:   "agent-X",
		SessionID: "sX",
		Status:    "tool_start",
		ToolName:  "fs_read",
	})

	select {
	case msg := <-ch:
		assert.Equal(t, master.EventTypeAgentProgress, msg.Type)
		assert.Equal(t, "sX", msg.SessionID,
			"R-1 regression: envelope SessionID 必须等于 ProgressEvent.SessionID — 若空，说明有人把 BroadcastSessionMessage 改回了裸 Broadcast")
	case <-time.After(time.Second):
		t.Fatal("等待 BroadcastMessage 超时（回调可能未执行 Broadcast 或路径错误）")
	}
}
