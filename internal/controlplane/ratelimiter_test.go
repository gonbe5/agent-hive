package controlplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_Allow(t *testing.T) {
	// 每秒 2 个，桶容量 3
	rl := NewRateLimiter(2, 3)

	// 初始桶满，前 3 个应该通过
	assert.True(t, rl.Allow("src1"))
	assert.True(t, rl.Allow("src1"))
	assert.True(t, rl.Allow("src1"))

	// 第 4 个应该失败（桶空了）
	assert.False(t, rl.Allow("src1"))

	// 不同 source 独立限速
	assert.True(t, rl.Allow("src2"))
}

func TestRateLimiter_Defaults(t *testing.T) {
	rl := NewRateLimiter(0, 0)
	assert.Equal(t, float64(10), rl.rate)
	assert.Equal(t, 20, rl.burst)
}
