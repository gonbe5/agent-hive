package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// Harness 是 eval 的行为层入口。显式构造 + 显式 Runner，不用包级全局——
// Codex review P0-1/P0-6 红线修复。未设置 Runner 的 Harness 调用 RunFixtures
// 会立即 t.Fatal（fail-closed），这样 TG2+ wiring 前 CI gate 不会出现"空跑全绿"。
type Harness struct {
	// Runner 是被测实现。未注入时 RunFixtures 直接 Fatal。
	Runner Runner
}

// Summary 是 RunFixtures 的结构化产出。CI 可以把 RequiredFailed 列表作为
// audit artifact 输出给 dual-flag gate promotion 决策（spec.md#L84 要求）。
type Summary struct {
	Total          int
	Passed         int
	RequiredPassed int
	RequiredTotal  int
	RequiredFailed []string
	OptionalFailed []string
}

// String 返回摘要行，格式与 spec-eval-harness 里的 scenario 匹配：
// passed / required / total 计数，以及 optional_failed 数。
func (s Summary) String() string {
	return fmt.Sprintf(
		"eval summary: passed=%d required=%d/%d optional_failed=%d total=%d",
		s.Passed, s.RequiredPassed, s.RequiredTotal, len(s.OptionalFailed), s.Total,
	)
}

// RequiredSetComplete 验证 testdata 中 fm01~fm08 的必选集齐全。
// 所有判定基于 LoadedCase.Path 的 filename prefix，不看 Case.Name
// （Codex P0-3 红线修复：JSON 内 name 字段不能冒充 required 身份）。
// 同时要求每个 fm01~fm08 fixture 必须 Required:true，否则 required-set 形同虚设。
func RequiredSetComplete(cases []LoadedCase) error {
	need := []string{"fm01", "fm02", "fm03", "fm04", "fm05", "fm06", "fm07", "fm08"}
	found := map[string]LoadedCase{}
	for _, lc := range cases {
		for _, pre := range need {
			if strings.HasPrefix(lc.Base(), pre+"_") || strings.HasPrefix(lc.Base(), pre+".") {
				found[pre] = lc
			}
		}
	}
	var missing, nonRequired []string
	for _, pre := range need {
		lc, ok := found[pre]
		if !ok {
			missing = append(missing, pre)
			continue
		}
		if !lc.Case.Required {
			nonRequired = append(nonRequired, lc.Base())
		}
	}
	sort.Strings(missing)
	sort.Strings(nonRequired)
	if len(missing) == 0 && len(nonRequired) == 0 {
		return nil
	}
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing required fixtures: %v", missing))
	}
	if len(nonRequired) > 0 {
		parts = append(parts, fmt.Sprintf("required-set fixtures not marked required:true: %v", nonRequired))
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

// ValidateSchema 对单条 fixture 做纯静态的 schema 校验——
// 不依赖 Runner,可以在 TG1 合入时就独立跑通做 schema gate。
func ValidateSchema(c Case) error {
	switch c.WantContinuation.Decision {
	case "resume":
		if c.WantContinuation.ChangeID == "" {
			return fmt.Errorf("decision=resume requires want_continuation.change_id")
		}
	case "ask":
		if c.WantContinuation.AskReason == "" {
			return fmt.Errorf("decision=ask requires want_continuation.ask_reason")
		}
	case "new":
		// no extra constraints
	case "":
		return fmt.Errorf("want_continuation.decision missing")
	default:
		return fmt.Errorf("want_continuation.decision %q not in {resume, ask, new}", c.WantContinuation.Decision)
	}
	if c.Input == "" {
		return fmt.Errorf("input missing")
	}
	if c.StoreState != nil && c.StoreState.Revision < c.StoreState.ExportedRevision {
		return fmt.Errorf("store_state.revision (%d) must be >= exported_revision (%d)",
			c.StoreState.Revision, c.StoreState.ExportedRevision)
	}
	return nil
}

// validate 抽出 Harness 的前置不变量,独立于 testing.T 以便可测。
// Runner 为 nil 时返回包含 "Runner"/"nil"/"fail-closed" 关键词的错误——
// 这些关键词是防回潮自测的锚点。
func (h Harness) validate() error {
	if h.Runner == nil {
		return fmt.Errorf("eval.Harness: Runner is nil — behavior gate must have a wired Runner; refusing to fake green (fail-closed)")
	}
	return nil
}

// caseLogger 是 recordCaseResult 所需的最小 logging 接口——抽出是为
// (a) 让单元测试不必构造 *testing.T 就能覆盖 recordCaseResult 全部分支
// (b) 把 RunFixtures 的 t 约束限制在最小面，便于未来 Runner 层面独立复用。
type caseLogger interface {
	Logf(format string, args ...any)
}

// recordCaseResult 把单条 fixture 的结果（ok / required-fail / optional-fail）归入 Summary。
//
// 抽出背景（Sprint 2.4 蓝军 R1 产物）：原先此 switch 内嵌 RunFixtures，需要
// 触发 t.Run 的 subtest failure 才能驱动 required-fail / optional-fail 分支；
// 但 Go testing 框架下 subtest 失败会向上传递到 parent test——测这两个分支
// 就意味着"让这个 test 本身红"，反向阻碍 CI gate。抽成独立方法后可直接
// 单测 `&Summary{} → recordCaseResult(ok=false, required=true, ...)`
// 之类任意矩阵，覆盖 100% 分支而 test 整体保持绿。
//
// Codex Round 2 红线：optional fixture 失败"仅 warn 不 fatal"——
// 塞进 OptionalFailed 之外必须显式 warn log，PR reviewer 看不见 optional 红
// 就只能靠汇总行的数字猜。
func (s *Summary) recordCaseResult(ok bool, lc LoadedCase, logger caseLogger) {
	switch {
	case ok:
		s.Passed++
		if lc.Case.Required {
			s.RequiredPassed++
		}
	case lc.Case.Required:
		s.RequiredFailed = append(s.RequiredFailed, lc.Base())
	default:
		s.OptionalFailed = append(s.OptionalFailed, lc.Base())
		logger.Logf("WARN optional fixture failed: %s (not blocking rollout, tracked in summary)", lc.Base())
	}
}

// preflight 合并 RunFixtures 的两步静态校验，抽出为纯函数以便独立测试。
//  1. Harness 自身合法（Runner 非空）
//  2. required fixture 全集齐全（fm01-fm08）
//
// 抽出动机：原先两条 t.Fatal 路径无法在不触发父-子失败联动的前提下单测覆盖，
// 抽成纯 error 接口后，TestHarness_Preflight 可直接验证。
func (h Harness) preflight(cases []LoadedCase) error {
	if err := h.validate(); err != nil {
		return err
	}
	if err := RequiredSetComplete(cases); err != nil {
		return fmt.Errorf("required-set incomplete: %w", err)
	}
	return nil
}

// RunFixtures 是 behavior gate 入口。未注入 Runner 时 fail-closed——
// 这是 D7 dual-flag rollout 闸门的核心：不允许"空 Runner 全绿"的假门。
func (h Harness) RunFixtures(t *testing.T, cases []LoadedCase) Summary {
	t.Helper()
	if err := h.preflight(cases); err != nil {
		t.Fatal(err)
	}
	var s Summary
	s.Total = len(cases)
	for _, lc := range cases {
		if lc.Case.Required {
			s.RequiredTotal++
		}
		name := strings.TrimSuffix(lc.Base(), ".json")
		ok := t.Run(name, func(t *testing.T) {
			if err := ValidateSchema(lc.Case); err != nil {
				t.Fatalf("schema invalid: %v", err)
			}
			runCaseAgainstRunner(t, lc, h.Runner)
		})
		s.recordCaseResult(ok, lc, t)
	}
	t.Log(s.String())
	if err := s.terminalGate(); err != nil {
		t.Fatal(err)
	}
	return s
}

// terminalGate 把 "required fixture 失败 → block rollout" 的语义从 t.Fatalf 里
// 拆出来作为纯函数，便于在不触发父-子 testing.T 失败联动的前提下直接验证
// 该分支（Sprint 2.4 blue army R2）。
func (s Summary) terminalGate() error {
	if len(s.RequiredFailed) > 0 {
		return fmt.Errorf("required fixtures failed — blocking dual-flag rollout: %v", s.RequiredFailed)
	}
	return nil
}

// runCaseAgainstRunner 是 runCaseOnce 的 testing wrapper：把 error 上抛给 t.Fatal。
// 保留此薄层是为 RunFixtures 的现有调用点不改签名。
func runCaseAgainstRunner(t *testing.T, lc LoadedCase, r Runner) {
	t.Helper()
	if err := runCaseOnce(context.Background(), lc, r); err != nil {
		t.Fatal(err)
	}
}

// runCaseOnce 执行单条 fixture 的行为断言并返回 error（纯函数，不依赖 testing.T）：
//  1. continuation decision 精确匹配
//  2. WantPlan 若非 nil，逐字段深比对（Codex P0-2 红线修复）
//  3. WantError 若非空，断言返回 error 含指定子串
//  4. WantFallback=true 时触发 ExecuteFallback
//
// 抽出为纯函数的目的：让 TG1 behavior 覆盖测试可以 in-process 验证每条分支，
// 而不用借道 t.Run 的父-子失败联动（Go testing 的硬行为，无法 bypass）。
func runCaseOnce(ctx context.Context, lc LoadedCase, r Runner) error {
	dec, err := r.ResolveContinuation(ctx, lc.Case)
	if lc.Case.WantError != "" {
		if err == nil {
			return fmt.Errorf("expected error containing %q, got nil", lc.Case.WantError)
		}
		if !strings.Contains(err.Error(), lc.Case.WantError) {
			return fmt.Errorf("error mismatch: want substr %q, got %q", lc.Case.WantError, err.Error())
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("ResolveContinuation: %w", err)
	}
	if string(dec.Kind) != lc.Case.WantContinuation.Decision {
		return fmt.Errorf("decision mismatch: want=%s got=%s", lc.Case.WantContinuation.Decision, dec.Kind)
	}
	if lc.Case.WantContinuation.Decision == "resume" && dec.ChangeID != lc.Case.WantContinuation.ChangeID {
		return fmt.Errorf("resume change_id mismatch: want=%s got=%s",
			lc.Case.WantContinuation.ChangeID, dec.ChangeID)
	}
	if lc.Case.WantContinuation.Decision == "ask" && dec.AskReason != lc.Case.WantContinuation.AskReason {
		return fmt.Errorf("ask reason mismatch: want=%q got=%q",
			lc.Case.WantContinuation.AskReason, dec.AskReason)
	}
	if lc.Case.WantPlan != nil {
		plan, err := r.Plan(ctx, lc.Case)
		if err != nil {
			return fmt.Errorf("Plan: %w", err)
		}
		if err := equalPlan(lc.Case.WantPlan, plan); err != nil {
			return fmt.Errorf("plan mismatch: %w", err)
		}
	}
	if lc.Case.WantFallback {
		if err := r.ExecuteFallback(ctx, lc.Case); err != nil {
			return fmt.Errorf("ExecuteFallback: %w", err)
		}
	}
	return nil
}

// equalPlan 做逐字段深比对:TaskKey string 严格相等,ToolName 严格相等,
// Args 经 canonical JSON 规范化后 byte 相等。Codex P0-2 红线修复。
func equalPlan(want, got *specdriven.Plan) error {
	if got == nil {
		return fmt.Errorf("plan is nil")
	}
	if len(got.Steps) != len(want.Steps) {
		return fmt.Errorf("step count: want=%d got=%d", len(want.Steps), len(got.Steps))
	}
	for i := range want.Steps {
		ws, gs := want.Steps[i], got.Steps[i]
		if ws.TaskKey != gs.TaskKey {
			return fmt.Errorf("step[%d].task_key: want=%q got=%q", i, ws.TaskKey, gs.TaskKey)
		}
		if ws.ToolName != gs.ToolName {
			return fmt.Errorf("step[%d].tool_name: want=%q got=%q", i, ws.ToolName, gs.ToolName)
		}
		wb, err := canonicalJSON(ws.Args)
		if err != nil {
			return fmt.Errorf("step[%d] want.args encode: %w", i, err)
		}
		gb, err := canonicalJSON(gs.Args)
		if err != nil {
			return fmt.Errorf("step[%d] got.args encode: %w", i, err)
		}
		if !bytes.Equal(wb, gb) {
			return fmt.Errorf("step[%d].args diff:\n  want=%s\n  got =%s", i, wb, gb)
		}
	}
	return nil
}

// canonicalJSON 把任意 Args 值规范化为稳定 byte 序列:
// 通过 Marshal → Decode(UseNumber) → Marshal 消除 map 顺序/空白差异,
// 同时用 json.Number 保留原始数字字面量——避免 json.Unmarshal 到 any
// 时整数被坍塌成 float64 导致大于 2^53 的值丢精度（Round 4 Codex R5-1 红线）。
func canonicalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tmp any
	if err := dec.Decode(&tmp); err != nil {
		return nil, err
	}
	return json.Marshal(tmp)
}
