package planner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// fakeLLMClient 按预设 content+usage+err 响应 Chat——测试隔离不依赖真 provider。
//
// 蓝军视角：fake 只实现 Chat 单方法——证明 planner.LLMClient interface 真的只
// 暴露 Chat，不小心偷加依赖（如 ChatJSON）立刻 compile 红。
type fakeLLMClient struct {
	content      string
	usage        llm.Usage
	err          error
	lastReq      llm.ChatRequest
	chatCalls    int
}

func (f *fakeLLMClient) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	f.chatCalls++
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return &llm.ChatResponse{
		Content:      f.content,
		FinishReason: "stop",
		Usage:        f.usage,
	}, nil
}

// scriptedLLMClient 按 script 按调用顺序响应——task 7.5 retry-once 必备。
// 每次 Chat 返回 script[callIdx]，越界则复用最后一项（防止测试代码越界 panic）。
// 蓝军视角：一次调 N script 项，就是 N 次通信契约，不会被"重发同一响应"蒙骗。
type scriptedLLMClient struct {
	script    []scriptedResponse
	calls     []llm.ChatRequest
}

type scriptedResponse struct {
	content string
	usage   llm.Usage
	err     error
}

func (s *scriptedLLMClient) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	s.calls = append(s.calls, req)
	idx := len(s.calls) - 1
	if idx >= len(s.script) {
		idx = len(s.script) - 1
	}
	r := s.script[idx]
	if r.err != nil {
		return nil, r.err
	}
	return &llm.ChatResponse{
		Content:      r.content,
		FinishReason: "stop",
		Usage:        r.usage,
	}, nil
}

// TestPlannerGenerate_HappyPath —— LLM 返回合法 JSON，Generate 返回 Plan + Usage。
func TestPlannerGenerate_HappyPath(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"change_id":"add-user-auth","steps":[{"task_key":"1.1","tool_name":"bash","args":{"cmd":"ls"}}]}`,
		usage:   llm.Usage{PromptTokens: 42, CompletionTokens: 18, TotalTokens: 60},
	}

	plan, usage, err := Generate(context.Background(), fake, "帮我搞定用户登录", 800)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "add-user-auth", plan.ChangeID)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "1.1", plan.Steps[0].TaskKey)
	assert.Equal(t, "bash", plan.Steps[0].ToolName)

	assert.Equal(t, int64(60), usage.TotalTokens,
		"Usage 必须原样透传——budget 比对全靠这个数")
	assert.Equal(t, 1, fake.chatCalls, "Generate 单次只调一次 Chat")

	// 契约：MaxTokens 必须从入参透传
	assert.Equal(t, int64(800), fake.lastReq.MaxTokens,
		"MaxTokens 入参必须透传到 ChatRequest——否则 provider 端 budget 帽子失效")
	// 契约：JSONMode 必须开启
	assert.True(t, fake.lastReq.JSONMode, "planner 输出必须是 JSON，JSONMode 必须强制开启")
	// 契约：Temperature=0 确定性
	assert.Equal(t, float64(0), fake.lastReq.Temperature, "planner 必须 Temperature=0 防抖")
}

// TestPlannerGenerate_SchemaInvalid_ReturnsUsage —— 两次都返 schema 非法 JSON
// （task 7.5 retry-once 场景），Generate 必须：
//  1. 触发一次重试（chatCalls == 2）
//  2. Usage 两次累加（60 + 60 = 120）
//  3. 最终仍返回 ErrPlannerSchemaInvalid
//
// 这是 budget 统计的关键契约——decode 失败不能吞 Usage，否则 retry 多烧的 token
// 无法被 budget 统计识别，导致 over_budget 判定漏报。
func TestPlannerGenerate_SchemaInvalid_ReturnsUsage(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"steps":[{"task_key":"not-a-number","tool_name":"bash"}]}`, // task_key 正则不匹配
		usage:   llm.Usage{PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60},
	}

	plan, usage, err := Generate(context.Background(), fake, "test request", 800)
	assert.Nil(t, plan, "schema 非法时 plan 必须 nil——调用方不能读部分结果")
	assert.True(t, errors.Is(err, ErrPlannerSchemaInvalid),
		"非法 JSON 必须返回 ErrPlannerSchemaInvalid（sentinel 可被 errors.Is 识别）")
	assert.Equal(t, 2, fake.chatCalls,
		"task 7.5：schema_invalid 必须触发一次 retry——chatCalls=1 表示没 retry 是回退")
	assert.Equal(t, int64(120), usage.TotalTokens,
		"两次都 schema 非法时 Usage 必须累加（60+60=120）——不累加会掩盖 retry 的真实 token 消耗")
}

// TestPlannerGenerate_LLMTimeout_Propagates —— LLM 本身失败（ctx 超时）时，
// 原样返回 err（保证 errors.Is(err, context.DeadlineExceeded) 在 caller 层仍成立），
// Usage 返回零值（本次通信无意义，不污染统计）。
func TestPlannerGenerate_LLMTimeout_Propagates(t *testing.T) {
	fake := &fakeLLMClient{
		err: fmt.Errorf("provider timeout: %w", context.DeadlineExceeded),
	}

	plan, usage, err := Generate(context.Background(), fake, "req", 800)
	assert.Nil(t, plan)
	assert.True(t, errors.Is(err, context.DeadlineExceeded),
		"LLM transport 错误必须原样透传——caller 层 errors.Is 才能路由到 llm_timeout 分类")
	// Sprint 3.3.x 7.8：同时 wrap 为 ErrPlannerTimeout sentinel，双向兼容
	assert.True(t, errors.Is(err, ErrPlannerTimeout),
		"超时场景必须 wrap 为 ErrPlannerTimeout sentinel——显式路由优于隐式字符串匹配")
	assert.Equal(t, llm.Usage{}, usage, "transport 失败 usage 必须零值——无有效 tokens 可统计")
}

// TestPlannerGenerate_NonTimeoutErr_NotWrappedAsPlannerTimeout —— 保证 ErrPlannerTimeout
// 不被"凡 Chat 失败都 wrap"误发：非 DeadlineExceeded 的普通 transport err 不应被吞到
// ErrPlannerTimeout，否则 classify 表会把"LLM 5xx"当成"planner 超时"误报。
//
// 蓝军 mutation：去掉 errors.Is(err, context.DeadlineExceeded) 判断直接 wrap →
// 本测试 `assert.False(errors.Is(err, ErrPlannerTimeout))` 红 ✓ 杀穿。
func TestPlannerGenerate_NonTimeoutErr_NotWrappedAsPlannerTimeout(t *testing.T) {
	fake := &fakeLLMClient{
		err: errors.New("provider 5xx bad gateway"),
	}

	plan, _, err := Generate(context.Background(), fake, "req", 800)
	assert.Nil(t, plan)
	assert.False(t, errors.Is(err, ErrPlannerTimeout),
		"非超时 transport 错误必须原样返回，不能 wrap 成 ErrPlannerTimeout——防止把 5xx 误标 timeout")
	assert.False(t, errors.Is(err, context.DeadlineExceeded),
		"非超时错误与 DeadlineExceeded 无关")
}

// TestPlannerGenerate_EmptySteps_ReturnsEmptyPlan —— LLM 返回 `{"steps":[]}` 走
// ErrPlannerEmptyPlan sentinel 分支（与 schema_invalid 区分）。
// task 7.5：empty_plan 也触发 retry，fake 两次都返空 → 最终 ErrPlannerEmptyPlan，Usage 累加。
func TestPlannerGenerate_EmptySteps_ReturnsEmptyPlan(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"steps":[]}`,
		usage:   llm.Usage{TotalTokens: 20},
	}

	plan, usage, err := Generate(context.Background(), fake, "req", 800)
	assert.Nil(t, plan)
	assert.True(t, errors.Is(err, ErrPlannerEmptyPlan),
		"空 steps 必须区别于 schema_invalid——caller 可以按 sentinel 分流不同 fallback reason")
	assert.Equal(t, 2, fake.chatCalls, "task 7.5：empty_plan 也走 retry once 分支")
	assert.Equal(t, int64(40), usage.TotalTokens,
		"两次都 empty 时 Usage 必须累加（20+20=40）")
}

// TestPlannerGenerate_MaxTokensZero_PassesThrough —— maxTokens=0 表示"provider 默认"，
// 必须原样传 0，不能偷偷替换成硬编码默认值。
func TestPlannerGenerate_MaxTokensZero_PassesThrough(t *testing.T) {
	fake := &fakeLLMClient{
		content: `{"steps":[{"task_key":"1.1","tool_name":"noop"}]}`,
	}

	_, _, err := Generate(context.Background(), fake, "req", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), fake.lastReq.MaxTokens,
		"maxTokens=0 必须透传为 0——若偷偷替换成默认值（如 800），Sprint 3 的 over_budget 判定就废了")
}

// ——— task 7.5：retry-once 契约 tests ———
//
// 蓝军 mutation 对照：
//   R1 去掉 retry 分支（err != nil 就直接 return）→ TestRetrySucceedsOnSecondAttempt 红
//   R2 retry 用 plannerSystemPrompt 不换强化版 → TestRetryUsesReinforcedPrompt 红
//   R3 usage 只保留第二次（不累加）→ TestRetryAccumulatesUsage 红
//   R4 timeout 错误也触发 retry → TestNoRetryOnTimeout chatCalls==2 红
//   R5 非 schema/empty 错误也触发 retry → TestNoRetryOnTransportErr chatCalls==2 红

// TestPlannerGenerate_RetrySucceedsOnSecondAttempt —— 首次 schema_invalid，retry 返合法 Plan。
//
// 核心契约：Generate 必须真的做了 retry（chatCalls==2），retry 的结果被采纳。
// 如果 retry 分支被删，首次的 err 直接透出，plan 仍 nil —— TestRetrySucceedsOnSecondAttempt
// 的 require.NotNil(plan) 会红，R1 杀穿。
func TestPlannerGenerate_RetrySucceedsOnSecondAttempt(t *testing.T) {
	client := &scriptedLLMClient{
		script: []scriptedResponse{
			// 首次：schema 非法（task_key 是 "not-a-number"）
			{
				content: `{"steps":[{"task_key":"not-a-number","tool_name":"bash"}]}`,
				usage:   llm.Usage{PromptTokens: 30, CompletionTokens: 10, TotalTokens: 40},
			},
			// 二次：合法 JSON
			{
				content: `{"change_id":"add-user-auth","steps":[{"task_key":"1.1","tool_name":"bash","args":{"cmd":"ls"}}]}`,
				usage:   llm.Usage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
			},
		},
	}

	plan, usage, err := Generate(context.Background(), client, "帮我搞定登录", 800)
	require.NoError(t, err,
		"retry 成功后 Generate 必须返回 nil err——否则上层会误当失败 downshift")
	require.NotNil(t, plan, "retry 成功的 plan 必须返回，不能吞")
	assert.Equal(t, "add-user-auth", plan.ChangeID)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "1.1", plan.Steps[0].TaskKey)

	assert.Len(t, client.calls, 2,
		"chatCalls 必须 == 2：首次 schema_invalid 必须触发一次且仅一次 retry（R1 mutation 杀穿）")
	assert.Equal(t, int64(110), usage.TotalTokens,
		"Usage 必须两次累加（40 + 70 = 110）——即使 retry 成功也要统计首次消耗（R3 mutation 杀穿）")
}

// TestPlannerGenerate_RetryUsesReinforcedPrompt —— retry 必须换强化 prompt（R2 mutation 杀穿）。
//
// 反向契约：如果 retry 用原 prompt，LLM 大概率再犯同样错误，retry 机制等同作废。
// 用 scriptedLLMClient.calls 记录 SystemPrompt，校验第二次不等于第一次。
func TestPlannerGenerate_RetryUsesReinforcedPrompt(t *testing.T) {
	client := &scriptedLLMClient{
		script: []scriptedResponse{
			{content: `{"steps":[{"task_key":"bad","tool_name":"bash"}]}`, usage: llm.Usage{TotalTokens: 30}},
			{content: `{"steps":[{"task_key":"1.1","tool_name":"bash"}]}`, usage: llm.Usage{TotalTokens: 30}},
		},
	}

	_, _, err := Generate(context.Background(), client, "req", 800)
	require.NoError(t, err)
	require.Len(t, client.calls, 2, "fixture 验证：script 确实被调了两次")

	assert.Equal(t, plannerSystemPrompt, client.calls[0].SystemPrompt,
		"首次必须用 plannerSystemPrompt")
	assert.Equal(t, plannerSystemPromptReinforced, client.calls[1].SystemPrompt,
		"retry 必须换用 plannerSystemPromptReinforced——R2 mutation（retry 复用原 prompt）杀穿点")
	assert.NotEqual(t, client.calls[0].SystemPrompt, client.calls[1].SystemPrompt,
		"双向保险：两次 SystemPrompt 字面量不得相等")
}

// TestPlannerGenerate_NoRetryOnTimeout —— DeadlineExceeded / ErrPlannerTimeout 禁止 retry。
//
// 原因：ctx 已 cancel，retry 必然再超时；而且 timeout 的 root cause 是 provider 慢
// 或网络问题，不是 LLM 输出结构性错误——prompt 强化无效，retry 纯浪费 budget。
//
// 蓝军 R4：如果把 timeout 也拉进 retry → chatCalls==2，本测试 == 1 断言红。
func TestPlannerGenerate_NoRetryOnTimeout(t *testing.T) {
	client := &scriptedLLMClient{
		script: []scriptedResponse{
			{err: fmt.Errorf("provider timeout: %w", context.DeadlineExceeded)},
		},
	}

	_, _, err := Generate(context.Background(), client, "req", 800)
	assert.True(t, errors.Is(err, ErrPlannerTimeout),
		"timeout 错误必须 wrap 成 ErrPlannerTimeout（task 7.8 契约不变）")
	assert.Len(t, client.calls, 1,
		"timeout 场景禁止 retry——R4 mutation（把 timeout 拉进 retry）杀穿点")
}

// TestPlannerGenerate_NoRetryOnTransportErr —— 非 schema/empty 的一般 transport 错误禁止 retry。
//
// 非 schema_invalid/empty_plan 的错误（如 5xx）代表 provider 层问题，
// LLM 还没真的输出——retry 也打不到 decode 流程，浪费 budget。
func TestPlannerGenerate_NoRetryOnTransportErr(t *testing.T) {
	client := &scriptedLLMClient{
		script: []scriptedResponse{
			{err: errors.New("provider 5xx bad gateway")},
		},
	}

	_, _, err := Generate(context.Background(), client, "req", 800)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrPlannerSchemaInvalid))
	assert.False(t, errors.Is(err, ErrPlannerEmptyPlan))
	assert.Len(t, client.calls, 1,
		"transport err（非 schema/empty）必须 no-retry——R5 mutation 杀穿点")
}

// TestPlannerGenerate_EmptyPlanRetrySucceeds —— 首次 empty，retry 成功。
// 与 SchemaInvalid retry 对称——两类 sentinel 都进入 retry 分支。
func TestPlannerGenerate_EmptyPlanRetrySucceeds(t *testing.T) {
	client := &scriptedLLMClient{
		script: []scriptedResponse{
			{content: `{"steps":[]}`, usage: llm.Usage{TotalTokens: 15}},
			{content: `{"steps":[{"task_key":"1.1","tool_name":"bash"}]}`, usage: llm.Usage{TotalTokens: 40}},
		},
	}

	plan, usage, err := Generate(context.Background(), client, "req", 800)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Len(t, client.calls, 2, "empty_plan 必须触发 retry")
	assert.Equal(t, int64(55), usage.TotalTokens,
		"Usage 累加（15+40=55）——empty 场景也要保留首次 tokens")
}
