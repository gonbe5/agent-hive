package i18n

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PromptStoreReader 是 PromptLoader 依赖的 DB 层接口（方便 mock 测试）
type PromptStoreReader interface {
	Get(ctx context.Context, key, language string) (string, bool, error)
}

// fileCacheEntry 文件缓存条目
type fileCacheEntry struct {
	content   string
	mtime     time.Time
	checkedAt time.Time
}

// dbCacheEntry DB 缓存条目（TTL 缓存，避免每次查 DB）
type dbCacheEntry struct {
	content   string
	found     bool
	expiresAt time.Time
}

// PromptLoader 三层优先级加载器：DB > 文件 > go:embed 硬编码
//
// Session 隔离语义：buildSystemPrompt() 每个 turn 调用一次，
// prompt 更新在下一个 turn 生效（最多 30 秒 DB 缓存延迟）。
// 正在运行的 turn 不受影响，这是有意设计。
type PromptLoader struct {
	store     PromptStoreReader
	baseDir   string
	language  string
	cacheTTL  time.Duration
	pollInterval time.Duration

	mu        sync.RWMutex
	fileCache map[string]fileCacheEntry
	dbCache   map[string]dbCacheEntry

	logger *zap.Logger
}

// NewPromptLoader 创建 PromptLoader 实例
// store 可为 nil（跳过 DB 层，直接走文件/embed）
// baseDir 可为空（跳过文件层，直接走 embed）
func NewPromptLoader(store PromptStoreReader, baseDir, language string, logger *zap.Logger) *PromptLoader {
	cacheTTL := 30 * time.Second
	if v := os.Getenv("PROMPT_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cacheTTL = d
		}
	}

	l := &PromptLoader{
		store:        store,
		baseDir:      baseDir,
		language:     language,
		cacheTTL:     cacheTTL,
		pollInterval: 30 * time.Second,
		fileCache:    make(map[string]fileCacheEntry),
		dbCache:      make(map[string]dbCacheEntry),
		logger:       logger,
	}

	// 启动日志：打印 prompts 目录路径和是否找到
	if baseDir != "" {
		if _, err := os.Stat(baseDir); err == nil {
			logger.Info("PromptLoader 初始化", zap.String("prompts_dir", baseDir), zap.Bool("dir_found", true))
		} else {
			logger.Info("PromptLoader 初始化（prompts 目录不存在，将使用 embed 兜底）",
				zap.String("prompts_dir", baseDir), zap.Bool("dir_found", false))
		}
	} else {
		logger.Info("PromptLoader 初始化（未配置 prompts 目录，将使用 embed 兜底）")
	}

	return l
}

// Start 启动文件轮询 goroutine，ctx cancel 时自动退出
func (l *PromptLoader) Start(ctx context.Context) {
	go l.startFileWatcher(ctx)
}

// Load 按优先级加载 prompt：DB > 文件 > embed
// 返回空字符串表示三层都未找到
func (l *PromptLoader) Load(relPath string) string {
	return l.LoadOrDefault(relPath, "")
}

// LoadOrDefault 加载 prompt，三层都没有时返回 defaultVal
func (l *PromptLoader) LoadOrDefault(relPath, defaultVal string) string {
	start := time.Now()

	// 1. DB 层（带 TTL 缓存）
	if l.store != nil {
		if content, found := l.loadFromDB(relPath); found {
			l.logger.Debug("prompt 加载",
				zap.String("key", relPath),
				zap.String("source", "db"),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			return content
		}
	}

	// 2. 文件层
	if l.baseDir != "" {
		if content, found := l.loadFromFile(relPath); found {
			l.logger.Debug("prompt 加载",
				zap.String("key", relPath),
				zap.String("source", "file"),
				zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			return content
		}
	}

	// 3. go:embed 层
	if content := loadEmbedded(relPath); content != "" {
		l.logger.Debug("prompt 加载",
			zap.String("key", relPath),
			zap.String("source", "embedded"),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		return content
	}

	// 4. 硬编码默认值
	if defaultVal != "" {
		l.logger.Debug("prompt 加载",
			zap.String("key", relPath),
			zap.String("source", "hardcoded"),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	}
	return defaultVal
}

// InvalidateDBCache 管理界面更新 prompt 后调用，立即失效当前实例缓存
func (l *PromptLoader) InvalidateDBCache(key string) {
	l.mu.Lock()
	delete(l.dbCache, key)
	l.mu.Unlock()
}

// loadFromDB 从 DB 缓存或实际 DB 加载 prompt
func (l *PromptLoader) loadFromDB(key string) (string, bool) {
	l.mu.RLock()
	entry, ok := l.dbCache[key]
	l.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.content, entry.found
	}

	// 缓存过期或不存在，查 DB
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	content, found, err := l.store.Get(ctx, key, l.language)
	if err != nil {
		l.logger.Warn("prompt DB 查询失败，fallback 到文件",
			zap.String("key", key),
			zap.Error(err),
		)
		// DB 错误时不缓存，直接 fallback
		return "", false
	}

	// 写入缓存
	l.mu.Lock()
	l.dbCache[key] = dbCacheEntry{
		content:   content,
		found:     found,
		expiresAt: time.Now().Add(l.cacheTTL),
	}
	l.mu.Unlock()

	return content, found
}

// loadFromFile 从文件缓存或实际文件加载 prompt
func (l *PromptLoader) loadFromFile(relPath string) (string, bool) {
	filePath := filepath.Join(l.baseDir, relPath+".md")

	l.mu.RLock()
	entry, ok := l.fileCache[relPath]
	l.mu.RUnlock()

	if ok {
		// 检查 mtime（每 30 秒检查一次）
		if time.Since(entry.checkedAt) < l.pollInterval {
			return entry.content, true
		}
		// 超过轮询间隔，检查 mtime
		info, err := os.Stat(filePath)
		if err != nil {
			// 文件消失了，清除缓存
			l.mu.Lock()
			delete(l.fileCache, relPath)
			l.mu.Unlock()
			return "", false
		}
		if info.ModTime().Equal(entry.mtime) {
			// mtime 未变，更新 checkedAt
			l.mu.Lock()
			e := l.fileCache[relPath]
			e.checkedAt = time.Now()
			l.fileCache[relPath] = e
			l.mu.Unlock()
			return entry.content, true
		}
		// mtime 变了，重新读取
	}

	// 读取文件（与 mtime 检查原子化：先读内容，再取 mtime，避免 TOCTOU）
	data, err := os.ReadFile(filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			l.logger.Warn("prompt 文件读取失败",
				zap.String("path", filePath),
				zap.Error(err),
			)
		}
		return "", false
	}

	info, err := os.Stat(filePath)
	if err != nil {
		// 文件刚被删除，返回刚读到的内容（本次调用有效）
		return string(data), true
	}

	content := string(data)
	l.mu.Lock()
	l.fileCache[relPath] = fileCacheEntry{
		content:   content,
		mtime:     info.ModTime(),
		checkedAt: time.Now(),
	}
	l.mu.Unlock()

	return content, true
}

// startFileWatcher 后台 goroutine，每 30 秒检查已缓存文件的 mtime
func (l *PromptLoader) startFileWatcher(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			l.logger.Error("prompt 文件监听 goroutine panic，热重载已停止", zap.Any("panic", r))
		}
	}()
	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.checkFileChanges()
		}
	}
}

// checkFileChanges 检查所有已缓存文件的 mtime，有变更则刷新
func (l *PromptLoader) checkFileChanges() {
	if l.baseDir == "" {
		return
	}

	l.mu.RLock()
	keys := make([]string, 0, len(l.fileCache))
	for k := range l.fileCache {
		keys = append(keys, k)
	}
	l.mu.RUnlock()

	for _, relPath := range keys {
		filePath := filepath.Join(l.baseDir, relPath+".md")
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		l.mu.RLock()
		entry, ok := l.fileCache[relPath]
		l.mu.RUnlock()
		if !ok {
			continue
		}

		if !info.ModTime().Equal(entry.mtime) {
			// 文件变更，重新读取
			data, err := os.ReadFile(filePath)
			if err != nil {
				l.logger.Warn("prompt 文件轮询读取失败",
					zap.String("path", filePath),
					zap.Error(err),
				)
				continue
			}
			l.logger.Info("prompt 文件变更，已刷新缓存",
				zap.String("path", filePath),
				zap.Time("old_mtime", entry.mtime),
				zap.Time("new_mtime", info.ModTime()),
			)
			l.mu.Lock()
			l.fileCache[relPath] = fileCacheEntry{
				content:   string(data),
				mtime:     info.ModTime(),
				checkedAt: time.Now(),
			}
			l.mu.Unlock()
		}
	}
}
