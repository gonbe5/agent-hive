package planner

import (
	"context"
	"errors"
	"fmt"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// LLMClient 是 planner.Plan 对 LLM 的最小依赖面。
//
// 为什么不直接依赖 *llm.Client：
//  1. 测试需要注入 FakeLLMClient——结构性接口让 *llm.Client 自动满足，无需改 llm 包。
//  2. 后续支持多 provider 路由（airouter → *llm.Client）时，planner 可复用而不改签名。
//
// 只暴露 Chat——不暴露 ChatJSON，因为 planner.Plan 需要精确拿到 Usage（*ChatResponse 字段），
// ChatJSON 内部吞掉 Usage 只返回 error，不符合 budget 统计需求。
type LLMClient interface {
	Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
}

// plannerSystemPrompt 是 planner LLM 的系统提示。
//
// Sprint 3.3.b b5 MVP：最小可跑 prompt。语义调优归 Sprint 4（openspec 另立 change）。
// 当前仅要求 LLM 输出严格的 {"steps": [...]} JSON——其余语义正确性由 planner.Decode
// 做 schema 校验，不在 prompt 里过度约束。
const plannerSystemPrompt = `你是 spec-driven agent 的 planner。
输入：用户自然语言需求。
输出：严格 JSON，形如 {"steps": [{"task_key": "1.1", "tool_name": "bash", "args": {...}}, ...]}。
task_key 必须字符串（如 "1.10"，不要写成数字 1.10 防止 1.1 坍塌）。
只输出 JSON，不要任何 markdown fence 或解释文本。`

// plannerSystemPromptReinforced 是 task 7.5 的 retry-once schema 强化 prompt。
//
// 设计纪律：
//   - 明确列出字段 + 给正向样例（正例比反例更有效，模型对"NOT"指令遵循差）
//   - steps 至少 1 项（防 ErrPlannerEmptyPlan 二次触发）
//   - task_key 字符串 + 正则 `^[0-9]+(\.[0-9]+)*$` 与 Decode schema 严格对齐
//   - 禁止 markdown fence / 解释文本——JSONMode 已经强制 JSON，此处是双保险
//
// 不设"你刚才错了"这类惩罚措辞——让 LLM 重新按 schema 生成，而非让它反思上一轮。
const plannerSystemPromptReinforced = `你是 spec-driven agent 的 planner。上一轮输出未通过 schema 校验，请严格按以下格式输出：

{
  "change_id": "string (可选，但若提供必须是 kebab-case)",
  "steps": [
    {"task_key": "1.1", "tool_name": "bash", "args": {"cmd": "ls"}},
    {"task_key": "1.2", "tool_name": "read", "args": {"path": "/tmp/foo"}}
  ]
}

硬性约束：
  1. 必须是单个 JSON 对象——不要数组包裹，不要 markdown fence，不要任何解释文本
  2. steps 必须非空（至少 1 项）
  3. task_key 必须字符串，且匹配正则 ^[0-9]+(\.[0-9]+)*$（如 "1.1"、"2.10"），禁止纯数字
  4. tool_name 必须字符串
  5. args 必须对象（可为空 {}）

输入：用户自然语言需求。只输出上述 JSON，其他一律禁止。`

// Generate 调用 LLM 生成计划并 decode 为 planner.Plan（与 Decode 同类型）。
//
// 函数名用 Generate 而非 Plan——后者是本包的类型名，避免重名冲突。
//
// 返回三元组 (*Plan, Usage, error)——关键设计：
//   - Usage 即使 decode 失败也返回（从 LLM 拿到的 tokens 已经消耗，不能吞掉
//     给 budget 统计用）；仅当 Chat 本身失败（err != nil）时 Usage 返回零值。
//   - error 语义：
//     - LLM transport/timeout 错误 → 原样返回（调用方用 errors.Is 路由 DeadlineExceeded）
//     - LLM 返回但 JSON schema 非法 → 返回 ErrPlannerSchemaInvalid（task 7.5：先 retry once）
//     - LLM 返回空 steps → 返回 ErrPlannerEmptyPlan（task 7.5：先 retry once）
//
// task 7.5 retry-once 纪律：
//   - 仅在 ErrPlannerSchemaInvalid / ErrPlannerEmptyPlan 触发 retry（LLM 输出结构性错误，
//     模型重生成可能修复）
//   - 不在 transport/timeout 错误上 retry（网络级问题，retry 浪费 budget 且 DeadlineExceeded
//     ctx 已经 cancel，retry 必然再失败）
//   - retry 用 plannerSystemPromptReinforced——给出正向样例 + 硬性约束，不是简单重发
//   - Usage 必须累加两次 Chat 的 tokens（都真实消耗）——budget 统计看总量
//   - retry 后仍失败 → 返回 retry 的 error（不是首次的，防止"首次 schema_invalid 遮蔽
//     retry 的 empty_plan"之类的诊断失真）
//
// maxTokens = 0 表示不设上限（交给 provider 默认），> 0 作为硬帽子传递给 ChatRequest.MaxTokens。
// 注：这里只传输 MaxTokens 不做本地 budget 比对——budget 超限判定归 applySpecDrivenIntake
// 层（Sprint 3.3.b b5：emit MetricPlanOverbudgetTotal 在那里，不在 planner 里，保持 planner
// 纯净只负责"调 LLM + decode"语义）。
func Generate(ctx context.Context, client LLMClient, request string, maxTokens int64) (*Plan, llm.Usage, error) {
	plan, usage, err := generateOnce(ctx, client, plannerSystemPrompt, request, maxTokens)
	if err == nil {
		return plan, usage, nil
	}

	// task 7.5：仅对"LLM 输出结构性错误"retry once——transport/timeout 直接返回。
	if !errors.Is(err, ErrPlannerSchemaInvalid) && !errors.Is(err, ErrPlannerEmptyPlan) {
		return nil, usage, err
	}

	retryPlan, retryUsage, retryErr := generateOnce(ctx, client, plannerSystemPromptReinforced, request, maxTokens)
	totalUsage := llm.Usage{
		PromptTokens:     usage.PromptTokens + retryUsage.PromptTokens,
		CompletionTokens: usage.CompletionTokens + retryUsage.CompletionTokens,
		TotalTokens:      usage.TotalTokens + retryUsage.TotalTokens,
	}
	if retryErr != nil {
		return nil, totalUsage, retryErr
	}
	return retryPlan, totalUsage, nil
}

// generateOnce 是 Generate 单次通信 + decode 的内部原语，不含 retry 逻辑。
// 抽离出来是 task 7.5 retry-once 的结构必要——Generate 可以用不同 prompt 分别调用两次。
func generateOnce(ctx context.Context, client LLMClient, systemPrompt, request string, maxTokens int64) (*Plan, llm.Usage, error) {
	req := llm.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: llm.NewTextContent(request)},
		},
		Temperature: 0,
		MaxTokens:   maxTokens,
		JSONMode:    true,
	}
	resp, err := client.Chat(ctx, req)
	if err != nil {
		// Sprint 3.3.x 7.8：DeadlineExceeded 外裹 ErrPlannerTimeout 便于上游按 sentinel
		// 统一路由。errors.Is 链式解包保证 DeadlineExceeded 仍可识别——双向兼容。
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, llm.Usage{}, fmt.Errorf("%w: %w", ErrPlannerTimeout, err)
		}
		return nil, llm.Usage{}, err
	}
	plan, decodeErr := Decode([]byte(resp.Content))
	if decodeErr != nil {
		// Usage 已消耗——保留给 budget 统计，不吞。
		return nil, resp.Usage, decodeErr
	}
	return plan, resp.Usage, nil
}
