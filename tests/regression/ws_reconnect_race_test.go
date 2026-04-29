// Package regression — session-scope-regression-matrix Phase 3
//
// WebSocket reconnect race（Go 后端 envelope 层）：测试 eventBus 层在 subscriber
// 断开/重连抖动下仍保证
//   - Scenario A: envelope SessionID 不丢（每次重连后第一条消息的 envelope 正确）
//   - Scenario B: 重连后 sub-agent stream 的首个 chunk 能在 5s 内到达新订阅者
//   - 三方 race 变体：reconnect mid-emit / mid-loadMessages / mid-handleDisconnected
//
// 范围边界（codex Round 2 修订）：本文件只断言后端可观测量——envelope SessionID +
// recv queue 时序。前端 store spy（useChatStore / setCurrentSessionId(null) 等
// zustand 行为）由 frontend-ws-handshake-regression Phase 2 的 playwright spec
// 覆盖，运行在 session-scope-regression-matrix Phase 4 提供的共享 harness 里。
//
// frontend store spy delegated to frontend-ws-handshake-regression Phase 2
package regression

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/master"
)

// fakeWSClient 只模拟后端 envelope 收发：Subscribe 得到 chan，按真实 WS filter
// 丢掉 session-mismatch 的消息，通过的消息进 recv 队列。
// 不模拟任何前端 store / zustand 行为。
type fakeWSClient struct {
	eb        *master.EventBus
	sessionID string
	subID     uint64
	in        chan master.BroadcastMessage
	recv      []master.BroadcastMessage
	mu        sync.Mutex
	stopCh    chan struct{}
}

func newFakeWSClient(eb *master.EventBus, sessionID string) *fakeWSClient {
	subID, ch := eb.Subscribe()
	c := &fakeWSClient{
		eb:        eb,
		sessionID: sessionID,
		subID:     subID,
		in:        ch,
		stopCh:    make(chan struct{}),
	}
	go c.loop()
	return c
}

func (c *fakeWSClient) loop() {
	for {
		select {
		case <-c.stopCh:
			return
		case msg, ok := <-c.in:
			if !ok {
				return
			}
			// 复刻 internal/streaming/websocket.go:358-367 的 filter
			if msg.SessionID != "" && msg.SessionID != c.sessionID {
				continue
			}
			c.mu.Lock()
			c.recv = append(c.recv, msg)
			c.mu.Unlock()
		}
	}
}

func (c *fakeWSClient) disconnect() {
	c.eb.Unsubscribe(c.subID)
	close(c.stopCh)
}

func (c *fakeWSClient) snapshot() []master.BroadcastMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]master.BroadcastMessage, len(c.recv))
	copy(out, c.recv)
	return out
}

// waitRecvCount 等待 recv 达到指定长度或超时；返回是否达到
func (c *fakeWSClient) waitRecvCount(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		got := len(c.recv)
		c.mu.Unlock()
		if got >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestWSReconnect_EnvelopeSessionIDPreserved — Scenario A
// 3× disconnect/reconnect 循环，每次重连后立刻 BroadcastSessionMessage，断言
// envelope SessionID 永远等于 sessionID 参数（不受 subscriber churn 影响）。
func TestWSReconnect_EnvelopeSessionIDPreserved(t *testing.T) {
	logger := zap.NewNop()
	eb := master.NewEventBus(logger)
	defer eb.Close()

	const sid = "sX"

	for cycle := 0; cycle < 3; cycle++ {
		client := newFakeWSClient(eb, sid)

		eb.BroadcastSessionMessage(sid, master.BroadcastMessage{
			Type: master.EventTypeAgentProgress,
			Payload: map[string]interface{}{
				"cycle": cycle,
				"mark":  "post-reconnect-first-msg",
			},
		})

		require.True(t, client.waitRecvCount(1, 2*time.Second),
			"cycle %d 重连后首条消息 2s 内未到达", cycle)

		snap := client.snapshot()
		require.Len(t, snap, 1)
		assert.Equal(t, sid, snap[0].SessionID,
			"cycle %d envelope SessionID 丢失（实测=%q，应=%q）",
			cycle, snap[0].SessionID, sid)

		client.disconnect()
	}
}

// TestWSReconnect_StreamFirstChunkDelivery — Scenario B
// Master stream callback 在 reconnect 后 500ms 内发首 chunk，必须在 5s 内到达
// 新订阅者的 recv 队列。
func TestWSReconnect_StreamFirstChunkDelivery(t *testing.T) {
	logger := zap.NewNop()
	eb := master.NewEventBus(logger)
	defer eb.Close()
	m := master.NewForRegressionTest(logger, eb)

	const sid = "sY"

	// 先建连、断开，模拟 reconnect 前一次会话
	preClient := newFakeWSClient(eb, sid)
	preClient.disconnect()

	// 重连
	client := newFakeWSClient(eb, sid)
	defer client.disconnect()

	cb := m.CreateAgentStreamCallback()
	require.NotNil(t, cb)

	// fake LLM 500ms 后出首 chunk
	go func() {
		time.Sleep(500 * time.Millisecond)
		cb("agent-reconn", sid, "first chunk content", "")
	}()

	require.True(t, client.waitRecvCount(1, 5*time.Second),
		"重连后首 chunk 5s 内未到达——触发 WebSocket session-mismatch drop？")

	snap := client.snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, sid, snap[0].SessionID,
		"首 chunk envelope SessionID 不匹配")
	payload, ok := snap[0].Payload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "first chunk content", payload["content"])
}

// TestWSReconnect_RaceVariants — 3.4 三方 race 变体
func TestWSReconnect_RaceVariants(t *testing.T) {
	t.Run("reconnect_mid_emit", func(t *testing.T) {
		// 场景：一个 goroutine 正在连续 BroadcastSessionMessage，另一个 goroutine
		// 同时 disconnect + reconnect。断言新 subscriber 收到的所有消息 envelope
		// SessionID 正确，不会出现"因 emit 进行中而错传"的情况。
		logger := zap.NewNop()
		eb := master.NewEventBus(logger)
		defer eb.Close()

		const sid = "race-emit"
		var stopEmit atomic.Bool
		emitCount := int64(0)

		// 持续 emit goroutine
		go func() {
			for !stopEmit.Load() {
				eb.BroadcastSessionMessage(sid, master.BroadcastMessage{
					Type:    master.EventTypeAgentProgress,
					Payload: map[string]interface{}{"n": atomic.AddInt64(&emitCount, 1)},
				})
				time.Sleep(1 * time.Millisecond)
			}
		}()

		// 连着建立 5 次 reconnect，各收 ≥1 条
		for i := 0; i < 5; i++ {
			c := newFakeWSClient(eb, sid)
			if c.waitRecvCount(1, 2*time.Second) {
				for _, m := range c.snapshot() {
					assert.Equal(t, sid, m.SessionID,
						"mid-emit race cycle %d 出现 envelope SessionID 错乱", i)
				}
			}
			c.disconnect()
		}
		stopEmit.Store(true)
	})

	t.Run("reconnect_mid_loadMessages", func(t *testing.T) {
		// 场景：subscriber 正在"加载历史消息"（我们用 sleep 模拟），这时 emit 持续
		// 发生。断言 subscribe 动作完成后能接到紧随的消息，不因"加载中"丢 envelope。
		logger := zap.NewNop()
		eb := master.NewEventBus(logger)
		defer eb.Close()

		const sid = "race-load"

		// 模拟：建连 → 模拟加载 30ms → 再建连收第一个 emit
		preClient := newFakeWSClient(eb, sid)
		time.Sleep(30 * time.Millisecond) // 模拟 loadMessages
		preClient.disconnect()

		client := newFakeWSClient(eb, sid)
		defer client.disconnect()

		eb.BroadcastSessionMessage(sid, master.BroadcastMessage{
			Type:    master.EventTypeMessage,
			Payload: map[string]interface{}{"post_load": true},
		})

		require.True(t, client.waitRecvCount(1, 2*time.Second),
			"loadMessages 后紧接的 emit 未到达 recv 队列")
		assert.Equal(t, sid, client.snapshot()[0].SessionID)
	})

	t.Run("reconnect_mid_handleDisconnected", func(t *testing.T) {
		// 场景：quick disconnect→reconnect 循环（modelling handleDisconnected
		// 处理期间又一次 reconnect）。断言最后一个 subscriber 接到的 envelope 正确。
		logger := zap.NewNop()
		eb := master.NewEventBus(logger)
		defer eb.Close()

		const sid = "race-disc"
		// 连续 quick cycle
		for i := 0; i < 4; i++ {
			c := newFakeWSClient(eb, sid)
			time.Sleep(2 * time.Millisecond)
			c.disconnect()
		}

		// 最终 subscriber
		final := newFakeWSClient(eb, sid)
		defer final.disconnect()

		eb.BroadcastSessionMessage(sid, master.BroadcastMessage{
			Type:    master.EventTypeError,
			Payload: map[string]interface{}{"final": true},
		})

		require.True(t, final.waitRecvCount(1, 2*time.Second),
			"quick cycle 后 final subscriber 未收到消息")
		assert.Equal(t, sid, final.snapshot()[0].SessionID)
	})
}
