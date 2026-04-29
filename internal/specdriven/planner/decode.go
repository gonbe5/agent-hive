// Package planner 实现 spec-driven Phase 2 的 Guard 4 business side：
// LLM planner 输出解码 + schema 正则二次校验。
//
// 核心目标（design.md FM-4 反例）：
//   - LLM 有概率输出 `"task_key": 3.1`（数字）而非字符串；json.Unmarshal 到 any
//     会悄悄坍塌成 float64，后续按 "1.10 vs 1.9" 的语义比较会炸。
//   - 即使字段类型正确，LLM 也可能输出 unknown field（幻觉一个 "priority"）、
//     缺字段、或 task_key 格式不规范（"1.10.0.0" vs "1.10"）。
//
// 策略：两道闸。
//  1. json.Decoder.DisallowUnknownFields — 拒未知字段；强类型 (`task_key string`) 自带"数字会报错"。
//  2. 正则 `^\d+(\.\d+)+$` — 防 LLM 写 "1" / "1.10." / "a.1" 这类形似 kebab 的脏数据。
//
// 所有失败路径产出 sentinel error，上游按 `ErrPlannerSchemaInvalid` fallback
// 到 legacy ReAct（绝不 construct tool call——见 tasks.md 7.6）。
package planner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Sentinel errors——对齐 tasks.md 7.8 的三种 fallback reason。
var (
	// ErrPlannerSchemaInvalid：JSON 可解但 schema 违反（未知字段 / 类型错 / 格式错）。
	ErrPlannerSchemaInvalid = errors.New("planner output schema invalid")

	// ErrPlannerEmptyPlan：解码成功但 Plan.Steps 为空——planner 没出活。
	// 不作为硬错误归为 schema，而是独立信号以便 metric 标签区分。
	ErrPlannerEmptyPlan = errors.New("planner produced empty plan")

	// ErrPlannerTimeout：planner LLM 调用超时。
	//
	// 语义契约（与 Generate 实现对齐）：本 sentinel 包一层 context.DeadlineExceeded，
	// errors.Is(err, ErrPlannerTimeout) 和 errors.Is(err, context.DeadlineExceeded)
	// 都成立——caller 可按任一路由，classifyPlannerErr 优先匹配 ErrPlannerTimeout 保
	// 持分类颗粒度（避免"凡是 DeadlineExceeded 都叫 planner timeout"把 tool-use 超时
	// 误标）。
	//
	// Task 7.8 闭合：三路 fallback sentinel 齐全（Schema / OverBudget / Timeout）。
	ErrPlannerTimeout = errors.New("planner LLM timeout")

	// ErrPlannerOverBudget：planner 消耗的 total_tokens > 配置 token_budget。
	//
	// 设计约束（sprint 3.3.b b5 设计文档）：planner 纯函数自己不做 budget 比对，
	// budget 判定归 RealRunner 层——但为了上游（applySpecDrivenIntake）能用
	// 统一的 sentinel 路由到 FallbackReasonOverBudget 标签，这里定义 sentinel
	// 给 RealRunner 用。RealRunner 在 (BudgetExceeded && decodeErr == nil) 场景
	// 把成功的 decode 结果降级为 ErrPlannerOverBudget——budget 超顶不能让 Plan
	// 真正下发执行，否则 budget 形同虚设。
	ErrPlannerOverBudget = errors.New("planner over token budget")
)

// Plan 是 planner 的顶层输出结构。
// JSON 示例：
//
//	{
//	  "change_id": "add-user-auth",
//	  "steps": [
//	    {"task_key": "1.1", "tool_name": "codegen", "args": {"path": "..."}},
//	    {"task_key": "1.2", "tool_name": "test_run", "args": {"pkg": "..."}}
//	  ]
//	}
type Plan struct {
	ChangeID string     `json:"change_id"`
	Steps    []PlanStep `json:"steps"`
}

// PlanStep 是 plan 的单步。
//
// 关键纪律：TaskKey 必须是 string，**绝对禁止** number。
// FM-4 反例：LLM 写 `"task_key": 3.1`（float64）→ downstream 按字符串比较
// `CompareTaskKey("3.1", "3.10")` 直接废掉。
//
// Args 用 json.RawMessage 保留原字节——见 tasks.md 1.17 的 P1 修缮，
// 防 `any` 反序列化时 int 坍塌为 float64 导致 eval 比对漏检。
type PlanStep struct {
	TaskKey  string          `json:"task_key"`
	ToolName string          `json:"tool_name"`
	Args     json.RawMessage `json:"args,omitempty"`
}

// taskKeyPattern：严格 `\d+(\.\d+)+`。
//   - 必须至少两段（`"1"` 视为非法——planner 不该只给单段，spec 任务都是 `N.M` 形式）
//   - 允许任意层级 `1.10`、`2.1.3`（未来扩展）
//   - 拒绝尾点 `1.10.`、字母 `1.a`、空 `""`
var taskKeyPattern = regexp.MustCompile(`^\d+(\.\d+)+$`)

// Decode 把 raw JSON 字节解成 Plan，并做 schema 二次校验。
//
// 错误 -> sentinel 映射：
//   - 解码失败（含 unknown field / 类型错） → ErrPlannerSchemaInvalid（wrap 原 err）
//   - Steps 为空                            → ErrPlannerEmptyPlan
//   - 任一 step.TaskKey 不符合正则         → ErrPlannerSchemaInvalid
//   - 任一 step.ToolName 为空               → ErrPlannerSchemaInvalid
//
// 返回的 *Plan 在 error != nil 时为 nil——调用方不得读部分结果。
func Decode(raw []byte) (*Plan, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrPlannerSchemaInvalid)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // FM-4 第一道闸

	var plan Plan
	if err := dec.Decode(&plan); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlannerSchemaInvalid, err)
	}
	// 确认没有尾随字节（防 `{}` + trailing 半个 obj 的"部分成功"）
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data after plan object", ErrPlannerSchemaInvalid)
	}

	if len(plan.Steps) == 0 {
		return nil, ErrPlannerEmptyPlan
	}

	// 逐步校验：TaskKey regex + ToolName 非空
	for i, step := range plan.Steps {
		if !taskKeyPattern.MatchString(step.TaskKey) {
			return nil, fmt.Errorf("%w: step[%d].task_key %q violates pattern ^\\d+(\\.\\d+)+$",
				ErrPlannerSchemaInvalid, i, step.TaskKey)
		}
		if step.ToolName == "" {
			return nil, fmt.Errorf("%w: step[%d].tool_name is empty",
				ErrPlannerSchemaInvalid, i)
		}
	}

	return &plan, nil
}
