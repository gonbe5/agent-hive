package feishu

import "testing"

func TestSingleClientRegistry_Get(t *testing.T) {
	reg := NewSingleClientRegistry(&Client{}, DefaultTenantKey)

	if _, err := reg.Get(DefaultTenantKey); err != nil {
		t.Fatalf("Get(default) unexpected error: %v", err)
	}
	if _, err := reg.Get(""); err != nil {
		t.Fatalf("Get(\"\") unexpected error: %v", err)
	}
	if _, err := reg.Get("tenant-x"); err == nil {
		t.Fatal("Get(tenant-x) expected error")
	}
}

func TestSingleClientRegistry_List(t *testing.T) {
	reg := NewSingleClientRegistry(&Client{}, "tenant-a")

	keys := reg.List()
	if len(keys) != 1 || keys[0] != "tenant-a" {
		t.Fatalf("List() = %v, want [tenant-a]", keys)
	}
}

func TestSingleClientRegistry_Register_AllowsReplacingSameTenant(t *testing.T) {
	first := &Client{}
	second := &Client{}
	reg := NewSingleClientRegistry(first, "tenant-a")

	if err := reg.Register("tenant-a", second); err != nil {
		t.Fatalf("Register(same tenant) unexpected error: %v", err)
	}
	got, err := reg.Get("tenant-a")
	if err != nil {
		t.Fatalf("Get(tenant-a) unexpected error after Register: %v", err)
	}
	if got != second {
		t.Fatalf("Get(tenant-a) = %p, want replaced client %p", got, second)
	}
}

func TestSingleClientRegistry_Register_RejectsDifferentTenant(t *testing.T) {
	reg := NewSingleClientRegistry(&Client{}, "tenant-a")

	if err := reg.Register("tenant-b", &Client{}); err == nil {
		t.Fatal("Register(different tenant) expected error")
	}
}

func TestSingleClientRegistry_Unregister_OnlyClearsConfiguredTenant(t *testing.T) {
	reg := NewSingleClientRegistry(&Client{}, "tenant-a")

	if err := reg.Unregister("tenant-b"); err == nil {
		t.Fatal("Unregister(different tenant) expected error")
	}
	if err := reg.Unregister("tenant-a"); err != nil {
		t.Fatalf("Unregister(tenant-a) unexpected error: %v", err)
	}
	if _, err := reg.Get("tenant-a"); err == nil {
		t.Fatal("Get(tenant-a) expected error after Unregister")
	}
	if len(reg.List()) != 0 {
		t.Fatalf("List() after Unregister = %v, want empty", reg.List())
	}
}

func TestSingleClientRegistry_Register_RejectsNilClient(t *testing.T) {
	reg := NewSingleClientRegistry(&Client{}, "tenant-a")

	if err := reg.Register("tenant-a", nil); err == nil {
		t.Fatal("Register(nil client) expected error")
	}
}
