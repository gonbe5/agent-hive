package resilience

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RetryPolicy 决定是否重试以及等待多久。
type RetryPolicy interface {
	// ShouldRetry 返回是否重试和等待时长。
	// attempt 从 1 开始（第一次重试 = 1）。
	ShouldRetry(err error, attempt int) (bool, time.Duration)
}

// ExponentialBackoff 指数退避重试策略，带 jitter。
type ExponentialBackoff struct {
	MaxAttempts  int           // 最大重试次数（不含首次调用）
	BaseDelay    time.Duration // 初始等待时长
	MaxDelay     time.Duration // delay 上界（0 表示不限制）
	MaxJitterPct float64       // jitter 比例（0.3 = ±30%）
	IsRetryable  func(err error) bool
}

// ShouldRetry 实现 RetryPolicy。
func (e *ExponentialBackoff) ShouldRetry(err error, attempt int) (bool, time.Duration) {
	if attempt > e.MaxAttempts {
		return false, 0
	}
	if e.IsRetryable != nil && !e.IsRetryable(err) {
		return false, 0
	}
	// 计算指数延迟，防止 int64 溢出。大 attempt 时直接 cap 到 MaxDelay
	maxDelay := e.MaxDelay
	if maxDelay == 0 {
		maxDelay = time.Hour // 无 MaxDelay 时上限 1 小时
	}
	shift := attempt - 1
	delay := e.BaseDelay
	for shift > 0 && delay < maxDelay {
		next := delay * 2
		if next < delay { // 溢出
			delay = maxDelay
			break
		}
		delay = next
		shift--
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := time.Duration(float64(delay) * e.MaxJitterPct * (2*rand.Float64() - 1))
	return true, delay + jitter
}

// Do 使用给定策略执行 fn，失败时按策略重试。
// ctx 取消时立即返回。
// cb 可选：传入非 nil 的 CircuitBreaker 时，每次调用前检查熔断状态，并根据结果更新计数。
// 注意：nil breaker 不会触发任何熔断逻辑；避免传入 nil 元素以免产生混淆。
func Do[T any](ctx context.Context, policy RetryPolicy, logger *zap.Logger, callName string, fn func() (T, error), cb ...*CircuitBreaker) (T, error) {
	var breaker *CircuitBreaker
	if len(cb) > 0 && cb[0] != nil {
		breaker = cb[0]
	}

	call := func() (T, error) {
		if breaker != nil && !breaker.Allow() {
			var zero T
			return zero, ErrCircuitOpen
		}
		result, err := fn()
		if breaker != nil {
			if err != nil {
				breaker.RecordFailure()
			} else {
				breaker.RecordSuccess()
			}
		}
		return result, err
	}

	result, err := call()
	if err == nil {
		return result, nil
	}
	// 熔断器打开时不重试
	if errors.Is(err, ErrCircuitOpen) {
		return result, err
	}

	for attempt := 1; ; attempt++ {
		ok, delay := policy.ShouldRetry(err, attempt)
		if !ok {
			return result, err
		}

		if logger != nil {
			logger.Warn("调用失败，准备重试",
				zap.String("call", callName),
				zap.Int("attempt", attempt),
				zap.Duration("backoff", delay),
				zap.Error(err),
			)
		}

		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(delay):
		}

		result, err = call()
		if err == nil {
			if logger != nil {
				logger.Info("重试成功",
					zap.String("call", callName),
					zap.Int("attempt", attempt),
				)
			}
			return result, nil
		}
		if errors.Is(err, ErrCircuitOpen) {
			return result, err
		}
	}
}

// CircuitBreaker 熔断器：连续失败超过阈值后进入 Open 状态，拒绝请求。
// Open 状态持续 ResetTimeout 后进入 HalfOpen，只允许一次探测请求。
type CircuitBreaker struct {
	mu           sync.Mutex
	state        cbState
	failures     int
	lastFailure  time.Time
	probing      bool // 是否已有 goroutine 在进行半开探测
	Threshold    int           // 连续失败多少次后熔断
	ResetTimeout time.Duration // Open → HalfOpen 的等待时长
}

type cbState int

const (
	cbClosed   cbState = iota // 正常
	cbOpen                    // 熔断中
	cbHalfOpen                // 探测中
)

// ErrCircuitOpen 熔断器打开时返回此错误。
var ErrCircuitOpen = &circuitOpenError{}

type circuitOpenError struct{}

func (e *circuitOpenError) Error() string { return "circuit breaker open" }

// Allow 返回是否允许本次请求通过。
// HalfOpen 状态下仅允许一次探测通过，后续请求被拒绝直到探测完成。
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case cbClosed:
		return true
	case cbOpen:
		if time.Since(cb.lastFailure) >= cb.ResetTimeout {
			// 在 transition 前设置，防止第二个 goroutine 在 HalfOpen 分支里以 probing=false 通过
			cb.probing = true
			cb.state = cbHalfOpen
			return true
		}
		return false
	case cbHalfOpen:
		if cb.probing {
			return false // 已有探测请求，拒绝
		}
		cb.probing = true
		return true
	}
	return true
}

// RecordSuccess 记录成功，重置熔断器。
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.probing = false
	cb.state = cbClosed
}

// RecordFailure 记录失败，达到阈值时打开熔断器。
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	// HalfOpen → Open：probe 失败，probing 保持 true，下一个 Allow() 直接拒绝，
	// 等待 Open → HalfOpen 超时后才允许新 probe
	if cb.state == cbHalfOpen {
		cb.state = cbOpen
		return
	}
	cb.probing = false
	if cb.failures >= cb.Threshold {
		cb.state = cbOpen
	}
}

// State 返回当前熔断器状态字符串（用于日志/监控）。
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case cbOpen:
		return "open"
	case cbHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}
