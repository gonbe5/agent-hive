package eval_test

// Bridge test：把 eval/testdata 的 fm04 fixture 的"预期 LLM 输出"喂进生产
// spine `ingress.RealRunner`（带 fakeLLMClient），断言 RealRunner 能把 fixture
// 的 task_key 字符串契约真正解出 specdriven.Context。
//
// 为什么需要这层 bridge（Codex Round 6 review N2 红线）：
//   - eval/harness_behavior_test.go 里的 fakeRunner / naiveRunner 都不调真
//     planner.Decode，fm04 的 "task_key 必须 string" 反例锁只在 fakeRunner
//     echo-back 路径上验证——这是 tautology。
//   - 真生产路径 session_loop_specdriven.go → ingress.RealRunner.Run →
//     planner.Generate → planner.Decode 才是 fm04 守护的对象，必须有 test
//     直接驱动它，否则 fixture 的"反例锁"在落到 prod 时形同虚设。
//
// 蓝军 mutation 点位：
//   - R1 把 task_key string→float（"1.1" → 1.1）→ Decode 报 ErrPlannerSchemaInvalid，
//     happy bridge 红，counterexample bridge 绿——双向锁。
//   - R2 把 RealRunner CurrentTaskKey 改读 Steps[1] → 断言 "1.1" 红。
//   - R3 LLM 返回空 steps → ErrPlannerEmptyPlan，happy bridge 红。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/specdriven/eval"
	"github.com/chef-guo/agents-hive/internal/specdriven/ingress"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// bridgeFakeLLMClient 复用 ingress 包测试里的同名 fake 形态——独立定义避免
// 跨包 export 内部 fake 类型。callCount 是蓝军 R-A：RealRunner 必须真调 LLM
// 一次，伪装成功（return zero stats, nil）会让 callCount==0 被捕获。
type bridgeFakeLLMClient struct {
	content   string
	usage     llm.Usage
	err       error
	callCount int
}

func (f *bridgeFakeLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return &llm.ChatResponse{Content: f.content, Usage: f.usage, FinishReason: "stop"}, nil
}

// fixtureToPlannerJSON 把 eval Case.WantPlan 翻译成 planner LLM 期望的 JSON
// （`{"change_id":"...","steps":[...]}` 形态）。eval 的 specdriven.Plan 没有
// ChangeID 字段，bridge 显式注入一个稳定值给 specCtx 用。
//
// 注意：必须用 json.Marshal(args) 而不是 json.RawMessage 内联——eval fixture
// 的 Args 是 any（map[string]any），需要 marshal 一次得到 RawMessage 等价物。
func fixtureToPlannerJSON(t *testing.T, c eval.Case, changeID string) string {
	t.Helper()
	type plannerStep struct {
		TaskKey  string          `json:"task_key"`
		ToolName string          `json:"tool_name"`
		Args     json.RawMessage `json:"args,omitempty"`
	}
	type plannerOut struct {
		ChangeID string        `json:"change_id,omitempty"`
		Steps    []plannerStep `json:"steps"`
	}
	out := plannerOut{ChangeID: changeID}
	for _, s := range c.WantPlan.Steps {
		raw, err := json.Marshal(s.Args)
		require.NoError(t, err, "marshal fixture step.Args")
		out.Steps = append(out.Steps, plannerStep{
			TaskKey:  s.TaskKey,
			ToolName: s.ToolName,
			Args:     raw,
		})
	}
	b, err := json.Marshal(out)
	require.NoError(t, err, "marshal planner JSON")
	return string(b)
}

// TestBridge_FM04_FixtureFedThroughRealRunner_HappyPath 是 N2 闭环的 happy
// 端：fm04 fixture 的预期 plan 经 fakeLLMClient 注入 RealRunner，Run 必须
// 返回 specCtx{ChangeID, CurrentTaskKey, Revision=1}，Usage 必须透传非零。
//
// 这条 test 同时覆盖 fm04 的 task_key 字符串契约：fixture step.TaskKey="1.1"
// 字符串 → fixtureToPlannerJSON 输出 `"task_key":"1.1"` → planner.Decode 严
// 格按 string 字段解码 → CurrentTaskKey="1.1" 字符串。任何环节悄悄把 "1.1"
// 坍塌成 float64(1.1) 都会在 Decode 层被 ErrPlannerSchemaInvalid 截胡。
func TestBridge_FM04_FixtureFedThroughRealRunner_HappyPath(t *testing.T) {
	lc, err := eval.LoadCase("testdata/fm04_planner_drift.json")
	require.NoError(t, err, "load fm04 fixture")
	require.NotNil(t, lc.Case.WantPlan, "fm04 必须带 want_plan，否则 bridge 假设崩")
	require.NotEmpty(t, lc.Case.WantPlan.Steps, "fm04 want_plan.steps 必须非空")

	const wantChangeID = "tests-run"
	const wantBudget = int64(800)

	llmJSON := fixtureToPlannerJSON(t, lc.Case, wantChangeID)
	fake := &bridgeFakeLLMClient{
		content: llmJSON,
		usage:   llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, wantBudget, zaptest.NewLogger(t))

	specCtx, stats, err := runner.Run(context.Background(), "sess-fm04", lc.Case.Input)
	require.NoError(t, err, "fm04 happy bridge：fixture 喂入 RealRunner 必须成功")
	require.NotNil(t, specCtx, "RealRunner 成功必须返回非 nil specCtx")

	assert.Equal(t, wantChangeID, specCtx.ChangeID,
		"RealRunner 必须把 LLM JSON 的 change_id 透传到 specCtx")
	assert.Equal(t, lc.Case.WantPlan.Steps[0].TaskKey, specCtx.CurrentTaskKey,
		"RealRunner.CurrentTaskKey 必须取 Steps[0].TaskKey（fm04 锁 task_key 字符串契约）")
	assert.Equal(t, 1, specCtx.Revision,
		"Sprint 3.3.b MVP：新建 plan Revision 恒 1")

	assert.Equal(t, 1, fake.callCount,
		"RealRunner 必须真调 LLM 一次（蓝军 R-A：伪装成功 callCount==0 红）")
	assert.Equal(t, int64(150), stats.Usage.TotalTokens,
		"Usage 必须透传——budget 统计依赖此")
	assert.False(t, stats.BudgetExceeded,
		"150 < 800 不超 budget")
}

// TestBridge_FM04_TaskKeyAsFloat_RejectedBySchema 是 fm04 反例锁的真 prod
// 端验证：把 fm04 want_plan.steps[0].task_key 从 "1.1" 改写成 number 1.1
// （JSON 里 `"task_key": 1.1` 不带引号），喂进 RealRunner → planner.Decode
// 必须返 ErrPlannerSchemaInvalid，specCtx 必须 nil。
//
// 这是 fm04 fixture notes 里强调的"FM-4: planner 必须输出 task_key='1.1'
// string（禁止 1.1 float）"在生产 spine 上的端到端断言。eval harness 的
// fakeRunner echo-back 不可能 catch 这条——只有 RealRunner 能。
func TestBridge_FM04_TaskKeyAsFloat_RejectedBySchema(t *testing.T) {
	// 手工拼装 LLM 返回：task_key 写成数字而非字符串
	const malformed = `{"change_id":"tests-run","steps":[{"task_key":1.1,"tool_name":"bash","args":{"command":"go test ./...","timeout":30}}]}`

	fake := &bridgeFakeLLMClient{
		content: malformed,
		usage:   llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, zaptest.NewLogger(t))

	specCtx, stats, err := runner.Run(context.Background(), "sess-fm04-bad", "帮我把 tests 都跑一遍")

	require.Error(t, err, "task_key 数字必须被 RealRunner 拒绝")
	assert.True(t, errors.Is(err, planner.ErrPlannerSchemaInvalid),
		"错误必须是 ErrPlannerSchemaInvalid，让上游 classifyPlannerErr 路由到 schema_invalid label")
	assert.Nil(t, specCtx, "schema 失败 specCtx 必须 nil")

	// task 7.5 retry-once：schema_invalid 触发一次 retry，所以 callCount=2 且 Usage 累加
	assert.Equal(t, 2, fake.callCount,
		"schema_invalid 触发 task 7.5 retry-once，LLM 必被调 2 次")
	assert.Equal(t, int64(300), stats.Usage.TotalTokens,
		"task 7.5 retry-once Usage 累加：150 + 150 = 300（两次 tokens 都消耗）")
}

// TestBridge_FM04_EmptySteps_FailsClosed 反例锁第二条：planner 返合法 JSON
// 但 steps 为空 → ErrPlannerEmptyPlan，specCtx nil。fm04 fixture 隐含的另
// 一条 invariant（`steps 必须非空`，对应 planner.ErrPlannerEmptyPlan）必须
// 在生产 spine 端可触发。
func TestBridge_FM04_EmptySteps_FailsClosed(t *testing.T) {
	const empty = `{"change_id":"tests-run","steps":[]}`

	fake := &bridgeFakeLLMClient{
		content: empty,
		usage:   llm.Usage{TotalTokens: 50},
	}
	provider := func(_ context.Context) (planner.LLMClient, error) { return fake, nil }
	runner := ingress.NewRealRunner(provider, 800, nil)

	specCtx, _, err := runner.Run(context.Background(), "sess-fm04-empty", "x")

	require.Error(t, err, "empty steps 必须报错")
	assert.True(t, errors.Is(err, planner.ErrPlannerEmptyPlan),
		"empty steps 必须返 ErrPlannerEmptyPlan sentinel")
	assert.Nil(t, specCtx, "empty steps specCtx 必须 nil")
}

// TestBridge_FM04_FixtureToPlannerJSON_Roundtrip 锁 fixtureToPlannerJSON 工
// 具函数本身不能丢字段——bridge 测试的整套断言依赖它正确翻译 fixture，
// 翻译错了上面所有 happy/counterexample 都会假绿/假红。
func TestBridge_FM04_FixtureToPlannerJSON_Roundtrip(t *testing.T) {
	lc, err := eval.LoadCase("testdata/fm04_planner_drift.json")
	require.NoError(t, err)
	require.NotNil(t, lc.Case.WantPlan)

	const cid = "rt-cid"
	js := fixtureToPlannerJSON(t, lc.Case, cid)

	plan, err := planner.Decode([]byte(js))
	require.NoError(t, err, "fixtureToPlannerJSON 输出必须能被 planner.Decode 解开")
	assert.Equal(t, cid, plan.ChangeID)
	require.Len(t, plan.Steps, len(lc.Case.WantPlan.Steps))
	for i, want := range lc.Case.WantPlan.Steps {
		assert.Equal(t, want.TaskKey, plan.Steps[i].TaskKey, fmt.Sprintf("step[%d].task_key", i))
		assert.Equal(t, want.ToolName, plan.Steps[i].ToolName, fmt.Sprintf("step[%d].tool_name", i))
	}
}
