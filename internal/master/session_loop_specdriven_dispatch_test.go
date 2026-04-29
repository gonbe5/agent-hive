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
	"github.com/chef-guo/agents-hive/internal/specdriven/intake"
	"github.com/chef-guo/agents-hive/internal/specdriven/planner"
)

// TestMaster_EmitDualDiff_Agree —— task 10.5 契约：mode=dual + spec 成功 →
// differs="false"，Name/Value/Labels 逐字段锁。
//
// 蓝军 mutation 点位：
//   - R1 改 Name 常量 → Name 断言红
//   - R2 label key "differs" → "diff" → Labels 断言红
//   - R3 写死 label 值 "true" → Agree 场景断言红
func TestMaster_EmitDualDiff_Agree(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitDualDiff(specdriven.DualDiffAgree)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "specdriven.dual_diff_total", got.Name,
		"Name 必须是 MetricDualDiffTotal 常量——硬编码串 = label drift 隐患")
	assert.Equal(t, float64(1), got.Value, "counter 每次 +1")
	require.NotNil(t, got.Labels)
	assert.Equal(t, "false", got.Labels["differs"],
		"Agree enum 字面量必须是 \"false\"——与 DualDiffLabel 常量锁对应")
}

// TestMaster_EmitDualDiff_Differ —— task 10.5 契约：mode=dual + spec 失败 →
// differs="true"。Agree/Differ 双 case 同 test 保证互不污染（Name 相同、label 值不同）。
func TestMaster_EmitDualDiff_Differ(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitDualDiff(specdriven.DualDiffDiffer)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "specdriven.dual_diff_total", got.Name)
	assert.Equal(t, float64(1), got.Value)
	assert.Equal(t, "true", got.Labels["differs"],
		"Differ enum 字面量必须是 \"true\"")
}

// TestMaster_EmitDualDiff_LabelKeyIsDiffers —— label key 锚点：防
// differs→diff/differ/comparison 之类的 rename drift。Prom dashboard 按 key
// 聚合，一旦 key 漂移 alert 就全瞎。
func TestMaster_EmitDualDiff_LabelKeyIsDiffers(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitDualDiff(specdriven.DualDiffAgree)

	got := drainMetric(t, m, 100*time.Millisecond)
	require.NotNil(t, got.Labels)
	// 白名单：仅允许 "differs" key；其余 key 出现即 cardinality 漂移。
	for k := range got.Labels {
		assert.Equal(t, "differs", k,
			"只允许 differs label key——发现非白名单 key=%q = cardinality 漂移", k)
	}
}

// TestMaster_EmitSpecFallback_PerReason —— task 10.6 契约：MetricSpecFallbackTotal
// 的 reason 标签必须映射 PlanFallbackReason 白名单的 4 个值。每个 case 独立
// emit 一次 drainMetric 一次，防串扰。
//
// 蓝军 mutation 点位：
//   - R1 改 Name 常量（用 PlanFallbackTotal 代替）→ Name 断言红（前者是 10.6 的独立 counter）
//   - R2 label key "reason" → "fallback_reason" → Labels 断言红
//   - R3 删 enqueue 调用 → drainMetric 超时红
//   - R4 label 写死 → 交叉子测断言红
func TestMaster_EmitSpecFallback_PerReason(t *testing.T) {
	tests := []struct {
		name   string
		reason specdriven.PlanFallbackReason
		want   string
	}{
		{"schema_invalid", specdriven.FallbackReasonSchemaInvalid, "schema_invalid"},
		{"llm_timeout", specdriven.FallbackReasonLLMTimeout, "llm_timeout"},
		{"over_budget", specdriven.FallbackReasonOverBudget, "over_budget"},
		{"unknown", specdriven.FallbackReasonUnknown, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newCASTestMaster(t)
			m.emitSpecFallback(tc.reason)
			got := drainMetric(t, m, 100*time.Millisecond)
			assert.Equal(t, "specdriven.spec_fallback_total", got.Name,
				"Name 必须是 MetricSpecFallbackTotal 独立常量，不能复用 PlanFallbackTotal")
			assert.Equal(t, float64(1), got.Value)
			require.NotNil(t, got.Labels)
			assert.Equal(t, tc.want, got.Labels["reason"],
				"reason label 值必须与 PlanFallbackReason enum 字面量一致")
		})
	}
}

// TestMaster_EmitSpecFallback_LabelKeyIsReason —— reason key 锚点，防 rename drift。
// 与 plan_fallback_total 的 label key 刻意保持一致（同是"触发原因"语义），但两
// metric Name 不同（plan_fallback 涵盖所有 non-legacy 路径，spec_fallback 仅 mode=spec 子集）。
func TestMaster_EmitSpecFallback_LabelKeyIsReason(t *testing.T) {
	m := newCASTestMaster(t)
	m.emitSpecFallback(specdriven.FallbackReasonSchemaInvalid)
	got := drainMetric(t, m, 100*time.Millisecond)
	require.NotNil(t, got.Labels)
	for k := range got.Labels {
		assert.Equal(t, "reason", k,
			"只允许 reason label key——发现非白名单 key=%q", k)
	}
}

// TestMaster_EmitDispatchMetrics_ModeDual_SpecOK —— 契约：mode=dual + err=nil +
// path=PathDual → emit DualDiff(Agree)，不 emit SpecFallback。
//
// 这是 10.5 的**集成 test**：从 (mode, decision, specErr) 三元组直接驱动 emit
// 路径，覆盖实际 call site 的分流逻辑（不是单纯 helper 单测）。
func TestMaster_EmitDispatchMetrics_ModeDual_SpecOK(t *testing.T) {
	m := newCASTestMaster(t)
	decision := intake.IntakeDecision{Path: intake.PathDual}
	m.emitDispatchMetrics(intake.ModeDual, decision, nil)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "specdriven.dual_diff_total", got.Name)
	assert.Equal(t, "false", got.Labels["differs"],
		"mode=dual + Path=PathDual + err=nil → Agree（\"false\"）")

	// 必须只有一条 metric——SpecFallback 在 mode=dual 下禁止触发
	select {
	case extra := <-m.obsCh:
		t.Fatalf("mode=dual 路径禁止 emit 第二条 metric，却抽到 %+v", extra)
	case <-time.After(50 * time.Millisecond):
		// 预期：无 extra emit
	}
}

// TestMaster_EmitDispatchMetrics_ModeDual_SpecErr —— 契约：mode=dual + err!=nil →
// DualDiff(Differ)。decision.Path 此时必然被 ResolveIntakeDecision 降级到 PathLegacy。
func TestMaster_EmitDispatchMetrics_ModeDual_SpecErr(t *testing.T) {
	m := newCASTestMaster(t)
	decision := intake.IntakeDecision{
		Path:      intake.PathLegacy,
		Downshift: intake.DownshiftPlannerSchemaFailed,
	}
	m.emitDispatchMetrics(intake.ModeDual, decision, planner.ErrPlannerSchemaInvalid)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "specdriven.dual_diff_total", got.Name)
	assert.Equal(t, "true", got.Labels["differs"],
		"mode=dual + err!=nil → Differ（\"true\"），dual 本轮退化到 legacy")
}

// TestMaster_EmitDispatchMetrics_ModeSpec_SpecErr —— 契约：mode=spec + err!=nil →
// SpecFallback(reason=classifyPlannerErr)，不 emit DualDiff。
//
// 蓝军 mutation 点位：
//   - R1 在 ModeSpec 分支 emit DualDiff → DualDiff Name 断言红
//   - R2 classifier reason 写死 → reason 交叉断言红
func TestMaster_EmitDispatchMetrics_ModeSpec_SpecErr(t *testing.T) {
	m := newCASTestMaster(t)
	decision := intake.IntakeDecision{
		Path:      intake.PathLegacy,
		Downshift: intake.DownshiftPlannerTimeout,
	}
	timeoutErr := fmt.Errorf("airouter: %w", context.DeadlineExceeded)

	m.emitDispatchMetrics(intake.ModeSpec, decision, timeoutErr)

	got := drainMetric(t, m, 100*time.Millisecond)
	assert.Equal(t, "specdriven.spec_fallback_total", got.Name,
		"mode=spec + err → spec_fallback，不是 dual_diff")
	assert.Equal(t, "llm_timeout", got.Labels["reason"],
		"classifyPlannerErr 应把 DeadlineExceeded wrap 映射到 llm_timeout")

	// 禁止同时 emit DualDiff
	select {
	case extra := <-m.obsCh:
		t.Fatalf("mode=spec 路径禁止 emit 第二条 metric，却抽到 %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestMaster_EmitDispatchMetrics_ModeSpec_SpecOK —— 契约：mode=spec + err=nil
// （primary-spec 成功）→ 不 emit SpecFallback（不是"spec path 次数"counter）。
//
// 这是**反向测试**——防有人把"spec_fallback"误理解成"spec path 计数"，
// 从而在成功路径也 +1。operators 看 rate 就算错了。
func TestMaster_EmitDispatchMetrics_ModeSpec_SpecOK(t *testing.T) {
	m := newCASTestMaster(t)
	decision := intake.IntakeDecision{Path: intake.PathSpec}
	m.emitDispatchMetrics(intake.ModeSpec, decision, nil)

	select {
	case extra := <-m.obsCh:
		t.Fatalf("mode=spec + err=nil 禁止 emit 任何 dispatch metric，却抽到 %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestMaster_EmitDispatchMetrics_ModeLegacy_NoEmit —— 契约：mode=legacy 调用
// emitDispatchMetrics 是契约违约（caller 应在 legacy 分支直接 return），但本
// 函数 default 分支保 no-op，不因误调崩。
//
// 这个 test 防"契约违约→连锁误 emit"——有人未来把 emitDispatchMetrics 挪到
// legacy 路径调用，就会被 default 分支吞掉（绿色）。若 default 改成 emit
// 兜底（R1 蓝军），本 test 红——守住"mode=legacy 零 spec-driven metric 开销"的性能契约。
func TestMaster_EmitDispatchMetrics_ModeLegacy_NoEmit(t *testing.T) {
	m := newCASTestMaster(t)
	decision := intake.IntakeDecision{Path: intake.PathLegacy}

	m.emitDispatchMetrics(intake.ModeLegacy, decision, nil)
	m.emitDispatchMetrics(intake.ModeLegacy, decision, errors.New("any err"))
	m.emitDispatchMetrics(intake.Mode("bogus"), decision, nil)

	select {
	case extra := <-m.obsCh:
		t.Fatalf("mode=legacy/invalid 禁止 emit dispatch metric，却抽到 %+v", extra)
	case <-time.After(80 * time.Millisecond):
	}
}

// TestMaster_AllowedDualDiffLabels_EnumLocked —— metric.go 白名单完整性：
// AllowedDualDiffLabels 必须恰好包含两个 enum 值，顺序不重要但集合必须相等。
//
// 蓝军 mutation 点位：增删白名单元素、改 enum 字面量都会红。
func TestMaster_AllowedDualDiffLabels_EnumLocked(t *testing.T) {
	assert.ElementsMatch(t,
		[]specdriven.DualDiffLabel{
			specdriven.DualDiffAgree,
			specdriven.DualDiffDiffer,
		},
		specdriven.AllowedDualDiffLabels,
		"白名单必须和枚举值字面量 1:1 对齐——任何漂移直接红")

	// 字面量硬锁（防有人把 Agree/Differ 的 string 值改来改去）
	assert.Equal(t, specdriven.DualDiffLabel("false"), specdriven.DualDiffAgree)
	assert.Equal(t, specdriven.DualDiffLabel("true"), specdriven.DualDiffDiffer)
}
