package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewFileCache(t *testing.T) {
	t.Run("valid size", func(t *testing.T) {
		fc, err := NewFileCache(10, 5*time.Minute)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fc == nil {
			t.Fatal("expected non-nil FileCache")
		}
	})

	t.Run("zero size returns error", func(t *testing.T) {
		_, err := NewFileCache(0, 5*time.Minute)
		if err == nil {
			t.Fatal("expected error for size 0")
		}
	})

	t.Run("negative size returns error", func(t *testing.T) {
		_, err := NewFileCache(-1, 5*time.Minute)
		if err == nil {
			t.Fatal("expected error for negative size")
		}
	})
}

func TestFileCache_GetMiss(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := fc.Get("/nonexistent/path")
	if ok {
		t.Error("expected cache miss for key never set")
	}
}

func TestFileCache_SetAndGet(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)

	fc.Set(tmpFile, content, stat.ModTime())

	got, ok := fc.Get(tmpFile)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestFileCache_InvalidationOnFileModification(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)
	fc.Set(tmpFile, "v1", stat.ModTime())

	// Modify the file -- sleep to ensure mod time changes
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(tmpFile, []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	_, ok := fc.Get(tmpFile)
	if ok {
		t.Error("expected cache miss after file modification")
	}
}

func TestFileCache_InvalidationOnFileDelete(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)
	fc.Set(tmpFile, "data", stat.ModTime())

	// Delete the file
	os.Remove(tmpFile)

	_, ok := fc.Get(tmpFile)
	if ok {
		t.Error("expected cache miss after file deletion")
	}
}

func TestFileCache_TTLExpiration(t *testing.T) {
	// Use a very short TTL
	fc, err := NewFileCache(10, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)
	fc.Set(tmpFile, "data", stat.ModTime())

	// Should hit before TTL
	if _, ok := fc.Get(tmpFile); !ok {
		t.Error("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	_, ok := fc.Get(tmpFile)
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestFileCache_LRUEviction(t *testing.T) {
	// Cache size = 2
	fc, err := NewFileCache(2, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	files := make([]string, 3)
	for i := 0; i < 3; i++ {
		f := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("content%d", i)), 0644); err != nil {
			t.Fatal(err)
		}
		files[i] = f
	}

	for i := 0; i < 3; i++ {
		stat, _ := os.Stat(files[i])
		fc.Set(files[i], fmt.Sprintf("content%d", i), stat.ModTime())
	}

	// file0 should have been evicted (LRU with size 2)
	if _, ok := fc.Get(files[0]); ok {
		t.Error("expected file0 to be evicted by LRU")
	}

	// file1 and file2 should still be present
	if _, ok := fc.Get(files[1]); !ok {
		t.Error("expected file1 to still be cached")
	}
	if _, ok := fc.Get(files[2]); !ok {
		t.Error("expected file2 to still be cached")
	}
}

func TestFileCache_OverwriteExistingKey(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)
	fc.Set(tmpFile, "v1", stat.ModTime())

	// Overwrite with new content but same modTime (simulating an in-place update)
	fc.Set(tmpFile, "v2", stat.ModTime())

	got, ok := fc.Get(tmpFile)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "v2" {
		t.Errorf("got %q, want %q", got, "v2")
	}
}

func TestFileCache_ConcurrentAccess(t *testing.T) {
	fc, err := NewFileCache(100, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	const n = 50
	files := make([]string, n)
	for i := 0; i < n; i++ {
		f := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("content%d", i)), 0644); err != nil {
			t.Fatal(err)
		}
		files[i] = f
	}

	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stat, _ := os.Stat(files[idx])
			fc.Set(files[idx], fmt.Sprintf("content%d", idx), stat.ModTime())
		}(i)
	}
	wg.Wait()

	// Concurrent readers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fc.Get(files[idx])
		}(i)
	}
	wg.Wait()

	// Mixed read/write
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			fc.Get(files[idx])
		}(i)
		go func(idx int) {
			defer wg.Done()
			stat, _ := os.Stat(files[idx])
			fc.Set(files[idx], fmt.Sprintf("updated%d", idx), stat.ModTime())
		}(i)
	}
	wg.Wait()
}

func TestFileCache_EmptyContent(t *testing.T) {
	fc, err := NewFileCache(10, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(tmpFile)

	fc.Set(tmpFile, "", stat.ModTime())

	got, ok := fc.Get(tmpFile)
	if !ok {
		t.Fatal("expected cache hit for empty content")
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
