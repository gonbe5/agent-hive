package feishu

import (
	"net/http/httptest"
	"testing"
)

func TestSingleTenantResolver_DefaultsToDefaultTenant(t *testing.T) {
	r := NewSingleTenantResolver("")
	if got := r.FromEvent(); got != DefaultTenantKey {
		t.Fatalf("FromEvent() = %q, want %q", got, DefaultTenantKey)
	}
	got, err := r.FromHTTP(httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("FromHTTP() unexpected error: %v", err)
	}
	if got != DefaultTenantKey {
		t.Fatalf("FromHTTP() = %q, want %q", got, DefaultTenantKey)
	}
}

func TestSingleTenantResolver_UsesConfiguredTenantKey(t *testing.T) {
	r := NewSingleTenantResolver("tenant-a")
	if got := r.FromEvent(); got != "tenant-a" {
		t.Fatalf("FromEvent() = %q, want tenant-a", got)
	}
}
