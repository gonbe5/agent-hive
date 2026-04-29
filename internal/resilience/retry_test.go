package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- ExponentialBackoff.ShouldRetry ---

func TestExponentialBackoff_ShouldRetry(t *testing.T) {
	errFoo := errors.New("foo")

	t.Run("attempt exceeds MaxAttempts", func(t *testing.T) {
		p := &ExponentialBackoff{MaxAttempts: 2, BaseDelay: time.Millisecond}
		ok, _ := p.ShouldRetry(errFoo, 3)
		if ok {
			t.Error("expected false when attempt > MaxAttempts")
		}
	})

	t.Run("IsRetryable returns false", func(t *testing.T) {
		p := &ExponentialBackoff{
			MaxAttempts: 3,
			BaseDelay:   time.Millisecond,
			IsRetryable: func(err error) bool { return false },
		}
		ok, _ := p.ShouldRetry(errFoo, 1)
		if ok {
			t.Error("expected false when IsRetryable returns false")
		}
	})

	t.Run("normal path returns delay", func(t *testing.T) {
		p := &ExponentialBackoff{
			MaxAttempts:  3,
			BaseDelay:    100 * time.Millisecond,
			MaxJitterPct: 0,
		}
		ok, delay := p.ShouldRetry(errFoo, 1)
		if !ok {
			t.Error("expected true")
		}
		if delay < 50*time.Millisecond {
			t.Errorf("delay too small: %v", delay)
		}
	})

	t.Run("nil IsRetryable always retries", func(t *testing.T) {
		p := &ExponentialBackoff{MaxAttempts: 3, BaseDelay: time.Millisecond}
		ok, _ := p.ShouldRetry(errFoo, 1)
		if !ok {
			t.Error("expected true when IsRetryable is nil")
		}
	})
}

// --- Do ---

func TestDo_FirstCallSucceeds(t *testing.T) {
	calls := 0
	_, err := Do(context.Background(), &ExponentialBackoff{MaxAttempts: 3, BaseDelay: time.Millisecond}, nil, "test", func() (int, error) {
		calls++
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetrySucceeds(t *testing.T) {
	errTemp := errors.New("temp")
	calls := 0
	policy := &ExponentialBackoff{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		IsRetryable: func(err error) bool { return true },
	}
	_, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errTemp
		}
		return 99, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDo_PolicyRejectsRetry(t *testing.T) {
	errPerm := errors.New("permanent")
	calls := 0
	policy := &ExponentialBackoff{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		IsRetryable: func(err error) bool { return false },
	}
	_, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		calls++
		return 0, errPerm
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls)
	}
}

func TestDo_CtxCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	policy := &ExponentialBackoff{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		IsRetryable: func(err error) bool { return true },
	}
	_, err := Do(ctx, policy, nil, "test", func() (int, error) {
		return 0, errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDo_CircuitBreakerOpen(t *testing.T) {
	cb := &CircuitBreaker{Threshold: 1, ResetTimeout: time.Hour}
	cb.RecordFailure() // 触发熔断

	policy := &ExponentialBackoff{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0
	_, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		calls++
		return 0, errors.New("should not reach")
	}, cb)

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if calls != 0 {
		t.Errorf("expected 0 calls when circuit open, got %d", calls)
	}
}

func TestDo_NilCircuitBreaker(t *testing.T) {
	policy := &ExponentialBackoff{MaxAttempts: 3, BaseDelay: time.Millisecond}
	result, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		return 7, nil
	})
	if err != nil || result != 7 {
		t.Errorf("unexpected result: %v, %v", result, err)
	}
}

// --- CircuitBreaker 状态机 ---

func TestCircuitBreaker_ClosedAllows(t *testing.T) {
	cb := &CircuitBreaker{Threshold: 3, ResetTimeout: time.Hour}
	if !cb.Allow() {
		t.Error("closed circuit should allow")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := &CircuitBreaker{Threshold: 2, ResetTimeout: time.Hour}
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Error("should still be closed after 1 failure")
	}
	cb.RecordFailure()
	if cb.State() != "open" {
		t.Errorf("expected open, got %s", cb.State())
	}
	if cb.Allow() {
		t.Error("open circuit should not allow")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := &CircuitBreaker{Threshold: 1, ResetTimeout: time.Millisecond}
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond)
	if !cb.Allow() {
		t.Error("should allow after reset timeout (half-open)")
	}
	if cb.State() != "half-open" {
		t.Errorf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_RecordSuccessResets(t *testing.T) {
	cb := &CircuitBreaker{Threshold: 1, ResetTimeout: time.Hour}
	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatal("expected open")
	}
	cb.RecordSuccess()
	if cb.State() != "closed" {
		t.Errorf("expected closed after success, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Error("closed circuit should allow after reset")
	}
}

func TestCircuitBreaker_HalfOpenOnlyOneProbe(t *testing.T) {
	// HalfOpen 状态下，多个 goroutine 同时调用 Allow()，只有第一个通过
	cb := &CircuitBreaker{Threshold: 1, ResetTimeout: time.Millisecond}
	cb.RecordFailure()
	time.Sleep(5 * time.Millisecond) // 等待进入 HalfOpen

	results := make([]bool, 10)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(idx int) {
			results[idx] = cb.Allow()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// 恰好只有 1 个 true
	trueCount := 0
	for _, r := range results {
		if r {
			trueCount++
		}
	}
	if trueCount != 1 {
		t.Errorf("expected exactly 1 Allow=true in half-open, got %d", trueCount)
	}
}

func TestExponentialBackoff_MaxDelayCap(t *testing.T) {
	p := &ExponentialBackoff{
		MaxAttempts:  100,
		BaseDelay:    1 * time.Second,
		MaxDelay:     5 * time.Second,
		MaxJitterPct: 0,
	}
	_, delay := p.ShouldRetry(errors.New("x"), 10) // 2^9 * 1s = 512s， 应被 cap 到 5s
	if delay > 5*time.Second || delay < 4*time.Second {
		t.Errorf("expected delay near 5s, got %v", delay)
	}
}

func TestExponentialBackoff_LargeAttemptNoOverflow(t *testing.T) {
	p := &ExponentialBackoff{
		MaxAttempts:  100,
		BaseDelay:   1 * time.Second,
		MaxJitterPct: 0,
	}
	_, delay := p.ShouldRetry(errors.New("x"), 100) // attempt 远超 62，不应 panic 或负数
	if delay <= 0 {
		t.Errorf("expected positive delay for large attempt, got %v", delay)
	}
}

func TestDo_VariadicNilDoesNotSwallowRealBreaker(t *testing.T) {
	// Do(..., nil, realCB) — nil 不应吞掉 realCB
	cb := &CircuitBreaker{Threshold: 1, ResetTimeout: time.Hour}
	cb.RecordFailure() // 触发熔断

	policy := &ExponentialBackoff{
		MaxAttempts:  3,
		BaseDelay:   time.Millisecond,
		IsRetryable: func(err error) bool { return true },
	}
	calls := 0
	_, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		calls++
		return 0, errors.New("fail")
	}, nil, cb)

	// 熔断器应为 nil，不应触发 ErrCircuitOpen
	if errors.Is(err, ErrCircuitOpen) {
		t.Error("nil in variadic should not swallow real breaker; got ErrCircuitOpen")
	}
	// 实际应该走了重试逻辑，尝试 MaxAttempts 次
	if calls != 4 { // 1 次首次 + 3 次重试
		t.Errorf("expected 4 calls, got %d", calls)
	}
}

func TestDo_RetryBlockedWhenNotRetryable(t *testing.T) {
	// IsRetryable=false 时，首次失败即停止，不重试
	errPerm := errors.New("permanent")
	calls := 0
	policy := &ExponentialBackoff{
		MaxAttempts:  3,
		BaseDelay:    time.Millisecond,
		IsRetryable:  func(err error) bool { return false },
	}
	_, err := Do(context.Background(), policy, nil, "test", func() (int, error) {
		calls++
		return 0, errPerm
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 call when not retryable, got %d", calls)
	}
}
