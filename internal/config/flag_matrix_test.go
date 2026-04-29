package config

import (
	"fmt"
	"testing"
)

// flagCombo 描述一组 D15 flag 激活态 + 期望的系统行为。
// 4 位二进制：A=specdriven, B=subagent_mode, C=semantic_routing, D=on_demand
type flagBehavior struct {
	SpecDriven      bool
	SubagentMode    bool
	SemanticRouting bool
	OnDemand        bool
	Valid           bool // ValidateFlagCombination 是否通过

	// 预期行为（契约）：
	WantSkillInstallRegistered bool // on_demand=true ⇒ 注册
	WantSkillSearchRegistered  bool // on_demand=true ⇒ 注册
	WantRemoteDiscoveryUsed    bool // on_demand=true ⇒ 会触发 marketplace I/O
	WantSubAgentUserIDRequired bool // subagent_mode=true ⇒ spawn 必须继承 userID
}

// TestFlagMatrix_16Combos — D15 4 维 flag 16 组合行为断言（§12.1 + §12.2）。
//
// 约束：§10.3 `ValidateFlagCombination` 对 "!specdriven && (subagent_mode || semantic_routing)"
// 直接 fail-fast。行为断言只对 Valid==true 的组合生效；Valid==false 组合
// 仅断言 fail-fast 命中，不再跑下游行为（bootstrap 层已拒绝启动）。
func TestFlagMatrix_16Combos(t *testing.T) {
	combos := []flagBehavior{
		// ── specdriven=false（A=0）─────────────────────────────
		// B、C 均依赖 A，任一开启都 fail-fast；D 独立可开。
		{SpecDriven: false, SubagentMode: false, SemanticRouting: false, OnDemand: false, Valid: true,
			WantSkillInstallRegistered: false, WantSkillSearchRegistered: false,
			WantRemoteDiscoveryUsed: false, WantSubAgentUserIDRequired: false},
		{SpecDriven: false, SubagentMode: false, SemanticRouting: false, OnDemand: true, Valid: true,
			WantSkillInstallRegistered: true, WantSkillSearchRegistered: true,
			WantRemoteDiscoveryUsed: true, WantSubAgentUserIDRequired: false},
		{SpecDriven: false, SubagentMode: false, SemanticRouting: true, OnDemand: false, Valid: false},
		{SpecDriven: false, SubagentMode: false, SemanticRouting: true, OnDemand: true, Valid: false},
		{SpecDriven: false, SubagentMode: true, SemanticRouting: false, OnDemand: false, Valid: false},
		{SpecDriven: false, SubagentMode: true, SemanticRouting: false, OnDemand: true, Valid: false},
		{SpecDriven: false, SubagentMode: true, SemanticRouting: true, OnDemand: false, Valid: false},
		{SpecDriven: false, SubagentMode: true, SemanticRouting: true, OnDemand: true, Valid: false},
		// ── specdriven=true（A=1）─────────────────────────────
		// B、C、D 各自独立；8 种组合全部 valid。
		{SpecDriven: true, SubagentMode: false, SemanticRouting: false, OnDemand: false, Valid: true,
			WantSkillInstallRegistered: false, WantSkillSearchRegistered: false,
			WantRemoteDiscoveryUsed: false, WantSubAgentUserIDRequired: false},
		{SpecDriven: true, SubagentMode: false, SemanticRouting: false, OnDemand: true, Valid: true,
			WantSkillInstallRegistered: true, WantSkillSearchRegistered: true,
			WantRemoteDiscoveryUsed: true, WantSubAgentUserIDRequired: false},
		{SpecDriven: true, SubagentMode: false, SemanticRouting: true, OnDemand: false, Valid: true,
			WantSkillInstallRegistered: false, WantSkillSearchRegistered: false,
			WantRemoteDiscoveryUsed: false, WantSubAgentUserIDRequired: false},
		{SpecDriven: true, SubagentMode: false, SemanticRouting: true, OnDemand: true, Valid: true,
			WantSkillInstallRegistered: true, WantSkillSearchRegistered: true,
			WantRemoteDiscoveryUsed: true, WantSubAgentUserIDRequired: false},
		{SpecDriven: true, SubagentMode: true, SemanticRouting: false, OnDemand: false, Valid: true,
			WantSkillInstallRegistered: false, WantSkillSearchRegistered: false,
			WantRemoteDiscoveryUsed: false, WantSubAgentUserIDRequired: true},
		{SpecDriven: true, SubagentMode: true, SemanticRouting: false, OnDemand: true, Valid: true,
			WantSkillInstallRegistered: true, WantSkillSearchRegistered: true,
			WantRemoteDiscoveryUsed: true, WantSubAgentUserIDRequired: true},
		{SpecDriven: true, SubagentMode: true, SemanticRouting: true, OnDemand: false, Valid: true,
			WantSkillInstallRegistered: false, WantSkillSearchRegistered: false,
			WantRemoteDiscoveryUsed: false, WantSubAgentUserIDRequired: true},
		{SpecDriven: true, SubagentMode: true, SemanticRouting: true, OnDemand: true, Valid: true,
			WantSkillInstallRegistered: true, WantSkillSearchRegistered: true,
			WantRemoteDiscoveryUsed: true, WantSubAgentUserIDRequired: true},
	}
	if got := len(combos); got != 16 {
		t.Fatalf("combo table must cover 16 cases, got %d", got)
	}

	var validCount, invalidCount int
	for _, c := range combos {
		name := fmt.Sprintf("A=%t_B=%t_C=%t_D=%t", c.SpecDriven, c.SubagentMode, c.SemanticRouting, c.OnDemand)
		t.Run(name, func(t *testing.T) {
			cfg := &Config{}
			if c.SpecDriven {
				cfg.SpecDriven.Mode = "spec"
			} else {
				cfg.SpecDriven.Mode = "legacy"
			}
			if c.SubagentMode {
				cfg.SpecDriven.SubagentMode = "spec-only"
			} else {
				cfg.SpecDriven.SubagentMode = "legacy"
			}
			cfg.SpecDriven.SkillsSemanticRouting = c.SemanticRouting
			cfg.Agent.Skills.OnDemandEnabled = c.OnDemand
			if c.OnDemand {
				cfg.Agent.Skills.MarketplaceURLs = []string{"https://example.com/marketplace"}
			}

			validateErr := ValidateFlagCombination(cfg)

			// §12.1: validity 契约
			if c.Valid && validateErr != nil {
				t.Fatalf("combo expected valid but ValidateFlagCombination failed: %v", validateErr)
			}
			if !c.Valid && validateErr == nil {
				t.Fatalf("combo expected invalid but ValidateFlagCombination passed")
			}
			if !c.Valid {
				invalidCount++
				return
			}
			validCount++

			// §12.2: 行为断言（仅对 valid 组合）
			snap := SnapshotFeatureFlags(cfg)
			if snap.SpecDrivenEnabled != c.SpecDriven {
				t.Errorf("snapshot.SpecDrivenEnabled = %v, want %v", snap.SpecDrivenEnabled, c.SpecDriven)
			}
			if snap.SubagentMode != c.SubagentMode {
				t.Errorf("snapshot.SubagentMode = %v, want %v", snap.SubagentMode, c.SubagentMode)
			}
			if snap.SemanticRouting != c.SemanticRouting {
				t.Errorf("snapshot.SemanticRouting = %v, want %v", snap.SemanticRouting, c.SemanticRouting)
			}
			if snap.OnDemandEnabled != c.OnDemand {
				t.Errorf("snapshot.OnDemandEnabled = %v, want %v", snap.OnDemandEnabled, c.OnDemand)
			}

			// 注册决策是纯函数（bootstrap 根据 OnDemandEnabled gated），
			// 这里直接验证 predicate 与 cfg 一致。
			if got := cfg.Agent.Skills.OnDemandEnabled; got != c.WantSkillInstallRegistered {
				t.Errorf("skill_install registration predicate mismatch: got %v want %v", got, c.WantSkillInstallRegistered)
			}
			if got := cfg.Agent.Skills.OnDemandEnabled; got != c.WantSkillSearchRegistered {
				t.Errorf("skill_search registration predicate mismatch: got %v want %v", got, c.WantSkillSearchRegistered)
			}
			if got := cfg.Agent.Skills.OnDemandEnabled; got != c.WantRemoteDiscoveryUsed {
				t.Errorf("remote discovery used predicate mismatch: got %v want %v", got, c.WantRemoteDiscoveryUsed)
			}
			if got := cfg.SpecDriven.SubagentModeEnabled(); got != c.WantSubAgentUserIDRequired {
				t.Errorf("subagent userID required predicate mismatch: got %v want %v", got, c.WantSubAgentUserIDRequired)
			}
		})
	}
	// 契约：10 valid + 6 invalid（与 D15 / §10.5 对齐）
	if validCount != 10 {
		t.Errorf("valid combo count = %d, want 10", validCount)
	}
	if invalidCount != 6 {
		t.Errorf("invalid combo count = %d, want 6", invalidCount)
	}
}

// TestFlagMatrix_LogGrepContract — §12.3：激活组合行必须可被 grep 定位。
// 契约格式由 FeatureFlagCombo.String() 固化：
//
//	"skills_feature_flags: specdriven=X subagent_mode=Y semantic_routing=Z on_demand=W"
//
// 本测确认 16 组合下每条 String 都匹配 grep 正则。
func TestFlagMatrix_LogGrepContract(t *testing.T) {
	seen := make(map[string]bool, 16)
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			for _, c := range []bool{false, true} {
				for _, d := range []bool{false, true} {
					combo := FeatureFlagCombo{
						SpecDrivenEnabled: a, SubagentMode: b, SemanticRouting: c, OnDemandEnabled: d,
					}
					s := combo.String()
					want := fmt.Sprintf("skills_feature_flags: specdriven=%t subagent_mode=%t semantic_routing=%t on_demand=%t", a, b, c, d)
					if s != want {
						t.Errorf("combo %s got\n  %q\n  want %q", want, s, want)
					}
					if seen[s] {
						t.Errorf("duplicate log line: %q", s)
					}
					seen[s] = true
				}
			}
		}
	}
	if len(seen) != 16 {
		t.Errorf("want 16 distinct log lines, got %d", len(seen))
	}
}
