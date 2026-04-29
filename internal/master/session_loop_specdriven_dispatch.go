package master

import (
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/intake"
)

// emitDualDiff 打 MetricDualDiffTotal{differs}（task 10.5）。
//
// 语义：只在 mode=dual 才 emit——mode=legacy/spec 调用此函数是契约违约（不会发生，
// 但防御性不做 no-op，caller 负责按 mode 分流；测试会捕获误 emit）。
//
//   - differs=DualDiffAgree（"false"）：spec 路径成功（decision.Path == PathDual），
//     operators 读此 counter 认知到本轮 dual 真正双跑，spec 观点可见。
//   - differs=DualDiffDiffer（"true"）：spec 路径失败，decision 被 downshift 到 PathLegacy；
//     dual 实质退化成 legacy，用于估算 dual rollout 健康度。
//
// 蓝军 mutation 点位：
//   - R1 改 Name 常量 → Name 断言红
//   - R2 删 enqueue 调用 → drainMetric 超时红
//   - R3 label key "differs" 改名 → Labels 断言红
//   - R4 label 值写反（Agree/Differ 对调）→ scenario 交叉断言红
func (m *Master) emitDualDiff(differs specdriven.DualDiffLabel) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricDualDiffTotal,
		Value: 1,
		Labels: map[string]any{
			"differs": string(differs),
		},
		Ts: time.Now(),
	})
}

// emitSpecFallback 打 MetricSpecFallbackTotal{reason}（task 10.6）。
//
// 语义：只在 mode=spec AND specErr != nil 时 emit。与 plan_fallback_total 的区别：
//   - plan_fallback_total 覆盖所有 non-legacy mode（dual 也算）；
//   - spec_fallback_total 只计 primary-spec 失败——对应 operators"用户体验到了 fallback"
//     的那条 SLO 分子（docs/运维手册/spec-driven-rollout.md §SLO 门槛 ≤ 5%）。
//
// reason 来自 PlanFallbackReason 白名单——复用 classifyPlannerErr 的分类，
// 避免 enum 双写分歧（同一 err 在两个 metric 上应映射到同 reason）。
//
// 蓝军 mutation 点位：
//   - R1 改 Name 常量 → Name 断言红
//   - R2 label key "reason" 改名 → Labels 断言红
//   - R3 删 enqueue 调用 → drainMetric 超时红
//   - R4 写死 label 值 → scenario 交叉断言红
func (m *Master) emitSpecFallback(reason specdriven.PlanFallbackReason) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricSpecFallbackTotal,
		Value: 1,
		Labels: map[string]any{
			"reason": string(reason),
		},
		Ts: time.Now(),
	})
}

// emitExecutionPath 计每次 ingress 实际走的执行路径，label=path（Round 5 G2）。
// 与 emitIntakeMetric 互补：后者按"决策结果"打，前者按"实际路由"打——
// 在 spec_runner 接入完整后两者应一致；分歧即指 routing bug。
func (m *Master) emitExecutionPath(path intake.Path) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricExecutionPathTotal,
		Value: 1,
		Labels: map[string]any{
			"path": string(path),
		},
		Ts: time.Now(),
	})
}

// emitDispatchMetrics 在 applySpecDrivenIntake decision 得出后按 mode 分流打 10.5/10.6
// 的两个 counter。独立函数是为了 caller 可读性 + 单测 surface（fake Master 直接喂
// mode + decision + err 组合，不必重走整条 applySpecDrivenIntake）。
//
// 语义分流表（mode × decision.Path × err）：
//
//	mode=dual  × path=PathDual   × err=nil   → DualDiff(Agree)
//	mode=dual  × path=PathLegacy × err!=nil  → DualDiff(Differ)
//	mode=dual  × path=PathLegacy × err=nil   → DualDiff(Differ)   // 罕见：ResolveIntakeDecision 降级但无 err
//	mode=spec  × path=PathLegacy × err!=nil  → SpecFallback(reason=classifyPlannerErr)
//	mode=spec  × path=PathSpec   × err=nil   → 不 emit（primary 成功，无 fallback）
//	mode=legacy × any                        → 不 emit（本函数不会被 legacy 路径调用）
//
// 蓝军 mutation 点位：
//   - R1 把 mode=dual / mode=spec 判断删掉（都 emit）→ legacy 路径断言红
//   - R2 differs 判断反转（err=nil → Differ）→ Agree 场景断言红
//   - R3 SpecFallback reason 写死 → 交叉 case 断言红
func (m *Master) emitDispatchMetrics(mode intake.Mode, decision intake.IntakeDecision, specErr error) {
	switch mode {
	case intake.ModeDual:
		if decision.Path == intake.PathDual && specErr == nil {
			m.emitDualDiff(specdriven.DualDiffAgree)
			return
		}
		// dual 下被降级到 legacy（或 err != nil）→ differs=true
		m.emitDualDiff(specdriven.DualDiffDiffer)
	case intake.ModeSpec:
		if specErr != nil {
			m.emitSpecFallback(m.classifyPlannerErr(specErr))
		}
		// primary-spec 成功则不 emit——spec_fallback 是"发生 fallback"的 counter，
		// 不是"spec path 次数"的 counter（后者由 intake_decision_total 承担）。
	default:
		// mode=legacy 或 mode 非法——caller 契约是只在 mode=dual/spec 才调本函数。
		// 不 emit，让 contract 测试捕获违约。
	}
}
