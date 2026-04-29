package feishu

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/config"
)

func TestClientReloadFromConfig_UpdatesOutboundAndSecuritySettings(t *testing.T) {
	client := NewClient("app-id", "app-secret", nil)

	err := client.ReloadFromConfig(config.FeishuConfig{
		Outbound: config.FeishuOutboundConfig{
			GlobalQPS:  12,
			PerChatQPS: 4,
			MaxRetries: 7,
		},
		Security: config.FeishuSecurityConfig{
			PermissionDegradeThreshold: 2,
		},
	})
	if err != nil {
		t.Fatalf("ReloadFromConfig() error = %v", err)
	}

	if client.maxRetries != 7 {
		t.Fatalf("maxRetries = %d, want 7", client.maxRetries)
	}
	if client.health.permissionDegradeThreshold != 2 {
		t.Fatalf("permissionDegradeThreshold = %d, want 2", client.health.permissionDegradeThreshold)
	}
	if client.rateLimiter == nil {
		t.Fatal("rateLimiter = nil, want initialized")
	}
}

func TestWebhookHandlerReloadFromConfig_UpdatesSecuritySettings(t *testing.T) {
	h := NewWebhookHandler("old-token", "old-encrypt", nil, nil).WithEventEncryptEnabled(false)

	err := h.ReloadFromConfig(config.FeishuConfig{
		VerificationToken:   "new-token",
		EncryptKey:          "new-encrypt",
		EventEncryptEnabled: true,
	})
	if err != nil {
		t.Fatalf("ReloadFromConfig() error = %v", err)
	}
	if h.verificationToken != "new-token" {
		t.Fatalf("verificationToken = %q, want new-token", h.verificationToken)
	}
	if h.encryptKey != "new-encrypt" {
		t.Fatalf("encryptKey = %q, want new-encrypt", h.encryptKey)
	}
	if !h.eventEncryptEnabled {
		t.Fatal("eventEncryptEnabled = false, want true")
	}
}

func TestPluginReloadFromConfig_UpdatesWebhookAndClient(t *testing.T) {
	p := New(config.FeishuConfig{}, nil, nil)

	err := p.ReloadFromConfig(config.FeishuConfig{
		VerificationToken:   "reload-token",
		EncryptKey:          "reload-encrypt",
		EventEncryptEnabled: true,
		Outbound: config.FeishuOutboundConfig{
			GlobalQPS:  12,
			PerChatQPS: 4,
			MaxRetries: 7,
		},
		Security: config.FeishuSecurityConfig{
			PermissionDegradeThreshold: 2,
		},
	})
	if err != nil {
		t.Fatalf("ReloadFromConfig() error = %v", err)
	}
	if p.client.maxRetries != 7 {
		t.Fatalf("client.maxRetries = %d, want 7", p.client.maxRetries)
	}
	if p.webhook.verificationToken != "reload-token" {
		t.Fatalf("webhook.verificationToken = %q, want reload-token", p.webhook.verificationToken)
	}
	if p.webhook.encryptKey != "reload-encrypt" {
		t.Fatalf("webhook.encryptKey = %q, want reload-encrypt", p.webhook.encryptKey)
	}
	if !p.webhook.eventEncryptEnabled {
		t.Fatal("webhook.eventEncryptEnabled = false, want true")
	}
}
