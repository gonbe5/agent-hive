package feishu

import (
	"context"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/observability"
	"golang.org/x/sync/singleflight"
)

const defaultUserCacheTTL = 12 * time.Hour

type cachedUserEntry struct {
	user      *UserDetail
	fetchedAt time.Time
	ttl       time.Duration
	lastUsed  time.Time
}

type userCache struct {
	ttl        time.Duration
	maxEntries int
	tenantKey  string

	mu            sync.RWMutex
	entries       map[string]cachedUserEntry
	sf            singleflight.Group
	fetchUserInfo func(context.Context, string, *Client) (*UserDetail, error)
	metricsWriter observability.MetricsWriter
}

func newUserCache(maxEntries int, ttl time.Duration) *userCache {
	if ttl <= 0 {
		ttl = defaultUserCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	return &userCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]cachedUserEntry),
		fetchUserInfo: func(ctx context.Context, openID string, client *Client) (*UserDetail, error) {
			return client.GetUserInfoByOpenID(ctx, openID)
		},
	}
}

func (c *userCache) WithTenantKey(tenantKey string) *userCache {
	if c == nil {
		return nil
	}
	c.tenantKey = tenantKey
	return c
}

func (c *userCache) GetOrFetch(ctx context.Context, openID string, client *Client) (*UserDetail, error) {
	if c == nil || client == nil || openID == "" {
		return nil, nil
	}
	if user := c.get(openID); user != nil {
		return user, nil
	}

	v, err, _ := c.sf.Do(openID, func() (any, error) {
		if user := c.get(openID); user != nil {
			return user, nil
		}
		user, fetchErr := c.fetchUserInfo(ctx, openID, client)
		if fetchErr != nil {
			c.store(openID, nil, 5*time.Minute)
			return nil, nil
		}
		c.store(openID, user, c.ttl)
		return user, nil
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	return v.(*UserDetail), nil
}

func (c *userCache) get(openID string) *UserDetail {
	c.mu.RLock()
	entry, ok := c.entries[openID]
	c.mu.RUnlock()
	if !ok {
		c.emitMetric(MetricUserCacheMiss, openID)
		return nil
	}
	if time.Since(entry.fetchedAt) > entry.ttl {
		c.mu.Lock()
		delete(c.entries, openID)
		c.mu.Unlock()
		c.emitMetric(MetricUserCacheMiss, openID)
		return nil
	}
	c.touch(openID, entry)
	c.emitMetric(MetricUserCacheHit, openID)
	return entry.user
}

func (c *userCache) store(openID string, user *UserDetail, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxEntries {
		var oldestKey string
		var oldestAt time.Time
		first := true
		for k, v := range c.entries {
			lastUsed := v.lastUsed
			if lastUsed.IsZero() {
				lastUsed = v.fetchedAt
			}
			if first || lastUsed.Before(oldestAt) {
				first = false
				oldestKey = k
				oldestAt = lastUsed
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	now := time.Now()
	c.entries[openID] = cachedUserEntry{
		user:      user,
		fetchedAt: now,
		ttl:       ttl,
		lastUsed:  now,
	}
}

func (c *userCache) touch(openID string, entry cachedUserEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.entries[openID]
	if !ok || current.fetchedAt != entry.fetchedAt {
		return
	}
	current.lastUsed = time.Now()
	c.entries[openID] = current
}

func (c *userCache) emitMetric(name, openID string) {
	if c == nil || c.metricsWriter == nil {
		return
	}
	_ = c.metricsWriter.Record(context.Background(), observability.Metric{
		Name:  name,
		Value: 1,
		Labels: map[string]any{
			"tenant_key_hash": channel.TenantKeyHashLabel(c.tenantKey),
			"cache_key":       SafeSenderID(openID),
		},
		Ts: time.Now(),
	})
}
