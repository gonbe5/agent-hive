package controlplane

import (
	"encoding/json"
	"os"
	"sync"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// BindingEntry 绑定条目
type BindingEntry struct {
	Platform  string `json:"platform"`
	ChatID    string `json:"chat_id"`
	SessionID string `json:"session_id"`
}

// BindingStore 管理 IM channel -> session 的绑定持久化
type BindingStore struct {
	filePath string
	entries  map[string]string // "platform:chatID" -> sessionID
	mu       sync.RWMutex
	logger   *zap.Logger
}

// NewBindingStore 创建绑定存储
func NewBindingStore(filePath string, logger *zap.Logger) (*BindingStore, error) {
	bs := &BindingStore{
		filePath: filePath,
		entries:  make(map[string]string),
		logger:   logger,
	}
	if filePath != "" {
		if err := bs.load(); err != nil {
			if !os.IsNotExist(err) {
				return nil, errs.Wrap(errs.CodeStoreError, "加载绑定失败", err)
			}
		}
	}
	return bs, nil
}

// Lookup 查找绑定的 sessionID
func (bs *BindingStore) Lookup(platform, chatID string) string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return bs.entries[platform+":"+chatID]
}

// Bind 创建绑定
func (bs *BindingStore) Bind(platform, chatID, sessionID string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.entries[platform+":"+chatID] = sessionID
	return bs.save()
}

// Unbind 删除绑定
func (bs *BindingStore) Unbind(platform, chatID string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	delete(bs.entries, platform+":"+chatID)
	if err := bs.save(); err != nil {
		bs.logger.Warn("保存绑定失败", zap.Error(err))
	}
}

// ListBindings 列出所有绑定
func (bs *BindingStore) ListBindings() []BindingEntry {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	var result []BindingEntry
	for key, sessionID := range bs.entries {
		parts := splitKey(key)
		if len(parts) == 2 {
			result = append(result, BindingEntry{
				Platform:  parts[0],
				ChatID:    parts[1],
				SessionID: sessionID,
			})
		}
	}
	return result
}

func (bs *BindingStore) load() error {
	data, err := os.ReadFile(bs.filePath)
	if err != nil {
		return err
	}
	var entries []BindingEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	for _, e := range entries {
		bs.entries[e.Platform+":"+e.ChatID] = e.SessionID
	}
	return nil
}

func (bs *BindingStore) save() error {
	if bs.filePath == "" {
		return nil
	}
	entries := bs.listBindingsLocked()
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(bs.filePath, data, 0644)
}

// listBindingsLocked 在已持有锁的情况下列出绑定（save 内部使用）
func (bs *BindingStore) listBindingsLocked() []BindingEntry {
	var result []BindingEntry
	for key, sessionID := range bs.entries {
		parts := splitKey(key)
		if len(parts) == 2 {
			result = append(result, BindingEntry{
				Platform:  parts[0],
				ChatID:    parts[1],
				SessionID: sessionID,
			})
		}
	}
	return result
}

func splitKey(key string) []string {
	for i, ch := range key {
		if ch == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return []string{key}
}
