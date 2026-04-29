package feishu

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
)

func newFeishuRetryableError(message string) error {
	return errors.New(message)
}

// retryReason 区分错误类别,Phase 4 缺口 9:限流和 5xx 用不同退避策略。
//
// 限流(99991400 系列 + ErrPatchRateLimited):飞书是按 60s 平均 QPS 节流,
// 短退避会立即触发再次限流形成"重试-限流-重试"循环,需要更长退避。
//
// 服务端错误(5xx):通常是飞书侧偶发抖动,短退避快速重试更合适。
type retryReason int

const (
	retryReasonNone retryReason = iota
	retryReasonRateLimited
	retryReasonServerError
)

// classifyFeishuError 返回错误类别。Phase 4 缺口 9 修复:
// 让 withRetry 按 reason 选退避公式,避免限流场景死循环。
func classifyFeishuError(err error) retryReason {
	if err == nil {
		return retryReasonNone
	}
	if errors.Is(err, ErrPatchRateLimited) {
		return retryReasonRateLimited
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "code=99991400"),
		strings.Contains(msg, "code=99991401"),
		strings.Contains(msg, "code=99991402"):
		return retryReasonRateLimited
	case strings.Contains(msg, "status=500"),
		strings.Contains(msg, "status=502"),
		strings.Contains(msg, "status=503"),
		strings.Contains(msg, "status=504"),
		strings.Contains(msg, "HTTP 500"),
		strings.Contains(msg, "HTTP 502"),
		strings.Contains(msg, "HTTP 503"),
		strings.Contains(msg, "HTTP 504"):
		return retryReasonServerError
	default:
		return retryReasonNone
	}
}

func isFeishuRetryableError(err error) bool {
	return classifyFeishuError(err) != retryReasonNone
}

// retryBackoff 按 reason + attempt 返回退避时间。
//
// 限流:500ms / 1s / 2s / 4s … 上限 8s。给飞书 60s 限流窗口足够冷却,避免循环。
// 5xx :100ms / 200ms / 400ms / 800ms … 上限 2s。快速重试,飞书侧抖动通常 < 1s。
//
// 缺口 9 修复:旧实现两类共用 100/200/400ms,限流场景不够冷却 → "重试-限流"循环。
func retryBackoff(reason retryReason, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	switch reason {
	case retryReasonRateLimited:
		// 500ms × 2^(attempt-1),cap 8s
		backoff := time.Duration(500*(1<<(attempt-1))) * time.Millisecond
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
		return backoff
	default:
		// 5xx 或未知:100ms × 2^(attempt-1),cap 2s
		backoff := time.Duration(100*(1<<(attempt-1))) * time.Millisecond
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
		return backoff
	}
}

func withRetry(ctx context.Context, maxAttempts int, logger *zap.Logger, op func() error) error {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = op()
		if lastErr == nil {
			return nil
		}
		reason := classifyFeishuError(lastErr)
		if reason == retryReasonNone {
			return lastErr
		}
		if attempt == maxAttempts {
			return lastErr
		}
		backoff := retryBackoff(reason, attempt)
		if logger != nil {
			logger.Warn("飞书 API 可重试错误，准备退避重试",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", maxAttempts),
				zap.String("retry_reason", retryReasonName(reason)),
				zap.Duration("backoff", backoff),
				zap.Error(lastErr))
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func retryReasonName(reason retryReason) string {
	switch reason {
	case retryReasonRateLimited:
		return "rate_limited"
	case retryReasonServerError:
		return "server_error"
	default:
		return "none"
	}
}
