package bootstrap

import (
	"context"
	"testing"

	"github.com/chef-guo/agents-hive/internal/auth"
)

func TestAuthAdminChecker_Anonymous(t *testing.T) {
	c := NewAuthAdminChecker()
	if c.IsAdmin(context.Background(), "") {
		t.Error("anonymous ctx must not be admin")
	}
	if c.IsAdmin(context.Background(), "any-userid") {
		t.Error("userID param alone must never make anon admin")
	}
}

func TestAuthAdminChecker_RoleUser(t *testing.T) {
	c := NewAuthAdminChecker()
	u := &auth.User{ID: "alice", Role: "user", Status: "active"}
	ctx := auth.WithUser(context.Background(), u)
	if c.IsAdmin(ctx, "alice") {
		t.Error("role=user must not be admin")
	}
}

func TestAuthAdminChecker_RoleAdminActive(t *testing.T) {
	c := NewAuthAdminChecker()
	u := &auth.User{ID: "root", Role: "admin", Status: "active"}
	ctx := auth.WithUser(context.Background(), u)
	if !c.IsAdmin(ctx, "root") {
		t.Error("role=admin active must be admin")
	}
}

// TestAuthAdminChecker_DisabledAdminDenied — disabled admin account must NOT
// retain privileges even if JWT still carries role=admin.
func TestAuthAdminChecker_DisabledAdminDenied(t *testing.T) {
	c := NewAuthAdminChecker()
	u := &auth.User{ID: "root", Role: "admin", Status: "disabled"}
	ctx := auth.WithUser(context.Background(), u)
	if c.IsAdmin(ctx, "root") {
		t.Error("disabled admin must not be admin")
	}
}

// TestAuthAdminChecker_UserIDArgIgnored — the userID param is deliberately
// ignored; only ctx-bound auth.User decides. This prevents tool inputs from
// forging userID to escalate.
func TestAuthAdminChecker_UserIDArgIgnored(t *testing.T) {
	c := NewAuthAdminChecker()
	u := &auth.User{ID: "alice", Role: "user", Status: "active"}
	ctx := auth.WithUser(context.Background(), u)
	// 攻击者传入 "root" 假装是 admin —— 必须看 ctx user (alice, role=user)
	if c.IsAdmin(ctx, "root") {
		t.Error("userID param must not override ctx user — would be a privilege escalation bug")
	}
}
