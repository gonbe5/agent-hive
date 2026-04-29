package master

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestClaimEvent_FirstClaimSucceeds 单 claim 路径：fresh eventID → ok=true。
func TestClaimEvent_FirstClaimSucceeds(t *testing.T) {
	c := NewMemoryEventClaimer(0, zap.NewNop())
	tok, ok := c.ClaimEvent("evt-1", DefaultClaimLease)
	if !ok {
		t.Fatalf("first claim must succeed")
	}
	if tok.EventID != "evt-1" {
		t.Fatalf("token EventID mismatch: %q", tok.EventID)
	}
	if tok.Nonce == 0 {
		t.Fatalf("token nonce must be non-zero (atomic counter starts at 1)")
	}
	if state := c.State("evt-1"); state != ClaimStateClaimed {
		t.Fatalf("state should be Claimed, got %d", state)
	}
}

// TestClaimEvent_SecondClaimBlocked 已 claim 未过期 → 第二次 ok=false。
func TestClaimEvent_SecondClaimBlocked(t *testing.T) {
	c := NewMemoryEventClaimer(0, zap.NewNop())
	if _, ok := c.ClaimEvent("evt-1", time.Hour); !ok {
		t.Fatalf("first claim failed")
	}
	if _, ok := c.ClaimEvent("evt-1", time.Hour); ok {
		t.Fatalf("second claim should be blocked while lease holds")
	}
}

// TestClaimThenComplete_DedupTakesEffect 完成后 dedup 生效：第二次 claim 永远 ok=false（直到 GC）。
func TestClaimThenComplete_DedupTakesEffect(t *testing.T) {
	c := NewMemoryEventClaimer(time.Hour, zap.NewNop())
	tok, ok := c.ClaimEvent("evt-1", time.Hour)
	if !ok {
		t.Fatalf("claim failed")
	}
	if err := c.CompleteEvent(tok); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if state := c.State("evt-1"); state != ClaimStateCompleted {
		t.Fatalf("state should be Completed, got %d", state)
	}
	if _, ok := c.ClaimEvent("evt-1", time.Hour); ok {
		t.Fatalf("post-complete claim should be blocked (dedup)")
	}
}

// TestCompleteEvent_TokenMismatchAfterReclaim 蓝军场景模拟：
// 老 worker 拿了 token T1 → 卡死 → lease 过期 → 新 worker 拿到 T2 → 老 worker 才返回想 Complete(T1)。
// 必须 ErrClaimTokenMismatch，否则会让"已被 reclaim 的事件"错误地标 Completed → 永久丢失。
func TestCompleteEvent_TokenMismatchAfterReclaim(t *testing.T) {
	now := time.Now()
	c := NewMemoryEventClaimer(time.Hour, zap.NewNop())
	c.SetNow(func() time.Time { return now })

	tok1, ok := c.ClaimEvent("evt-x", 10*time.Second)
	if !ok {
		t.Fatalf("first claim failed")
	}

	// 时间快进让 lease 过期
	now = now.Add(20 * time.Second)
	c.SetNow(func() time.Time { return now })

	// 触发自动 reclaim：再次 ClaimEvent 应该成功（拿到 T2）
	tok2, ok := c.ClaimEvent("evt-x", 10*time.Second)
	if !ok {
		t.Fatalf("re-claim after lease expiry should succeed")
	}
	if tok1.Nonce == tok2.Nonce {
		t.Fatalf("re-claimed token must have a different nonce; got both %d", tok1.Nonce)
	}

	// 老 worker 想用 T1 完成 → 必须 mismatch
	if err := c.CompleteEvent(tok1); err != ErrClaimTokenMismatch {
		t.Fatalf("expected ErrClaimTokenMismatch, got %v", err)
	}
	// 新 worker 用 T2 完成 → 成功
	if err := c.CompleteEvent(tok2); err != nil {
		t.Fatalf("complete with new token failed: %v", err)
	}
}

// TestReclaim_ReturnsExpiredOnly 校验 Reclaim 只摘除过期 claim，未到期的不动。
func TestReclaim_ReturnsExpiredOnly(t *testing.T) {
	now := time.Now()
	c := NewMemoryEventClaimer(time.Hour, zap.NewNop())
	c.SetNow(func() time.Time { return now })

	if _, ok := c.ClaimEvent("short", 1*time.Second); !ok {
		t.Fatalf("claim short failed")
	}
	if _, ok := c.ClaimEvent("long", time.Hour); !ok {
		t.Fatalf("claim long failed")
	}

	// 跳到 short 已过期、long 仍有效的时间
	now = now.Add(2 * time.Second)
	tokens := c.Reclaim(now)
	if len(tokens) != 1 || tokens[0].EventID != "short" {
		t.Fatalf("Reclaim should return only the expired event, got %+v", tokens)
	}
	if state := c.State("short"); state != ClaimStateUnknown {
		t.Fatalf("short should be removed after reclaim, state=%d", state)
	}
	if state := c.State("long"); state != ClaimStateClaimed {
		t.Fatalf("long should still be claimed, state=%d", state)
	}
}

// TestReclaimWorker_DropDeleteCompleteCallsReclaimer 模拟 P0-#8 红队 mutation：
// "把 CompleteEvent 调用直接删掉" → reclaim worker 在 lease 过期后必然把孤立事件再交回。
//
// onReclaim 回调的 eventID 必须等于第一次 claim 的 eventID。
func TestReclaimWorker_LostEventReturnedToCallback(t *testing.T) {
	now := time.Now()
	c := NewMemoryEventClaimer(time.Hour, zap.NewNop())
	c.SetNow(func() time.Time { return now })

	var wg sync.WaitGroup
	wg.Add(1)
	var got atomic.Value // ClaimToken
	w := NewReclaimWorker(c, 5*time.Second, func(tok ClaimToken) {
		got.Store(tok)
		wg.Done()
	}, zap.NewNop())
	w.SetNow(func() time.Time { return now })

	// 模拟"业务 worker 拿到 claim 然后崩溃，CompleteEvent 永远没被调"
	if _, ok := c.ClaimEvent("evt-zombie", 10*time.Second); !ok {
		t.Fatalf("claim failed")
	}
	// 时间快进让 lease 过期
	now = now.Add(20 * time.Second)
	w.SetNow(func() time.Time { return now })

	// 手动驱动一次 tick（不开后台 goroutine 也能断言）
	w.Tick()

	wg.Wait()
	tok, _ := got.Load().(ClaimToken)
	if tok.EventID != "evt-zombie" {
		t.Fatalf("reclaim worker should return zombie event, got %q", tok.EventID)
	}

	// reclaim 后状态应为 Unknown，下一个 worker 重新 claim 应成功
	if _, ok := c.ClaimEvent("evt-zombie", 10*time.Second); !ok {
		t.Fatalf("after reclaim, new claim must succeed (otherwise zombie永久卡死)")
	}
}

// TestClaimEvent_EmptyIDRejected — 防御：空 eventID 不应 claim。
func TestClaimEvent_EmptyIDRejected(t *testing.T) {
	c := NewMemoryEventClaimer(0, zap.NewNop())
	if _, ok := c.ClaimEvent("", time.Hour); ok {
		t.Fatalf("empty eventID must not be claimable")
	}
}

// TestCompleteEvent_NotFound — 完成不存在的 eventID 必须 ErrClaimNotFound（不是 silent success）。
func TestCompleteEvent_NotFound(t *testing.T) {
	c := NewMemoryEventClaimer(0, zap.NewNop())
	if err := c.CompleteEvent(ClaimToken{EventID: "nope", Nonce: 1}); err != ErrClaimNotFound {
		t.Fatalf("expected ErrClaimNotFound, got %v", err)
	}
}
