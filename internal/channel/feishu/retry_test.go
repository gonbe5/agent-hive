package feishu

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestRetryBackoff_DifferentiatesRateLimitedAndServerError 是 Phase 4 缺口 9 修复的蓝军点。
//
// 不变式:限流(rateReason)退避必须明显长于 5xx(serverErr)退避,避免限流场景"重试-限流"循环。
//
// 蓝军 mutation 点:把 retryBackoff 的 case retryReasonRateLimited 删掉(走 default),
// 限流退避退化成 100ms,本测试 ratelimited_first 子用例必红(want >= 500ms got 100ms)。
func TestRetryBackoff_DifferentiatesRateLimitedAndServerError(t *testing.T) {
	tests := []struct {
		name     string
		reason   retryReason
		attempt  int
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		{"ratelimited_first", retryReasonRateLimited, 1, 500 * time.Millisecond, 500 * time.Millisecond},
		{"ratelimited_second", retryReasonRateLimited, 2, 1 * time.Second, 1 * time.Second},
		{"ratelimited_capped", retryReasonRateLimited, 10, 8 * time.Second, 8 * time.Second},
		{"server_first", retryReasonServerError, 1, 100 * time.Millisecond, 100 * time.Millisecond},
		{"server_second", retryReasonServerError, 2, 200 * time.Millisecond, 200 * time.Millisecond},
		{"server_capped", retryReasonServerError, 10, 2 * time.Second, 2 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := retryBackoff(tc.reason, tc.attempt)
			if got < tc.wantMin || got > tc.wantMax {
				t.Fatalf("retryBackoff(%v, %d) = %v, want [%v, %v]",
					tc.reason, tc.attempt, got, tc.wantMin, tc.wantMax)
			}
		})
	}

	// 不变式硬约束:同 attempt 下,限流退避 必 >= 5xx 退避 × 4(给 60s 限流窗口足够冷却)。
	for _, attempt := range []int{1, 2, 3} {
		rate := retryBackoff(retryReasonRateLimited, attempt)
		serv := retryBackoff(retryReasonServerError, attempt)
		if rate < serv*4 {
			t.Fatalf("attempt=%d: 限流退避 %v 必须 >= 5xx 退避 %v × 4", attempt, rate, serv)
		}
	}
}

func TestClassifyFeishuError_DistinguishesReasons(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want retryReason
	}{
		{"nil", nil, retryReasonNone},
		{"99991400", newFeishuRetryableError("发送失败: code=99991400"), retryReasonRateLimited},
		{"99991401", newFeishuRetryableError("发送失败: code=99991401"), retryReasonRateLimited},
		{"PatchRateLimited sentinel", ErrPatchRateLimited, retryReasonRateLimited},
		{"500", newFeishuRetryableError("HTTP 500 error"), retryReasonServerError},
		{"503 status", newFeishuRetryableError("status=503"), retryReasonServerError},
		{"business error not retryable", newFeishuRetryableError("code=230001"), retryReasonNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyFeishuError(tc.err); got != tc.want {
				t.Fatalf("classifyFeishuError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWithRetry_RetriesRetryableErrorsThenSucceeds(t *testing.T) {
	var calls int
	err := withRetry(context.Background(), 3, zap.NewNop(), func() error {
		calls++
		if calls < 3 {
			return newFeishuRetryableError("发送失败: code=99991400, msg=rate limited")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry returned err = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestWithRetry_DoesNotRetryNonRetryableErrors(t *testing.T) {
	var calls int
	want := errors.New("发送失败: code=230001, msg=invalid param")
	err := withRetry(context.Background(), 3, zap.NewNop(), func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("withRetry err = %v, want %v", err, want)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestWithRetry_StopsAfterMaxRetries(t *testing.T) {
	var calls int
	err := withRetry(context.Background(), 3, zap.NewNop(), func() error {
		calls++
		return newFeishuRetryableError("发送失败: code=99991400, msg=rate limited")
	})
	if err == nil {
		t.Fatal("expected final error after retries exhausted")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestIsFeishuRetryableError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "rate limited", err: errors.New("code=99991400"), want: true},
		{name: "rate limited alt", err: errors.New("code=99991402"), want: true},
		{name: "server 500", err: errors.New("status=500"), want: true},
		{name: "server 503", err: errors.New("HTTP 503 unavailable"), want: true},
		{name: "business error", err: errors.New("code=230099"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFeishuRetryableError(tc.err); got != tc.want {
				t.Fatalf("isFeishuRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWithRetry_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls int
	err := withRetry(ctx, 3, zap.NewNop(), func() error {
		calls++
		return newFeishuRetryableError("code=99991400")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRateLimiterWait_ZeroConfigBypasses(t *testing.T) {
	rl := newFeishuRateLimiter(0, 0)
	if err := rl.Wait(context.Background(), "chat-1"); err != nil {
		t.Fatalf("Wait err = %v, want nil", err)
	}
}

func TestRateLimiterWait_RespectsContextDeadline(t *testing.T) {
	rl := newFeishuRateLimiter(1, 1)
	if err := rl.Wait(context.Background(), "chat-1"); err != nil {
		t.Fatalf("first Wait err = %v, want nil", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := rl.Wait(ctx, "chat-1"); err == nil {
		t.Fatal("expected second Wait to fail on deadline")
	}
}
