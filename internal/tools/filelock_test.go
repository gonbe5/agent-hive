package tools

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLock_BasicLockUnlock(t *testing.T) {
	fl := NewFileLock()

	unlock := fl.Lock("/tmp/test.txt")
	assert.Equal(t, 1, fl.Len(), "锁条目应为 1")

	unlock()
	// 解锁后条目仍然存在（懒清理）
	assert.Equal(t, 1, fl.Len(), "解锁后条目仍存在")
}

func TestFileLock_SamePathSerialized(t *testing.T) {
	fl := NewFileLock()
	path := "/tmp/concurrent_test.txt"

	var counter int64
	var wg sync.WaitGroup
	iterations := 100

	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			unlock := fl.Lock(path)
			defer unlock()

			// 模拟非原子操作：读取-修改-写入
			val := atomic.LoadInt64(&counter)
			// 短暂 yield，增加竞争概率
			time.Sleep(time.Microsecond)
			atomic.StoreInt64(&counter, val+1)
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(iterations), atomic.LoadInt64(&counter),
		"有锁保护的并发操作应正确序列化")
}

func TestFileLock_DifferentPathsConcurrent(t *testing.T) {
	fl := NewFileLock()

	var wg sync.WaitGroup
	pathCount := 10
	iterPerPath := 50

	counters := make([]int64, pathCount)

	for p := 0; p < pathCount; p++ {
		for i := 0; i < iterPerPath; i++ {
			wg.Add(1)
			go func(pathIdx int) {
				defer wg.Done()
				path := "/tmp/path_" + string(rune('a'+pathIdx)) + ".txt"
				unlock := fl.Lock(path)
				defer unlock()

				val := atomic.LoadInt64(&counters[pathIdx])
				time.Sleep(time.Microsecond)
				atomic.StoreInt64(&counters[pathIdx], val+1)
			}(p)
		}
	}

	wg.Wait()

	for p := 0; p < pathCount; p++ {
		assert.Equal(t, int64(iterPerPath), counters[p],
			"路径 %d 的计数器应为 %d", p, iterPerPath)
	}
}

func TestFileLock_PathNormalization(t *testing.T) {
	fl := NewFileLock()

	tests := []struct {
		name  string
		paths []string
	}{
		{
			name:  "Clean 规范化",
			paths: []string{"/tmp/./test.txt", "/tmp/test.txt"},
		},
		{
			name:  "双斜杠规范化",
			paths: []string{"/tmp//test.txt", "/tmp/test.txt"},
		},
		{
			name:  "父目录规范化",
			paths: []string{"/tmp/sub/../test.txt", "/tmp/test.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fl2 := NewFileLock()

			// 用第一个路径加锁
			unlock1 := fl2.Lock(tt.paths[0])
			unlock1()

			// 用第二个路径加锁
			unlock2 := fl2.Lock(tt.paths[1])
			unlock2()

			// 应该只有一个锁条目（路径规范化后相同）
			assert.Equal(t, 1, fl2.Len(),
				"规范化后的路径 %v 应使用同一把锁", tt.paths)
		})
	}

	_ = fl
}

func TestFileLock_Cleanup(t *testing.T) {
	fl := NewFileLock()

	// 创建一些锁条目
	for i := 0; i < 5; i++ {
		unlock := fl.Lock("/tmp/cleanup_" + string(rune('a'+i)) + ".txt")
		unlock()
	}

	require.Equal(t, 5, fl.Len())

	// 使用很短的过期时间
	time.Sleep(10 * time.Millisecond)
	removed := fl.Cleanup(5 * time.Millisecond)
	assert.Equal(t, 5, removed, "所有过期条目应被清理")
	assert.Equal(t, 0, fl.Len(), "清理后应无条目")
}

func TestFileLock_CleanupSkipsActive(t *testing.T) {
	fl := NewFileLock()

	// 创建一个正在使用的锁
	unlock := fl.Lock("/tmp/active.txt")

	// 创建一个已释放的锁
	unlock2 := fl.Lock("/tmp/idle.txt")
	unlock2()

	time.Sleep(10 * time.Millisecond)

	// 清理：活跃的锁不应被清理
	removed := fl.Cleanup(5 * time.Millisecond)
	assert.Equal(t, 1, removed, "只有空闲的锁应被清理")
	assert.Equal(t, 1, fl.Len(), "活跃的锁应保留")

	unlock()
}

func TestFileLock_GlobalInstance(t *testing.T) {
	// 验证全局实例已初始化
	require.NotNil(t, globalFileLock, "globalFileLock 应已初始化")

	// 验证全局实例可正常使用
	unlock := globalFileLock.Lock("/tmp/global_test.txt")
	unlock()
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:  "已经是绝对路径",
			input: "/tmp/test.txt",
		},
		{
			name:  "带 . 的路径",
			input: "/tmp/./test.txt",
		},
		{
			name:  "带 .. 的路径",
			input: "/tmp/sub/../test.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.input)
			// 结果应该是绝对路径
			assert.True(t, len(result) > 0, "结果不应为空")
			// 结果不应包含 /./  或 /../
			assert.NotContains(t, result, "/./")
			assert.NotContains(t, result, "/../")
		})
	}
}

func TestFileLock_LockFiles(t *testing.T) {
	t.Run("空路径列表", func(t *testing.T) {
		fl := NewFileLock()
		unlock := fl.LockFiles([]string{})
		// 应该无害地返回，不 panic
		unlock()
		assert.Equal(t, 0, fl.Len(), "空路径不应创建锁条目")
	})

	t.Run("重复路径去重", func(t *testing.T) {
		fl := NewFileLock()
		unlock := fl.LockFiles([]string{"/tmp/a.txt", "/tmp/a.txt", "/tmp/a.txt"})
		// 应该只创建一个锁条目
		assert.Equal(t, 1, fl.Len(), "重复路径应去重为 1 个锁")
		unlock()
	})

	t.Run("多路径排序加锁", func(t *testing.T) {
		fl := NewFileLock()
		// 逆序传入，验证不会死锁
		unlock := fl.LockFiles([]string{"/tmp/z.txt", "/tmp/a.txt", "/tmp/m.txt"})
		assert.Equal(t, 3, fl.Len())
		unlock()
	})

	t.Run("路径别名去重防自死锁", func(t *testing.T) {
		fl := NewFileLock()
		tmpDir := t.TempDir()
		absPath := filepath.Join(tmpDir, "file.txt")

		// 用两个指向同一文件的路径调用 LockFiles，不应自死锁
		done := make(chan struct{})
		go func() {
			// 使用 tmpDir 下的两种拼写
			unlock := fl.LockFiles([]string{
				absPath,
				filepath.Join(tmpDir, ".", "file.txt"), // 等价路径
			})
			unlock()
			close(done)
		}()

		select {
		case <-done:
			// 成功，无自死锁
		case <-time.After(2 * time.Second):
			t.Fatal("LockFiles 自死锁：两个路径别名指向同一文件")
		}

		// 验证只创建了一个锁条目
		assert.Equal(t, 1, fl.Len(), "路径别名应归一化为同一个锁")
	})

	t.Run("并发调用不死锁", func(t *testing.T) {
		fl := NewFileLock()
		paths := []string{"/tmp/file1.txt", "/tmp/file2.txt"}

		done := make(chan struct{}, 10)
		for i := 0; i < 10; i++ {
			go func() {
				unlock := fl.LockFiles(paths)
				// 短暂持有锁
				unlock()
				done <- struct{}{}
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
		// 所有 goroutine 完成，无死锁
	})
}
