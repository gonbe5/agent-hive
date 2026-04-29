package bootstrap

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/skills"
)

// authAdminChecker 把 auth.User.Role == "admin" 桥接到 skills.AdminChecker 接口。
//
// 位于 bootstrap 包（而非 skills 包）是刻意的：让 internal/skills 保持对 auth
// 零依赖，方便测试和外部 import。bootstrap 作为组装层负责把两个模块粘合起来。
//
// 并发契约（tasks.md 5.1 + design.md D16）：
//   - 无可变状态 → 天然 goroutine-safe
//   - ctx 上的 auth.UserFrom 本身 goroutine-safe（内部只读）
//   - 热更新由 JWT 新 claims 天然体现——用户 role 变更后下一次请求即生效，
//     无需缓存失效逻辑
//
// 降级规则：
//   - ctx 无 user（匿名） → 返回 false（owner 意识：未认证请求绝不是 admin）
//   - user.Role != "admin" → false
//   - user.Role == "admin" AND Status == "active" → true
//     （disabled admin 不视为 admin，防止老 JWT 在用户停用后仍享特权）
type authAdminChecker struct{}

// NewAuthAdminChecker 返回基于 auth.UserFrom(ctx) 的 AdminChecker 实现。
// 适用于已启用 AuthEngine 的部署；未启用时请使用 skills.NewDenyAllAdminChecker()。
func NewAuthAdminChecker() skills.AdminChecker {
	return &authAdminChecker{}
}

// IsAdmin implements skills.AdminChecker.
//
// 注意：userID 参数 intentionally ignored —— ctx 里的 auth.User 是权威来源，
// userID 只用于接口对齐。这样设计是防止调用方传入伪造 userID 绕过权限检查
// （例如从 tool input 读的 userID，而非中间件认证过的 ctx user）。
func (a *authAdminChecker) IsAdmin(ctx context.Context, _ string) bool {
	u := auth.UserFrom(ctx)
	if u == nil {
		return false
	}
	if u.Status != "active" {
		return false
	}
	return u.Role == "admin"
}
