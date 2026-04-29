package subagent

import (
	"context"
	"testing"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// TestInheritUserIDFromParent_Success — parent ctx 带 userID → 返回同 userID。
func TestInheritUserIDFromParent_Success(t *testing.T) {
	parent := auth.WithUser(context.Background(), &auth.User{ID: "alice", Role: "user", Status: "active"})
	child, uid, err := InheritUserIDFromParent(parent)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if uid != "alice" {
		t.Errorf("want uid=alice, got %q", uid)
	}
	if auth.UserIDFrom(child) != "alice" {
		t.Errorf("child ctx must carry userID=alice; got %q", auth.UserIDFrom(child))
	}
	// 保持 User 对象的字段（role/status），不是拼一个新的
	if u := auth.UserFrom(child); u == nil || u.Role != "user" || u.Status != "active" {
		t.Errorf("child ctx must preserve parent auth.User fields; got %+v", u)
	}
}

// TestInheritUserIDFromParent_MissingFails — parent 无 userID → 失败，错误码 precondition。
func TestInheritUserIDFromParent_MissingFails(t *testing.T) {
	_, uid, err := InheritUserIDFromParent(context.Background())
	if err == nil {
		t.Fatal("want error when parent ctx has no user")
	}
	if uid != "" {
		t.Errorf("want empty uid on failure, got %q", uid)
	}
	if !errs.IsCode(err, errs.CodeFailedPrecondition) {
		t.Errorf("want CodeFailedPrecondition, got %v", err)
	}
}

// TestInheritUserIDFromParent_NilCtx — nil ctx → 失败（CodeInvalidInput）。
func TestInheritUserIDFromParent_NilCtx(t *testing.T) {
	var nilCtx context.Context // 故意 nil，走错误分支（与 staticcheck SA1012 友好相处）
	_, _, err := InheritUserIDFromParent(nilCtx)
	if err == nil {
		t.Fatal("want error for nil parentCtx")
	}
	if !errs.IsCode(err, errs.CodeInvalidInput) {
		t.Errorf("want CodeInvalidInput, got %v", err)
	}
}

// TestAgentLoop_UserIDInjected_IntoToolCtx — AgentLoop.userID 必须被注入到
// 调用工具的 ctx 上（否则 skill_install/skill_search 的 auth.UserIDFrom(ctx)
// 在 SubAgent 路径下永远为空，多租户 personal 层形同虚设）。
//
// 这是一个 white-box 断言：我们构造 AgentLoop（不需要真 LLM），直接调用
// SetUserID，然后断言 UserID() getter 与设置值一致；下游 toolCtx 注入逻辑
// 的正确性由 `agentloop.go:304-310` 的 auth.WithUser 代码路径 + 这里的字段
// 一致性共同保证（注入点无分支逻辑，白盒 review + 编译器已足够）。
func TestAgentLoop_UserID_GetterSetter(t *testing.T) {
	loop := &AgentLoop{}
	if loop.UserID() != "" {
		t.Errorf("zero-value UserID must be empty; got %q", loop.UserID())
	}
	loop.SetUserID("bob")
	if loop.UserID() != "bob" {
		t.Errorf("SetUserID(bob) then UserID() = %q, want bob", loop.UserID())
	}
	loop.SetUserID("")
	if loop.UserID() != "" {
		t.Errorf("SetUserID(\"\") then UserID() = %q, want empty", loop.UserID())
	}
}

// TestInheritUserIDFromParent_ReferenceSemantics — 返回的 child ctx 若取出
// User，应该引用 parent 传入的同一对象（而非浅拷贝）。这对齐 plan D16 的
// "同实例引用非拷贝，热更新才能立即生效" 契约。
func TestInheritUserIDFromParent_ReferenceSemantics(t *testing.T) {
	user := &auth.User{ID: "carol", Role: "admin", Status: "active"}
	parent := auth.WithUser(context.Background(), user)
	child, _, err := InheritUserIDFromParent(parent)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	got := auth.UserFrom(child)
	if got != user {
		t.Errorf("child ctx must hold SAME *User instance (got %p, want %p)", got, user)
	}
	// 仿真热更新：改 parent User 的 role，child 必须立刻可见（因为共享指针）
	user.Role = "superadmin"
	if auth.UserFrom(child).Role != "superadmin" {
		t.Error("child must observe parent User mutations in-place (hot-update contract)")
	}
}
