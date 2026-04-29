package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
)

const smallFileThreshold = 4096

// FileTracker 追踪已编辑文件的 hash，防止编辑后被外部修改而未察觉。
type FileTracker struct {
	mu      sync.Mutex
	hashes  map[string]string // path -> hash
	maxSize int               // 最大追踪文件数（默认 500）
}

// NewFileTracker 创建新的 FileTracker。maxSize <= 0 时使用默认值 500。
func NewFileTracker(maxSize int) *FileTracker {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &FileTracker{
		hashes:  make(map[string]string),
		maxSize: maxSize,
	}
}

// Track 计算文件 hash 并存储。如果已达上限，先 evict 10%。
func (ft *FileTracker) Track(path string) error {
	h, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("FileTracker.Track: %w", err)
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()

	// 仅在新增条目时检查容量（更新已有 key 不计入新增）
	if _, exists := ft.hashes[path]; !exists && len(ft.hashes) >= ft.maxSize {
		ft.evictOldest()
	}

	ft.hashes[path] = h
	return nil
}

// HasChanged 比较当前 hash 与存储 hash。
// 未追踪的 path 返回 (false, nil)——未知视为未变化。
func (ft *FileTracker) HasChanged(path string) (bool, error) {
	ft.mu.Lock()
	stored, ok := ft.hashes[path]
	ft.mu.Unlock()

	if !ok {
		return false, nil
	}

	current, err := hashFile(path)
	if err != nil {
		return false, fmt.Errorf("FileTracker.HasChanged: %w", err)
	}

	return current != stored, nil
}

// evictOldest 批量删除 maxSize/10 个条目（map 无序遍历）。
// 调用方必须已持有 ft.mu。
func (ft *FileTracker) evictOldest() {
	toDelete := ft.maxSize / 10
	if toDelete < 1 {
		toDelete = 1
	}
	count := 0
	for k := range ft.hashes {
		if count >= toDelete {
			break
		}
		delete(ft.hashes, k)
		count++
	}
}

// hashFile 计算文件的采样 SHA-256 hash。
// 小文件（< smallFileThreshold bytes）直接全文 SHA-256。
// 大文件：文件大小 + 开头 512 bytes + 中间 512 bytes + 末尾 512 bytes。
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	h := sha256.New()

	if size < smallFileThreshold {
		// 小文件：直接全文 hash
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	// 大文件：采样策略
	// 将文件大小写入 hash，区分内容相同但大小不同的（理论不存在，但加强唯一性）
	fmt.Fprintf(h, "%d", size)

	buf := make([]byte, 512)

	// 1. 开头 512 bytes
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if _, err := readFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)

	// 2. 中间 512 bytes（从 size/2 处）
	mid := size / 2
	if _, err := f.Seek(mid, io.SeekStart); err != nil {
		return "", err
	}
	if _, err := readFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)

	// 3. 末尾 512 bytes（从 size-512 处，不足 512 bytes 则从 0）
	var tailOffset int64
	if size >= 512 {
		tailOffset = size - 512
	} else {
		tailOffset = 0
	}
	if _, err := f.Seek(tailOffset, io.SeekStart); err != nil {
		return "", err
	}
	if _, err := readFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)

	return hex.EncodeToString(h.Sum(nil)), nil
}

// readFull 读满 buf，不足则读到 EOF（不报 io.ErrUnexpectedEOF 为错误）。
func readFull(r io.Reader, buf []byte) (int, error) {
	n, err := io.ReadFull(r, buf)
	if err == io.ErrUnexpectedEOF {
		// 文件剩余字节不足 buf，只取实际读到的部分，正常
		return n, nil
	}
	return n, err
}
