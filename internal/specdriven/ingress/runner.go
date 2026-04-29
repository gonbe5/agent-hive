// Package ingress 实现 spec-driven intake path 的 runner 执行器。
//
// Sprint 3.3.a 骨架 + Sprint 3.3.b spine：
//
//  1. Runner interface 定义 intake path 的单点执行入口；session_loop 不再
//     直接返回 ErrSpecRunnerNotImplemented，而是调用 m.specRunner.Run。
//  2. MinimalRunner：fail-closed 实现——返回 planner.ErrPlannerSchemaInvalid
//     + 零值 RunStats。Sprint 3.3.a 时就绪，现在保留给未注入 LLM 的测试场景
//     兜底（bootstrap 未 wire、单元测试不想起 provider）。
//  3. RealRunner：Sprint 3.3.b b5 spine——持 airouter.Router + token_budget，
//     实际调 planner.Generate（LLM + decode），产出真 Usage + over_budget 判定。
//
// 蓝军视角（Sprint 3.3.b 收口时必须杀穿）：
//   - R1：RealRunner.Run 伪装成功（return &Context{}, zero stats, nil）→ 没真调
//     LLM → 单测 fake provider 的 Chat call count == 0 暴露。
//   - R2：RealRunner 忘了透传 Usage（stats.Usage 恒零）→ over_budget 永远 false、
//     token_cost 恒 0，applySpecDrivenIntake 层 emit 测试红。
//   - R3：over_budget 比对方向反转（> → <）→ 边界断言红。
package ingress

import (
	"context"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/airouter"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// RunStats 把 Runner 内部的 LLM 调用可观测数据暴露给 caller（applySpecDrivenIntake）。
//
// 字段语义：
//   - Usage：本次 planner LLM 调用的 token 开销；未调用 LLM（MinimalRunner）时为零值。
//   - BudgetExceeded：Usage.TotalTokens 是否超过配置的 token_budget。零值 budget（=0）
//     表示不设限，此时 BudgetExceeded 恒 false——禁止"budget=0 当无限"被误判为"恒超"。
//
// 为什么不直接用 llm.Usage：BudgetExceeded 是 Runner 层的决策，planner 不关心 budget，
// 把判定逻辑推到 caller 会导致 budget 比对散布，不如在 Runner 里集中决策。
type RunStats struct {
	Usage          llm.Usage
	BudgetExceeded bool
}

// Runner 是 spec-driven intake path 的执行器接口。
//
// Sprint 3.3.b b5 签名升级：新增 RunStats 返回值——所有实现必须填 Usage
// （未调 LLM 则填零值），BudgetExceeded 由实现自己判定（budget=0 意为不设限）。
//
// 设计契约：
//   - Run 返回 (*specdriven.Context, RunStats, error)。
//   - success 返回非 nil Context 供 session_loop StoreSpecCtx。
//   - 任何 error 都会被 intake.DowngradeOnError 翻译成 legacy downshift，
//     但 RunStats 仍可能非零（tokens 已消耗，caller 需按 stats emit）。
//   - Run 保证不会 panic——任何 panic 等价于 fail-closed，上游走 legacy。
type Runner interface {
	Run(ctx context.Context, sessionID, request string) (*specdriven.Context, RunStats, error)
}

// MinimalRunner 是 Sprint 3.3.a 保留的最小实现——持 router/logger 但不调 LLM。
// Run 直接返回 planner.ErrPlannerSchemaInvalid + 零值 Stats。用于：
//   - bootstrap 未 wire 的降级路径（router==nil 时）
//   - 单元测试默认兜底
type MinimalRunner struct {
	router *airouter.Router
	logger *zap.Logger
}

// NewMinimalRunner 构造 Sprint 3.3.a 的 wiring 入口。
func NewMinimalRunner(router *airouter.Router, logger *zap.Logger) *MinimalRunner {
	return &MinimalRunner{router: router, logger: logger}
}

// Run MinimalRunner 语义：不调 LLM、不写 store，返回 planner.ErrPlannerSchemaInvalid。
// RunStats 恒零（未调 LLM = 无 Usage）。
func (r *MinimalRunner) Run(ctx context.Context, sessionID, request string) (*specdriven.Context, RunStats, error) {
	_ = ctx
	_ = sessionID
	_ = request
	_ = r.router
	if r.logger != nil {
		r.logger.Debug("specdriven.ingress.MinimalRunner.Run 返回 schema_invalid（兜底骨架）",
			zap.String("session_id", sessionID),
		)
	}
	return nil, RunStats{}, planner.ErrPlannerSchemaInvalid
}

// RealRunner 是 Sprint 3.3.b b5 的真实实现——拿 LLMClient 调 planner.Generate，
// 按 tokenBudget 判定 BudgetExceeded，构造 Context 给 caller。
//
// clientProvider 抽象 LLM client 获取方式：
//   - 生产：闭包 router.GetLLMClient(airouter.TaskAgent)
//   - 测试：闭包返回 fakeLLMClient
//
// 这层闭包防止 Runner 直接耦合 *airouter.Router——router 是 provider 定位器，
// Runner 只关心"能 Chat 的东西"。
type RealRunner struct {
	clientProvider func(ctx context.Context) (planner.LLMClient, error)
	tokenBudget    int64
	logger         *zap.Logger
}

// NewRealRunner 构造生产路径 Runner。
//
// tokenBudget <= 0 表示"不设限"——此时 BudgetExceeded 恒 false，over_budget
// metric 永不 emit。注意：这与"budget 设为无穷大"语义等价，但 emit 逻辑必须
// 显式跳过，不能依赖 `totalTokens > math.MaxInt64` 这种脆弱判断。
func NewRealRunner(clientProvider func(ctx context.Context) (planner.LLMClient, error), tokenBudget int64, logger *zap.Logger) *RealRunner {
	return &RealRunner{
		clientProvider: clientProvider,
		tokenBudget:    tokenBudget,
		logger:         logger,
	}
}

// Run 调用 planner.Generate 生成 plan，构造 Context 返回。
//
// 返回语义：
//   - success：(*Context, stats{Usage: 非零, BudgetExceeded: budget 判定}, nil)
//   - schema 非法：(nil, stats{Usage: 已消耗 tokens, BudgetExceeded: 判定}, ErrPlannerSchemaInvalid)
//   - LLM transport：(nil, stats{Usage: {}, BudgetExceeded: false}, 原 err)
//
// 关键：Usage 先透传再判定 BudgetExceeded——即使 decode 失败也先算 budget，
// 因为 tokens 已消耗，统计不能丢。
//
// Context.ChangeID 取 Plan.ChangeID（planner.Plan 带这个字段）；
// CurrentTaskKey 取第一步（Sprint 3.3.b MVP，未来按 progress 推进时重新计算）；
// Revision 恒 1——Sprint 3.3.b 尚未对接 SpecChangeStore 的 CAS，新建语义。
func (r *RealRunner) Run(ctx context.Context, sessionID, request string) (*specdriven.Context, RunStats, error) {
	client, err := r.clientProvider(ctx)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("specdriven.ingress.RealRunner.Run clientProvider 失败",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
		return nil, RunStats{}, err
	}

	plan, usage, genErr := planner.Generate(ctx, client, request, r.tokenBudget)
	stats := RunStats{
		Usage:          usage,
		BudgetExceeded: r.tokenBudget > 0 && usage.TotalTokens > r.tokenBudget,
	}
	if genErr != nil {
		return nil, stats, genErr
	}

	// Sprint 3.3.x 7.8：budget 超顶时禁止把 decode 成功的 Plan 下发——budget 是硬墙，
	// 超了就当 fallback 处理（caller intake 层会 downshift 到 legacy）。用 ErrPlannerOverBudget
	// sentinel 让 classifyPlannerErr 能路由到 FallbackReasonOverBudget 标签。
	//
	// 顺序：必须在 decode 成功后判 budget——如果 decode 已失败，genErr 优先于 budget，
	// 因为 schema 错本身就是更严重的结构化失败信号。
	if stats.BudgetExceeded {
		return nil, stats, planner.ErrPlannerOverBudget
	}

	firstTaskKey := ""
	if len(plan.Steps) > 0 {
		firstTaskKey = plan.Steps[0].TaskKey
	}
	specCtx := &specdriven.Context{
		ChangeID:       plan.ChangeID,
		CurrentTaskKey: firstTaskKey,
		Revision:       1,
	}
	return specCtx, stats, nil
}

// 编译期契约：两个实现都必须满足 Runner。
var (
	_ Runner = (*MinimalRunner)(nil)
	_ Runner = (*RealRunner)(nil)
)
