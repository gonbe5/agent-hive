package master

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/continuation"
)

// session_loop_specdriven_ambiguous_event_test.go 覆盖 task 6.5：
// DecisionAsk 路径必须通过 eventBus 广播 spec_continuation_ambiguous 事件，
// payload 含 AskReason + Trigger + Candidates；nil eventBus 必须静默 no-op。
//
// 蓝军 mutation 对照：
//   R1 Type 常量改字符串 → TestResolve_AskBroadcastsAmbiguousEvent Type 断言红
//   R2 去掉 BroadcastSessionMessage 调用 → 订阅者 select 超时红
//   R3 payload 漏 Candidates → TestResolve_AskBroadcastsAmbiguousEvent Candidates 断言红
//   R4 DecisionResume 也 broadcast（多打）→ TestResolve_ResumeDoesNotBroadcast 红

// newEventTestMaster 构造带 eventBus + obsCh 的最小 Master。
// 与 newCASTestMaster 刻意分离：后者用于纯 metric 测试，eventBus 未 wire 的 nil
// 路径也是契约（保留纯 metric 单测的最小面）。本函数专供 broadcast 测试。
func newEventTestMaster(t *testing.T) *Master {
	t.Helper()
	logger := zaptest.NewLogger(t)
	m := &Master{
		logger:   logger,
		obsCh:    make(chan observabilityEntry, 8),
		eventBus: NewEventBus(logger),
	}
	t.Cleanup(func() { m.eventBus.Close() })
	return m
}

// TestResolve_AskBroadcastsAmbiguousEvent ——
// 正向契约：DecisionAsk 路径必须广播 EventTypeSpecContinuationAmbiguous，
// payload 字段完整、SessionID envelope 正确填充。
func TestResolve_AskBroadcastsAmbiguousEvent(t *testing.T) {
	m := newEventTestMaster(t)

	// Subscribe 先于 Resolve——BroadcastSessionMessage 不持久化，订阅晚了会漏。
	subID, ch := m.eventBus.Subscribe()
	require.NotZero(t, subID, "Subscribe 必须返回有效 subID")
	defer m.eventBus.Unsubscribe(subID)

	// 构造 Ask 触发场景：ActiveChangeID 存在 + LastTouched 在 WeakWindow 内 +
	// user input 无关键词 → TriggerMRUOnly → DecisionAsk
	state := buildStateWithChange(
		"change-beta",
		"Rewrite billing adapter",
		time.Now().Add(-10*time.Minute),
		true,
	)

	result := m.resolveContinuationAndEmit("sess-broadcast-01", "hi", state)
	require.Equal(t, specdriven.DecisionAsk, result.Decision.Kind,
		"fixture 退化——构造场景必须触发 Ask 才能验证 broadcast 契约")
	require.Equal(t, continuation.TriggerMRUOnly, result.Trigger)

	// 从订阅通道收事件——50ms 足够同步广播 + drain
	select {
	case msg := <-ch:
		assert.Equal(t, EventTypeSpecContinuationAmbiguous, msg.Type,
			"Type 必须用 EventTypeSpecContinuationAmbiguous 常量——R1 mutation 杀穿点")
		assert.Equal(t, "sess-broadcast-01", msg.SessionID,
			"SessionID 必须从 BroadcastSessionMessage 注入——前端按 session 过滤")

		payload, ok := msg.Payload.(*SpecContinuationAmbiguousEvent)
		require.True(t, ok,
			"Payload 必须是 *SpecContinuationAmbiguousEvent 指针——保证序列化/反序列化一致")
		assert.NotEmpty(t, payload.AskReason,
			"AskReason 字段必须填充——前端弹框标题依赖此")
		assert.Equal(t, string(continuation.TriggerMRUOnly), payload.Trigger,
			"Trigger 必须是 Result.Trigger 原文")
		assert.Len(t, payload.Candidates, 1,
			"Candidates 必须携带识别出的 ChangeRef——R3 mutation 杀穿点")
		assert.Equal(t, "change-beta", payload.Candidates[0].ID,
			"Candidate[0].ID 必须是 fixture 注入的 change-beta")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("订阅者未在 100ms 内收到 spec_continuation_ambiguous 事件——" +
			"Broadcast 调用可能被 R2 mutation 摘除，或 eventBus 没 wire")
	}
}

// TestResolve_ResumeDoesNotBroadcast ——
// 反向契约：DecisionResume 路径禁止广播 ambiguous 事件（那是 UI 询问动作，
// resume 是"已决策"，不能弹框打扰用户）。
//
// R4 mutation 场景：若把 broadcast 放到 switch 外（所有路径都打），本测试抓穿。
func TestResolve_ResumeDoesNotBroadcast(t *testing.T) {
	m := newEventTestMaster(t)

	subID, ch := m.eventBus.Subscribe()
	defer m.eventBus.Unsubscribe(subID)

	state := buildStateWithChange("harden-spec-driven-phase2", "phase2", time.Now(), true)
	result := m.resolveContinuationAndEmit("sess-resume", "continue harden-spec-driven-phase2 now", state)
	require.Equal(t, specdriven.DecisionResume, result.Decision.Kind,
		"fixture 必须触发 Resume 才能验证 resume 路径不广播")

	select {
	case msg := <-ch:
		if msg.Type == EventTypeSpecContinuationAmbiguous {
			t.Fatalf("Resume 路径禁止广播 ambiguous 事件，但抽到了: %+v", msg)
		}
		// 其他类型事件（例如未来注入的 resume 事件）在此测试不校验——只否决 ambiguous
	case <-time.After(50 * time.Millisecond):
		// 预期超时——resume 路径本来就不广播本事件
	}
}

// TestResolve_NewDoesNotBroadcast ——
// 反向契约：DecisionNew 路径（完全空载）不广播任何事件。
func TestResolve_NewDoesNotBroadcast(t *testing.T) {
	m := newEventTestMaster(t)

	subID, ch := m.eventBus.Subscribe()
	defer m.eventBus.Unsubscribe(subID)

	state := specdriven.SessionSpecState{}
	result := m.resolveContinuationAndEmit("sess-new", "hello", state)
	require.Equal(t, specdriven.DecisionNew, result.Decision.Kind)

	select {
	case msg := <-ch:
		if msg.Type == EventTypeSpecContinuationAmbiguous {
			t.Fatalf("New 路径禁止广播 ambiguous 事件，但抽到了: %+v", msg)
		}
	case <-time.After(50 * time.Millisecond):
		// 预期超时
	}
}

// TestResolve_AskWithNilEventBusIsNoOp ——
// nil 安全契约：eventBus 未 wire（例如纯 metric 单测的 newCASTestMaster）时，
// Ask 路径广播必须 no-op 不 panic。
//
// 这一条保证 task 6.5 引入的 broadcast 不破坏现有 continuation metric 测试。
func TestResolve_AskWithNilEventBusIsNoOp(t *testing.T) {
	m := newCASTestMaster(t) // eventBus == nil
	state := buildStateWithChange(
		"change-gamma",
		"Some other change",
		time.Now().Add(-5*time.Minute),
		true,
	)

	// 不 panic 即通过；metric 仍要打出来（ask counter）
	require.NotPanics(t, func() {
		m.resolveContinuationAndEmit("sess-no-bus", "hi", state)
	})

	// 仍有 ask metric——证明 broadcast nil 分支不影响 metric emit 顺序
	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricContinuationAskTotal, got.Name,
		"eventBus 为 nil 不能短路 metric——broadcast 只是旁路，metric 主路径必须独立触发")
}
