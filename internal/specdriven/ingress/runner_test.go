package ingress_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/specdriven/ingress"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// TestMinimalRunner_ReturnsSchemaInvalid — Sprint 3.3.a 合同 test。
//
// Sprint 3.3.a MinimalRunner 不调 LLM（3.3.b 补），直接返回
// planner.ErrPlannerSchemaInvalid，让上游 intake.DowngradeOnError 走
// `legacy_downshift_planner_schema` label。这条 test 锁死这个契约——
// 3.3.b 把 Run 改为真实 LLM 路径时，本 test 会自然因新实现成功而红，
// 届时 test 应该重写为 "LLM success → 非 nil Context" + 保留一个
// fallback test 专测 error 路径。
//
// 蓝军 mutation（验证本 test 是活 test，不是摆设）：
//   R1：把 runner.go Run 的 `return nil, planner.ErrPlannerSchemaInvalid`
//       改为 `return nil, nil`（伪装成功）→ `assert.ErrorIs` 必须红（err 为 nil）。
//   R2：改为 `return &specdriven.Context{ChangeID:"fake"}, planner.ErrPlannerSchemaInvalid`
//       （非 nil ctx）→ `assert.Nil(t, ctx)` 必须红。
//   R3：改为 `return nil, errors.New("boom")` → `assert.ErrorIs(planner.ErrPlannerSchemaInvalid)`
//       必须红（不同 sentinel）。
//
// 蓝军执行证据（2026-04-19，Sprint 3.3.a 收口）：
//   R1: test FAIL as expected（`expected err to be planner schema invalid, got nil`）
//   R2: test FAIL as expected（`expected nil context, got &{ChangeID:"fake"...}`）
//   R3: test FAIL as expected（`expected planner.ErrPlannerSchemaInvalid, got "boom"`）
//   R1/R2/R3 回滚后 test 绿。
func TestMinimalRunner_ReturnsSchemaInvalid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	runner := ingress.NewMinimalRunner(nil, logger)

	ctx := context.Background()
	specCtx, stats, err := runner.Run(ctx, "sess-1", "any user request")

	assert.Nil(t, specCtx,
		"Sprint 3.3.a MinimalRunner 禁止返回非 nil Context——避免 session 挂上幽灵 specCtx 污染 react_processor 读侧")
	assert.ErrorIs(t, err, planner.ErrPlannerSchemaInvalid,
		"Sprint 3.3.a 契约：必须返回 planner.ErrPlannerSchemaInvalid sentinel（对齐 intake.DowngradeOnError 的 reason 映射）")
	assert.Zero(t, stats.Usage.TotalTokens,
		"MinimalRunner 未调 LLM，RunStats.Usage 必须零——防止假 tokens 污染 Sprint 3.3.b b5 budget 统计")
	assert.False(t, stats.BudgetExceeded, "未调 LLM 不可能超 budget")
}

// TestMinimalRunner_NilRouterSafe — 验证 MinimalRunner 构造时 router 可为 nil。
//
// 意图：测试场景经常不想起 airouter（需要真实网络 / config），构造时传 nil 必须
// 不 panic、不影响 Run 行为。生产路径 bootstrap 会注入非 nil。
func TestMinimalRunner_NilRouterSafe(t *testing.T) {
	runner := ingress.NewMinimalRunner(nil, nil) // router=nil, logger=nil
	_, _, err := runner.Run(context.Background(), "sess-x", "req")
	assert.ErrorIs(t, err, planner.ErrPlannerSchemaInvalid,
		"nil router/logger 不应该改变 Run 契约")
}

// TestMinimalRunner_SatisfiesRunnerInterface — 编译期接口契约 + 显式运行期交叉验证。
func TestMinimalRunner_SatisfiesRunnerInterface(t *testing.T) {
	var r ingress.Runner = ingress.NewMinimalRunner(nil, nil)
	_, _, err := r.Run(context.Background(), "sess", "req")
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
}
