package config

import (
	"fmt"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// FeatureFlagCombo 快照 D15 4 维 flag 激活状态，用于启动期校验 + 日志打印。
//
// 4 维含义：
//   - SpecDrivenEnabled   = cfg.SpecDriven.Enabled()           // Mode != "legacy"
//   - SubagentMode        = cfg.SpecDriven.SubagentModeEnabled() // SubagentMode != "legacy"
//   - SemanticRouting     = cfg.SpecDriven.SkillsSemanticRouting
//   - OnDemandEnabled     = cfg.Agent.Skills.OnDemandEnabled
//
// 16 组合 → 9 有效 + 7 无效（见 design.md D15 / spec.md §Feature Flag Matrix）。
type FeatureFlagCombo struct {
	SpecDrivenEnabled bool
	SubagentMode      bool
	SemanticRouting   bool
	OnDemandEnabled   bool
}

// Snapshot 从 Config 提取 4 维 flag 激活态快照。
func SnapshotFeatureFlags(cfg *Config) FeatureFlagCombo {
	return FeatureFlagCombo{
		SpecDrivenEnabled: cfg.SpecDriven.Enabled(),
		SubagentMode:      cfg.SpecDriven.SubagentModeEnabled(),
		SemanticRouting:   cfg.SpecDriven.SkillsSemanticRouting,
		OnDemandEnabled:   cfg.Agent.Skills.OnDemandEnabled,
	}
}

// String 以 bootstrap 启动日志的标准格式输出 flag 组合。
// 格式契约（tasks.md 10.4）：
//
//	skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W
//
// grep 测试（tasks.md 12.3）依赖此前缀定位。
func (f FeatureFlagCombo) String() string {
	return fmt.Sprintf(
		"skills_feature_flags: specdriven=%t subagent_mode=%t semantic_routing=%t on_demand=%t",
		f.SpecDrivenEnabled, f.SubagentMode, f.SemanticRouting, f.OnDemandEnabled,
	)
}

// ValidateFlagCombination 启动期校验 D15 4 维 flag 有效性（MINOR 2）。
//
// 16 combos 中 7 组无效——任何 subagent_mode / semantic_routing 的依赖项在
// specdriven.enabled=false 时被置 true，都是依赖违反。Bootstrap 必须在打印激活
// 组合之前先跑本校验，错误消息明确指出缺失的 prerequisite。
//
// 返回：
//   - nil → 9 组有效组合之一
//   - errs.New(CodeConfigInvalid, ...) → 7 组无效组合之一（fail-fast）
func ValidateFlagCombination(cfg *Config) error {
	combo := SnapshotFeatureFlags(cfg)
	if combo.SpecDrivenEnabled {
		return nil // specdriven 开 → 子维度任意组合都不违反本 invariant
	}

	// specdriven 关；只要 subagent_mode 或 semantic_routing 任一开 = 依赖违反
	violations := []string{}
	if combo.SubagentMode {
		violations = append(violations, "subagent_mode requires specdriven.enabled: true")
	}
	if combo.SemanticRouting {
		violations = append(violations, "skills_semantic_routing requires specdriven.enabled: true")
	}
	if len(violations) == 0 {
		return nil // (F,F,F,*) — 2 组合法 specdriven-off 组合
	}
	return errs.New(
		errs.CodeConfigInvalid,
		"invalid feature flag combination: "+strings.Join(violations, "; ")+
			" (set spec_driven.mode to \"dual\" or \"spec\", or turn off the dependent flags)",
	)
}

// ValidateSkillsConfig 启动期校验 skills 相关配置一致性（task 10.3 一部分）。
//
// 当前规则：
//   - OnDemandEnabled=true 且 MarketplaceURLs 空 → fail-fast（R7 风险化解）
//   - URL 字段 trim 后仍非空，否则视为配置 typo 拒绝
//
// Paths 字段保持软校验（只 warn）——老配置可能留空串，由 finder 过滤。
func ValidateSkillsConfig(sc SkillsConfig) error {
	if !sc.OnDemandEnabled {
		return nil
	}
	urls := 0
	for _, u := range sc.MarketplaceURLs {
		if strings.TrimSpace(u) != "" {
			urls++
		}
	}
	if urls == 0 {
		return errs.New(
			errs.CodeConfigInvalid,
			"agent.skills.on_demand_enabled: true requires agent.skills.marketplace_urls to be non-empty",
		)
	}
	return nil
}
