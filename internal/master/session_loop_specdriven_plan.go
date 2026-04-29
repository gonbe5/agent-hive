package master

import (
	"context"
	"errors"
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// classifyPlannerErr 把 spec runner 吐出的 err 路由到 PlanFallbackReason enum。
//
// 严格白名单：未匹配到已知 sentinel → FallbackReasonUnknown（不吞没，留 unknown
// 维度作为"分布观察点"，后续看到 unknown 暴涨再拆子类型）。
//
// 为什么不搞成 map：errors.Is 是链式解包，不能用相等比较——必须 if/else 串行。
//
// Sprint 3.3.b b4 纪律：
//   - schema_invalid / llm_timeout / over_budget / unknown 四值严格对齐
//     specdriven.AllowedPlanFallbackReasons 白名单（metrics.go:78）
//   - over_budget 由 b5（Token budget 比对）自己 emit，不经过本 classifier
//
// 蓝军 mutation 点位：
//   - 改 errors.Is → == → deep-wrap err 红
//   - 删 DeadlineExceeded 分支 → timeout 场景退化到 unknown 红
//   - 默认 return schema_invalid → unknown case 断言红
func (m *Master) classifyPlannerErr(err error) specdriven.PlanFallbackReason {
	if err == nil {
		// 调用方保证 err 非 nil 才进来——防御性返回 unknown，暴露 caller bug 而非静默。
		return specdriven.FallbackReasonUnknown
	}
	switch {
	case errors.Is(err, planner.ErrPlannerSchemaInvalid):
		return specdriven.FallbackReasonSchemaInvalid
	// Sprint 3.3.x 7.8：ErrPlannerTimeout sentinel 优先匹配——与 DeadlineExceeded 都命中
	// llm_timeout，但显式 sentinel 路径更语义清晰（避免把 tool 层 DeadlineExceeded 误归 planner timeout）。
	case errors.Is(err, planner.ErrPlannerTimeout):
		return specdriven.FallbackReasonLLMTimeout
	case errors.Is(err, context.DeadlineExceeded):
		return specdriven.FallbackReasonLLMTimeout
	// Sprint 3.3.x 7.8：ErrPlannerOverBudget 路由到独立 reason 标签。
	// 必须在 SchemaInvalid 之后——如果 decode 已失败，genErr 是 schema 错（非 over_budget），
	// 本分支只捕获"decode 成功但 tokens 超顶"的独立语义。
	case errors.Is(err, planner.ErrPlannerOverBudget):
		return specdriven.FallbackReasonOverBudget
	default:
		return specdriven.FallbackReasonUnknown
	}
}

// emitPlanFallback 打 MetricPlanFallbackTotal{reason}。
//
// label key 选用 `reason`（与 continuation_ask 同 key，刻意对齐——两者语义都是
// "为什么触发"）。与 continuation_resume 的 `trigger` key 区分开——这一层 Name
// 已不同，聚合不会误合并。
//
// Sprint 2.3 纪律（R2）：label key 常量化只隔行可读性，不抽常量——抽了反而增
// 加 lookup 成本，emit 是热路径。
func (m *Master) emitPlanFallback(reason specdriven.PlanFallbackReason) {
	m.enqueueMetric(observability.Metric{
		Name:  specdriven.MetricPlanFallbackTotal,
		Value: 1,
		Labels: map[string]any{
			"reason": string(reason),
		},
		Ts: time.Now(),
	})
}

// emitPlanTokenCost 打 MetricPlanTokenCostTotal（无 label），Value=本次调用消耗的总 tokens。
//
// 这是 counter 的 "sum" 形态——Prom 侧通过 rate() 聚合得到 tokens/sec，
// 通过 increase() 聚合得到任意时间窗的累计。无 label 是刻意的：token_cost
// 是"全局资源水位"，加 label（如 reason/model）会立刻炸 cardinality，
// 要做"按 model 分"需要单独 metric（Sprint 4+）。
//
// 零值保护：caller 必须在 tokens > 0 时才调——零值 emit 是浪费入队。
func (m *Master) emitPlanTokenCost(totalTokens int64) {
	m.enqueueMetric(observability.Metric{
		Name:   specdriven.MetricPlanTokenCostTotal,
		Value:  float64(totalTokens),
		Labels: nil, // 刻意无 label——cardinality 红线
		Ts:     time.Now(),
	})
}

// emitPlanTotal 计 spec runner 真正被调用的次数（Round 5 G1）——
// plan_fallback_total / spec_fallback_total 的 SLO 分母。
//
// 仅在 mode!=legacy AND 调用 runner 之前 emit（不论 runner 成败）。无 label。
// 没这条 counter，runbook §1 Stage 1 SLO 表里 "fallback 率 ≤ 5%" 公式分母悬空。
func (m *Master) emitPlanTotal() {
	m.enqueueMetric(observability.Metric{
		Name:   specdriven.MetricPlanTotal,
		Value:  1,
		Labels: nil,
		Ts:     time.Now(),
	})
}

// emitPlanOverbudget 打 MetricPlanOverbudgetTotal（无 label，每次超 budget +1）。
//
// 语义：与 emitPlanTokenCost 独立——前者是"有多少 tokens 被花掉"，后者是
// "有多少次请求触顶"。一个场景可以两者都打（花了 tokens 且超了）。
//
// 为什么 +1 而非 overshoot 大小：overshoot 大小意义不如 "触顶频率" 直观——
// budget 的存在就是硬墙，一旦撞墙就是事件，大小是细节。
func (m *Master) emitPlanOverbudget() {
	m.enqueueMetric(observability.Metric{
		Name:   specdriven.MetricPlanOverbudgetTotal,
		Value:  1,
		Labels: nil,
		Ts:     time.Now(),
	})
}
