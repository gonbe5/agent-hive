package intake_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/specdriven/intake"
)

// TestResolve_LegacyModeShortCircuits：mode=legacy → 无论 request 长啥样，都走 legacy，不尝试 spec。
func TestResolve_LegacyModeShortCircuits(t *testing.T) {
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:    intake.ModeLegacy,
		Request: "continue on add-user-auth",
		ResolvedSpecCtx: &specdriven.Context{
			ChangeID: "add-user-auth",
		},
	})
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftModeLegacy, d.Downshift)
	assert.Nil(t, d.SpecContext, "legacy 模式绝不能泄漏 SpecContext")
	assert.Equal(t, "legacy", d.MetricLabel)
}

// TestResolve_InvalidMode：未知 mode 值 → fail-closed 归 legacy。
func TestResolve_InvalidMode(t *testing.T) {
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:    intake.Mode("spec-experimental-2"),
		Request: "hello",
	})
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftModeInvalid, d.Downshift)
	assert.Equal(t, "legacy_downshift_mode_invalid", d.MetricLabel)
}

// TestResolve_EmptyRequest：空请求 → 不 plan、直接 legacy。
func TestResolve_EmptyRequest(t *testing.T) {
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:    intake.ModeSpec,
		Request: "",
	})
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftEmptyRequest, d.Downshift)
}

// TestResolve_SpecPathErrorDowngrades：spec path 失败 → downshift 带 reason。
func TestResolve_SpecPathErrorDowngrades(t *testing.T) {
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:              intake.ModeSpec,
		Request:           "add auth",
		SpecPathErr:       errors.New("planner schema invalid"),
		SpecPathErrReason: intake.DownshiftPlannerSchemaFailed,
	})
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftPlannerSchemaFailed, d.Downshift)
	assert.Equal(t, "legacy_downshift_planner_schema", d.MetricLabel)
	assert.Nil(t, d.SpecContext, "downshift 必须清掉 SpecContext")
}

// TestResolve_SpecPathErrorMissingReason：调用方忘了 map reason → 保守归为 planner_schema。
func TestResolve_SpecPathErrorMissingReason(t *testing.T) {
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:        intake.ModeSpec,
		Request:     "add auth",
		SpecPathErr: errors.New("unknown error"),
	})
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftPlannerSchemaFailed, d.Downshift)
}

// TestResolve_DualMode：dual 跑两路，响应取 legacy；SpecContext 透传供 diff log。
func TestResolve_DualMode(t *testing.T) {
	ctx := &specdriven.Context{ChangeID: "c1", Revision: 1}
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:            intake.ModeDual,
		Request:         "continue",
		ResolvedSpecCtx: ctx,
	})
	assert.Equal(t, intake.PathDual, d.Path)
	assert.Equal(t, intake.DownshiftNone, d.Downshift)
	assert.Same(t, ctx, d.SpecContext, "dual 模式必须透传 spec context 给 diff logger")
	assert.Equal(t, "dual", d.MetricLabel)
}

// TestResolve_SpecModeSuccess：mode=spec 且 spec path 成功 → 走 spec path。
func TestResolve_SpecModeSuccess(t *testing.T) {
	ctx := &specdriven.Context{ChangeID: "c1", CurrentTaskKey: "1.1", Revision: 2}
	d := intake.ResolveIntakeDecision(intake.ResolveInput{
		Mode:            intake.ModeSpec,
		Request:         "add auth",
		ResolvedSpecCtx: ctx,
	})
	assert.Equal(t, intake.PathSpec, d.Path)
	assert.Equal(t, intake.DownshiftNone, d.Downshift)
	assert.Same(t, ctx, d.SpecContext)
	assert.Equal(t, "spec_ok", d.MetricLabel)
}

// TestResolve_DowngradeOnError_Helper：辅助函数 DowngradeOnError 等价于 ResolveInput 全路径。
func TestResolve_DowngradeOnError_Helper(t *testing.T) {
	state := specdriven.SessionSpecState{ActiveChangeID: "c1"}
	err := errors.New("timeout")
	d := intake.DowngradeOnError(intake.ModeSpec, "do thing", state, err, intake.DownshiftPlannerTimeout)
	assert.Equal(t, intake.PathLegacy, d.Path)
	assert.Equal(t, intake.DownshiftPlannerTimeout, d.Downshift)
	assert.Equal(t, "legacy_downshift_planner_timeout", d.MetricLabel)
}

// TestResolve_FM3_SharedIngressContract：5.4 验收项——
// 模拟 ProcessMessage 与 ProcessMessageStream 到达 processTask 时用同一份决策。
// 两种调用路径只要 input 一致，输出必须字节级相同（纯函数保证）。
func TestResolve_FM3_SharedIngressContract(t *testing.T) {
	ctx := &specdriven.Context{ChangeID: "c1", Revision: 3}
	input := intake.ResolveInput{
		Mode:            intake.ModeSpec,
		Request:         "continue",
		SessionState:    specdriven.SessionSpecState{ActiveChangeID: "c1"},
		ResolvedSpecCtx: ctx,
	}

	// 模拟 non-streaming ingress
	dNonStream := intake.ResolveIntakeDecision(input)
	// 模拟 streaming ingress（同一 input 副本）
	dStream := intake.ResolveIntakeDecision(input)

	assert.Equal(t, dNonStream.Path, dStream.Path,
		"streaming 和非 streaming 必须对同一 input 得到同一 Path")
	assert.Equal(t, dNonStream.Downshift, dStream.Downshift)
	assert.Equal(t, dNonStream.MetricLabel, dStream.MetricLabel)
	assert.Same(t, dNonStream.SpecContext, dStream.SpecContext,
		"SpecContext 必须是同一指针（值语义 ResolveInput 保证透传）")
}

// TestResolve_MetricLabelAllPaths：所有 downshift reason 都要能压扁成合法 metric label。
// 防 label 字符串漏改导致 Prom cardinality 翻倍。
func TestResolve_MetricLabelAllPaths(t *testing.T) {
	cases := []struct {
		name      string
		in        intake.ResolveInput
		wantLabel string
	}{
		{"legacy", intake.ResolveInput{Mode: intake.ModeLegacy, Request: "x"}, "legacy"},
		{"empty_request", intake.ResolveInput{Mode: intake.ModeSpec, Request: ""}, "legacy_downshift_empty_request"},
		{"invalid_mode", intake.ResolveInput{Mode: "xxx", Request: "x"}, "legacy_downshift_mode_invalid"},
		{"dual", intake.ResolveInput{Mode: intake.ModeDual, Request: "x"}, "dual"},
		{"spec_ok", intake.ResolveInput{Mode: intake.ModeSpec, Request: "x"}, "spec_ok"},
		{"downshift_schema", intake.ResolveInput{
			Mode: intake.ModeSpec, Request: "x",
			SpecPathErr: errors.New("e"), SpecPathErrReason: intake.DownshiftPlannerSchemaFailed,
		}, "legacy_downshift_planner_schema"},
		{"downshift_budget", intake.ResolveInput{
			Mode: intake.ModeSpec, Request: "x",
			SpecPathErr: errors.New("e"), SpecPathErrReason: intake.DownshiftPlannerOverBudget,
		}, "legacy_downshift_planner_budget"},
		{"downshift_timeout", intake.ResolveInput{
			Mode: intake.ModeSpec, Request: "x",
			SpecPathErr: errors.New("e"), SpecPathErrReason: intake.DownshiftPlannerTimeout,
		}, "legacy_downshift_planner_timeout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := intake.ResolveIntakeDecision(c.in)
			assert.Equal(t, c.wantLabel, got.MetricLabel)
		})
	}
}
