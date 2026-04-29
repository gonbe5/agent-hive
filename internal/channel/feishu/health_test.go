package feishu

import (
	"testing"
	"time"
)

func TestClientHealthStatus_HealthyBelowPermissionDeniedThreshold(t *testing.T) {
	client := &Client{}
	client.ApplySecurityConfig(3)

	now := time.Now()
	client.observeAPIError(newPermissionDeniedError("code=99991663"), now.Add(-4*time.Minute))
	client.observeAPIError(newPermissionDeniedError("code=10013"), now.Add(-3*time.Minute))

	status := client.HealthStatus(t.Context())
	if status.Degraded {
		t.Fatal("HealthStatus().Degraded = true, want false")
	}
	if status.PermissionDeniedCount != 2 {
		t.Fatalf("HealthStatus().PermissionDeniedCount = %d, want 2", status.PermissionDeniedCount)
	}
	if status.Status != "healthy" {
		t.Fatalf("HealthStatus().Status = %q, want healthy", status.Status)
	}
}

func TestClientHealthStatus_DegradedAfterPermissionDeniedThreshold(t *testing.T) {
	client := &Client{}
	client.ApplySecurityConfig(3)

	now := time.Now()
	client.observeAPIError(newPermissionDeniedError("code=99991663"), now.Add(-4*time.Minute))
	client.observeAPIError(newPermissionDeniedError("code=10013"), now.Add(-3*time.Minute))
	client.observeAPIError(newPermissionDeniedError("permission denied"), now.Add(-2*time.Minute))

	status := client.HealthStatus(t.Context())
	if !status.Degraded {
		t.Fatal("HealthStatus().Degraded = false, want true")
	}
	if status.PermissionDeniedCount != 3 {
		t.Fatalf("HealthStatus().PermissionDeniedCount = %d, want 3", status.PermissionDeniedCount)
	}
	if status.Status != "degraded" {
		t.Fatalf("HealthStatus().Status = %q, want degraded", status.Status)
	}
}

func TestClientHealthStatus_ExpiresOldPermissionDeniedWindow(t *testing.T) {
	client := &Client{}
	client.ApplySecurityConfig(2)

	now := time.Now()
	client.observeAPIError(newPermissionDeniedError("code=99991663"), now.Add(-6*time.Minute))
	client.observeAPIError(newPermissionDeniedError("code=10013"), now.Add(-4*time.Minute))

	status := client.HealthStatus(t.Context())
	if status.PermissionDeniedCount != 1 {
		t.Fatalf("HealthStatus().PermissionDeniedCount = %d, want 1", status.PermissionDeniedCount)
	}
	if status.Degraded {
		t.Fatal("HealthStatus().Degraded = true, want false")
	}
}

func TestClientHealthStatus_ReflectsConfigState(t *testing.T) {
	client := &Client{}
	client.ApplyHealthConfig("cli_xxx", "secret", "verify-token", "encrypt-key")

	status := client.HealthStatus(t.Context())
	if !status.TokenConfigured {
		t.Fatal("HealthStatus().TokenConfigured = false, want true")
	}
	if !status.VerificationConfigured {
		t.Fatal("HealthStatus().VerificationConfigured = false, want true")
	}
	if !status.EncryptKeyConfigured {
		t.Fatal("HealthStatus().EncryptKeyConfigured = false, want true")
	}
}
