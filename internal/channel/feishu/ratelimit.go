package feishu

import (
	"context"
	"sync"
	"time"
)

// feishuFixedIntervalThrottle 是固定间隔节流器。
//
// Phase 4 缺口 8 修复:之前命名 feishuTokenBucket 误导,实际算法 = 1s/qps 间隔
// + 上次调用时间检查,**没有突发能力**。为避免与标准 token bucket 混淆,
// 改名为 fixed-interval throttle 并明确文档语义。
//
// 为什么不用真 token bucket(golang.org/x/time/rate.Limiter):
//   - 飞书 API 实际限流是 60s 平均 QPS,不是瞬时突发上限。
//   - 我们的 ratelimit 层是**本地保守节流**(45/s 上限),目标是远低于飞书侧
//     真限流阈值,留缓冲给:重连补偿瞬时压力 + 多副本并发。
//   - 允许突发反而可能在多副本同时发突发包时触发飞书 99991400 → 走 retry
//     路径成本更高(缺口 9 限流退避 500ms 起)。
//
// 简言之:**故意保守、故意均匀**,不是 token bucket。
type feishuFixedIntervalThrottle struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func newFeishuFixedIntervalThrottle(qps int) *feishuFixedIntervalThrottle {
	if qps <= 0 {
		return nil
	}
	return &feishuFixedIntervalThrottle{
		interval: time.Second / time.Duration(qps),
	}
}

func (b *feishuFixedIntervalThrottle) Wait(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.last.IsZero() {
		b.last = now
		return nil
	}

	next := b.last.Add(b.interval)
	if !next.After(now) {
		b.last = now
		return nil
	}

	wait := time.Until(next)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		b.last = time.Now()
		return nil
	}
}

// feishuRateLimiter 双层节流:全局 + per-chat。
// 行为细节见 feishuFixedIntervalThrottle 文档。
type feishuRateLimiter struct {
	global     *feishuFixedIntervalThrottle
	perChat    sync.Map
	perChatQPS int
}

func newFeishuRateLimiter(globalQPS, perChatQPS int) *feishuRateLimiter {
	return &feishuRateLimiter{
		global:     newFeishuFixedIntervalThrottle(globalQPS),
		perChatQPS: perChatQPS,
	}
}

func (r *feishuRateLimiter) Wait(ctx context.Context, chatID string) error {
	if r == nil {
		return nil
	}
	if err := r.global.Wait(ctx); err != nil {
		return err
	}
	if r.perChatQPS <= 0 || chatID == "" {
		return nil
	}
	value, _ := r.perChat.LoadOrStore(chatID, newFeishuFixedIntervalThrottle(r.perChatQPS))
	return value.(*feishuFixedIntervalThrottle).Wait(ctx)
}
