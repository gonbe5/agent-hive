package master

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// TestMaster_ClassifyPlannerErr 锁定 planner err → PlanFallbackReason 分类表。
//
// 契约：未知 err 不得静默消失——走 FallbackReasonUnknown 保留"分布观察"语义。
// 对 planner.ErrPlannerSchemaInvalid / context.DeadlineExceeded 的 sentinel 路由
// 必须用 errors.Is（支持 fmt.Errorf("...%w", ...) 包装），不能用 == 判等。
//
// 蓝军 mutation 点位：
//   - R1 改 errors.Is → == → wrapped err 场景退化到 unknown，断言红
//   - R2 删 DeadlineExceeded case → timeout 场景退化 unknown，断言红
//   - R3 默认分支 return FallbackReasonSchemaInvalid → unknown case 断言红
//   - R4 SchemaInvalid 和 LLMTimeout 两条 case 互换 → 交叉场景断言红
func TestMaster_ClassifyPlannerErr(t *testing.T) {
	m := newCASTestMaster(t)

	tests := []struct {
		name string
		err  error
		want specdriven.PlanFallbackReason
	}{
		{
			name: "schema_invalid 裸 sentinel",
			err:  planner.ErrPlannerSchemaInvalid,
			want: specdriven.FallbackReasonSchemaInvalid,
		},
		{
			name: "schema_invalid wrap 场景（fmt.Errorf %w）",
			err:  fmt.Errorf("planner decode step 3: %w", planner.ErrPlannerSchemaInvalid),
			want: specdriven.FallbackReasonSchemaInvalid,
		},
		{
			name: "llm_timeout 裸 ctx.DeadlineExceeded",
			err:  context.DeadlineExceeded,
			want: specdriven.FallbackReasonLLMTimeout,
		},
		{
			name: "llm_timeout wrap 场景",
			err:  fmt.Errorf("airouter chat: %w", context.DeadlineExceeded),
			want: specdriven.FallbackReasonLLMTimeout,
		},
		{
			name: "未知 err 落 unknown 桶",
			err:  errors.New("totally unexpected planner implosion"),
			want: specdriven.FallbackReasonUnknown,
		},
		{
			name: "nil err 防御性落 unknown（caller bug 暴露）",
			err:  nil,
			want: specdriven.FallbackReasonUnknown,
		},
		// Sprint 3.3.x 7.8 新增三路——两个 sentinel 和 wrap 场景
		{
			name: "over_budget 裸 sentinel",
			err:  planner.ErrPlannerOverBudget,
			want: specdriven.FallbackReasonOverBudget,
		},
		{
			name: "over_budget wrap 场景",
			err:  fmt.Errorf("runner decided budget cap: %w", planner.ErrPlannerOverBudget),
			want: specdriven.FallbackReasonOverBudget,
		},
		{
			name: "planner_timeout sentinel（包含 DeadlineExceeded）",
			err:  fmt.Errorf("%w: %w", planner.ErrPlannerTimeout, context.DeadlineExceeded),
			want: specdriven.FallbackReasonLLMTimeout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := m.classifyPlannerErr(tc.err)
			assert.Equal(t, tc.want, got,
				"classifyPlannerErr 路由漂移——白名单契约破裂")
		})
	}
}

// TestMaster_EmitPlanFallback_EnqueuesMetric 锁定 fallback counter 的 Name +
// label key + Value。
//
// 蓝军 mutation 点位：
//   - R1 Name 常量改成别的 Metric*Total → Name 断言红
//   - R2 label key "reason" 改成 "trigger"/"kind" → Labels["reason"] nil 红
//   - R3 删 enqueueMetric → drainMetric 超时红
//   - R4 reason 入参写死 → 第二个 scenario 断言红
func TestMaster_EmitPlanFallback_EnqueuesMetric(t *testing.T) {
	m := newCASTestMaster(t)

	t.Run("schema_invalid 单路", func(t *testing.T) {
		m.emitPlanFallback(specdriven.FallbackReasonSchemaInvalid)
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, specdriven.MetricPlanFallbackTotal, got.Name,
			"metric 名必须是 specdriven.plan_fallback_total 常量——不能硬编码字符串")
		assert.Equal(t, float64(1), got.Value)
		assert.Equal(t, string(specdriven.FallbackReasonSchemaInvalid), got.Labels["reason"])
	})

	t.Run("llm_timeout 单路", func(t *testing.T) {
		m.emitPlanFallback(specdriven.FallbackReasonLLMTimeout)
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, string(specdriven.FallbackReasonLLMTimeout), got.Labels["reason"],
			"reason 必须是入参原值——防写死漂移")
	})

	t.Run("unknown 单路", func(t *testing.T) {
		m.emitPlanFallback(specdriven.FallbackReasonUnknown)
		got := drainMetric(t, m, 100*time.Millisecond)
		assert.Equal(t, string(specdriven.FallbackReasonUnknown), got.Labels["reason"])
	})
}

// TestMaster_EmitPlanFallback_LabelKeyIsReason —— R2 mutation 锚点。
//
// 与 continuation_ask 同 key（都是 reason）——这是刻意的：
// 两者 Name 不同（plan_fallback_total vs continuation_ask_total），Prom 不会
// 误合并；label key 对齐反而降低 downstream Grafana 面板查询方言分裂。
// 但 resume 路径用 trigger——必须反向校验不要误混入。
func TestMaster_EmitPlanFallback_LabelKeyIsReason(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitPlanFallback(specdriven.FallbackReasonSchemaInvalid)

	got := drainMetric(t, m, 100*time.Millisecond)
	require.Equal(t, "schema_invalid", got.Labels["reason"])
	assert.Nil(t, got.Labels["trigger"],
		"plan_fallback 路径禁止出现 trigger key——防 R2 mutation 把 reason→trigger")
	assert.Nil(t, got.Labels["scenario"],
		"plan_fallback 路径禁止出现 scenario key（scenario 是 CAS 专用）")
}

// TestMaster_ClassifyAndEmit_Chain 证明 classify + emit 链路端到端一致：
// err 进来，最终 Labels["reason"] 出去必须与 classifier 返回严格一致。
// 防止未来 classifier 和 emitter 之间出现"翻译丢失"。
func TestMaster_ClassifyAndEmit_Chain(t *testing.T) {
	m := newCASTestMaster(t)

	inputErr := fmt.Errorf("runner step 2: %w", planner.ErrPlannerSchemaInvalid)
	reason := m.classifyPlannerErr(inputErr)
	m.emitPlanFallback(reason)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricPlanFallbackTotal, got.Name)
	assert.Equal(t, "schema_invalid", got.Labels["reason"],
		"classify → emit 链路必须无损——wrapped err 也得正确路由到 schema_invalid")
}

// TestMaster_EmitPlanTokenCost 锁定 Sprint 3.3.b b5 token_cost counter 契约。
//
// 关键点：
//   - 无 label（Labels==nil 或空 map）——cardinality 红线，加 label 立即污染 Prom
//   - Value=totalTokens 原值——不能四舍五入、不能截断
//   - Name 用 MetricPlanTokenCostTotal 常量
//
// 蓝军 mutation 点位：
//   - R1 Name 换成 MetricPlanFallbackTotal → Name 断言红
//   - R2 加 label（如 "reason": "ok"）→ Labels == nil 断言红
//   - R3 Value 写死 1（counter 而非 sum）→ Value 断言红
//   - R4 强转丢精度（float32 来回）→ 大 tokens 值断言红
func TestMaster_EmitPlanTokenCost(t *testing.T) {
	m := newCASTestMaster(t)

	m.emitPlanTokenCost(12345)
	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricPlanTokenCostTotal, got.Name,
		"token_cost metric 名必须是 specdriven.plan_token_cost_total 常量")
	assert.Equal(t, float64(12345), got.Value,
		"Value=totalTokens 原值——这是 counter 的 sum 形态，不是 +1")
	assert.Nil(t, got.Labels,
		"token_cost 禁止带 label——cardinality 红线：加 label 立即炸 Prom series")
}

// TestMaster_EmitPlanTokenCost_LargeValue —— 防 R4 数值精度 mutation。
// 64-bit 范围内的大 token 数必须无损传递到 float64 Value。
func TestMaster_EmitPlanTokenCost_LargeValue(t *testing.T) {
	m := newCASTestMaster(t)

	// 一个 M context window 的边界值——float64 在 2^53 内精度无损
	const largeTokens = int64(1_000_000)
	m.emitPlanTokenCost(largeTokens)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, float64(largeTokens), got.Value,
		"1M tokens 必须精度无损——若上游把 int64 cast 成 int32 会溢出为 0")
}

// TestMaster_EmitPlanOverbudget 锁定 overbudget counter 契约。
//
// 蓝军 mutation 点位：
//   - R1 Name 换错常量 → Name 断言红
//   - R2 加 label → Labels==nil 断言红
//   - R3 Value 写 0（noop）→ Value 断言红
//   - R4 删 enqueueMetric → drainMetric 超时
func TestMaster_EmitPlanOverbudget(t *testing.T) {
	m := newCASTestMaster(t)

	m.emitPlanOverbudget()
	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, specdriven.MetricPlanOverbudgetTotal, got.Name,
		"overbudget metric 名必须是 specdriven.plan_overbudget_total 常量")
	assert.Equal(t, float64(1), got.Value,
		"overbudget 每次触顶 +1——counter 语义")
	assert.Nil(t, got.Labels, "overbudget 无 label——触顶事件不需要分维度")
}
