package master

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// setupInputReceivedMaster 构建一个带内存 store 的 Master，
// 使 checkSessionAccess 对不存在的 session 走 ErrNotFound 放行路径，
// 从而让 ProcessMessageWithOptions 能推进到广播阶段。
func setupInputReceivedMaster(t *testing.T) (*Master, context.CancelFunc) {
	t.Helper()
	logger := zaptest.NewLogger(t)
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)
	st := store.NewMemoryStore()

	m := NewMaster(Config{Model: "test"}, config.HITLConfig{Enabled: false}, agentReg, skillReg, st, logger)

	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	return m, cancel
}

// TestMaster_InputReceivedBroadcast 验证 ProcessMessageWithOptions 在权限检查通过后、
// 委托给 SessionManager 之前广播 EventTypeInputReceived。
// 契约：Payload 必须是 InputReceivedEvent，SessionID 与 ChannelMessageID 透传 SessionRequest。
func TestMaster_InputReceivedBroadcast(t *testing.T) {
	m, cancelMaster := setupInputReceivedMaster(t)

	subID, ch := m.SubscribeWSBroadcast()

	const (
		sessionID = "test-session-input-received"
		chMsgID   = "im-msg-12345"
	)

	// ProcessMessageWithOptions 会同步推进到 sessionMgr.ProcessRequestWithResponse，
	// 该调用可能阻塞等待 LLM / 任务完成。我们只关心广播是否先于此发生，
	// 所以放到 goroutine 里跑；测试结束前先取消 reqCtx 让 goroutine 返回，
	// 再 Stop master，避免 channel send vs close 的 race。
	reqCtx, cancelReq := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = m.ProcessMessageWithOptions(reqCtx, sessionID, "hello",
			WithChannelMessageID(chMsgID),
		)
	})
	t.Cleanup(func() {
		cancelReq()
		wg.Wait()
		m.UnsubscribeWSBroadcast(subID)
		m.Stop()
		cancelMaster()
	})

	deadline := time.After(1 * time.Second)
	for {
		select {
		case msg := <-ch:
			if msg.Type != EventTypeInputReceived {
				continue
			}
			if msg.SessionID != sessionID {
				t.Fatalf("broadcast SessionID = %q, want %q", msg.SessionID, sessionID)
			}
			ev, ok := msg.Payload.(InputReceivedEvent)
			if !ok {
				t.Fatalf("payload type = %T, want InputReceivedEvent", msg.Payload)
			}
			if ev.SessionID != sessionID {
				t.Errorf("payload SessionID = %q, want %q", ev.SessionID, sessionID)
			}
			if ev.ChannelMessageID != chMsgID {
				t.Errorf("payload ChannelMessageID = %q, want %q", ev.ChannelMessageID, chMsgID)
			}
			return
		case <-deadline:
			t.Fatal("EventTypeInputReceived broadcast not received within 1s")
		}
	}
}

// TestMaster_InputReceivedBroadcast_EmptyChannelMessageID 验证非 IM 通道（不带 ChannelMessageID）时，
// input_received 事件仍然广播，Payload.ChannelMessageID 为空字符串。
// subscriber 端按空串跳过 ack 表情即可。
func TestMaster_InputReceivedBroadcast_EmptyChannelMessageID(t *testing.T) {
	m, cancelMaster := setupInputReceivedMaster(t)

	subID, ch := m.SubscribeWSBroadcast()

	const sessionID = "test-session-web"

	reqCtx, cancelReq := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = m.ProcessMessage(reqCtx, sessionID, "hello from web")
	})
	t.Cleanup(func() {
		cancelReq()
		wg.Wait()
		m.UnsubscribeWSBroadcast(subID)
		m.Stop()
		cancelMaster()
	})

	deadline := time.After(1 * time.Second)
	for {
		select {
		case msg := <-ch:
			if msg.Type != EventTypeInputReceived {
				continue
			}
			ev, ok := msg.Payload.(InputReceivedEvent)
			if !ok {
				t.Fatalf("payload type = %T, want InputReceivedEvent", msg.Payload)
			}
			if ev.ChannelMessageID != "" {
				t.Errorf("payload ChannelMessageID = %q, want empty", ev.ChannelMessageID)
			}
			return
		case <-deadline:
			t.Fatal("EventTypeInputReceived broadcast not received within 1s")
		}
	}
}

// TestMaster_InputReceivedBroadcast_EmptySessionIDSkipped 验证 sessionID 为空时
// 不广播 input_received（避免广播到"空 session" channel，subscriber 无从 filter）。
func TestMaster_InputReceivedBroadcast_EmptySessionIDSkipped(t *testing.T) {
	m, cancelMaster := setupInputReceivedMaster(t)

	subID, ch := m.SubscribeWSBroadcast()

	reqCtx, cancelReq := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() {
		_, _ = m.ProcessMessage(reqCtx, "", "hello")
	})
	t.Cleanup(func() {
		cancelReq()
		wg.Wait()
		m.UnsubscribeWSBroadcast(subID)
		m.Stop()
		cancelMaster()
	})

	select {
	case msg := <-ch:
		if msg.Type == EventTypeInputReceived {
			t.Fatalf("MUST NOT broadcast input_received when sessionID is empty, got: %+v", msg)
		}
	case <-time.After(300 * time.Millisecond):
		// 预期路径
	}
}
