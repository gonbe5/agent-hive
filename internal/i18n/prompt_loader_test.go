package i18n

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockStore 实现 PromptStoreReader 接口，用于测试
type mockStore struct {
	mu      sync.Mutex
	entries map[string]string // "key:lang" -> content
	err     error
	calls   int
}

func newMockStore() *mockStore {
	return &mockStore{entries: make(map[string]string)}
}

func (m *mockStore) set(key, lang, content string) {
	m.mu.Lock()
	m.entries[key+":"+lang] = content
	m.mu.Unlock()
}

func (m *mockStore) setErr(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
}

func (m *mockStore) Get(_ context.Context, key, language string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return "", false, m.err
	}
	v, ok := m.entries[key+":"+language]
	return v, ok, nil
}

func nopLogger() *zap.Logger { return zap.NewNop() }

// TestPromptLoader_EmbedFallback 三层都没有时走 go:embed
func TestPromptLoader_EmbedFallback(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	got := l.LoadOrDefault("system/base", "hardcoded")
	if got == "" || got == "hardcoded" {
		t.Errorf("expected embed content, got %q", got)
	}
}

// TestPromptLoader_HardcodedDefault 三层都没有时返回 defaultVal
func TestPromptLoader_HardcodedDefault(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	got := l.LoadOrDefault("nonexistent/key", "fallback-value")
	if got != "fallback-value" {
		t.Errorf("expected fallback-value, got %q", got)
	}
}

func TestPromptLoader_LoadWithMeta_Default(t *testing.T) {
	l := NewPromptLoader(nil, "", "zh-CN", nopLogger())
	got := l.LoadWithMeta("missing/key")

	if got.Content != "" {
		t.Fatalf("expected empty content, got %q", got.Content)
	}
	if got.Meta.Key != "missing/key" {
		t.Fatalf("key = %q, want missing/key", got.Meta.Key)
	}
	if got.Meta.Source != "default" {
		t.Fatalf("source = %q, want default", got.Meta.Source)
	}
	if got.Meta.Language != "zh-CN" {
		t.Fatalf("language = %q, want zh-CN", got.Meta.Language)
	}
	if got.Meta.Hash != "sha256:e3b0c44298fc1c14" {
		t.Fatalf("hash = %q, want empty-content hash", got.Meta.Hash)
	}
}

func TestPromptLoader_LoadWithMetaOrDefault_Hardcoded(t *testing.T) {
	l := NewPromptLoader(nil, "", "zh-CN", nopLogger())
	got := l.LoadWithMetaOrDefault("nonexistent/key", "fallback prompt")

	if got.Content != "fallback prompt" {
		t.Fatalf("content = %q, want fallback prompt", got.Content)
	}
	if got.Meta.Source != "hardcoded" {
		t.Fatalf("source = %q, want hardcoded", got.Meta.Source)
	}
	if got.Meta.Hash != "sha256:53b68f2b0e7d8170" {
		t.Fatalf("hash = %q, want fallback prompt hash", got.Meta.Hash)
	}
}

// TestPromptLoader_DBLayer_Hit DB 层命中时返回 DB 内容
func TestPromptLoader_DBLayer_Hit(t *testing.T) {
	store := newMockStore()
	store.set("subagents/title", "en-US", "db-title-prompt")

	l := NewPromptLoader(store, "", "en-US", nopLogger())
	got := l.LoadOrDefault("subagents/title", "default")
	if got != "db-title-prompt" {
		t.Errorf("expected db-title-prompt, got %q", got)
	}
}

// TestPromptLoader_DBLayer_Cache DB 层缓存：第二次调用不查 DB
func TestPromptLoader_DBLayer_Cache(t *testing.T) {
	store := newMockStore()
	store.set("subagents/title", "en-US", "db-content")

	l := NewPromptLoader(store, "", "en-US", nopLogger())
	l.LoadOrDefault("subagents/title", "")
	l.LoadOrDefault("subagents/title", "")
	l.LoadOrDefault("subagents/title", "")

	store.mu.Lock()
	calls := store.calls
	store.mu.Unlock()

	if calls != 1 {
		t.Errorf("expected 1 DB call (cached), got %d", calls)
	}
}

// TestPromptLoader_DBLayer_Miss DB 层未命中时 fallback 到 embed
func TestPromptLoader_DBLayer_Miss(t *testing.T) {
	store := newMockStore() // 空 store，不命中

	l := NewPromptLoader(store, "", "en-US", nopLogger())
	got := l.LoadOrDefault("system/base", "hardcoded")
	// 应该走 embed，不是 hardcoded
	if got == "hardcoded" {
		t.Error("expected embed fallback, got hardcoded default")
	}
	if got == "" {
		t.Error("expected embed content, got empty string")
	}
}

// TestPromptLoader_DBLayer_Error DB 查询失败时 fallback 到下一层
func TestPromptLoader_DBLayer_Error(t *testing.T) {
	store := newMockStore()
	store.setErr(errors.New("connection refused"))

	l := NewPromptLoader(store, "", "en-US", nopLogger())
	// DB 报错，应该 fallback 到 embed
	got := l.LoadOrDefault("system/base", "hardcoded")
	if got == "hardcoded" {
		t.Error("expected embed fallback on DB error, got hardcoded default")
	}
}

// TestPromptLoader_DBLayer_StoreNil store=nil 时跳过 DB 层
func TestPromptLoader_DBLayer_StoreNil(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	got := l.LoadOrDefault("system/base", "hardcoded")
	if got == "hardcoded" {
		t.Error("expected embed fallback when store=nil, got hardcoded default")
	}
}

// TestPromptLoader_FileLayer_Hit 文件层命中时返回文件内容
func TestPromptLoader_FileLayer_Hit(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "title.md"), []byte("file-title-prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewPromptLoader(nil, dir, "en-US", nopLogger())
	got := l.LoadOrDefault("subagents/title", "default")
	if got != "file-title-prompt" {
		t.Errorf("expected file-title-prompt, got %q", got)
	}
}

// TestPromptLoader_FileLayer_MtimeCache 文件未变时不重新读取
func TestPromptLoader_FileLayer_MtimeCache(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subDir, "title.md")
	if err := os.WriteFile(filePath, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewPromptLoader(nil, dir, "en-US", nopLogger())
	// 第一次读取
	got1 := l.LoadOrDefault("subagents/title", "")
	if got1 != "v1" {
		t.Fatalf("expected v1, got %q", got1)
	}

	// 修改文件内容但不改 mtime（模拟缓存命中场景）
	// 在 pollInterval 内再次读取，应该返回缓存值
	got2 := l.LoadOrDefault("subagents/title", "")
	if got2 != "v1" {
		t.Errorf("expected cached v1, got %q", got2)
	}
}

// TestPromptLoader_FileLayer_MtimeRefresh 文件 mtime 变化时刷新缓存
func TestPromptLoader_FileLayer_MtimeRefresh(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subDir, "title.md")
	if err := os.WriteFile(filePath, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	// 设置极短的 pollInterval 以便测试
	l := NewPromptLoader(nil, dir, "en-US", nopLogger())
	l.pollInterval = 1 * time.Millisecond

	got1 := l.LoadOrDefault("subagents/title", "")
	if got1 != "v1" {
		t.Fatalf("expected v1, got %q", got1)
	}

	// 等待超过 pollInterval，然后修改文件
	time.Sleep(5 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	// 更新 mtime（确保不同）
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(filePath, future, future); err != nil {
		t.Fatal(err)
	}

	got2 := l.LoadOrDefault("subagents/title", "")
	if got2 != "v2" {
		t.Errorf("expected v2 after file change, got %q", got2)
	}
}

// TestPromptLoader_FileLayer_NotExist 文件不存在时 fallback 到 embed
func TestPromptLoader_FileLayer_NotExist(t *testing.T) {
	dir := t.TempDir() // 空目录，没有任何 .md 文件

	l := NewPromptLoader(nil, dir, "en-US", nopLogger())
	got := l.LoadOrDefault("system/base", "hardcoded")
	// 文件不存在，应该走 embed
	if got == "hardcoded" {
		t.Error("expected embed fallback when file not found, got hardcoded default")
	}
}

// TestPromptLoader_FileLayer_BaseDirEmpty baseDir="" 时跳过文件层
func TestPromptLoader_FileLayer_BaseDirEmpty(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	got := l.LoadOrDefault("system/base", "hardcoded")
	if got == "hardcoded" {
		t.Error("expected embed fallback when baseDir empty, got hardcoded default")
	}
}

// TestPromptLoader_DBOverridesFile DB 层优先于文件层
func TestPromptLoader_DBOverridesFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "title.md"), []byte("file-content"), 0644); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	store.set("subagents/title", "en-US", "db-content")

	l := NewPromptLoader(store, dir, "en-US", nopLogger())
	got := l.LoadOrDefault("subagents/title", "default")
	if got != "db-content" {
		t.Errorf("expected DB to override file, got %q", got)
	}
}

// TestPromptLoader_InvalidateDBCache 失效缓存后下次查 DB
func TestPromptLoader_InvalidateDBCache(t *testing.T) {
	store := newMockStore()
	store.set("subagents/title", "en-US", "v1")

	l := NewPromptLoader(store, "", "en-US", nopLogger())

	// 第一次加载，缓存 v1
	got1 := l.LoadOrDefault("subagents/title", "")
	if got1 != "v1" {
		t.Fatalf("expected v1, got %q", got1)
	}

	// 更新 store 内容
	store.set("subagents/title", "en-US", "v2")

	// 不失效缓存，仍然返回 v1
	got2 := l.LoadOrDefault("subagents/title", "")
	if got2 != "v1" {
		t.Errorf("expected cached v1 before invalidation, got %q", got2)
	}

	// 失效缓存
	l.InvalidateDBCache("subagents/title")

	// 现在应该返回 v2
	got3 := l.LoadOrDefault("subagents/title", "")
	if got3 != "v2" {
		t.Errorf("expected v2 after cache invalidation, got %q", got3)
	}
}

// TestPromptLoader_InvalidateDBCache_NonExistentKey 失效不存在的 key 不 panic
func TestPromptLoader_InvalidateDBCache_NonExistentKey(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	// 不应该 panic
	l.InvalidateDBCache("nonexistent/key")
}

// TestPromptLoader_Concurrent 并发调用 LoadOrDefault 不 race
func TestPromptLoader_Concurrent(t *testing.T) {
	store := newMockStore()
	store.set("subagents/title", "en-US", "concurrent-content")

	l := NewPromptLoader(store, "", "en-US", nopLogger())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := l.LoadOrDefault("subagents/title", "default")
			if got != "concurrent-content" {
				t.Errorf("concurrent: expected concurrent-content, got %q", got)
			}
		}()
	}
	wg.Wait()
}

// TestPromptLoader_Concurrent_InvalidateAndLoad 并发失效 + 加载不 race
func TestPromptLoader_Concurrent_InvalidateAndLoad(t *testing.T) {
	store := newMockStore()
	store.set("subagents/title", "en-US", "content")

	l := NewPromptLoader(store, "", "en-US", nopLogger())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			l.LoadOrDefault("subagents/title", "default")
		}()
		go func() {
			defer wg.Done()
			l.InvalidateDBCache("subagents/title")
		}()
	}
	wg.Wait()
}

// TestPromptLoader_FileWatcher_ContextCancel ctx cancel 时 goroutine 正常退出
func TestPromptLoader_FileWatcher_ContextCancel(t *testing.T) {
	l := NewPromptLoader(nil, "", "en-US", nopLogger())
	l.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	l.Start(ctx)

	// 给 goroutine 启动时间
	time.Sleep(20 * time.Millisecond)
	cancel()

	// 等待 goroutine 退出（最多 100ms）
	time.Sleep(50 * time.Millisecond)
	// 如果 goroutine 没退出，-race 检测器会在测试结束时报告
}

// TestPromptLoader_CheckFileChanges_Refresh checkFileChanges 检测到变更时刷新
func TestPromptLoader_CheckFileChanges_Refresh(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subagents")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(subDir, "title.md")
	if err := os.WriteFile(filePath, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewPromptLoader(nil, dir, "en-US", nopLogger())
	// 先加载一次，填充 fileCache
	got1 := l.LoadOrDefault("subagents/title", "")
	if got1 != "initial" {
		t.Fatalf("expected initial, got %q", got1)
	}

	// 修改文件
	if err := os.WriteFile(filePath, []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(filePath, future, future); err != nil {
		t.Fatal(err)
	}

	// 手动触发 checkFileChanges
	l.checkFileChanges()

	// 直接读缓存验证已刷新
	l.mu.RLock()
	entry := l.fileCache["subagents/title"]
	l.mu.RUnlock()

	if entry.content != "updated" {
		t.Errorf("expected cache to be updated to 'updated', got %q", entry.content)
	}
}
