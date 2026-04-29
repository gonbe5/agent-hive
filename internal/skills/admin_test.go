package skills

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// TestDenyAllAdminChecker — 默认实现对所有 userID 返回 false（含空串）。
func TestDenyAllAdminChecker(t *testing.T) {
	c := NewDenyAllAdminChecker()
	ctx := context.Background()
	for _, uid := range []string{"", "alice", "bob", "admin"} {
		if c.IsAdmin(ctx, uid) {
			t.Errorf("DenyAll should return false for %q", uid)
		}
	}
}

// TestAllowListAdminChecker_Basic — 白名单内返回 true，外返回 false。
func TestAllowListAdminChecker_Basic(t *testing.T) {
	c := NewAllowListAdminChecker([]string{"alice", "root"})
	ctx := context.Background()
	if !c.IsAdmin(ctx, "alice") {
		t.Error("alice should be admin")
	}
	if !c.IsAdmin(ctx, "root") {
		t.Error("root should be admin")
	}
	if c.IsAdmin(ctx, "bob") {
		t.Error("bob should NOT be admin")
	}
	if c.IsAdmin(ctx, "") {
		t.Error("empty userID must NEVER be admin")
	}
}

// TestAllowListAdminChecker_HotReload — SetAdmins 热更新立即可见。
func TestAllowListAdminChecker_HotReload(t *testing.T) {
	c := NewAllowListAdminChecker([]string{"alice"})
	ctx := context.Background()
	if !c.IsAdmin(ctx, "alice") {
		t.Fatal("pre-reload alice fail")
	}
	c.SetAdmins([]string{"bob"})
	if c.IsAdmin(ctx, "alice") {
		t.Error("post-reload alice should no longer be admin")
	}
	if !c.IsAdmin(ctx, "bob") {
		t.Error("post-reload bob should now be admin")
	}
}

// TestAllowListAdminChecker_Concurrent — 并发 SetAdmins + IsAdmin 必须无 race 且结果一致。
// 运行：go test -race -count=50 -run TestAllowListAdminChecker_Concurrent
func TestAllowListAdminChecker_Concurrent(t *testing.T) {
	c := NewAllowListAdminChecker([]string{"alice"})
	ctx := context.Background()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var reads int64

	// 10 个 reader
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					// 仅做一致性校验：alice 要么是 admin 要么不是，IsAdmin 不能 panic/race
					_ = c.IsAdmin(ctx, "alice")
					_ = c.IsAdmin(ctx, "bob")
					_ = c.IsAdmin(ctx, "carol")
					atomic.AddInt64(&reads, 3)
				}
			}
		}()
	}

	// 3 个 writer 交替轮替白名单
	rosters := [][]string{
		{"alice"},
		{"alice", "bob"},
		{"bob"},
		{"carol", "dan"},
		{},
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				select {
				case <-stop:
					return
				default:
					c.SetAdmins(rosters[(idx+j)%len(rosters)])
				}
			}
		}(i)
	}

	// 让 goroutines 跑一会儿
	for atomic.LoadInt64(&reads) < 30000 {
		// spin until readers have done enough work
	}
	close(stop)
	wg.Wait()
}

// TestAllowListAdminChecker_Interface — 接口约束编译期校验。
func TestAllowListAdminChecker_Interface(t *testing.T) {
	var _ AdminChecker = NewDenyAllAdminChecker()
	var _ AdminChecker = NewAllowListAdminChecker(nil)
}
