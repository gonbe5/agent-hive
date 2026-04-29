package controlplane

import (
	"context"
	"sync"
	"time"
)

// RateLimiter 令牌桶速率限制器，按 source 独立限速
type RateLimiter struct {
	rate          float64 // 每秒生成令牌数
	burst         int     // 桶容量
	buckets       map[string]*bucket
	mu            sync.Mutex
	cleanupCancel context.CancelFunc // 用于停止清理 goroutine
}

type bucket struct {
	tokens     float64
	lastFill   time.Time
	lastAccess time.Time // 最后访问时间
}

// NewRateLimiter 创建令牌桶速率限制器
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	if rate <= 0 {
		rate = 10 // 默认每秒 10 个
	}
	if burst <= 0 {
		burst = 20
	}

	rl := &RateLimiter{
		rate:    rate,
		burst:   burst,
		buckets: make(map[string]*bucket),
	}

	// 启动后台清理器
	ctx, cancel := context.WithCancel(context.Background())
	rl.cleanupCancel = cancel
	go rl.cleanupLoop(ctx)

	return rl
}

// Allow 检查是否允许操作（消耗一个令牌）
func (rl *RateLimiter) Allow(source string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[source]
	if !ok {
		b = &bucket{
			tokens:     float64(rl.burst),
			lastFill:   now,
			lastAccess: now, // 初始化 lastAccess
		}
		rl.buckets[source] = b
	} else {
		b.lastAccess = now // 更新最后访问时间
	}

	// 补充令牌
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastFill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanupLoop 定期清理不活跃的 bucket
func (rl *RateLimiter) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

// cleanup 清理超过 30 分钟未访问的 bucket
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	inactiveThreshold := 30 * time.Minute

	for source, b := range rl.buckets {
		if now.Sub(b.lastAccess) > inactiveThreshold {
			delete(rl.buckets, source)
		}
	}
}

// Stop 停止清理器（优雅关闭）
func (rl *RateLimiter) Stop() {
	if rl.cleanupCancel != nil {
		rl.cleanupCancel()
	}
}
