package config

import (
	"strings"
	"testing"
	"time"
)

// TestFeishuConfig_Validate_DualIngressFatal 覆盖 P0-#14 唯一 invariant：
// LongconnEnabled && WebhookURL != "" → fatal。
//
// 红队链 B：webhook + longconn 同进程并存 → 飞书事件双投 → DB dedup 失效时
// 单消息触发两次 renderer → 用户看到两条相同回复。Validate 必须 fail-closed。
//
// 错误信息必须包含 "dual ingress" 字样，方便启动日志直接定位。
func TestFeishuConfig_Validate_DualIngressFatal(t *testing.T) {
	cfg := FeishuConfig{
		Enabled:         true,
		LongconnEnabled: true,
		WebhookURL:      "https://example.com/feishu/webhook",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() expected error for dual ingress, got nil")
	}
	if !strings.Contains(err.Error(), "dual ingress") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "dual ingress")
	}
}

// TestFeishuConfig_Validate_SingleIngressOK 验证三条单入口 / 全关闭路径都放行：
//   - 仅 webhook（生产 Phase 0 默认）
//   - 仅 longconn（开发 / 内网无公网）
//   - 二者都未配置（早期未接入）
func TestFeishuConfig_Validate_SingleIngressOK(t *testing.T) {
	cases := []struct {
		name string
		cfg  FeishuConfig
	}{
		{
			name: "webhook_only",
			cfg:  FeishuConfig{WebhookURL: "https://example.com/feishu/webhook"},
		},
		{
			name: "longconn_only",
			cfg:  FeishuConfig{Reliability: FeishuReliabilityConfig{LongconnEnabled: true}},
		},
		{
			name: "both_disabled",
			cfg:  FeishuConfig{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.cfg.Validate(); err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestFeishuConfig_Validate_ExplicitIngressMode(t *testing.T) {
	cases := []struct {
		name    string
		cfg     FeishuConfig
		wantErr string
	}{
		{
			name: "explicit_webhook_still_rejects_legacy_dual_shape",
			cfg: FeishuConfig{
				IngressMode: FeishuIngressModeWebhook,
				Reliability: FeishuReliabilityConfig{LongconnEnabled: true},
				WebhookURL:  "https://example.com/feishu/webhook",
			},
			wantErr: "dual ingress",
		},
		{
			name: "explicit_longconn_still_rejects_legacy_dual_shape",
			cfg: FeishuConfig{
				IngressMode: FeishuIngressModeLongconn,
				Reliability: FeishuReliabilityConfig{LongconnEnabled: true},
				WebhookURL:  "https://example.com/feishu/webhook",
			},
			wantErr: "dual ingress",
		},
		{
			name: "explicit_invalid_mode_fails_closed",
			cfg: FeishuConfig{
				IngressMode: "invalid",
			},
			wantErr: "ingress_mode",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestFeishuConfig_ResolvedIngressMode(t *testing.T) {
	cases := []struct {
		name string
		cfg  FeishuConfig
		want FeishuIngressMode
	}{
		{
			name: "explicit_webhook_wins",
			cfg: FeishuConfig{
				IngressMode:     FeishuIngressModeWebhook,
				LongconnEnabled: true,
			},
			want: FeishuIngressModeWebhook,
		},
		{
			name: "explicit_longconn_wins",
			cfg: FeishuConfig{
				IngressMode: FeishuIngressModeLongconn,
				WebhookURL:  "https://example.com/feishu/webhook",
			},
			want: FeishuIngressModeLongconn,
		},
		{
			name: "legacy_longconn_enabled_maps_to_longconn",
			cfg: FeishuConfig{
				LongconnEnabled: true,
			},
			want: FeishuIngressModeLongconn,
		},
		{
			name: "reliability_longconn_enabled_maps_to_longconn",
			cfg: FeishuConfig{
				Reliability: FeishuReliabilityConfig{LongconnEnabled: true},
			},
			want: FeishuIngressModeLongconn,
		},
		{
			name: "legacy_default_maps_to_webhook",
			cfg:  FeishuConfig{},
			want: FeishuIngressModeWebhook,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.ResolvedIngressMode(); got != c.want {
				t.Fatalf("ResolvedIngressMode() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFeishuConfig_ReliabilityResolvedDefaults(t *testing.T) {
	cfg := FeishuConfig{}
	if got := cfg.HeartbeatStaleWindowResolved(); got != 60*time.Second {
		t.Fatalf("HeartbeatStaleWindowResolved() = %v, want %v", got, 60*time.Second)
	}
	if got := cfg.GapFetchMaxWindowResolved(); got != 10*time.Minute {
		t.Fatalf("GapFetchMaxWindowResolved() = %v, want %v", got, 10*time.Minute)
	}
	if cfg.GapFetchEnabledResolved() {
		t.Fatal("GapFetchEnabledResolved() = true, want false")
	}
}

func TestFeishuConfig_Validate_ReliabilityRejectsNegativeDurations(t *testing.T) {
	cases := []FeishuConfig{
		{Reliability: FeishuReliabilityConfig{HeartbeatStaleWindow: -time.Second}},
		{Reliability: FeishuReliabilityConfig{GapFetchMaxWindow: -time.Second}},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected Validate() to reject negative reliability duration")
		}
	}
}

func TestFeishuConfig_IdentityResolvedDefaults(t *testing.T) {
	cfg := FeishuConfig{}
	if got := cfg.IdentityUserCacheSizeResolved(); got != 5000 {
		t.Fatalf("IdentityUserCacheSizeResolved() = %d, want 5000", got)
	}
	if got := cfg.IdentityUserCacheTTLResolved(); got != 12*time.Hour {
		t.Fatalf("IdentityUserCacheTTLResolved() = %v, want %v", got, 12*time.Hour)
	}
	if !cfg.GroupEnrichEnabledResolved() {
		t.Fatal("GroupEnrichEnabledResolved() = false, want true")
	}
	if got := cfg.IdentityNameLocaleResolved(); got != "zh-CN" {
		t.Fatalf("IdentityNameLocaleResolved() = %q, want zh-CN", got)
	}
}

func TestFeishuConfig_IdentityNameLocaleResolved_ByRegion(t *testing.T) {
	cfg := FeishuConfig{Region: "intl"}
	if got := cfg.IdentityNameLocaleResolved(); got != "en-US" {
		t.Fatalf("IdentityNameLocaleResolved() = %q, want en-US", got)
	}
}

func TestFeishuConfig_OutboundResolvedDefaults(t *testing.T) {
	cfg := FeishuConfig{}
	if got := cfg.OutboundGlobalQPSResolved(); got != 45 {
		t.Fatalf("OutboundGlobalQPSResolved() = %d, want 45", got)
	}
	if got := cfg.OutboundPerChatQPSResolved(); got != 8 {
		t.Fatalf("OutboundPerChatQPSResolved() = %d, want 8", got)
	}
	if got := cfg.OutboundMaxRetriesResolved(); got != 3 {
		t.Fatalf("OutboundMaxRetriesResolved() = %d, want 3", got)
	}
	if cfg.BinaryTransferEnabledResolved() {
		t.Fatal("BinaryTransferEnabledResolved() = true, want false")
	}
}

func TestFeishuConfig_OutboundResolvedCustomValues(t *testing.T) {
	cfg := FeishuConfig{
		Outbound: FeishuOutboundConfig{
			GlobalQPS:            20,
			PerChatQPS:           4,
			MaxRetries:           5,
			EnableBinaryTransfer: true,
		},
	}
	if got := cfg.OutboundGlobalQPSResolved(); got != 20 {
		t.Fatalf("OutboundGlobalQPSResolved() = %d, want 20", got)
	}
	if got := cfg.OutboundPerChatQPSResolved(); got != 4 {
		t.Fatalf("OutboundPerChatQPSResolved() = %d, want 4", got)
	}
	if got := cfg.OutboundMaxRetriesResolved(); got != 5 {
		t.Fatalf("OutboundMaxRetriesResolved() = %d, want 5", got)
	}
	if !cfg.BinaryTransferEnabledResolved() {
		t.Fatal("BinaryTransferEnabledResolved() = false, want true")
	}
}

func TestFeishuConfig_SecurityResolvedDefaults(t *testing.T) {
	cfg := FeishuConfig{}
	if got := cfg.PermissionDegradeThresholdResolved(); got != 5 {
		t.Fatalf("PermissionDegradeThresholdResolved() = %d, want 5", got)
	}
}

func TestFeishuConfig_SecurityResolvedCustomValues(t *testing.T) {
	cfg := FeishuConfig{
		Security: FeishuSecurityConfig{
			PermissionDegradeThreshold: 7,
		},
	}
	if got := cfg.PermissionDegradeThresholdResolved(); got != 7 {
		t.Fatalf("PermissionDegradeThresholdResolved() = %d, want 7", got)
	}
}
