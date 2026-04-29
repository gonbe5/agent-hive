package eval

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// Runner 被 specdriven 真实实现满足，供 harness 回放 Case。
// 每条 Case 按 Resolve → Plan → Execute 顺序调用，短路在首个失败点。
// 实现方应保证无副作用（不写真实 DB、不发外部 API），或通过 ctx 注入 mock 依赖。
type Runner interface {
	// ResolveContinuation 返回 intake / continuation 的决策。
	// 不应返回 Decision.Kind 为空字符串；错误仅在 harness 层面的系统错（ctx 取消、panic recover）时返回。
	ResolveContinuation(ctx context.Context, c Case) (specdriven.Decision, error)

	// Plan 生成 PlanStep 序列。Case.WantPlan == nil 时 harness 不调用本方法。
	Plan(ctx context.Context, c Case) (*specdriven.Plan, error)

	// ExecuteFallback 模拟 fallback 路径。Case.WantFallback == false 时 harness 不调用。
	ExecuteFallback(ctx context.Context, c Case) error
}
