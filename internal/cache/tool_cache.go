package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type toolCacheEntry struct {
	result   interface{}
	cachedAt time.Time
}

type ToolResultCache struct {
	cache *lru.Cache[string, *toolCacheEntry]
	ttl   time.Duration
	mu    sync.RWMutex
}

func NewToolResultCache(size int, ttl time.Duration) (*ToolResultCache, error) {
	cache, err := lru.New[string, *toolCacheEntry](size)
	if err != nil {
		return nil, err
	}
	return &ToolResultCache{
		cache: cache,
		ttl:   ttl,
	}, nil
}

// makeKey 生成缓存键
// 注意：Go 1.12+ 的 json.Marshal 已保证 map key 按字母序排列，
// 因此相同参数的 map 始终产生相同的 JSON 输出，缓存键是确定性的。
func (tc *ToolResultCache) makeKey(toolName string, params interface{}) string {
	data, _ := json.Marshal(map[string]interface{}{
		"tool":   toolName,
		"params": params,
	})
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func (tc *ToolResultCache) Get(toolName string, params interface{}) (interface{}, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	key := tc.makeKey(toolName, params)
	entry, ok := tc.cache.Get(key)
	if !ok {
		return nil, false
	}

	if time.Since(entry.cachedAt) > tc.ttl {
		tc.cache.Remove(key)
		return nil, false
	}

	return entry.result, true
}

func (tc *ToolResultCache) Set(toolName string, params interface{}, result interface{}) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	key := tc.makeKey(toolName, params)
	tc.cache.Add(key, &toolCacheEntry{
		result:   result,
		cachedAt: time.Now(),
	})
}
