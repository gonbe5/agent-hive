package config

import (
	"encoding/json"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

// TestFeishuConfig_Normalize_AckEmoji 覆盖 AckEmoji 归一化的全部分支：
// 空串 / 合法 CamelCase 值 / "none" 哨兵 / 老版全大写（静默迁移）/ 非法值（warn+回退）。
func TestFeishuConfig_Normalize_AckEmoji(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		want       string
		wantWarned bool
	}{
		{"empty_defaults_to_Get", "", "Get", false},
		{"legal_Get_kept", "Get", "Get", false},
		{"legal_Typing_kept", "Typing", "Typing", false},
		// 老版本错把飞书 emoji_type 写成全大写 GET/KEYBOARD，DB 里可能仍存着旧值。
		// Normalize 透明迁移，不 warn——升级不打扰运维，避免 reactions API 231001 报错。
		{"legacy_GET_migrated_to_Get", "GET", "Get", false},
		{"legacy_KEYBOARD_migrated_to_Typing", "KEYBOARD", "Typing", false},
		// "none" 保留原样——renderer 端同时识别 "" 和 "none" 作为 skip 条件。
		// 保留字面量可保证 Normalize 幂等（不会与空串默认值相互转换）。
		{"legal_none_kept_literal", "none", "none", false},
		{"illegal_lowercase_get_warn_fallback", "get", "Get", true},
		{"illegal_random_warn_fallback", "lolwat", "Get", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := FeishuConfig{AckEmoji: tc.input}
			warned := false
			var gotOriginal, gotFallback string
			cfg.Normalize(func(_, original, fallback string) {
				warned = true
				gotOriginal = original
				gotFallback = fallback
			})
			if cfg.AckEmoji != tc.want {
				t.Errorf("AckEmoji = %q, want %q", cfg.AckEmoji, tc.want)
			}
			if warned != tc.wantWarned {
				t.Errorf("warned = %v, want %v", warned, tc.wantWarned)
			}
			if tc.wantWarned {
				if gotOriginal != tc.input {
					t.Errorf("warn original = %q, want %q", gotOriginal, tc.input)
				}
				if gotFallback != "Get" {
					t.Errorf("warn fallback = %q, want Get", gotFallback)
				}
			}
		})
	}
}

// TestFeishuConfig_Normalize_Idempotent 验证 Normalize 幂等性。
// 契约要求：同一 cfg 连续调用两次 Normalize 必须产生相同终态，否则链路中多次调用会产生不一致。
func TestFeishuConfig_Normalize_Idempotent(t *testing.T) {
	cases := []FeishuConfig{
		{AckEmoji: ""},
		{AckEmoji: "none"},
		{AckEmoji: "illegal"},
		{AckEmoji: "Get"},
		{AckEmoji: "Typing"},
		{AckEmoji: "GET"},      // legacy → Get，第二次 Normalize 必须保持 Get
		{AckEmoji: "KEYBOARD"}, // legacy → Typing，第二次 Normalize 必须保持 Typing
		{Renderer: FeishuRendererConfig{ThrottleMs: 0}},
	}
	for _, tc := range cases {
		first := tc
		first.Normalize(nil)
		second := first
		second.Normalize(nil)
		if first.AckEmoji != second.AckEmoji || first.Renderer != second.Renderer {
			t.Errorf("Normalize non-idempotent for input %+v: first=%+v second=%+v", tc, first, second)
		}
	}
}

// TestFeishuConfig_Normalize_ThrottleMs 覆盖 Renderer.ThrottleMs 归一化：
// 0/负值回落到 300，正值保持原样。
func TestFeishuConfig_Normalize_ThrottleMs(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{"zero_defaults_to_300", 0, 300},
		{"negative_defaults_to_300", -1, 300},
		{"positive_kept", 500, 500},
		{"edge_1ms_kept", 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := FeishuConfig{Renderer: FeishuRendererConfig{ThrottleMs: tc.input}}
			cfg.Normalize(nil)
			if cfg.Renderer.ThrottleMs != tc.want {
				t.Errorf("ThrottleMs = %d, want %d", cfg.Renderer.ThrottleMs, tc.want)
			}
		})
	}
}

// TestFeishuConfig_NilWarnFn 显式验证：warn 函数为 nil 时 Normalize 不 panic。
func TestFeishuConfig_NilWarnFn(t *testing.T) {
	cfg := FeishuConfig{AckEmoji: "illegal"}
	cfg.Normalize(nil)
	if cfg.AckEmoji != "Get" {
		t.Errorf("AckEmoji = %q, want Get (illegal should fallback)", cfg.AckEmoji)
	}
}

// TestFeishuConfig_UpgradeFromOldDB 模拟向后兼容：老 DB 里存的 FeishuConfig JSON
// 没有 renderer 段 / 没有 ack_emoji 字段。Unmarshal 后调 Normalize，必须保证：
//   - AckEmoji = "Get"（空串默认值，飞书 reactions API emoji_type 的 CamelCase 默认）
//   - RendererEnabled() = true（Disabled 零值为 false，反向视图应为 true）
//   - ThrottleMs = 300（零值回退）
//
// 对应 MUST-FIX #2（Enabled zero-value 回归风险）的回归护栏。
func TestFeishuConfig_UpgradeFromOldDB(t *testing.T) {
	oldJSON := `{"enabled": true, "app_id": "cli-xxx", "app_secret": "s"}`
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(oldJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg.Normalize(nil)

	if cfg.AckEmoji != "Get" {
		t.Errorf("AckEmoji = %q, want Get", cfg.AckEmoji)
	}
	if !cfg.RendererEnabled() {
		t.Error("RendererEnabled() = false, want true (old DB no `renderer` field must default to enabled)")
	}
	if cfg.Renderer.ThrottleMs != 300 {
		t.Errorf("ThrottleMs = %d, want 300", cfg.Renderer.ThrottleMs)
	}
}

// TestFeishuConfig_RollbackDisabled 验证显式 disabled:true 能关掉 renderer。
func TestFeishuConfig_RollbackDisabled(t *testing.T) {
	rollbackJSON := `{"enabled": true, "renderer": {"disabled": true}}`
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(rollbackJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg.Normalize(nil)
	if cfg.RendererEnabled() {
		t.Error("RendererEnabled() = true, want false after explicit disabled:true")
	}
}

// TestConfig_FeishuRendererDefaults（对应 tasks.md 10.12）：
// 完全空 JSON {} Unmarshal 后再 Normalize，断言一组必保底默认值——
// 这是"用户从未配过 feishu、DB 新建空记录"场景的最小保证：
//   - AckEmoji = "Get"（不是空串、不是 "none"；飞书 reactions API emoji_type CamelCase 默认）
//   - RendererEnabled() = true（Disabled 零值 false → 启用）
//   - Renderer.ThrottleMs = 300
//   - Renderer.ShowAgentProgress = false
//
// 与 TestFeishuConfig_UpgradeFromOldDB 的区别：那个用老版 DB 携带字段的 JSON，
// 这个直接 `{}` 空对象，验证"零字段"路径也能达到同一终态——即 Normalize 的语义
// 对"缺字段"与"零值字段"等价处理（JSON unmarshal 两种情形都得到 Go 零值）。
func TestConfig_FeishuRendererDefaults(t *testing.T) {
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(`{}`), &cfg); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	cfg.Normalize(nil)

	if cfg.AckEmoji != "Get" {
		t.Errorf("AckEmoji = %q, want Get", cfg.AckEmoji)
	}
	if !cfg.RendererEnabled() {
		t.Error("RendererEnabled() = false, want true (empty JSON must default to enabled)")
	}
	if cfg.Renderer.ThrottleMs != 300 {
		t.Errorf("Renderer.ThrottleMs = %d, want 300", cfg.Renderer.ThrottleMs)
	}
	if cfg.Renderer.ShowAgentProgress {
		t.Error("Renderer.ShowAgentProgress = true, want false (zero-value default)")
	}
	if !cfg.InboundContextResolverEnabled() {
		t.Error("InboundContextResolverEnabled() = false, want true (Phase 1 默认开启)")
	}
}

// TestConfig_InvalidAckEmojiFallback（对应 tasks.md 10.13）：
// ack_emoji 写非法值（如 "WEIRD"）→ Normalize 必须：
//  1. 触发 warn 回调（message / original / fallback 三个参数齐全）
//  2. 最终 AckEmoji 归一到 "Get"
//  3. warn.original == 原值（审计用）
//  4. warn.fallback == "Get"（固定）
//
// 与 TestFeishuConfig_Normalize_AckEmoji 子用例的区别：那里是表驱动，这里是
// 契约锁——tasks.md 10.13 点名要求的单测名字，直接落地对应回归护栏。
func TestConfig_InvalidAckEmojiFallback(t *testing.T) {
	invalidJSON := `{"ack_emoji": "WEIRD"}`
	var cfg FeishuConfig
	if err := json.Unmarshal([]byte(invalidJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var gotMsg, gotOriginal, gotFallback string
	warnCalled := 0
	cfg.Normalize(func(msg, original, fallback string) {
		warnCalled++
		gotMsg = msg
		gotOriginal = original
		gotFallback = fallback
	})

	if warnCalled != 1 {
		t.Errorf("warn called %d times, want 1", warnCalled)
	}
	if cfg.AckEmoji != "Get" {
		t.Errorf("AckEmoji = %q, want Get (post-fallback)", cfg.AckEmoji)
	}
	if gotOriginal != "WEIRD" {
		t.Errorf("warn.original = %q, want WEIRD (must echo user input for audit)", gotOriginal)
	}
	if gotFallback != "Get" {
		t.Errorf("warn.fallback = %q, want Get", gotFallback)
	}
	if gotMsg == "" {
		t.Error("warn.message empty, want non-empty (ops needs human-readable reason)")
	}
}

// TestFeishuConfig_Normalize_LegacyMigration 验证 DB 里存的老值在 Normalize 时
// 静默迁移到合法 CamelCase，且不触发 warn 回调——这是线上升级的默契契约，
// 避免用户 DB 里旧数据升级后仍然触发 reactions API 231001 报错。
func TestFeishuConfig_Normalize_LegacyMigration(t *testing.T) {
	cases := []struct {
		legacy   string
		migrated string
	}{
		{"GET", "Get"},
		{"KEYBOARD", "Typing"},
	}
	for _, tc := range cases {
		t.Run(tc.legacy+"_to_"+tc.migrated, func(t *testing.T) {
			cfg := FeishuConfig{AckEmoji: tc.legacy}
			warnCalled := 0
			cfg.Normalize(func(_, _, _ string) { warnCalled++ })
			if cfg.AckEmoji != tc.migrated {
				t.Errorf("AckEmoji = %q, want %q (legacy silent migration)", cfg.AckEmoji, tc.migrated)
			}
			if warnCalled != 0 {
				t.Errorf("warn called %d times, want 0 (legacy migration must be silent)", warnCalled)
			}
		})
	}
}

func TestFeishuConfig_InboundContextResolverEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  FeishuConfig
		want bool
	}{
		{
			name: "default_enabled",
			cfg:  FeishuConfig{},
			want: true,
		},
		{
			name: "explicit_disabled",
			cfg: FeishuConfig{
				Inbound: FeishuInboundConfig{EnableContextResolver: boolPtr(false)},
			},
			want: false,
		},
		{
			name: "explicit_enabled",
			cfg: FeishuConfig{
				Inbound: FeishuInboundConfig{EnableContextResolver: boolPtr(true)},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.Normalize(nil)
			if got := tc.cfg.InboundContextResolverEnabled(); got != tc.want {
				t.Fatalf("InboundContextResolverEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
