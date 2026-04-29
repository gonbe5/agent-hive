package skills

import (
	"context"
	"sync/atomic"
)

// AdminChecker 判定某 userID 是否拥有管理员权限（用于 public scope skill 安装等敏感操作）。
//
// 并发契约（MAJOR 3 / design.md D16）：
// IsAdmin MUST be safe for concurrent invocation from multiple goroutines.
// 实现方必须保证对内部可变状态（如 DB-backed rule cache）使用 sync.RWMutex 或
// atomic.Pointer 防护，且要在 go test -race 下无数据竞争。D16 SubAgent 继承要求
// "同实例引用、非拷贝"以实现热更新——这要求 IsAdmin 必须 goroutine-safe，否则
// parent/child goroutine 并发调用即触发 race。
type AdminChecker interface {
	IsAdmin(ctx context.Context, userID string) bool
}

// DenyAllAdminChecker 保守默认实现：任何 userID 均返回 false。
// 无可变状态，天然 goroutine-safe。
type DenyAllAdminChecker struct{}

func (DenyAllAdminChecker) IsAdmin(_ context.Context, _ string) bool {
	return false
}

// NewDenyAllAdminChecker 便捷构造函数。
func NewDenyAllAdminChecker() AdminChecker {
	return DenyAllAdminChecker{}
}

// AllowListAdminChecker 基于静态 userID 白名单的 admin 判定器。
// 使用 atomic.Pointer 支持热更新白名单（SetAdmins 无锁替换整个 map）。
// 典型用于 auth 包尚未提供真实实现时的过渡方案。
type AllowListAdminChecker struct {
	admins atomic.Pointer[map[string]struct{}]
}

// NewAllowListAdminChecker 初始化白名单。
func NewAllowListAdminChecker(userIDs []string) *AllowListAdminChecker {
	c := &AllowListAdminChecker{}
	c.SetAdmins(userIDs)
	return c
}

// SetAdmins 热更新白名单（原子替换整个 set，读端无锁）。
func (c *AllowListAdminChecker) SetAdmins(userIDs []string) {
	set := make(map[string]struct{}, len(userIDs))
	for _, uid := range userIDs {
		if uid == "" {
			continue
		}
		set[uid] = struct{}{}
	}
	c.admins.Store(&set)
}

// IsAdmin 查白名单；goroutine-safe，读端只做一次 Load + map read。
func (c *AllowListAdminChecker) IsAdmin(_ context.Context, userID string) bool {
	if userID == "" {
		return false
	}
	p := c.admins.Load()
	if p == nil {
		return false
	}
	_, ok := (*p)[userID]
	return ok
}
