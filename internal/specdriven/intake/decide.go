// Package intake 实现 spec-driven Phase 2 的 Guard 4（intake 决策）+ Guard 5 防分叉：
// 所有进入 session_loop 的请求共享同一份 intake 决策——让未来的 streaming wrapper
// 与现有 ProcessMessage 走同一份判定，绝不会出现"非流式走 spec、流式走 legacy"的分裂 state。
//
// 设计纪律（design.md FM-3 反例）：
//   - `public_api.go` 不要下探 intake——保持 thin layer
//   - intake 决策在 `session_loop.go:757` 共享 ingress 处做
//   - Mode=legacy 时全路径 short-circuit（零开销，和今天一样）
//   - Mode=dual 时 spec path 跑但不决定响应（仅 diff 入日志）
//   - Mode=spec 时 spec path 为 primary；任何 spec path 失败 → 自动 downshift 到 legacy，
//     并记录 downshift reason 供 metric `specdriven.intake_decision_total{decision}` 使用
//
// 本包为纯模块，不依赖 config/metric 具体实现。Mode 由上游传入，metric emission 由调用方做。
package intake

import (
	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// Mode 是 spec-driven 的总开关模式，与 `config.json` 中 `spec_driven.mode` 对齐。
type Mode string

const (
	// ModeLegacy：所有 spec 路径完全跳过，行为与 Phase 2 前一致。**默认值**。
	ModeLegacy Mode = "legacy"
	// ModeDual：spec + legacy 都跑；响应以 legacy 为准，差异写 diff log。
	ModeDual Mode = "dual"
	// ModeSpec：spec 为 primary，legacy 仅作 fallback。
	ModeSpec Mode = "spec"
)

// IsValid 判断 Mode 取值是否合法。未知模式 → treat as legacy（fail-closed）。
func (m Mode) IsValid() bool {
	switch m {
	case ModeLegacy, ModeDual, ModeSpec:
		return true
	default:
		return false
	}
}

// Path 指示本次 ingress 走哪条路径。
type Path string

const (
	// PathLegacy：走 legacy ReAct（processTaskDirectExec with specCtx=nil）。
	PathLegacy Path = "legacy"
	// PathSpec：走 spec-driven primary（planner + continuation resolver + specCtx publish）。
	PathSpec Path = "spec"
	// PathDual：两条都跑，legacy 响应优先。
	PathDual Path = "dual"
)

// DownshiftReason 是 spec path 回退到 legacy 的原因枚举，对齐 metric label。
type DownshiftReason string

const (
	DownshiftNone                DownshiftReason = ""                 // 没发生 downshift
	DownshiftModeLegacy          DownshiftReason = "mode_legacy"      // 全局 mode=legacy
	DownshiftModeInvalid         DownshiftReason = "mode_invalid"     // mode 值不在 enum 内
	DownshiftEmptyRequest        DownshiftReason = "empty_request"    // 空请求无 spec 可言
	DownshiftPlannerSchemaFailed DownshiftReason = "planner_schema"   // planner 输出 schema fail
	DownshiftPlannerOverBudget   DownshiftReason = "planner_budget"   // planner token 超预算
	DownshiftPlannerTimeout      DownshiftReason = "planner_timeout"  // planner 超时
	DownshiftContinuationAsk     DownshiftReason = "continuation_ask" // continuation 返回 ASK 要问用户
)

// IntakeDecision 是 intake 阶段的最终决策。
// 调用方据此决定是否调 processTaskDirectExec、是否填 specCtx、是否 emit metric。
type IntakeDecision struct {
	// Path 指本次 request 走哪条路径。
	Path Path

	// SpecContext 是 spec path 决定挂到 session 上的 specCtx。
	// 仅在 Path=PathSpec / PathDual 且 spec path 成功时非 nil；
	// Path=PathLegacy 或 spec path 失败时为 nil（调用方 Store(nil) 清空）。
	SpecContext *specdriven.Context

	// Downshift 记录本次是否从 spec 降级；若降级，标明 reason。
	// DownshiftNone 表示正常 spec path 成功（或原本就是 legacy 决策，不算降级）。
	Downshift DownshiftReason

	// AskReason 仅当 Downshift=DownshiftContinuationAsk 时使用，
	// 承载 continuation resolver 吐出的 AskReason，透传给 UI 的 clarification prompt。
	AskReason string

	// MetricLabel 给调用方直接打 metric 用——decision × downshift 的压扁 label。
	// e.g. "spec_ok" / "legacy" / "dual_spec_ok" / "legacy_downshift_planner_schema"
	MetricLabel string
}

// ResolveInput 是 ResolveIntakeDecision 的输入载荷。
// 保持值语义：所有字段传值不传指针，避免被误改。
type ResolveInput struct {
	// Mode：来自 config.spec_driven.mode
	Mode Mode

	// Request：user 原始 text（可能空）
	Request string

	// SessionState：当前 session 的 spec state 快照（只读）
	SessionState specdriven.SessionSpecState

	// SpecPathErr：spec path（planner + continuation 等）执行后若失败，由调用方填入。
	// nil 表示 spec path 未尝试或成功。
	SpecPathErr error

	// SpecPathErrReason：把 SpecPathErr 映射到 DownshiftReason。
	// 调用方（session_loop）据 sentinel error 类型做映射，然后再调此函数。
	// 这层让本模块完全不依赖 planner/continuation 包，防止循环引用。
	SpecPathErrReason DownshiftReason

	// ResolvedSpecCtx：spec path 成功时的 specCtx（调用方提供）。
	// nil 表示 spec path 未给出 ctx。
	ResolvedSpecCtx *specdriven.Context
}

// ResolveIntakeDecision 是 intake 的总决策函数。
//
// 决策矩阵：
//  1. Mode=legacy             → Path=Legacy, Downshift=ModeLegacy
//  2. Mode 非法                → Path=Legacy, Downshift=ModeInvalid（fail-closed）
//  3. Request 完全空          → Path=Legacy, Downshift=EmptyRequest
//  4. SpecPathErr != nil       → Path=Legacy, Downshift=SpecPathErrReason
//  5. Mode=dual                → Path=Dual, SpecContext=ResolvedSpecCtx
//  6. Mode=spec 且无错         → Path=Spec, SpecContext=ResolvedSpecCtx
//
// 纯函数：给定相同 input 输出必相同，便于 test + future streaming wrapper 复用。
func ResolveIntakeDecision(in ResolveInput) IntakeDecision {
	// 1) Mode 校验（fail-closed：非法值 == legacy + metric 上报）
	if !in.Mode.IsValid() {
		return IntakeDecision{
			Path:        PathLegacy,
			Downshift:   DownshiftModeInvalid,
			MetricLabel: "legacy_downshift_mode_invalid",
		}
	}

	// 2) Mode=legacy 全路径 short-circuit
	if in.Mode == ModeLegacy {
		return IntakeDecision{
			Path:        PathLegacy,
			Downshift:   DownshiftModeLegacy,
			MetricLabel: "legacy",
		}
	}

	// 3) 空请求：spec path 没有 intent 可 plan
	if in.Request == "" {
		return IntakeDecision{
			Path:        PathLegacy,
			Downshift:   DownshiftEmptyRequest,
			MetricLabel: "legacy_downshift_empty_request",
		}
	}

	// 4) spec path 失败 → downshift
	if in.SpecPathErr != nil {
		reason := in.SpecPathErrReason
		if reason == "" || reason == DownshiftNone {
			// 调用方忘了 map reason——记为通用 schema 失败（保守归类）。
			reason = DownshiftPlannerSchemaFailed
		}
		return IntakeDecision{
			Path:        PathLegacy,
			Downshift:   reason,
			AskReason:   "", // downshift 不是 ask；continuation ASK 专走 5 号路径
			MetricLabel: "legacy_downshift_" + string(reason),
		}
	}

	// 5) Mode=dual：双跑，但响应取 legacy；spec 侧结果挂 specCtx 供 diff log 使用
	if in.Mode == ModeDual {
		return IntakeDecision{
			Path:        PathDual,
			SpecContext: in.ResolvedSpecCtx,
			Downshift:   DownshiftNone,
			MetricLabel: "dual",
		}
	}

	// 6) Mode=spec 且 spec path 成功
	return IntakeDecision{
		Path:        PathSpec,
		SpecContext: in.ResolvedSpecCtx,
		Downshift:   DownshiftNone,
		MetricLabel: "spec_ok",
	}
}

// DowngradeOnError 是给调用方使用的便利函数——spec path 尝试后拿到 err，
// 调一下这个就能得到 downshift 决策，不用手动构造 ResolveInput。
//
// 典型用法（session_loop.go:757 ingress）：
//
//	specCtx, specErr := runSpecPath(...)
//	if specErr != nil {
//	    reason := classifyReason(specErr)  // planner/continuation 错误 → DownshiftReason
//	    decision := intake.DowngradeOnError(mode, request, sessionState, specErr, reason)
//	    // 按 decision.Path 决定走 legacy 还是继续
//	}
func DowngradeOnError(
	mode Mode,
	request string,
	state specdriven.SessionSpecState,
	specErr error,
	reason DownshiftReason,
) IntakeDecision {
	return ResolveIntakeDecision(ResolveInput{
		Mode:              mode,
		Request:           request,
		SessionState:      state,
		SpecPathErr:       specErr,
		SpecPathErrReason: reason,
	})
}
