package master

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/ingress"
	"github.com/chef-guo/agents-hive/internal/specdriven/intake"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// applySpecDrivenIntake 在 processTaskDirectExec 前做 intake 决策，挂 specCtx，打 metric。
//
// 设计（design.md FM-3 反例）：pure-function 决策由 `intake.ResolveIntakeDecision` 做，
// 这里只负责副作用：config → mode 读取、specCtx Store/Clear、metric emission。
// Streaming wrapper 未来会调同一个 intake 函数，保证非流式与流式判定一致。
//
// 返回值语义：
//   - path：调用方据此决定是否真走 spec 路径（Phase 2 MVP：都走 legacy，
//     只是 specCtx 是否非 nil 有差别）。
//   - 不返回 error——intake 决策自己永远成功（spec path 失败会 downshift，不 bubble up）。
//
// 性能契约（mode=legacy，99% 用户命中）：
//   - 一次 map key 查，一次 sentinel 判断，一次 metric 入队（非阻塞 channel send with default）。
//   - 零 LLM 调用，零 DB 调用。
//
// Sprint 3.3.a（2026-04-19，task 12.13）：
//   - 拆除 `ErrSpecRunnerNotImplemented` 裸 sentinel，改为调用 m.specRunner.Run。
//   - m.specRunner == nil 时 fail-closed 回 planner.ErrPlannerSchemaInvalid（与 3.3.a
//     前 stub 行为语义等价，不污染现有 downshift metric label），让未注入 runner 的
//     测试 Master 保持绿。
//   - LLM / SpecChangeStore / CAS observer / metric 细颗粒打点归 Sprint 3.3.b。
func (m *Master) applySpecDrivenIntake(session *SessionState, request string) intake.Path {
	mode := intake.Mode(m.config.SpecDriven.Mode)

	// mode=legacy / invalid / 空 request — short-circuit：不尝试 spec path，
	// 直接拿决策，清 specCtx（防止老值残留）。
	if mode == intake.ModeLegacy || !mode.IsValid() || request == "" {
		decision := intake.ResolveIntakeDecision(intake.ResolveInput{
			Mode:         mode,
			Request:      request,
			SessionState: session.SpecState,
		})
		session.StoreSpecCtx(nil) // fail-closed：非 spec 路径必须清零
		m.emitIntakeMetric(session.ID, decision)
		return decision.Path
	}

	// Round 5 G1：mode!=legacy 进入 runner 前 +1 plan_total（SLO 分母）。
	// 必须在 Resolve / runner 调用之前——保证"无论 runner 成败，plan_total 都计入"，
	// 否则 fallback rate = fallback / total 的分母会比分子还小（loss-of-data bias）。
	m.emitPlanTotal()

	// mode=dual / spec：
	//   Sprint 3.3.b b3：runner 调用前先跑 continuation.Resolve，按 Decision emit ask/resume counter。
	//   Resolve 纯函数（request × SpecState × now × cfg），与 Runner spine 解耦——
	//   即使 Runner 仍 fail-closed 返回 planner.ErrPlannerSchemaInvalid，continuation metric 仍真实产出。
	_ = m.resolveContinuationAndEmit(session.ID, request, session.SpecState)

	// mode=dual / spec：调 runner；runner 未注入（nil）时 fail-closed 模拟 planner 失败。
	var specCtx *specdriven.Context
	var specStats ingress.RunStats
	var specErr error
	if m.specRunner != nil {
		// 使用 context.Background()：3.3.a 暂不引入 session 级 ctx 传播，避免把
		// applySpecDrivenIntake signature 改动辐射到所有 caller。3.3.b 真调 LLM
		// 时会补充 ctx 参数。
		specCtx, specStats, specErr = m.specRunner.Run(context.Background(), session.ID, request)
	} else {
		// 未注入 runner（test 或 bootstrap 未 wire）→ 等价于旧 stub 路径 fail-closed。
		specErr = planner.ErrPlannerSchemaInvalid
	}

	// Sprint 3.3.b b5：无论 runner 成功/失败，只要本次调用产生了 tokens（Usage>0），
	// 都必须 emit token_cost（真实 LLM 开销计入）+ 判 over_budget。
	// 顺序：token_cost 先打（始终打），再按 BudgetExceeded 决定是否 emit overbudget。
	if specStats.Usage.TotalTokens > 0 {
		m.emitPlanTokenCost(specStats.Usage.TotalTokens)
	}
	if specStats.BudgetExceeded {
		m.emitPlanOverbudget()
	}

	// Sprint 3.3.b b4：runner 返回 err 时按 sentinel 分类 emit PlanFallbackTotal{reason}。
	// 位于 DowngradeOnError 之前——downgrade 是语义决策，fallback 是诊断 metric，
	// 两条切面刻意解耦：即使未来 downgrade 逻辑变，fallback 计数仍真实反映 runner
	// 失败分布。
	if specErr != nil {
		m.emitPlanFallback(m.classifyPlannerErr(specErr))
	}

	// Sprint 3.3.d plumbing fix：直调 ResolveIntakeDecision，把 runner 产出的 specCtx
	// 穿透到 decision.SpecContext。之前调 DowngradeOnError 是 3.3.a 的简化——该函数
	// 不接 ResolvedSpecCtx 参数，success path 的 specCtx 被静默丢弃，导致 PathSpec/PathDual
	// 永不可达（所有 intake 都降级成 PathLegacy，掩盖 runner 的真实贡献）。
	//
	// reason 路由：classifyPlannerErr 已在 b4 把 err 映射到 PlanFallbackReason，这里
	// 顺便反向映到 DownshiftReason（两套 enum 同源不同域，不强耦合）。
	reason := intake.DownshiftPlannerSchemaFailed
	if specErr != nil {
		switch m.classifyPlannerErr(specErr) {
		case specdriven.FallbackReasonLLMTimeout:
			reason = intake.DownshiftPlannerTimeout
		case specdriven.FallbackReasonOverBudget:
			reason = intake.DownshiftPlannerOverBudget
		}
	}

	decision := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:              mode,
		Request:           request,
		SessionState:      session.SpecState,
		SpecPathErr:       specErr,
		SpecPathErrReason: reason,
		ResolvedSpecCtx:   specCtx, // 3.3.d 新增：success path 必须回灌，否则 PathSpec 永不可达
	})
	session.StoreSpecCtx(decision.SpecContext)
	m.emitIntakeMetric(session.ID, decision)

	// task 10.5 / 10.6：按 mode 分流打 dual_diff / spec_fallback counter。
	// 注意：本函数只在 mode=dual/spec 路径进入（legacy/invalid/empty 在上面 short-circuit
	// 早 return），因此这里 switch 只看 dual vs spec。
	m.emitDispatchMetrics(mode, decision, specErr)

	if m.logger != nil && decision.Downshift != intake.DownshiftNone {
		m.logger.Info("spec-driven intake downshift 到 legacy",
			zap.String("session_id", session.ID),
			zap.String("mode", string(mode)),
			zap.String("decision", decision.MetricLabel),
			zap.String("downshift_reason", string(decision.Downshift)),
		)
	}

	return decision.Path
}

// emitIntakeMetric 打 spec-driven intake 决策的 counter metric。
//
// metric 名 `specdriven.intake_decision_total`——累计计数，label 来自 decision.MetricLabel
// 已压扁 path × downshift 组合（见 intake/decide.go 里的 MetricLabel 定义，
// 取值集合有限，避免 Prom cardinality 漂移）。
//
// nil 安全：enqueueMetric 自带 nil pool / 满队丢弃兜底。
func (m *Master) emitIntakeMetric(sessionID string, decision intake.IntakeDecision) {
	m.enqueueMetric(observability.Metric{
		Name:  "specdriven.intake_decision_total",
		Value: 1,
		Labels: map[string]any{
			"decision":   decision.MetricLabel,
			"session_id": sessionID,
		},
		Ts: time.Now(),
	})
}
