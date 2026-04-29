package master

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateLazyDelay(t *testing.T) {
	tracker := NewCompactionTracker()

	// 超出10%: 等3分钟
	delay := tracker.CalculateLazyDelay(11000, 10000)
	assert.Equal(t, 3*time.Minute, delay, "超出10%应等待3分钟")

	// 超出30%: 紧急压缩，无延迟
	delay = tracker.CalculateLazyDelay(13500, 10000)
	assert.Equal(t, time.Duration(0), delay, "超出30%应立即压缩（紧急压缩）")

	// 超出50%: 紧急压缩，无延迟
	delay = tracker.CalculateLazyDelay(15500, 10000)
	assert.Equal(t, time.Duration(0), delay, "超出50%应立即压缩（紧急压缩）")

	// 未超出: 不延迟
	delay = tracker.CalculateLazyDelay(9000, 10000)
	assert.Equal(t, time.Duration(0), delay, "未超出不应延迟")
}

func TestCalculateLazyDelay_UserInactive(t *testing.T) {
	tracker := NewCompactionTracker()

	// 设置用户活动时间为3分钟前
	tracker.mu.Lock()
	tracker.lastUserAction = time.Now().Add(-3 * time.Minute)
	tracker.mu.Unlock()

	// 即使超出50%，用户暂停2分钟后应立即压缩
	delay := tracker.CalculateLazyDelay(16000, 10000)
	assert.Equal(t, time.Duration(0), delay, "用户暂停2分钟后应立即压缩")
}

func TestCalculateLazyDelay_UserActive(t *testing.T) {
	tracker := NewCompactionTracker()

	// 设置用户活动时间为30秒前
	tracker.UpdateUserActivity()

	// 超出50%但用户活跃，紧急压缩机制应立即触发
	delay := tracker.CalculateLazyDelay(16000, 10000)
	assert.Equal(t, time.Duration(0), delay, "用户活跃时超出50%应立即压缩（紧急压缩）")
}
