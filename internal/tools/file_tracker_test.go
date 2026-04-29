package tools

import (
	"os"
	"strings"
	"testing"
)

// TestHashFile_Small 验证小文件（< 4096 bytes）内容不同则 hash 不同
func TestHashFile_Small(t *testing.T) {
	f1, err := os.CreateTemp(t.TempDir(), "small1")
	if err != nil {
		t.Fatal(err)
	}
	f1.WriteString("hello world")
	f1.Close()

	f2, err := os.CreateTemp(t.TempDir(), "small2")
	if err != nil {
		t.Fatal(err)
	}
	f2.WriteString("hello WORLD")
	f2.Close()

	h1, err := hashFile(f1.Name())
	if err != nil {
		t.Fatalf("hashFile f1: %v", err)
	}
	h2, err := hashFile(f2.Name())
	if err != nil {
		t.Fatalf("hashFile f2: %v", err)
	}

	if h1 == h2 {
		t.Errorf("expected different hashes for different content, got same: %s", h1)
	}
}

// TestHashFile_Large 验证大文件（> 4096 bytes）内容相同则 hash 相同
func TestHashFile_Large(t *testing.T) {
	// 构造 8192 bytes 内容
	content := strings.Repeat("abcdefghij", 820) // 8200 bytes > 4096

	dir := t.TempDir()

	f1path := dir + "/large1.txt"
	f2path := dir + "/large2.txt"

	if err := os.WriteFile(f1path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := hashFile(f1path)
	if err != nil {
		t.Fatalf("hashFile f1: %v", err)
	}
	h2, err := hashFile(f2path)
	if err != nil {
		t.Fatalf("hashFile f2: %v", err)
	}

	if h1 != h2 {
		t.Errorf("expected same hash for identical content, got %s vs %s", h1, h2)
	}
}

// TestFileTracker_Track 验证 Track → HasChanged = false；修改文件 → HasChanged = true
func TestFileTracker_Track(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/edit.txt"

	if err := os.WriteFile(path, []byte("original content"), 0644); err != nil {
		t.Fatal(err)
	}

	ft := NewFileTracker(0) // 使用默认 maxSize=500

	if err := ft.Track(path); err != nil {
		t.Fatalf("Track: %v", err)
	}

	// 刚追踪后应该没有变化
	changed, err := ft.HasChanged(path)
	if err != nil {
		t.Fatalf("HasChanged after Track: %v", err)
	}
	if changed {
		t.Error("expected HasChanged=false immediately after Track")
	}

	// 修改文件
	if err := os.WriteFile(path, []byte("modified content!!!"), 0644); err != nil {
		t.Fatal(err)
	}

	// 修改后应该检测到变化
	changed, err = ft.HasChanged(path)
	if err != nil {
		t.Fatalf("HasChanged after modification: %v", err)
	}
	if !changed {
		t.Error("expected HasChanged=true after file modification")
	}
}

// TestFileTracker_Eviction 验证 maxSize=5，添加 6 个文件后 len(hashes) <= maxSize
func TestFileTracker_Eviction(t *testing.T) {
	dir := t.TempDir()
	ft := NewFileTracker(5)

	for i := 0; i < 6; i++ {
		path := dir + "/" + string(rune('a'+i)) + ".txt"
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := ft.Track(path); err != nil {
			t.Fatalf("Track file %d: %v", i, err)
		}
	}

	ft.mu.Lock()
	count := len(ft.hashes)
	ft.mu.Unlock()

	if count > ft.maxSize {
		t.Errorf("expected len(hashes) <= maxSize(%d), got %d", ft.maxSize, count)
	}
}
