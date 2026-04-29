package tools

import (
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileLock 提供基于文件路径的细粒度互斥锁。
// 在并发 SubAgent 场景下，多个 agent 可能同时写入同一文件，
// FileLock 通过 per-path 锁序列化写入，防止数据丢失。
type FileLock struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
}

// lockEntry 单个路径的锁条目，包含最后使用时间用于清理
type lockEntry struct {
	mu       sync.Mutex
	lastUsed time.Time
	refCount int // 当前引用计数（受 FileLock.mu 保护）
}

// 全局文件锁实例
var globalFileLock = NewFileLock()

// NewFileLock 创建新的 FileLock 实例
func NewFileLock() *FileLock {
	return &FileLock{
		locks: make(map[string]*lockEntry),
	}
}

// Lock 获取指定路径的锁，返回解锁函数。
// 路径会经过规范化处理（Clean + Abs），确保同一文件的不同路径表示使用同一把锁。
// 使用方式: unlock := fl.Lock(path); defer unlock()
func (fl *FileLock) Lock(path string) func() {
	normalized := normalizePath(path)

	fl.mu.Lock()
	entry, ok := fl.locks[normalized]
	if !ok {
		entry = &lockEntry{}
		fl.locks[normalized] = entry
	}
	entry.refCount++
	fl.mu.Unlock()

	entry.mu.Lock()
	entry.lastUsed = time.Now()

	return func() {
		entry.mu.Unlock()

		fl.mu.Lock()
		entry.refCount--
		fl.mu.Unlock()
	}
}

// LockFiles 批量获取多个文件路径的锁，按归一化路径排序避免死锁，返回统一解锁函数。
// 路径先经过 normalizePath 归一化（与 Lock 内部一致），再去重排序，
// 确保不同拼写指向同一文件时不会导致死锁或自死锁。
// 使用方式: unlock := fl.LockFiles(paths); defer unlock()
func (fl *FileLock) LockFiles(paths []string) func() {
	// 先归一化，再去重
	unique := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		unique[normalizePath(p)] = struct{}{}
	}
	sorted := make([]string, 0, len(unique))
	for p := range unique {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	unlocks := make([]func(), 0, len(sorted))
	for _, p := range sorted {
		unlocks = append(unlocks, fl.Lock(p))
	}

	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
}

// Cleanup 清理超过指定时间未使用的锁条目，防止内存泄漏。
// 建议定期调用（如每 10 分钟），staleAfter 推荐值为 30 分钟。
func (fl *FileLock) Cleanup(staleAfter time.Duration) int {
	fl.mu.Lock()
	defer fl.mu.Unlock()

	now := time.Now()
	removed := 0
	for path, entry := range fl.locks {
		// 有引用计数说明有 goroutine 正在使用或等待使用，跳过
		if entry.refCount > 0 {
			continue
		}
		// 尝试获取锁，如果获取不到说明正在使用，跳过
		if entry.mu.TryLock() {
			if now.Sub(entry.lastUsed) > staleAfter {
				delete(fl.locks, path)
				removed++
			}
			entry.mu.Unlock()
		}
	}
	return removed
}

// Len 返回当前锁条目数量（用于测试和监控）
func (fl *FileLock) Len() int {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return len(fl.locks)
}

// normalizePath 规范化文件路径，确保同一文件的不同路径表示映射到相同的 key
func normalizePath(path string) string {
	cleaned := filepath.Clean(path)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		// 如果无法获取绝对路径，使用 Clean 后的路径
		return cleaned
	}
	return abs
}
