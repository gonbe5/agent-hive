package master

import (
	"sync"
	"time"
)

// CompactionTracker 跟踪上下文压缩的统计信息（线程安全）
type CompactionTracker struct {
	triggerCount   uint64    // 触发压缩次数
	skippedCount   uint64    // 懒惰模式跳过次数
	totalDelay     int64     // 累计延迟时间（纳秒）
	lastUserAction time.Time // 最后一次用户活动时间
	mu             sync.RWMutex
}

// NewCompactionTracker 创建新的压缩统计跟踪器
func NewCompactionTracker() *CompactionTracker {
	return &CompactionTracker{}
}

// RecordSkipped 记录一次懒惰模式跳过
func (ct *CompactionTracker) RecordSkipped() {
	ct.mu.Lock()
	ct.skippedCount++
	ct.mu.Unlock()
}

// RecordTrigger 记录一次压缩触发及其耗时
func (ct *CompactionTracker) RecordTrigger(elapsed time.Duration) {
	ct.mu.Lock()
	ct.triggerCount++
	ct.totalDelay += elapsed.Nanoseconds()
	ct.mu.Unlock()
}

// Stats 获取压缩统计快照（线程安全）
func (ct *CompactionTracker) Stats() CompactionStatsSnapshot {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	var avgDelay time.Duration
	if ct.triggerCount > 0 {
		avgDelay = time.Duration(ct.totalDelay / int64(ct.triggerCount))
	}

	return CompactionStatsSnapshot{
		TriggerCount: ct.triggerCount,
		SkippedCount: ct.skippedCount,
		AverageDelay: avgDelay,
	}
}

// Reset 重置统计信息（测试用）
func (ct *CompactionTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.triggerCount = 0
	ct.skippedCount = 0
	ct.totalDelay = 0
	ct.lastUserAction = time.Time{}
}

// UpdateUserActivity 更新最后一次用户活动时间
func (ct *CompactionTracker) UpdateUserActivity() {
	ct.mu.Lock()
	ct.lastUserAction = time.Now()
	ct.mu.Unlock()
}

// CalculateLazyDelay 根据 Token 超出比例动态计算延迟时间
// 超出10%: 等3分钟
// 超出30%: 紧急压缩，无延迟
// 超出50%: 紧急压缩，无延迟
// 可选：用户暂停2分钟后立即压缩
func (ct *CompactionTracker) CalculateLazyDelay(currentTokens, maxTokens int) time.Duration {
	if currentTokens <= maxTokens {
		return 0
	}

	ct.mu.RLock()
	lastAction := ct.lastUserAction
	ct.mu.RUnlock()

	// 用户暂停2分钟后立即压缩
	if !lastAction.IsZero() && time.Since(lastAction) >= 2*time.Minute {
		return 0
	}

	// 计算超出比例
	overRatio := float64(currentTokens-maxTokens) / float64(maxTokens)

	// 紧急压缩：超过阈值 30% 或以上，立即触发压缩
	if overRatio >= 0.3 {
		return 0
	} else if overRatio >= 0.1 {
		return 3 * time.Minute
	}

	return 3 * time.Minute
}
