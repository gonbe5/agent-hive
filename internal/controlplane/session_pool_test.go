package controlplane

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestSessionPool_AcquireRelease(t *testing.T) {
	pool := NewSessionPool(3, zap.NewNop())

	// 获取 3 个会话
	assert.True(t, pool.Acquire("s1"))
	assert.True(t, pool.Acquire("s2"))
	assert.True(t, pool.Acquire("s3"))
	assert.Equal(t, 3, pool.Active())

	// 第 4 个应该失败
	assert.False(t, pool.Acquire("s4"))

	// 已存在的会话应该通过
	assert.True(t, pool.Acquire("s1"))

	// 释放后可以获取新的
	pool.Release("s1")
	assert.Equal(t, 2, pool.Active())
	assert.True(t, pool.Acquire("s4"))
}

func TestSessionPool_Concurrent(t *testing.T) {
	pool := NewSessionPool(100, zap.NewNop())
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := fmt.Sprintf("session-%d", id)
			pool.Acquire(sid)
			pool.Release(sid)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, 0, pool.Active())
}
