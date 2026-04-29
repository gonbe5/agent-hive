package config

import (
	"strings"
	"testing"
)

// TestValidateSkillsConfig_OnDemandRequiresMarketplace — task 10.3 fail-fast 主路径。
func TestValidateSkillsConfig_OnDemandRequiresMarketplace(t *testing.T) {
	cases := []struct {
		name    string
		sc      SkillsConfig
		wantErr bool
		wantMsg string
	}{
		{
			name:    "on_demand_off_no_marketplace_ok",
			sc:      SkillsConfig{OnDemandEnabled: false},
			wantErr: false,
		},
		{
			name:    "on_demand_off_with_marketplace_ok",
			sc:      SkillsConfig{OnDemandEnabled: false, MarketplaceURLs: []string{"https://example.com"}},
			wantErr: false,
		},
		{
			name:    "on_demand_on_no_marketplace_rejected",
			sc:      SkillsConfig{OnDemandEnabled: true},
			wantErr: true,
			wantMsg: "agent.skills.on_demand_enabled: true requires agent.skills.marketplace_urls",
		},
		{
			name:    "on_demand_on_empty_string_rejected",
			sc:      SkillsConfig{OnDemandEnabled: true, MarketplaceURLs: []string{"", "   "}},
			wantErr: true,
			wantMsg: "requires agent.skills.marketplace_urls to be non-empty",
		},
		{
			name:    "on_demand_on_with_marketplace_ok",
			sc:      SkillsConfig{OnDemandEnabled: true, MarketplaceURLs: []string{"https://example.com"}},
			wantErr: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSkillsConfig(c.sc)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.wantMsg) {
				t.Errorf("error msg %q does not contain %q", err.Error(), c.wantMsg)
			}
		})
	}
}

// TestValidateFlagCombination_AllSixteen — 16 组合全量枚举；校验 6 组违反 +
// 10 组合 passes (9 D15-listed + 1 unlisted-but-valid (T,T,F,T))。
//
// 注：tasks.md 说 "7 组无效"，是按 D15 列出的 9 组 + 其余 7 组未列 算的；
// 真正依赖违反只 6 组（specdriven=F 且 subagent_mode/semantic 任一 T）。
// (T,T,F,T) 技术上满足依赖约束，D15 未列仅因表格不穷举——validator 不拒绝。
func TestValidateFlagCombination_AllSixteen(t *testing.T) {
	type combo struct {
		specdriven, subagent, semantic, onDemand bool
	}
	// 有效组合：specdriven=T 的 8 种 + specdriven=F 且 subagent=F 且 semantic=F 的 2 种
	validSet := map[combo]bool{
		{false, false, false, false}: true,
		{false, false, false, true}:  true,
		{true, false, false, false}:  true,
		{true, false, false, true}:   true,
		{true, false, true, false}:   true,
		{true, false, true, true}:    true,
		{true, true, false, false}:   true,
		{true, true, false, true}:    true, // D15 未列但依赖无违反 → validator pass
		{true, true, true, false}:    true,
		{true, true, true, true}:     true,
	}

	invalidCount := 0
	validCount := 0

	for specdriven := 0; specdriven < 2; specdriven++ {
		for subagent := 0; subagent < 2; subagent++ {
			for semantic := 0; semantic < 2; semantic++ {
				for onDemand := 0; onDemand < 2; onDemand++ {
					cfg := &Config{}
					if specdriven == 1 {
						cfg.SpecDriven.Mode = "spec"
					} else {
						cfg.SpecDriven.Mode = DefaultSpecDrivenMode
					}
					if subagent == 1 {
						cfg.SpecDriven.SubagentMode = "spec-only"
					} else {
						cfg.SpecDriven.SubagentMode = "legacy"
					}
					cfg.SpecDriven.SkillsSemanticRouting = semantic == 1
					cfg.Agent.Skills.OnDemandEnabled = onDemand == 1

					c := combo{specdriven == 1, subagent == 1, semantic == 1, onDemand == 1}
					err := ValidateFlagCombination(cfg)
					if validSet[c] {
						validCount++
						if err != nil {
							t.Errorf("combo %+v should be valid, got err: %v", c, err)
						}
					} else {
						invalidCount++
						if err == nil {
							t.Errorf("combo %+v should be invalid, got nil err", c)
						}
					}
				}
			}
		}
	}
	if validCount != 10 || invalidCount != 6 {
		t.Errorf("combo counts: valid=%d invalid=%d; want 10/6", validCount, invalidCount)
	}
}

// TestValidateFlagCombination_ErrorMessageClarity — 错误消息必须明确指出
// prerequisite，否则运维无法诊断。
func TestValidateFlagCombination_ErrorMessageClarity(t *testing.T) {
	cfg := &Config{}
	cfg.SpecDriven.Mode = DefaultSpecDrivenMode // specdriven=off
	cfg.SpecDriven.SubagentMode = "spec-only"
	err := ValidateFlagCombination(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "subagent_mode requires specdriven.enabled: true") {
		t.Errorf("error message missing subagent_mode prerequisite: %v", err)
	}

	cfg = &Config{}
	cfg.SpecDriven.Mode = DefaultSpecDrivenMode
	cfg.SpecDriven.SkillsSemanticRouting = true
	err = ValidateFlagCombination(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "skills_semantic_routing requires specdriven.enabled: true") {
		t.Errorf("error message missing semantic_routing prerequisite: %v", err)
	}
}

// TestFeatureFlagCombo_StringFormat — bootstrap 激活日志 grep 契约（task 10.4 + 12.3）。
func TestFeatureFlagCombo_StringFormat(t *testing.T) {
	cfg := &Config{}
	cfg.SpecDriven.Mode = "spec"
	cfg.SpecDriven.SubagentMode = "spec-only"
	cfg.SpecDriven.SkillsSemanticRouting = true
	cfg.Agent.Skills.OnDemandEnabled = true

	combo := SnapshotFeatureFlags(cfg)
	s := combo.String()
	expected := "skills_feature_flags: specdriven=true subagent_mode=true semantic_routing=true on_demand=true"
	if s != expected {
		t.Errorf("format mismatch:\n  got:  %s\n  want: %s", s, expected)
	}
	// 反向校验：全 off
	zero := FeatureFlagCombo{}
	expectZero := "skills_feature_flags: specdriven=false subagent_mode=false semantic_routing=false on_demand=false"
	if zero.String() != expectZero {
		t.Errorf("zero format mismatch: %s", zero.String())
	}
}

// TestSpecDrivenConfig_EnabledSemantics — Mode 到 Enabled() 的映射固化。
func TestSpecDrivenConfig_EnabledSemantics(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"", false},        // 零值视为 legacy
		{"legacy", false},  // 默认 mode
		{"dual", true},     // 双跑模式，视为开
		{"spec", true},     // spec primary，视为开
		{"unknown", true},  // 非法值 fail-closed 在 intake 里处理，此处按"非 legacy 即开"
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			s := SpecDrivenConfig{Mode: c.mode}
			if got := s.Enabled(); got != c.want {
				t.Errorf("Mode=%q Enabled()=%v, want %v", c.mode, got, c.want)
			}
		})
	}
}

// TestSpecDrivenConfig_SubagentModeEnabledSemantics — SubagentMode 到
// SubagentModeEnabled() 的映射固化。
func TestSpecDrivenConfig_SubagentModeEnabledSemantics(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"", false},
		{"legacy", false},
		{"dual", true},
		{"spec-only", true},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			s := SpecDrivenConfig{SubagentMode: c.mode}
			if got := s.SubagentModeEnabled(); got != c.want {
				t.Errorf("SubagentMode=%q SubagentModeEnabled()=%v, want %v", c.mode, got, c.want)
			}
		})
	}
}
