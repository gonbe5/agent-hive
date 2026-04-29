package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func mpServer(t *testing.T, idx SkillIndex, hits *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		_ = json.NewEncoder(w).Encode(idx)
	})
	// SKILL.md 文件：所有 skill 都返回同一份默认 SKILL.md
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if filepath.Base(r.URL.Path) == "SKILL.md" {
			w.Write([]byte("---\nname: x\ndescription: x\n---\ncontent\n"))
			return
		}
		http.NotFound(w, r)
	})
	return httptest.NewServer(mux)
}

// TestDiscovery_ResolveByName_Hit — 单 marketplace 命中返回 ResolvedSkill。
func TestDiscovery_ResolveByName_Hit(t *testing.T) {
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{
		{Name: "alpha", Version: "1.2.0", Files: []string{"SKILL.md"}},
	}}, nil)
	defer srv.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv.URL}, zap.NewNop())
	got, err := d.ResolveByName(context.Background(), "alpha", false)
	if err != nil {
		t.Fatalf("ResolveByName: %v", err)
	}
	if got == nil || got.Entry.Name != "alpha" || got.Entry.Version != "1.2.0" {
		t.Errorf("unexpected: %+v", got)
	}
	if got.Source != srv.URL {
		t.Errorf("Source mismatch: %s", got.Source)
	}
}

// TestDiscovery_ResolveByName_NotFound — 所有 marketplace 均 miss 返回 CodeSkillNotFound。
func TestDiscovery_ResolveByName_NotFound(t *testing.T) {
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{{Name: "beta"}}}, nil)
	defer srv.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv.URL}, zap.NewNop())
	_, err := d.ResolveByName(context.Background(), "missing", false)
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !errs.IsCode(err, errs.CodeSkillNotFound) {
		t.Errorf("expected CodeSkillNotFound, got %v", err)
	}
}

// TestDiscovery_ResolveByName_Ambiguous — 多 marketplace 同名命中 → CodeSkillAmbiguous。
func TestDiscovery_ResolveByName_Ambiguous(t *testing.T) {
	srv1 := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{{Name: "nuwa"}}}, nil)
	defer srv1.Close()
	srv2 := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{{Name: "nuwa"}}}, nil)
	defer srv2.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv1.URL, srv2.URL}, zap.NewNop())
	_, err := d.ResolveByName(context.Background(), "nuwa", false)
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	if !errs.IsCode(err, errs.CodeSkillAmbiguous) {
		t.Errorf("expected CodeSkillAmbiguous, got %v", err)
	}
}

// TestDiscovery_ResolveByRequirements_Ranking — 按 provides_requirements 覆盖度降序返回。
func TestDiscovery_ResolveByRequirements_Ranking(t *testing.T) {
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{
		{Name: "lo", ProvidesRequirements: []string{"a"}},             // 1 hit
		{Name: "hi", ProvidesRequirements: []string{"a", "b", "c"}},   // 3 hits
		{Name: "mid", ProvidesRequirements: []string{"a", "b"}},       // 2 hits
		{Name: "zero", ProvidesRequirements: []string{"z"}},           // 0 hits
	}}, nil)
	defer srv.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv.URL}, zap.NewNop())
	got, err := d.ResolveByRequirements(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("ResolveByRequirements: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 hits (zero excluded), got %d", len(got))
	}
	if got[0].Entry.Name != "hi" || got[1].Entry.Name != "mid" || got[2].Entry.Name != "lo" {
		t.Errorf("ranking broken: %s %s %s",
			got[0].Entry.Name, got[1].Entry.Name, got[2].Entry.Name)
	}
}

// TestDiscovery_IndexCacheTTL — 连续两次 Resolve 只打一次 index.json；refresh=true 强制再打一次。
func TestDiscovery_IndexCacheTTL(t *testing.T) {
	var hits int32
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{{Name: "alpha"}}}, &hits)
	defer srv.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv.URL}, zap.NewNop())
	if _, err := d.ResolveByName(context.Background(), "alpha", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := d.ResolveByName(context.Background(), "alpha", false); err != nil {
		t.Fatalf("second (cached): %v", err)
	}
	if h := atomic.LoadInt32(&hits); h != 1 {
		t.Errorf("cache miss: expected 1 index hit, got %d", h)
	}
	if _, err := d.ResolveByName(context.Background(), "alpha", true); err != nil {
		t.Fatalf("forced refresh: %v", err)
	}
	if h := atomic.LoadInt32(&hits); h != 2 {
		t.Errorf("refresh failed: expected 2 hits after refresh, got %d", h)
	}
}

// TestDiscovery_PullOne_AtomicWrite — 下载失败时不留部分目录；成功时 final dir 存在且含 SKILL.md。
func TestDiscovery_PullOne_AtomicWrite(t *testing.T) {
	// Case A: 成功路径
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{
		{Name: "gamma", Files: []string{"SKILL.md"}},
	}}, nil)
	defer srv.Close()

	cache := t.TempDir()
	d := NewDiscoveryWithMarketplaces(cache, []string{srv.URL}, zap.NewNop())
	dir, err := d.PullOne(context.Background(), srv.URL, "gamma")
	if err != nil {
		t.Fatalf("PullOne: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	// 确认没有 .tmp 残留
	if _, err := os.Stat(dir + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("leaked .tmp dir: %v", err)
	}

	// Case B: 下载失败（server 关闭）→ 原子性：final dir 不存在
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json" {
			_ = json.NewEncoder(w).Encode(SkillIndex{Skills: []SkillIndexEntry{
				{Name: "delta", Files: []string{"missing.md"}},
			}})
			return
		}
		http.Error(w, "gone", http.StatusInternalServerError)
	}))
	defer srv2.Close()

	cache2 := t.TempDir()
	d2 := NewDiscoveryWithMarketplaces(cache2, []string{srv2.URL}, zap.NewNop())
	_, err = d2.PullOne(context.Background(), srv2.URL, "delta")
	if err == nil {
		t.Fatal("expected download failure, got nil")
	}
	if _, err := os.Stat(filepath.Join(cache2, "delta")); !os.IsNotExist(err) {
		t.Error("partial final dir leaked on failed download")
	}
	if _, err := os.Stat(filepath.Join(cache2, "delta.tmp")); !os.IsNotExist(err) {
		t.Error(".tmp dir leaked on failed download")
	}
}

// TestDiscovery_NoMarketplaces — 空 marketplace 列表时 ResolveByName 返回 not-found。
func TestDiscovery_NoMarketplaces(t *testing.T) {
	d := NewDiscoveryWithMarketplaces(t.TempDir(), nil, zap.NewNop())
	_, err := d.ResolveByName(context.Background(), "x", false)
	if !errs.IsCode(err, errs.CodeSkillNotFound) {
		t.Errorf("expected CodeSkillNotFound, got %v", err)
	}
}

// TestDiscovery_CacheTTLExpiry — 人为压缩 TTL 到 10ms，验证过期后重新拉取。
func TestDiscovery_CacheTTLExpiry(t *testing.T) {
	var hits int32
	srv := mpServer(t, SkillIndex{Skills: []SkillIndexEntry{{Name: "alpha"}}}, &hits)
	defer srv.Close()

	d := NewDiscoveryWithMarketplaces(t.TempDir(), []string{srv.URL}, zap.NewNop())
	d.cacheTTL = 10 * time.Millisecond
	if _, err := d.ResolveByName(context.Background(), "alpha", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := d.ResolveByName(context.Background(), "alpha", false); err != nil {
		t.Fatal(err)
	}
	if h := atomic.LoadInt32(&hits); h != 2 {
		t.Errorf("TTL expiry failed: expected 2 hits, got %d", h)
	}
}
