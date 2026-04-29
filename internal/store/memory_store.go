package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// MemoryStore 内存存储实现，仅用于测试
type MemoryStore struct {
	mu           sync.RWMutex
	sessions     map[string]*SessionRecord
	messages     map[string][]MessageRecord // sessionID -> messages
	msgID        int64
	llmProviders map[string]*LLMProviderRecord
	llmModels    map[string]*LLMModelRecord
	schedules    map[string]*ScheduledPushRecord
}

var _ SessionStore = (*MemoryStore)(nil)

// NewMemoryStore 创建内存存储实例（仅用于测试）
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:  make(map[string]*SessionRecord),
		messages:  make(map[string][]MessageRecord),
		schedules: make(map[string]*ScheduledPushRecord),
	}
}

func (m *MemoryStore) CreateSession(_ context.Context, record *SessionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sessions[record.ID]; exists {
		return fmt.Errorf("session %s already exists", record.ID)
	}
	cp := *record
	m.sessions[record.ID] = &cp
	return nil
}

func (m *MemoryStore) SaveSession(_ context.Context, record *SessionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *record
	m.sessions[record.ID] = &cp
	return nil
}

func (m *MemoryStore) LoadSession(_ context.Context, sessionID string) (*SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *MemoryStore) DeleteSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
	delete(m.messages, sessionID)
	return nil
}

func (m *MemoryStore) ListSessions(_ context.Context) ([]*SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*SessionRecord, 0, len(m.sessions))
	for _, s := range m.sessions {
		cp := *s
		result = append(result, &cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastAccessedAt > result[j].LastAccessedAt
	})
	return result, nil
}

func (m *MemoryStore) GetLastActiveSession(_ context.Context) (*SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var latest *SessionRecord
	for _, s := range m.sessions {
		if latest == nil || s.LastAccessedAt > latest.LastAccessedAt {
			latest = s
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	cp := *latest
	return &cp, nil
}

func (m *MemoryStore) AddMessage(_ context.Context, sessionID, role, content string, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgID++
	msg := MessageRecord{
		ID:        m.msgID,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if len(metadata) > 0 {
		if data, err := json.Marshal(metadata); err == nil {
			msg.Metadata = data
		}
	}
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	if s, ok := m.sessions[sessionID]; ok {
		s.MessageCount = len(m.messages[sessionID])
	}
	return nil
}

func (m *MemoryStore) GetMessages(_ context.Context, sessionID string, limit int) ([]MessageRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.messages[sessionID]
	if limit > 0 && limit < len(msgs) {
		msgs = msgs[len(msgs)-limit:]
	}
	result := make([]MessageRecord, len(msgs))
	copy(result, msgs)
	return result, nil
}

func (m *MemoryStore) ForkSession(_ context.Context, parentID string, forkPoint int, newSessionID, newName, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, ok := m.sessions[parentID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().Format(time.RFC3339)
	m.sessions[newSessionID] = &SessionRecord{
		ID:        newSessionID,
		Name:      newName,
		CreatedAt: now,
		UpdatedAt: now,
		ParentID:  parentID,
		ForkPoint: forkPoint,
		UserID:    userID,
	}
	// Copy messages up to fork point
	if msgs, ok := m.messages[parentID]; ok && forkPoint <= len(msgs) {
		forked := make([]MessageRecord, forkPoint)
		copy(forked, msgs[:forkPoint])
		m.messages[newSessionID] = forked
	}
	parent.Children = append(parent.Children, newSessionID)
	return nil
}

func (m *MemoryStore) RevertSession(_ context.Context, sessionID string, revertTo int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs, ok := m.messages[sessionID]
	if !ok {
		return nil
	}
	if revertTo < len(msgs) {
		m.messages[sessionID] = msgs[:revertTo]
	}
	if s, ok := m.sessions[sessionID]; ok {
		s.MessageCount = len(m.messages[sessionID])
	}
	return nil
}

func (m *MemoryStore) ListSessionsByUser(_ context.Context, userID string, _ bool) ([]*SessionRecord, error) {
	if userID == "" {
		return []*SessionRecord{}, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var records []*SessionRecord
	for _, r := range m.sessions {
		if r.Deleted {
			continue
		}
		if r.UserID != userID {
			continue
		}
		cp := *r
		records = append(records, &cp)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].IsStarred != records[j].IsStarred {
			return records[i].IsStarred
		}
		return records[i].LastAccessedAt > records[j].LastAccessedAt
	})
	return records, nil
}

func (m *MemoryStore) UpsertSessionPref(_ context.Context, _, _ string, _ bool) error { return nil }
func (m *MemoryStore) GetSessionStarred(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}
func (m *MemoryStore) UpdateSessionTags(_ context.Context, sessionID string, tags []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.Tags = tags
	}
	return nil
}

// LLM Provider/Model — MemoryStore 提供内存实现，供测试使用
func (m *MemoryStore) GetLLMProvider(_ context.Context, name string) (*LLMProviderRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.llmProviders {
		if p.Name == name {
			cp := *p
			return &cp, nil
		}
	}
	return nil, errs.New(errs.CodeNotFound, "llm provider not found: "+name)
}
func (m *MemoryStore) SaveLLMProvider(_ context.Context, rec *LLMProviderRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.llmProviders == nil {
		m.llmProviders = map[string]*LLMProviderRecord{}
	}
	cp := *rec
	m.llmProviders[rec.Name] = &cp
	return nil
}
func (m *MemoryStore) DeleteLLMProvider(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mod := range m.llmModels {
		if mod.ProviderName == name {
			delete(m.llmModels, mod.Name)
		}
	}
	delete(m.llmProviders, name)
	return nil
}
func (m *MemoryStore) ListLLMProviders(_ context.Context) ([]*LLMProviderRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*LLMProviderRecord, 0, len(m.llmProviders))
	for _, p := range m.llmProviders {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}
func (m *MemoryStore) SetDefaultLLMProvider(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.llmProviders {
		if p.Name == name {
			p.IsDefault = true
		} else {
			p.IsDefault = false
		}
	}
	return nil
}
func (m *MemoryStore) GetLLMModel(_ context.Context, name string) (*LLMModelRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mod := range m.llmModels {
		if mod.Name == name {
			cp := *mod
			return &cp, nil
		}
	}
	return nil, errs.New(errs.CodeNotFound, "llm model not found: "+name)
}
func (m *MemoryStore) SaveLLMModel(_ context.Context, rec *LLMModelRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.llmModels == nil {
		m.llmModels = map[string]*LLMModelRecord{}
	}
	cp := *rec
	m.llmModels[rec.Name] = &cp
	return nil
}
func (m *MemoryStore) DeleteLLMModel(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.llmModels, name)
	return nil
}
func (m *MemoryStore) ListLLMModels(_ context.Context) ([]*LLMModelRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*LLMModelRecord, 0, len(m.llmModels))
	for _, mod := range m.llmModels {
		cp := *mod
		out = append(out, &cp)
	}
	return out, nil
}
func (m *MemoryStore) SetDefaultLLMModel(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mod := range m.llmModels {
		if mod.Name == name {
			mod.IsDefault = true
		} else {
			mod.IsDefault = false
		}
	}
	return nil
}

func (m *MemoryStore) Close() error {
	return nil
}

// Store interface stubs — 仅用于测试，不提供实际功能
func (m *MemoryStore) GetConfig(_ context.Context, _ string) (string, error)     { return "", nil }
func (m *MemoryStore) SetConfig(_ context.Context, _, _ string) error            { return nil }
func (m *MemoryStore) GetAllConfig(_ context.Context) (map[string]string, error) { return nil, nil }
func (m *MemoryStore) GetChannelConfig(_ context.Context, _ string) (*ChannelConfigRecord, error) {
	return nil, ErrNotFound
}
func (m *MemoryStore) SaveChannelConfig(_ context.Context, _ *ChannelConfigRecord) error { return nil }
func (m *MemoryStore) ListChannelConfigs(_ context.Context) ([]*ChannelConfigRecord, error) {
	return nil, nil
}
func (m *MemoryStore) SaveScheduledPush(_ context.Context, rec *ScheduledPushRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.schedules == nil {
		m.schedules = make(map[string]*ScheduledPushRecord)
	}
	cp := *rec
	now := time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	m.schedules[rec.ID] = &cp
	return nil
}
func (m *MemoryStore) GetScheduledPush(_ context.Context, id string) (*ScheduledPushRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.schedules[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *rec
	return &cp, nil
}
func (m *MemoryStore) DeleteScheduledPush(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.schedules[id]; !ok {
		return ErrNotFound
	}
	delete(m.schedules, id)
	return nil
}
func (m *MemoryStore) ListScheduledPushes(_ context.Context, platform string) ([]*ScheduledPushRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ScheduledPushRecord, 0, len(m.schedules))
	for _, rec := range m.schedules {
		if platform != "" && rec.Platform != platform {
			continue
		}
		cp := *rec
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}
func (m *MemoryStore) UpdateScheduledPushRun(_ context.Context, id string, lastRunAt, nextRunAt time.Time, lastError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.schedules[id]
	if !ok {
		return ErrNotFound
	}
	rec.LastRunAt = lastRunAt
	rec.NextRunAt = nextRunAt
	rec.LastError = lastError
	rec.UpdatedAt = time.Now().UTC()
	return nil
}
func (m *MemoryStore) GetMCPServer(_ context.Context, _ string) (*MCPServerRecord, error) {
	return nil, ErrNotFound
}
func (m *MemoryStore) SaveMCPServer(_ context.Context, _ *MCPServerRecord) error { return nil }
func (m *MemoryStore) DeleteMCPServer(_ context.Context, _ string) error         { return nil }
func (m *MemoryStore) ListMCPServers(_ context.Context) ([]*MCPServerRecord, error) {
	return nil, nil
}
func (m *MemoryStore) GetExternalResource(_ context.Context, _ string) (*ExternalResourceRecord, error) {
	return nil, ErrNotFound
}
func (m *MemoryStore) SaveExternalResource(_ context.Context, _ *ExternalResourceRecord) error {
	return nil
}
func (m *MemoryStore) DeleteExternalResource(_ context.Context, _ string) error { return nil }
func (m *MemoryStore) ListExternalResources(_ context.Context) ([]*ExternalResourceRecord, error) {
	return nil, nil
}
func (m *MemoryStore) SaveGrant(_ context.Context, _ *PermissionGrantRecord) error { return nil }
func (m *MemoryStore) LoadGrants(_ context.Context) ([]PermissionGrantRecord, error) {
	return nil, nil
}
func (m *MemoryStore) DeleteGrant(_ context.Context, _ int64) error                { return nil }
func (m *MemoryStore) DeleteAllGrants(_ context.Context) error                     { return nil }
func (m *MemoryStore) SaveOAuthToken(_ context.Context, _ *OAuthTokenRecord) error { return nil }
func (m *MemoryStore) LoadOAuthToken(_ context.Context, _ string) (*OAuthTokenRecord, error) {
	return nil, ErrNotFound
}
func (m *MemoryStore) DeleteOAuthToken(_ context.Context, _ string) error { return nil }
func (m *MemoryStore) OnConfigChange(_ func(key string))                  {}
