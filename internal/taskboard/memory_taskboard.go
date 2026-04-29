package taskboard

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

// InMemoryTaskBoard 基于内存的 TaskBoard 实现（开发/测试环境）。
// 并发安全，数据不持久化。
type InMemoryTaskBoard struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	seq   int64 // 自增 ID
}

// NewInMemoryTaskBoard 创建内存 TaskBoard。
func NewInMemoryTaskBoard() *InMemoryTaskBoard {
	return &InMemoryTaskBoard{
		tasks: make(map[string]*Task),
	}
}

func (b *InMemoryTaskBoard) Create(_ context.Context, task *Task) (string, error) {
	// 校验
	if task.Status != "" {
		if err := validateStatus(task.Status); err != nil {
			return "", err
		}
	}
	if task.Priority != "" {
		if err := validatePriority(task.Priority); err != nil {
			return "", err
		}
	}
	if task.ParentID != "" {
		if _, err := mustParseID(task.ParentID); err != nil {
			return "", fmt.Errorf("invalid parent ID: %q", task.ParentID)
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 深拷贝入参，避免修改调用方对象
	t := cloneTask(task)

	b.seq++
	t.ID = fmt.Sprintf("task-%d", b.seq)
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = StatusPending
	}
	if t.Priority == "" {
		t.Priority = PriorityMedium
	}

	b.tasks[t.ID] = t
	return t.ID, nil
}

func (b *InMemoryTaskBoard) Get(_ context.Context, id string) (*Task, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	t, ok := b.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return cloneTask(t), nil
}

func (b *InMemoryTaskBoard) Update(_ context.Context, id string, patch TaskPatch) error {
	if patch.Status != nil {
		if err := validateStatus(*patch.Status); err != nil {
			return err
		}
	}
	if patch.Priority != nil {
		if err := validatePriority(*patch.Priority); err != nil {
			return err
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	t, ok := b.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}

	if patch.Title != nil {
		t.Title = *patch.Title
	}
	if patch.Description != nil {
		t.Description = *patch.Description
	}
	if patch.Status != nil {
		t.Status = *patch.Status
	}
	if patch.Priority != nil {
		t.Priority = *patch.Priority
	}
	if patch.Assignee != nil {
		t.Assignee = *patch.Assignee
	}
	if patch.Tags != nil {
		t.Tags = make([]string, len(patch.Tags))
		copy(t.Tags, patch.Tags)
	}
	t.UpdatedAt = time.Now()
	return nil
}

func (b *InMemoryTaskBoard) List(_ context.Context, filter TaskFilter) ([]*Task, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []*Task
	for _, t := range b.tasks {
		if !matchFilter(t, filter) {
			continue
		}
		result = append(result, cloneTask(t))
	}

	// 按 CreatedAt 排序（稳定）
	slices.SortStableFunc(result, func(a, b *Task) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})

	// 分页
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return nil, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (b *InMemoryTaskBoard) Delete(_ context.Context, id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.tasks[id]; !ok {
		return fmt.Errorf("task %q not found", id)
	}
	// 递归级联删除：收集自身及所有后代（任意深度），一次删除
	toDelete := b.collectDescendants(id)
	for _, did := range toDelete {
		delete(b.tasks, did)
	}
	return nil
}

// collectDescendants 返回 id 自身及其所有后代的 ID 列表。
// 调用方必须持有 b.mu 锁。
func (b *InMemoryTaskBoard) collectDescendants(id string) []string {
	result := []string{id}
	for cid, t := range b.tasks {
		if t.ParentID == id {
			result = append(result, b.collectDescendants(cid)...)
		}
	}
	return result
}

// --- helpers ---

func matchFilter(t *Task, f TaskFilter) bool {
	if f.SessionID != "" && t.SessionID != f.SessionID {
		return false
	}
	if f.Status != "" && t.Status != f.Status {
		return false
	}
	if f.Priority != "" && t.Priority != f.Priority {
		return false
	}
	if f.Assignee != "" && t.Assignee != f.Assignee {
		return false
	}
	if f.ParentID != "" && t.ParentID != f.ParentID {
		return false
	}
	if len(f.Tags) > 0 {
		if !hasAnyTag(t.Tags, f.Tags) {
			return false
		}
	}
	return true
}

func hasAnyTag(taskTags, filterTags []string) bool {
	set := make(map[string]struct{}, len(taskTags))
	for _, tag := range taskTags {
		set[tag] = struct{}{}
	}
	for _, tag := range filterTags {
		if _, ok := set[tag]; ok {
			return true
		}
	}
	return false
}

// cloneTask 手动深拷贝 Task
func cloneTask(src *Task) *Task {
	t := *src
	if src.Tags != nil {
		t.Tags = make([]string, len(src.Tags))
		copy(t.Tags, src.Tags)
	}
	return &t
}
