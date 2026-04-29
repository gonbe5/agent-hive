package master

import (
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/continuation"
)

// SpecContinuationAmbiguousEvent 是 EventTypeSpecContinuationAmbiguous 的 payload：
// continuation.Resolve 判定为 DecisionAsk 时广播给 UI，供前端展示候选 change
// 让用户 "是要继续这个、那个、还是都不是" 三选一。
//
// 字段设计纪律（task 6.5 要求）：
//   - AskReason：Decision.AskReason 原文，不做二次脱水（UI 直接展示）
//   - Trigger：continuation.Trigger enum 字符串，供前端按触发路径分色
//   - Candidates：Resolve 识别到的候选 ChangeRef，UI 据此渲染候选卡片
//
// SessionID 由 BroadcastSessionMessage 填充，不重复放 payload 里（避免与 envelope
// 的 session_id 双写产生歧义）。
type SpecContinuationAmbiguousEvent struct {
	AskReason  string                 `json:"ask_reason"`
	Trigger    string                 `json:"trigger"`
	Candidates []specdriven.ChangeRef `json:"candidates"`
}

// resolveContinuationAndEmit 在 spec-driven intake 进入 runner 前调 continuation.Resolve，
// 并按 Decision.Kind 打对应 Phase 2 metric（Sprint 3.3.b b3）+ 广播 UI 事件（task 6.5）。
//
// 这里独立于 Runner spine——Resolve 是纯函数（只依赖 session.SpecState + request + now + cfg），
// 不需要 LLM / DB，因此本切点可在 b4/b5 Runner 真调 planner 之前独立闭环。
//
// 语义契约：
//   - DecisionAsk    → MetricContinuationAskTotal{reason=trigger} + broadcast spec_continuation_ambiguous
//   - DecisionResume → MetricContinuationResumeTotal{trigger=trigger}
//   - DecisionNew    → 不 emit（语义：无历史 change，无事件发生）
//
// Labels key 与 metrics.go 注释严格对齐：
//   - ask 路径 key=reason
//   - resume 路径 key=trigger
//
// nil 安全：
//   - obsCh 未 wire → enqueueMetric 内部兜底
//   - eventBus 未 wire → emitContinuationAmbiguous 内部 nil 检查
func (m *Master) resolveContinuationAndEmit(sessionID string, request string, state specdriven.SessionSpecState) continuation.Result {
	result := continuation.Resolve(request, state, time.Now(), continuation.DefaultDecayConfig())

	switch result.Decision.Kind {
	case specdriven.DecisionAsk:
		m.emitContinuationAsk(string(result.Trigger))
		m.emitContinuationAmbiguous(sessionID, result)
	case specdriven.DecisionResume:
		m.emitContinuationResume(string(result.Trigger))
	case specdriven.DecisionNew:
		// 无事件——首轮会话或完全空载，不打 counter，也不广播 UI。
	}
	return result
}

// emitContinuationAmbiguous 广播 spec_continuation_ambiguous 事件（task 6.5）。
//
// 纪律：
//   - eventBus == nil 直接 no-op，保护 newCASTestMaster 这类纯 metric 单测
//   - Type 用 EventTypeSpecContinuationAmbiguous 常量，不能硬编码字符串
//   - Payload 用 *SpecContinuationAmbiguousEvent（指针）——BroadcastSessionMessage
//     把 payload 原样带到订阅者，前端反序列化要拿到完整候选列表
//
// 蓝军 mutation 点位：
//   - R1 改 Type 常量 → Type 断言红
//   - R2 去掉 Broadcast 调用 → 订阅者超时红
//   - R3 payload 漏字段（AskReason/Trigger/Candidates 任一）→ 字段断言红
func (m *Master) emitContinuationAmbiguous(sessionID string, result continuation.Result) {
	if m.eventBus == nil {
		return
	}
	m.eventBus.BroadcastSessionMessage(sessionID, BroadcastMessage{
		Type: EventTypeSpecContinuationAmbiguous,
		Payload: &SpecContinuationAmbiguousEvent{
			AskReason:  result.Decision.AskReason,
			Trigger:    string(result.Trigger),
			Candidates: result.Candidates,
		},
	})
}

// emitContinuationAsk 打 MetricContinuationAskTotal{reason}。
//
// Sprint 2.3 纪律（R2 label naming）：label key 固定 `reason`，取值为 continuation.Trigger enum 字符串。
// 蓝军 mutation 点位：
//   - 改 Name 常量 → Name 断言红
//   - 改 "reason" → "trigger" → Labels 断言红
//   - 删 enqueueMetric → drainMetric 超时红
//   - reason 值写死 → scenario 交叉断言红
func (m *Master) emitContinuationAsk(reason string) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricContinuationAskTotal,
		Value: 1,
		Labels: map[string]any{
			"reason": reason,
		},
		Ts: time.Now(),
	})
}

// emitContinuationResume 打 MetricContinuationResumeTotal{trigger}。
//
// Sprint 2.3 纪律（R2 label naming）：label key 固定 `trigger`，区别于 ask 路径的 `reason`。
// 两条 label key 刻意不同——防止 downstream Prom 聚合时误把 ask/resume 合并到同一维度。
func (m *Master) emitContinuationResume(trigger string) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricContinuationResumeTotal,
		Value: 1,
		Labels: map[string]any{
			"trigger": trigger,
		},
		Ts: time.Now(),
	})
}
