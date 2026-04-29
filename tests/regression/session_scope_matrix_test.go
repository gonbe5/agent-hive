// Package regression — session-scope-regression-matrix Phase 2
//
// Cross-session penetration matrix：N 个 session × M 类事件 × N 个订阅者，
// 断言 zero penetration（除匹配 sessionID 外，任何订阅者都不得收到该事件）。
// 订阅者过滤逻辑复刻 internal/streaming/websocket.go:358-367 的真实契约：
//   - broadcastMsg.SessionID == "" → 转发（生命周期事件，不在矩阵范围）
//   - broadcastMsg.SessionID != "" 且 != conn sessionID → drop
//   - 匹配 → 转发
//
// 设计要点：
//  1. N=3 含 same-UserID/different-SessionID 配对（sA/sB 都属 u1），覆盖 spec
//     要求的 "same-UserID isolation scenario"
//  2. M=7 覆盖所有 session-scoped 事件类型（AgentProgress/Message/ToolCall/
//     AgentStatus/Error/InputRequest/SpecContinuationAmbiguous）
//  3. 直接驱动 eventBus.BroadcastSessionMessage，不拉起 Master；测试的是
//     "envelope SessionID + subscriber filter" 两层组合契约
//  4. 性能目标：矩阵总耗时 < 30s（spec 性能约束）
package regression

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
)

// sessionFixture 模拟一个 WS 连接订阅者：持有 sessionID，按真实 WS filter 语义
// 过滤 eventBus 流。received 收集所有通过 filter 的消息（测试断言用）。
type sessionFixture struct {
	userID    string
	sessionID string
	subID     uint64
	inCh      chan master.BroadcastMessage
	received  []master.BroadcastMessage
}

// filterLoop 按 internal/streaming/websocket.go:358-367 的真实语义消费 inCh，
// 只把通过 filter 的消息追加到 received。test-goroutine 间同步由外层 matrix
// runner 的 settleDelay + mutex 保证。
func (f *sessionFixture) filterLoop(done <-chan struct{}, receivedCh chan<- master.BroadcastMessage) {
	for {
		select {
		case <-done:
			return
		case msg, ok := <-f.inCh:
			if !ok {
				return
			}
			// 复刻 WS filter：SessionID 空 → 转发（本矩阵里没这种事件，走不到这里）
			// SessionID 非空 → 仅 sessionID 匹配时转发
			if msg.SessionID != "" && msg.SessionID != f.sessionID {
				continue // session-mismatch drop
			}
			receivedCh <- msg
		}
	}
}

func TestSessionScopeMatrix_ZeroPenetration(t *testing.T) {
	start := time.Now()
	logger := zap.NewNop()
	eb := master.NewEventBus(logger)
	defer eb.Close()

	// N=3 sessions：sA/sB 同 user（验证 same-UserID isolation），sC 另一 user
	sessions := []*sessionFixture{
		{userID: "u1", sessionID: "sA"},
		{userID: "u1", sessionID: "sB"},
		{userID: "u2", sessionID: "sC"},
	}

	// 为每个 session 起一个 Subscribe + filterLoop goroutine
	done := make(chan struct{})
	defer close(done)

	for _, s := range sessions {
		id, ch := eb.Subscribe()
		s.subID = id
		s.inCh = ch
	}
	defer func() {
		for _, s := range sessions {
			eb.Unsubscribe(s.subID)
		}
	}()

	// 每个 fixture 有自己的 receivedCh；filterLoop 把通过 filter 的消息推进来
	type namedMsg struct {
		owner *sessionFixture
		msg   master.BroadcastMessage
	}
	allReceivedCh := make(chan namedMsg, 256)

	for _, s := range sessions {
		s := s // capture
		go func() {
			for {
				select {
				case <-done:
					return
				case msg, ok := <-s.inCh:
					if !ok {
						return
					}
					if msg.SessionID != "" && msg.SessionID != s.sessionID {
						continue
					}
					allReceivedCh <- namedMsg{owner: s, msg: msg}
				}
			}
		}()
	}

	// M=7 session-scoped event types（生产中均走 BroadcastSessionMessage）
	eventTypes := []string{
		master.EventTypeAgentProgress,
		master.EventTypeMessage,
		master.EventTypeToolCall,
		master.EventTypeAgentStatus,
		master.EventTypeError,
		master.EventTypeInputRequest,
		master.EventTypeSpecContinuationAmbiguous,
	}

	// 为每 (emitter, type) 组合生成唯一 marker，用于订阅端去重识别
	type cellKey struct {
		emitterSessionID string
		eventType        string
	}
	expected := map[cellKey]*sessionFixture{}

	// 发送矩阵：3 emitters × 7 types = 21 messages
	for _, emitter := range sessions {
		for _, et := range eventTypes {
			key := cellKey{emitter.sessionID, et}
			expected[key] = emitter
			eb.BroadcastSessionMessage(emitter.sessionID, master.BroadcastMessage{
				Type: et,
				Payload: map[string]interface{}{
					"cell_key":    emitter.sessionID + "/" + et,
					"emitter_sid": emitter.sessionID,
				},
			})
		}
	}

	// 等待所有消息扩散完毕（eventBus 同步 broadcast 后 filterLoop 是 goroutine）
	settle := 200 * time.Millisecond
	timer := time.NewTimer(settle)
	defer timer.Stop()

drain:
	for {
		select {
		case nm := <-allReceivedCh:
			nm.owner.received = append(nm.owner.received, nm.msg)
		case <-timer.C:
			break drain
		}
	}

	// ========== 断言 1: Zero-penetration ==========
	// 对每个 (emitter, type) cell，仅 emitter.sessionID 对应的订阅者可收到该消息
	totalCells := 0
	for _, emitter := range sessions {
		for _, et := range eventTypes {
			totalCells++
			for _, sub := range sessions {
				matched := findCell(sub.received, emitter.sessionID, et)
				if sub.sessionID == emitter.sessionID {
					assert.True(t, matched,
						"expect session=%s to receive own event [emitter=%s type=%s]",
						sub.sessionID, emitter.sessionID, et)
				} else {
					assert.False(t, matched,
						"CROSS-SESSION LEAK: session=%s (user=%s) received event from session=%s (user=%s) type=%s",
						sub.sessionID, sub.userID, emitter.sessionID, emitter.userID, et)
				}
			}
		}
	}

	// ========== 断言 2: Same-UserID isolation ==========
	// sA 和 sB 都是 u1，但 session 级别必须隔离。把前面的结果按这个维度再过一遍
	// 以明确输出失败信息（spec 要求覆盖此 scenario）
	require.Equal(t, sessions[0].userID, sessions[1].userID, "sA/sB 测试前提：同 user")
	for _, et := range eventTypes {
		// sA emit 的 event，sB（同 user）必须没收到
		assert.False(t,
			findCell(sessions[1].received, sessions[0].sessionID, et),
			"same-UserID leak: sB received sA's %s event", et)
		// sB emit 的 event，sA 必须没收到
		assert.False(t,
			findCell(sessions[0].received, sessions[1].sessionID, et),
			"same-UserID leak: sA received sB's %s event", et)
	}

	// ========== 性能观测 ==========
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 30*time.Second,
		"matrix 总耗时 %v 超过 30s，若持续超标需要 share subscriber fixture 或降维", elapsed)
	t.Logf("matrix done: N=%d sessions × M=%d types = %d cells, elapsed=%v",
		len(sessions), len(eventTypes), totalCells, elapsed)
}

// findCell 在 received 里查找 emitter_sid + type 匹配的 cell（依赖 payload.cell_key）
func findCell(received []master.BroadcastMessage, emitterSID, eventType string) bool {
	want := emitterSID + "/" + eventType
	for _, m := range received {
		if m.Type != eventType {
			continue
		}
		p, ok := m.Payload.(map[string]interface{})
		if !ok {
			continue
		}
		if p["cell_key"] == want {
			return true
		}
	}
	return false
}
