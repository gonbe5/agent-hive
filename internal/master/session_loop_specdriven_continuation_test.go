package master

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/continuation"
)

// buildStateWithChange 构造一个包含单个 active change 的 SessionSpecState，
// 用于 explicit/strong/weak 各路触发的测试隔离。
func buildStateWithChange(id, title string, lastTouched time.Time, active bool) specdriven.SessionSpecState {
	state := specdriven.SessionSpecState{
		FocusMRU: []string{id},
		Changes: map[string]specdriven.ChangeRef{
			id: {
				ID:          id,
				Title:       title,
				LastTouched: lastTouched,
			},
		},
	}
	if active {
		state.ActiveChangeID = id
	}
	return state
}

// TestMaster_ResolveContinuationAndEmit_AskPath — ASK 分支 emit MetricContinuationAskTotal{reason}。
//
// 场景：有 ActiveChangeID + LastTouched 在 WeakWindow 内，但 user input 无关键词 →
// TriggerMRUOnly → DecisionAsk（FM-1 反例：subagent MRU 不能自动 RESUME）。
//
// 蓝军 mutation 点位：
//   - R1 改 MetricContinuationAskTotal 常量 → Name 断言红
//   - R2 改 label key "reason" → "trigger" → Labels["reason"] 为 nil，断言红
//   - R3 删 enqueueMetric → drainMetric 超时
//   - R4 trigger 硬编码 → 第二个 scenario 断言红
func TestMaster_ResolveContinuationAndEmit_AskPath(t *testing.T) {
	m := newCASTestMaster(t)
	// ActiveChangeID + 10min 前 touch（在 StrongWindow 30min 内，但 user 无 keyword → MRU-only → ASK）
	state := buildStateWithChange("change-alpha", "Refactor user auth", time.Now().Add(-10*time.Minute), true)

	result := m.resolveContinuationAndEmit("sess-ask", "你好，帮我看看天气", state)
	require.Equal(t, specdriven.DecisionAsk, result.Decision.Kind,
		"MRU-only 无关键词场景必须走 ASK（FM-1 反例纪律）——test fixture 若退化到 Resume 说明 resolver 改坏了")
	require.Equal(t, continuation.TriggerMRUOnly, result.Trigger)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricContinuationAskTotal, got.Name,
		"ask 路径 metric 名必须是 continuation_ask_total 常量")
	assert.Equal(t, float64(1), got.Value)
	assert.Equal(t, string(continuation.TriggerMRUOnly), got.Labels["reason"],
		"ask 路径 label key 固定为 reason，值为 Trigger 原文")
}

// TestMaster_ResolveContinuationAndEmit_ResumePath — RESUME 分支 emit MetricContinuationResumeTotal{trigger}。
//
// 场景：user input 显式带 explicit kebab id 命中 state.Changes → TriggerExplicitID → DecisionResume。
func TestMaster_ResolveContinuationAndEmit_ResumePath(t *testing.T) {
	m := newCASTestMaster(t)
	state := buildStateWithChange("harden-spec-driven-phase2", "phase2", time.Now(), true)

	result := m.resolveContinuationAndEmit("sess-resume", "continue harden-spec-driven-phase2 now", state)
	require.Equal(t, specdriven.DecisionResume, result.Decision.Kind,
		"explicit kebab id 命中 state.Changes 必须 RESUME——若退化到 NEW 说明 resolver explicit_id 分支挂了")
	require.Equal(t, continuation.TriggerExplicitID, result.Trigger)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricContinuationResumeTotal, got.Name,
		"resume 路径 metric 名必须是 continuation_resume_total 常量")
	assert.Equal(t, float64(1), got.Value)
	assert.Equal(t, string(continuation.TriggerExplicitID), got.Labels["trigger"],
		"resume 路径 label key 固定为 trigger（区别于 ask 的 reason，防 Prom 聚合误合并）")
	// 反向校验：resume 路径绝不出现 reason key
	_, hasReason := got.Labels["reason"]
	assert.False(t, hasReason, "resume 路径 Labels 禁止混入 reason key——ask/resume 维度必须刻意分离")
}

// TestMaster_ResolveContinuationAndEmit_NewPath — NEW 分支不 emit。
//
// 场景：空 state + 空信号 → DecisionNew → 零 metric 事件。
// 证明 "Decision=New 不该占位 counter"——否则 Prom series 爆炸。
func TestMaster_ResolveContinuationAndEmit_NewPath(t *testing.T) {
	m := newCASTestMaster(t)
	state := specdriven.SessionSpecState{} // 全零

	result := m.resolveContinuationAndEmit("sess-new", "hello world", state)
	require.Equal(t, specdriven.DecisionNew, result.Decision.Kind)
	require.Equal(t, continuation.TriggerNoSignal, result.Trigger)

	// drainMetric 超时 = 未 emit，正确
	select {
	case e := <-m.obsCh:
		t.Fatalf("DecisionNew 路径禁止 emit，但抽到了 metric: name=%s labels=%+v", e.metric.Name, e.metric.Labels)
	case <-time.After(50 * time.Millisecond):
		// 预期超时——通过
	}
}

// TestMaster_EmitContinuationAsk_LabelKeyIsReason — R2 mutation 锚点：
// 即使 trigger 常量相同，label key 从 reason 改成别的也必须被抓到。
func TestMaster_EmitContinuationAsk_LabelKeyIsReason(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitContinuationAsk("weak_signal")

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "weak_signal", got.Labels["reason"])
	assert.Nil(t, got.Labels["trigger"], "ask 路径禁止出现 trigger key")
}

// TestMaster_EmitContinuationResume_LabelKeyIsTrigger — 对称锚点，锁死 resume label key。
func TestMaster_EmitContinuationResume_LabelKeyIsTrigger(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitContinuationResume("explicit_id")

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "explicit_id", got.Labels["trigger"])
	assert.Nil(t, got.Labels["reason"], "resume 路径禁止出现 reason key")
}
