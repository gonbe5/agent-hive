package cache

import (
	"os"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type fileCacheEntry struct {
	content string
	modTime time.Time
	cachedAt time.Time
}

type FileCache struct {
	cache *lru.Cache[string, *fileCacheEntry]
	ttl   time.Duration
	mu    sync.RWMutex
}

func NewFileCache(size int, ttl time.Duration) (*FileCache, error) {
	cache, err := lru.New[string, *fileCacheEntry](size)
	if err != nil {
		return nil, err
	}
	return &FileCache{
		cache: cache,
		ttl:   ttl,
	}, nil
}

func (fc *FileCache) Get(path string) (string, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	entry, ok := fc.cache.Get(path)
	if !ok {
		return "", false
	}

	// Check TTL
	if time.Since(entry.cachedAt) > fc.ttl {
		fc.cache.Remove(path)
		return "", false
	}

	// Check file modification time
	stat, err := os.Stat(path)
	if err != nil || !stat.ModTime().Equal(entry.modTime) {
		fc.cache.Remove(path)
		return "", false
	}

	return entry.content, true
}

func (fc *FileCache) Set(path, content string, modTime time.Time) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.cache.Add(path, &fileCacheEntry{
		content:  content,
		modTime:  modTime,
		cachedAt: time.Now(),
	})
}
