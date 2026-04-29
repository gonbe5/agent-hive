package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/observability"
)

func TestClientUserCache_EvictsLeastRecentlyUsed(t *testing.T) {
	cache := newUserCache(2, time.Hour)

	cache.store("ou_a", &UserDetail{OpenID: "ou_a", Name: "A"}, time.Hour)
	cache.store("ou_b", &UserDetail{OpenID: "ou_b", Name: "B"}, time.Hour)
	if got := cache.get("ou_a"); got == nil || got.Name != "A" {
		t.Fatalf("expected to touch ou_a before eviction, got %+v", got)
	}

	cache.store("ou_c", &UserDetail{OpenID: "ou_c", Name: "C"}, time.Hour)

	if got := cache.get("ou_b"); got != nil {
		t.Fatalf("ou_b should be evicted as LRU, got %+v", got)
	}
	if got := cache.get("ou_a"); got == nil || got.Name != "A" {
		t.Fatalf("ou_a should stay after LRU touch, got %+v", got)
	}
	if got := cache.get("ou_c"); got == nil || got.Name != "C" {
		t.Fatalf("ou_c should exist, got %+v", got)
	}
}

func TestClientBotOpenID_CachesAcrossConcurrentCalls(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "token",
				"expire":              7200,
			})
		case "/open-apis/bot/v3/info":
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"bot": map[string]any{
					"open_id": "ou_bot_cached",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("app", "secret", zap.NewNop(), lark.WithOpenBaseUrl(server.URL))

	var wg sync.WaitGroup
	results := make(chan string, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- client.BotOpenID()
		}()
	}
	wg.Wait()
	close(results)

	for got := range results {
		if got != "ou_bot_cached" {
			t.Fatalf("BotOpenID() = %q, want ou_bot_cached", got)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("bot info endpoint calls = %d, want 1", got)
	}
}

func TestClientUserCache_EmptyResultUsesShortTombstone(t *testing.T) {
	cache := newUserCache(128, time.Hour)
	cache.store("ou_missing", nil, 50*time.Millisecond)

	if user := cache.get("ou_missing"); user != nil {
		t.Fatalf("tombstone get() = %+v, want nil", user)
	}
	time.Sleep(80 * time.Millisecond)
	_ = cache.get("ou_missing")
	if _, ok := cache.entries["ou_missing"]; ok {
		t.Fatal("expired tombstone should be evicted")
	}
}

func TestClientUserCache_FetchErrorStoresTombstone(t *testing.T) {
	client := &Client{}
	cache := newUserCache(128, time.Hour)
	cache.fetchUserInfo = func(context.Context, string, *Client) (*UserDetail, error) {
		return nil, errors.New("permission denied")
	}

	user, err := cache.GetOrFetch(context.Background(), "ou_err", client)
	if err != nil {
		t.Fatalf("GetOrFetch returned err = %v, want nil tombstone degrade", err)
	}
	if user != nil {
		t.Fatalf("GetOrFetch returned user = %+v, want nil", user)
	}
	if _, ok := cache.entries["ou_err"]; !ok {
		t.Fatal("expected tombstone entry after fetch error")
	}
}

func TestClientUserCache_EmitsHitAndMissMetrics(t *testing.T) {
	cache := newUserCache(128, time.Hour)
	cache.WithTenantKey("tenant-a")
	writer := &identityMetricCaptureWriter{}
	cache.metricsWriter = writer

	cache.store("ou_hit", &UserDetail{OpenID: "ou_hit", Name: "张三"}, time.Hour)
	_ = cache.get("ou_hit")
	_ = cache.get("ou_miss")

	if hit := writer.find("feishu.user_cache.hit"); hit == nil {
		t.Fatal("expected feishu.user_cache.hit metric")
	} else {
		if got := hit.Labels["tenant_key_hash"]; got != "tk_80a707af" {
			t.Fatalf("hit tenant_key_hash = %v, want tk_80a707af", got)
		}
		if _, ok := hit.Labels["open_id"]; ok {
			t.Fatal("open_id label must not be emitted")
		}
	}
	if miss := writer.find("feishu.user_cache.miss"); miss == nil {
		t.Fatal("expected feishu.user_cache.miss metric")
	} else if got := miss.Labels["tenant_key_hash"]; got != "tk_80a707af" {
		t.Fatalf("miss tenant_key_hash = %v, want tk_80a707af", got)
	}
}

func TestClientUserCache_GetOrFetchCachesByOpenID(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "token",
				"expire":              7200,
			})
		case "/open-apis/contact/v3/users/ou_user_1":
			calls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"user": map[string]any{
						"user_id": "user_1",
						"open_id": "ou_user_1",
						"name":    "张三",
						"en_name": "Zhang San",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("app", "secret", zap.NewNop(), lark.WithOpenBaseUrl(server.URL))
	cache := newUserCache(128, 0)

	first, err := cache.GetOrFetch(context.Background(), "ou_user_1", client)
	if err != nil {
		t.Fatalf("first GetOrFetch failed: %v", err)
	}
	second, err := cache.GetOrFetch(context.Background(), "ou_user_1", client)
	if err != nil {
		t.Fatalf("second GetOrFetch failed: %v", err)
	}

	if first.Name != "张三" || second.Name != "张三" {
		t.Fatalf("cached user name mismatch: first=%q second=%q", first.Name, second.Name)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("contact user endpoint calls = %d, want 1", got)
	}
}

func TestClientUserCache_TTLExpiryRefetches(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "token",
				"expire":              7200,
			})
		case "/open-apis/contact/v3/users/ou_user_1":
			n := calls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"user": map[string]any{
						"user_id": "user_1",
						"open_id": "ou_user_1",
						"name":    "张三",
						"en_name": "v" + string(rune('0'+n)),
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("app", "secret", zap.NewNop(), lark.WithOpenBaseUrl(server.URL))
	cache := newUserCache(128, 10*time.Millisecond)

	if _, err := cache.GetOrFetch(context.Background(), "ou_user_1", client); err != nil {
		t.Fatalf("first GetOrFetch failed: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := cache.GetOrFetch(context.Background(), "ou_user_1", client); err != nil {
		t.Fatalf("second GetOrFetch failed: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("contact user endpoint calls after ttl expiry = %d, want 2", got)
	}
}

type identityMetricCaptureWriter struct {
	items []observability.Metric
}

func (w *identityMetricCaptureWriter) Record(_ context.Context, metric observability.Metric) error {
	w.items = append(w.items, metric)
	return nil
}

func (w *identityMetricCaptureWriter) find(name string) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name {
			return &w.items[i]
		}
	}
	return nil
}

// TestClientBotOpenID_RecoversAfterFailure 是 Phase 3 缺口 6 修复的蓝军点。
//
// 不变式:启动期飞书 API 抖动让 BotOpenID 第一次失败后,后续调用必须可恢复。
//
// 旧 sync.Once 实现:失败一次 botOpenID 永空 → 自反射防御失效 → bot 死循环。
// 新 atomic.Pointer 实现:失败不缓存,下次调用重试。
//
// 蓝军 mutation 点:把 BotOpenID 改回 sync.Once 永久缓存失败 → 第二次 BotOpenID
// 调用拿到的还是旧失败状态 ""(因为 once.Do 不再触发)→ 本测试 second 子用例必红。
func TestClientBotOpenID_RecoversAfterFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "token",
				"expire":              7200,
			})
		case "/open-apis/bot/v3/info":
			n := calls.Add(1)
			if n == 1 {
				// 第一次模拟飞书侧抖动
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":99991400,"msg":"服务暂时不可用"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"msg":  "success",
				"bot": map[string]any{
					"open_id": "ou_bot_recovered",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient("app", "secret", zap.NewNop(), lark.WithOpenBaseUrl(server.URL))

	// 第一次:失败,拿空字符串(不缓存)
	first := client.BotOpenID()
	if first != "" {
		t.Fatalf("first BotOpenID() = %q, want empty (server returned 503)", first)
	}

	// 第二次:服务器恢复,新调用应拿到 ou_bot_recovered
	second := client.BotOpenID()
	if second != "ou_bot_recovered" {
		t.Fatalf("second BotOpenID() = %q, want ou_bot_recovered (recovery)", second)
	}

	// 第三次:命中缓存,不再打 API
	third := client.BotOpenID()
	if third != "ou_bot_recovered" {
		t.Fatalf("third BotOpenID() = %q, want ou_bot_recovered (cached)", third)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("API 调用次数 = %d, want 2(失败一次 + 成功一次,第三次走缓存)", got)
	}
}
