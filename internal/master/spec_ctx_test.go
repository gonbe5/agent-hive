package master

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// TestSpecCtx_AtomicLoadStore：裸 Load/Store 基础冒烟。
func TestSpecCtx_AtomicLoadStore(t *testing.T) {
	s := &SessionState{ID: "s1"}
	assert.Nil(t, s.LoadSpecCtx(), "未 Store 时 Load 必须返回 nil")

	ctx := &specdriven.Context{ChangeID: "c1", CurrentTaskKey: "1.1", Revision: 1}
	s.StoreSpecCtx(ctx)
	got := s.LoadSpecCtx()
	require.NotNil(t, got)
	assert.Equal(t, "c1", got.ChangeID)
}

// TestSpecCtx_RaceLoadVsStore：4.5 red-line race test。
// 1000 reader goroutines 并发 Load，同时一个 ingress goroutine 滚动 Store。
// 任何 race（如果 specCtx 不是 atomic.Pointer）都会被 -race 标记 WARN/FAIL。
// 这是 Codex Round 1 P0-6 的核心验证——proposal 原始的 mu-based 互斥方案
// 在此场景下会死锁或 race；atomic.Pointer 必须完全纯净。
func TestSpecCtx_RaceLoadVsStore(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	s := &SessionState{ID: "race"}
	// 预置一个初始 ctx
	s.StoreSpecCtx(&specdriven.Context{ChangeID: "init", Revision: 0})

	var wg sync.WaitGroup
	const readers = 200 // 保持 test 时间短；race 在 goroutine×loop 积上触发
	const iters = 50

	// writer：单 goroutine 单调递增 Revision
	wg.Add(1)
	go func() {
		defer wg.Done()
		for rev := 1; rev <= iters; rev++ {
			s.StoreSpecCtx(&specdriven.Context{ChangeID: "c1", Revision: rev})
		}
	}()

	// readers：并发 Load，校验任意时刻读到的 ctx 都是 non-nil
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iters {
				got := s.LoadSpecCtx()
				if got == nil {
					t.Errorf("Load got nil mid-flight")
					return
				}
				// 读 ChangeID 字段以触发 memory load——race detector 会检测
				_ = got.ChangeID
			}
		}()
	}
	wg.Wait()

	// 最终状态必须是最后一次 Store 的值
	final := s.LoadSpecCtx()
	require.NotNil(t, final)
	assert.Equal(t, iters, final.Revision)
}

// TestSpecCtxGuarded_AllowedPasses：allowed=true 路径走 Store，不计数。
func TestSpecCtxGuarded_AllowedPasses(t *testing.T) {
	ResetSpecCtxGuardCounter()
	s := &SessionState{ID: "s-allow"}
	ctx := &specdriven.Context{ChangeID: "c", Revision: 1}
	s.StoreSpecCtxGuarded(ctx, true, zap.NewNop())
	assert.Equal(t, uint64(0), SpecCtxUnauthorizedWrites())
	assert.Equal(t, "c", s.LoadSpecCtx().ChangeID)
}

// TestSpecCtxGuarded_BlockedCounter：allowed=false 路径在 prod 模式（panic 关闭）下
// 累计计数器，不改 specCtx。
func TestSpecCtxGuarded_BlockedCounter(t *testing.T) {
	ResetSpecCtxGuardCounter()
	SetSpecCtxGuardPanic(false)

	s := &SessionState{ID: "s-block"}
	orig := &specdriven.Context{ChangeID: "orig", Revision: 1}
	s.StoreSpecCtx(orig)

	attempt := &specdriven.Context{ChangeID: "evil", Revision: 999}
	s.StoreSpecCtxGuarded(attempt, false, zap.NewNop())

	assert.Equal(t, uint64(1), SpecCtxUnauthorizedWrites())
	// specCtx 保持 orig，未被未授权写入覆盖
	assert.Equal(t, "orig", s.LoadSpecCtx().ChangeID)
}

// TestSpecCtxGuarded_PanicMode：allowed=false 在 panic 模式下直接 panic。
// 测试 harness setup 时启用，把"隐藏 bug"变"编译/测试期炸开"。
func TestSpecCtxGuarded_PanicMode(t *testing.T) {
	ResetSpecCtxGuardCounter()
	SetSpecCtxGuardPanic(true)
	defer SetSpecCtxGuardPanic(false)

	s := &SessionState{ID: "s-panic"}
	assert.Panics(t, func() {
		s.StoreSpecCtxGuarded(&specdriven.Context{}, false, zap.NewNop())
	})
	assert.Equal(t, uint64(1), SpecCtxUnauthorizedWrites())
}

// TestSpecCtxGuarded_ConcurrentUnauthorized：多 goroutine 未授权写入
// counter 单调递增，无丢计数。
func TestSpecCtxGuarded_ConcurrentUnauthorized(t *testing.T) {
	ResetSpecCtxGuardCounter()
	SetSpecCtxGuardPanic(false)

	s := &SessionState{ID: "s-conc"}
	const workers = 50
	var wg sync.WaitGroup
	var started atomic.Int32
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			started.Add(1)
			s.StoreSpecCtxGuarded(&specdriven.Context{}, false, nil)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(workers), started.Load())
	assert.Equal(t, uint64(workers), SpecCtxUnauthorizedWrites(), "所有未授权写必须都被计入")
}
