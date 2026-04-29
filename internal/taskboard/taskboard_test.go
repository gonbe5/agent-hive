package taskboard

import (
	"context"
	"testing"
)

func TestInMemoryCreate(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	task := &Task{Title: "test task", SessionID: "s1"}
	id, err := b.Create(ctx, task)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty ID")
	}
	// 验证入参未被修改
	if task.ID != "" {
		t.Errorf("Create modified caller's task.ID: got %q", task.ID)
	}
}

func TestInMemoryCreateDefaults(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id, _ := b.Create(ctx, &Task{Title: "t"})
	got, _ := b.Get(ctx, id)
	if got.Status != StatusPending {
		t.Errorf("default status: got %q, want %q", got.Status, StatusPending)
	}
	if got.Priority != PriorityMedium {
		t.Errorf("default priority: got %q, want %q", got.Priority, PriorityMedium)
	}
}

func TestInMemoryGet(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id, _ := b.Create(ctx, &Task{Title: "t"})
	got, err := b.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "t" {
		t.Errorf("Title: got %q, want %q", got.Title, "t")
	}
}

func TestInMemoryGetNotFound(t *testing.T) {
	b := NewInMemoryTaskBoard()
	_, err := b.Get(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Get nonexistent: expected error")
	}
}

func TestInMemoryUpdate(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id, _ := b.Create(ctx, &Task{Title: "old"})

	newTitle := "new"
	newStatus := StatusDone
	err := b.Update(ctx, id, TaskPatch{Title: &newTitle, Status: &newStatus})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := b.Get(ctx, id)
	if got.Title != "new" {
		t.Errorf("Title: got %q, want %q", got.Title, "new")
	}
	if got.Status != StatusDone {
		t.Errorf("Status: got %q, want %q", got.Status, StatusDone)
	}
}

func TestInMemoryUpdateNotFound(t *testing.T) {
	b := NewInMemoryTaskBoard()
	s := "x"
	err := b.Update(context.Background(), "nonexistent", TaskPatch{Title: &s})
	if err == nil {
		t.Fatal("Update nonexistent: expected error")
	}
}

func TestInMemoryList(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	b.Create(ctx, &Task{Title: "a", SessionID: "s1"})
	b.Create(ctx, &Task{Title: "b", SessionID: "s2"})
	b.Create(ctx, &Task{Title: "c", SessionID: "s1"})

	// 无过滤
	all, _ := b.List(ctx, TaskFilter{})
	if len(all) != 3 {
		t.Fatalf("List all: got %d, want 3", len(all))
	}

	// 按 SessionID 过滤
	s1, _ := b.List(ctx, TaskFilter{SessionID: "s1"})
	if len(s1) != 2 {
		t.Fatalf("List session s1: got %d, want 2", len(s1))
	}

	// 空结果
	empty, _ := b.List(ctx, TaskFilter{SessionID: "nonexistent"})
	if len(empty) != 0 {
		t.Fatalf("List empty: got %d, want 0", len(empty))
	}
}

func TestInMemoryListPagination(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		b.Create(ctx, &Task{Title: "t"})
	}

	page, _ := b.List(ctx, TaskFilter{Limit: 2, Offset: 1})
	if len(page) != 2 {
		t.Fatalf("List paginated: got %d, want 2", len(page))
	}
}

func TestInMemoryListByStatus(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id1, _ := b.Create(ctx, &Task{Title: "a"})
	b.Create(ctx, &Task{Title: "b"})

	done := StatusDone
	b.Update(ctx, id1, TaskPatch{Status: &done})

	doneList, _ := b.List(ctx, TaskFilter{Status: StatusDone})
	if len(doneList) != 1 {
		t.Fatalf("List done: got %d, want 1", len(doneList))
	}
}

func TestInMemoryDelete(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id, _ := b.Create(ctx, &Task{Title: "t"})
	if err := b.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := b.Get(ctx, id)
	if err == nil {
		t.Fatal("Get after delete: expected error")
	}
}

func TestInMemoryDeleteNotFound(t *testing.T) {
	b := NewInMemoryTaskBoard()
	err := b.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Delete nonexistent: expected error")
	}
}

func TestInMemoryDeleteCascade(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	parentID, _ := b.Create(ctx, &Task{Title: "parent"})
	childID, _ := b.Create(ctx, &Task{Title: "child", ParentID: parentID})

	if err := b.Delete(ctx, parentID); err != nil {
		t.Fatalf("Delete parent: %v", err)
	}
	// 子任务也应该被删除
	_, err := b.Get(ctx, childID)
	if err == nil {
		t.Fatal("child should be deleted after parent deletion")
	}
}

func TestInMemoryDeleteCascadeDeep(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	// grandparent -> parent -> child (3 levels)
	grandparentID, _ := b.Create(ctx, &Task{Title: "grandparent"})
	parentID, _ := b.Create(ctx, &Task{Title: "parent", ParentID: grandparentID})
	childID, _ := b.Create(ctx, &Task{Title: "child", ParentID: parentID})

	if err := b.Delete(ctx, grandparentID); err != nil {
		t.Fatalf("Delete grandparent: %v", err)
	}
	// all descendants should be deleted
	for _, id := range []string{grandparentID, parentID, childID} {
		if _, err := b.Get(ctx, id); err == nil {
			t.Errorf("task %s should be deleted after grandparent deletion", id)
		}
	}
}

func TestInMemoryCreateInvalidParentID(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	// invalid format should be rejected
	for _, bad := range []string{"bad", "123", "task-", "task-abc"} {
		_, err := b.Create(ctx, &Task{Title: "t", ParentID: bad})
		if err == nil {
			t.Errorf("Create with ParentID=%q: expected error", bad)
		}
	}

	// valid format should be accepted
	_, err := b.Create(ctx, &Task{Title: "t", ParentID: "task-1"})
	if err != nil {
		t.Errorf("Create with valid ParentID: %v", err)
	}

	// empty ParentID should be accepted
	_, err = b.Create(ctx, &Task{Title: "t"})
	if err != nil {
		t.Errorf("Create with empty ParentID: %v", err)
	}
}

func TestInMemoryCreateInvalidStatus(t *testing.T) {
	b := NewInMemoryTaskBoard()
	_, err := b.Create(context.Background(), &Task{Title: "t", Status: "garbage"})
	if err == nil {
		t.Fatal("Create with invalid status: expected error")
	}
}

func TestInMemoryUpdateInvalidPriority(t *testing.T) {
	b := NewInMemoryTaskBoard()
	ctx := context.Background()

	id, _ := b.Create(ctx, &Task{Title: "t"})
	bad := Priority("garbage")
	err := b.Update(ctx, id, TaskPatch{Priority: &bad})
	if err == nil {
		t.Fatal("Update with invalid priority: expected error")
	}
}
