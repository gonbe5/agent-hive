package cache

import (
	"sync"
	"testing"
	"time"
)

func TestNewToolResultCache(t *testing.T) {
	t.Run("valid size", func(t *testing.T) {
		tc, err := NewToolResultCache(10, 5*time.Minute)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tc == nil {
			t.Fatal("expected non-nil ToolResultCache")
		}
	})

	t.Run("zero size returns error", func(t *testing.T) {
		_, err := NewToolResultCache(0, 5*time.Minute)
		if err == nil {
			t.Fatal("expected error for size 0")
		}
	})
}

func TestToolResultCache_GetMiss(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := tc.Get("nonexistent_tool", nil)
	if ok {
		t.Error("expected cache miss for key never set")
	}
}

func TestToolResultCache_SetAndGet(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	toolName := "read_file"
	params := map[string]string{"path": "/test/file.txt"}
	result := "file content"

	tc.Set(toolName, params, result)

	got, ok := tc.Get(toolName, params)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != result {
		t.Errorf("got %v, want %v", got, result)
	}
}

func TestToolResultCache_DifferentParams(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	params1 := map[string]string{"path": "/a.txt"}
	params2 := map[string]string{"path": "/b.txt"}

	tc.Set("read_file", params1, "content_a")
	tc.Set("read_file", params2, "content_b")

	got1, ok1 := tc.Get("read_file", params1)
	got2, ok2 := tc.Get("read_file", params2)

	if !ok1 || got1 != "content_a" {
		t.Errorf("params1: got %v (ok=%v), want content_a", got1, ok1)
	}
	if !ok2 || got2 != "content_b" {
		t.Errorf("params2: got %v (ok=%v), want content_b", got2, ok2)
	}
}

func TestToolResultCache_DifferentTools(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	params := map[string]string{"path": "/test.txt"}

	tc.Set("read_file", params, "read_result")
	tc.Set("write_file", params, "write_result")

	got1, ok1 := tc.Get("read_file", params)
	got2, ok2 := tc.Get("write_file", params)

	if !ok1 || got1 != "read_result" {
		t.Errorf("read_file: got %v (ok=%v), want read_result", got1, ok1)
	}
	if !ok2 || got2 != "write_result" {
		t.Errorf("write_file: got %v (ok=%v), want write_result", got2, ok2)
	}
}

func TestToolResultCache_TTLExpiration(t *testing.T) {
	tc, err := NewToolResultCache(10, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	tc.Set("tool", nil, "result")

	// Should hit before TTL
	if _, ok := tc.Get("tool", nil); !ok {
		t.Error("expected cache hit before TTL expiry")
	}

	time.Sleep(60 * time.Millisecond)

	_, ok := tc.Get("tool", nil)
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestToolResultCache_LRUEviction(t *testing.T) {
	// Cache size = 2
	tc, err := NewToolResultCache(2, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tc.Set("tool1", nil, "result1")
	tc.Set("tool2", nil, "result2")
	tc.Set("tool3", nil, "result3") // should evict tool1

	if _, ok := tc.Get("tool1", nil); ok {
		t.Error("expected tool1 to be evicted by LRU")
	}
	if _, ok := tc.Get("tool2", nil); !ok {
		t.Error("expected tool2 to still be cached")
	}
	if _, ok := tc.Get("tool3", nil); !ok {
		t.Error("expected tool3 to still be cached")
	}
}

func TestToolResultCache_OverwriteExistingKey(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	params := map[string]string{"key": "val"}
	tc.Set("tool", params, "v1")
	tc.Set("tool", params, "v2")

	got, ok := tc.Get("tool", params)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "v2" {
		t.Errorf("got %v, want v2", got)
	}
}

func TestToolResultCache_ComplexResultTypes(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Map result
	mapResult := map[string]interface{}{"status": "ok", "count": 42}
	tc.Set("api_call", nil, mapResult)
	got, ok := tc.Get("api_call", nil)
	if !ok {
		t.Fatal("expected cache hit for map result")
	}
	gotMap, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", got)
	}
	if gotMap["status"] != "ok" {
		t.Errorf("got status=%v, want ok", gotMap["status"])
	}

	// Slice result
	sliceResult := []string{"a", "b", "c"}
	tc.Set("list_items", nil, sliceResult)
	got2, ok := tc.Get("list_items", nil)
	if !ok {
		t.Fatal("expected cache hit for slice result")
	}
	gotSlice, ok := got2.([]string)
	if !ok {
		t.Fatalf("expected []string result, got %T", got2)
	}
	if len(gotSlice) != 3 || gotSlice[0] != "a" {
		t.Errorf("unexpected slice result: %v", gotSlice)
	}

	// Nil result
	tc.Set("empty_tool", nil, nil)
	got3, ok := tc.Get("empty_tool", nil)
	if !ok {
		t.Fatal("expected cache hit for nil result")
	}
	if got3 != nil {
		t.Errorf("expected nil result, got %v", got3)
	}
}

func TestToolResultCache_MakeKeyDeterminism(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Same params in different order should produce the same key
	// because json.Marshal sorts map keys
	params1 := map[string]string{"a": "1", "b": "2", "c": "3"}
	params2 := map[string]string{"c": "3", "a": "1", "b": "2"}

	key1 := tc.makeKey("tool", params1)
	key2 := tc.makeKey("tool", params2)

	if key1 != key2 {
		t.Errorf("keys should be equal for same params in different order:\n  key1=%s\n  key2=%s", key1, key2)
	}

	// Different params should produce different keys
	params3 := map[string]string{"a": "1", "b": "999"}
	key3 := tc.makeKey("tool", params3)
	if key1 == key3 {
		t.Error("keys should differ for different params")
	}

	// Different tool names should produce different keys
	key4 := tc.makeKey("other_tool", params1)
	if key1 == key4 {
		t.Error("keys should differ for different tool names")
	}
}

func TestToolResultCache_ConcurrentAccess(t *testing.T) {
	tc, err := NewToolResultCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	const n = 50

	// Concurrent writes
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			params := map[string]int{"idx": idx}
			tc.Set("tool", params, idx)
		}(i)
	}
	wg.Wait()

	// Concurrent reads
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			params := map[string]int{"idx": idx}
			tc.Get("tool", params)
		}(i)
	}
	wg.Wait()

	// Mixed concurrent read/write
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			params := map[string]int{"idx": idx}
			tc.Get("tool", params)
		}(i)
		go func(idx int) {
			defer wg.Done()
			params := map[string]int{"idx": idx}
			tc.Set("tool", params, idx*10)
		}(i)
	}
	wg.Wait()
}

func TestToolResultCache_NilParams(t *testing.T) {
	tc, err := NewToolResultCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tc.Set("tool_a", nil, "result_a")
	tc.Set("tool_b", nil, "result_b")

	got, ok := tc.Get("tool_a", nil)
	if !ok || got != "result_a" {
		t.Errorf("tool_a: got %v (ok=%v), want result_a", got, ok)
	}
	got, ok = tc.Get("tool_b", nil)
	if !ok || got != "result_b" {
		t.Errorf("tool_b: got %v (ok=%v), want result_b", got, ok)
	}
}
