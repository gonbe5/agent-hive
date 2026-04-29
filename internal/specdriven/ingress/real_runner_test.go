package ingress_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/specdriven/ingress"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// fakeLLMClient 与 planner 包 fake 同构——测试内复制一份避免跨包 expose 内部类型。
type fakeLLMClient struct {
	content string
	usage   llm.Usage
	err     error
}

func (f *fakeLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &llm.ChatResponse{Content: f.content, Usage: f.usage, FinishReason: "stop"}, nil
}

// TestRealRunner_HappyPath_ContextAndStats ——
//
// 契约：LLM 返回合法 plan，Runner 构造 Context（ChangeID/CurrentTaskKey/Revision=1），
// RunStats.Usage 非零，BudgetExceeded=false（tokens 未超 budget）。
func TestRealRunner_HappyPath_ContextAndStats(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"change_id":"add-payment","steps":[{"task_key":"1.1","tool_name":"codegen"},{"task_key":"1.2","tool_name":"test_run"}]}`,
		usage:   llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, zaptest.NewLogger(t))

	specCtx, stats, err := runner.Run(context.Background(), "sess-1", "add stripe payment flow")
	require.NoError(t, err)
	require.NotNil(t, specCtx, "success 必须返回非 nil Context")

	assert.Equal(t, "add-payment", specCtx.ChangeID)
	assert.Equal(t, "1.1", specCtx.CurrentTaskKey, "CurrentTaskKey 必须是第一步——Sprint 3.3.b MVP")
	assert.Equal(t, 1, specCtx.Revision, "Revision=1 是新建 plan 的固定值（CAS 归 Sprint 3.3.d）")

	assert.Equal(t, int64(150), stats.Usage.TotalTokens, "Usage 必须原样透传")
	assert.False(t, stats.BudgetExceeded, "150 < 800 不超 budget")
}

// TestRealRunner_BudgetExceeded_TokensOverBudget ——
//
// 关键契约（Sprint 3.3.x 7.8 升级）：Usage.TotalTokens > tokenBudget 时，
//  1. RunStats.BudgetExceeded=true（给上游 emit overbudget metric）
//  2. 返回 ErrPlannerOverBudget sentinel（上游 classify 路由到 over_budget reason）
//  3. specCtx 必须 nil——budget 超顶的 Plan 禁止下发执行，否则 budget 形同虚设
//
// 蓝军：
//   - R1 比对方向反转（> 改成 <）→ BudgetExceeded 断言红
//   - R2 跳过 ErrPlannerOverBudget return → errors.Is 断言红 / specCtx nil 断言红
func TestRealRunner_BudgetExceeded_TokensOverBudget(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"change_id":"big-task","steps":[{"task_key":"1.1","tool_name":"bash"}]}`,
		usage:   llm.Usage{PromptTokens: 500, CompletionTokens: 400, TotalTokens: 900}, // 超 800
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, nil)

	specCtx, stats, err := runner.Run(context.Background(), "sess-big", "large request")
	// 3.3.x 升级：budget 超顶 = fallback error，不是成功
	require.Error(t, err, "budget 超顶必须返回 error，不能伪装成功")
	assert.True(t, errors.Is(err, planner.ErrPlannerOverBudget),
		"err 必须是 ErrPlannerOverBudget sentinel——上游按此路由到 FallbackReasonOverBudget")
	assert.Nil(t, specCtx, "budget 超顶时 specCtx 必须 nil——禁止下发超 budget 的 Plan")
	assert.True(t, stats.BudgetExceeded,
		"TotalTokens=900 > budget=800 → BudgetExceeded 必须 true，防 Prom overbudget counter 永不 emit")
	assert.Equal(t, int64(900), stats.Usage.TotalTokens,
		"Usage 必须透传——tokens 已消耗，token_cost counter 必须看到")
}

// TestRealRunner_BudgetZero_NeverExceeds ——
//
// 契约：tokenBudget=0 表示"不设限"，此时 BudgetExceeded 必须恒 false，不论
// Usage 多大。防止"budget=0 被当成 0 上限"导致所有请求都被误判超限。
func TestRealRunner_BudgetZero_NeverExceeds(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"change_id":"x","steps":[{"task_key":"1.1","tool_name":"bash"}]}`,
		usage:   llm.Usage{TotalTokens: 9_999_999}, // 巨大 usage
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 0, nil) // budget=0=不设限

	_, stats, err := runner.Run(context.Background(), "sess", "req")
	require.NoError(t, err)
	assert.False(t, stats.BudgetExceeded,
		"budget=0 语义是'不设限'——Usage 再大也不能判超，否则 overbudget 会全量爆")
}

// TestRealRunner_SchemaInvalid_UsageStillReported ——
//
// 契约：LLM 连续两次都返非法 JSON（task 7.5 retry-once 触发后仍失败）→
// 返回 ErrPlannerSchemaInvalid，但 Usage 必须透传**两次累加**
// （每次 tokens 都已消耗，budget 统计不能丢任何一次）。
func TestRealRunner_SchemaInvalid_UsageStillReported(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"not_steps": "garbage"}`, // 缺 steps 字段
		usage:   llm.Usage{TotalTokens: 200},
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, nil)

	specCtx, stats, err := runner.Run(context.Background(), "sess", "req")
	assert.Nil(t, specCtx)
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid))
	assert.Equal(t, int64(400), stats.Usage.TotalTokens,
		"task 7.5 retry-once：两次 schema_invalid Usage 累加（200+200=400）——"+
			"不累加会掩盖 retry 多烧的 tokens，over_budget 判定漏报")
}

// TestRealRunner_LLMTransportErr_ZeroUsage ——
//
// 契约：LLM transport 错误（ctx 超时、网络中断）时，Usage 为零值（没产生有效 tokens）。
// 这是 schema_invalid 与 transport 失败的 Usage 语义分野。
func TestRealRunner_LLMTransportErr_ZeroUsage(t *testing.T) {
	fake := &fakeLLMClient{
		err: fmt.Errorf("provider down: %w", context.DeadlineExceeded),
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, nil)

	_, stats, err := runner.Run(context.Background(), "sess", "req")
	assert.True(t, errors.Is(err, context.DeadlineExceeded),
		"transport 错误必须原样透传——caller 层 errors.Is 才能路由到 llm_timeout")
	assert.Equal(t, llm.Usage{}, stats.Usage,
		"transport 失败 Usage 必须零值——没有效 tokens 可统计")
	assert.False(t, stats.BudgetExceeded, "零 Usage 不应触发 overbudget")
}

// TestRealRunner_ClientProviderErr_NoLLMCall ——
//
// 契约：clientProvider 返回 err 时，Runner 不调 LLM（planner.Generate 未触发），
// 直接返回 err + 零 stats。防止 provider 失败时还误发零 tokens 的 cost metric。
func TestRealRunner_ClientProviderErr_NoLLMCall(t *testing.T) {
	wantErr := errors.New("router not ready")
	provider := func(_ context.Context) (planner.LLMClient, error) { return nil, wantErr }
	runner := ingress.NewRealRunner(provider, 800, nil)

	specCtx, stats, err := runner.Run(context.Background(), "sess", "req")
	assert.Nil(t, specCtx)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, llm.Usage{}, stats.Usage, "provider 错误时 LLM 没调，Usage 必零")
}

// TestRealRunner_SatisfiesRunnerInterface —— 编译期接口契约。
func TestRealRunner_SatisfiesRunnerInterface(t *testing.T) {
	fake := &fakeLLMClient{content: `{"change_id":"x","steps":[{"task_key":"1.1","tool_name":"bash"}]}`}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	var r ingress.Runner = ingress.NewRealRunner(provider, 0, nil)
	_, _, err := r.Run(context.Background(), "sess", "req")
	assert.NoError(t, err)
}
