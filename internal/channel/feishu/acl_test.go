package feishu

import (
	"context"
	"errors"
	"testing"
)

func TestACL_EmptyTenantFailsClosed(t *testing.T) {
	acl := NewStaticAllowlistACL(nil)
	allowed, err := acl.CanExecute(context.Background(), "", "oc_chat", "ou_user", true, "reset")
	if err == nil || allowed {
		t.Fatalf("missing tenant must fail closed, allowed=%v err=%v", allowed, err)
	}
}

func TestACL_GroupResetRequiresAllowlist(t *testing.T) {
	acl := NewStaticAllowlistACL(map[string][]string{
		"tenant-a": {"ou-admin"},
	})

	allowed, err := acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-user", false, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("non allowlisted group user must be denied")
	}

	allowed, err = acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-admin", false, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("allowlisted group user must be allowed")
	}
}

func TestACL_DirectResetAlwaysAllowed(t *testing.T) {
	acl := NewStaticAllowlistACL(nil)
	allowed, err := acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-user", true, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("direct chat reset must be allowed")
	}
}

type stubGroupAdminChecker struct {
	admins map[string]map[string]bool
	err    error
}

func (s stubGroupAdminChecker) IsGroupAdmin(_ context.Context, _, chatID, openID string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.admins[chatID][openID], nil
}

func TestACL_GroupResetUsesGroupAdminChecker(t *testing.T) {
	acl := NewGroupAdminACL(stubGroupAdminChecker{
		admins: map[string]map[string]bool{
			"oc_chat": {
				"ou-admin": true,
			},
		},
	}, nil)

	allowed, err := acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-user", false, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("non-admin group user must be denied")
	}

	allowed, err = acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-admin", false, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("group admin must be allowed")
	}
}

func TestACL_GroupResetAllowsConfiguredSuperAdmin(t *testing.T) {
	acl := NewGroupAdminACL(stubGroupAdminChecker{}, []string{"ou-super"})

	allowed, err := acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-super", false, "reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("super admin must bypass group admin check")
	}
}

func TestACL_GroupResetCheckerErrorFailsClosed(t *testing.T) {
	acl := NewGroupAdminACL(stubGroupAdminChecker{err: errors.New("boom")}, nil)

	allowed, err := acl.CanExecute(context.Background(), "tenant-a", "oc_chat", "ou-user", false, "reset")
	if err == nil {
		t.Fatal("expected checker error")
	}
	if allowed {
		t.Fatal("checker error must fail closed")
	}
}
