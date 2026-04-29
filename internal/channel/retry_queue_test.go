package channel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestMemoryRetryQueue_BasicEnqueue 覆盖默认实现的 happy path：
// Enqueue 后 Snapshot/Len 都能看到，Reason 和 EnqueuedAt 被正确填充。
func TestMemoryRetryQueue_BasicEnqueue(t *testing.T) {
	q := NewMemoryRetryQueue(0, zap.NewNop())
	defer q.Stop()

	if err := q.Enqueue(RetryItem{
		MessageID: "m1",
		Platform:  "feishu",
		Reason:    RetryReasonHandlerError,
		ErrorMsg:  "boom",
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if got := q.Len(); got != 1 {
		t.Fatalf("len = %d, want 1", got)
	}
	snap := q.Snapshot()
	if snap[0].MessageID != "m1" || snap[0].Reason != RetryReasonHandlerError {
		t.Fatalf("snapshot mismatch: %+v", snap[0])
	}
	if snap[0].EnqueuedAt.IsZero() {
		t.Fatalf("EnqueuedAt should be auto-filled")
	}
}

// TestMemoryRetryQueue_NilReceiverNoop 验证 nil receiver 上 Enqueue/Len/Snapshot 都 no-op、不 panic。
// 这是 plugin 端 wrapper "Router 没注入 RetryQueue 时仍然能跑" 的兜底契约。
func TestMemoryRetryQueue_NilReceiverNoop(t *testing.T) {
	var q *MemoryRetryQueue
	if err := q.Enqueue(RetryItem{Reason: RetryReasonHandlerError}); err != nil {
		t.Fatalf("nil enqueue should be noop, got %v", err)
	}
	if q.Len() != 0 {
		t.Fatalf("nil len should be 0")
	}
	if q.Snapshot() != nil {
		t.Fatalf("nil snapshot should be nil")
	}
	if err := q.Stop(); err != nil {
		t.Fatalf("nil stop should be noop, got %v", err)
	}
}

// TestMemoryRetryQueue_RejectEmptyReason 校验缺 Reason 必须报错——避免 PR 把空字符串塞进去导致告警丢失语义。
func TestMemoryRetryQueue_RejectEmptyReason(t *testing.T) {
	q := NewMemoryRetryQueue(0, zap.NewNop())
	if err := q.Enqueue(RetryItem{MessageID: "m"}); err == nil {
		t.Fatalf("missing Reason must return error")
	}
}

// TestMemoryRetryQueue_DropOldestOnOverflow 校验软上限：超过 maxItems 时丢最旧。
// 不允许为了"防丢失"而无限增长（OOM 风险大于丢失少量条目）。
func TestMemoryRetryQueue_DropOldestOnOverflow(t *testing.T) {
	q := NewMemoryRetryQueue(2, zap.NewNop())
	defer q.Stop()
	for i, id := range []string{"a", "b", "c"} {
		if err := q.Enqueue(RetryItem{MessageID: id, Reason: RetryReasonHandlerError}); err != nil {
			t.Fatalf("enqueue[%d]: %v", i, err)
		}
	}
	snap := q.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("len = %d, want 2 after overflow drop", len(snap))
	}
	if snap[0].MessageID != "b" || snap[1].MessageID != "c" {
		t.Fatalf("oldest not dropped: %+v", snap)
	}
}

// TestFileBackedRetryQueue_Persists 校验 FileBackedRetryQueue：
// Enqueue 后，磁盘文件每行是一条 RetryItem JSON，重启进程也能读出来。
func TestFileBackedRetryQueue_Persists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "retry.jsonl")
	q, err := NewFileBackedRetryQueue(path, 0, zap.NewNop())
	if err != nil {
		t.Fatalf("new file queue: %v", err)
	}
	defer q.Stop()

	items := []RetryItem{
		{MessageID: "m1", Reason: RetryReasonHandlerError, ErrorMsg: "e1"},
		{MessageID: "m2", Reason: RetryReasonHandlerPanic, ErrorMsg: "e2"},
	}
	for _, it := range items {
		if err := q.Enqueue(it); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var got []RetryItem
	for dec.More() {
		var it RetryItem
		if err := dec.Decode(&it); err != nil {
			t.Fatalf("decode: %v", err)
		}
		got = append(got, it)
	}
	if len(got) != 2 {
		t.Fatalf("decoded %d items, want 2", len(got))
	}
	if got[0].MessageID != "m1" || got[1].MessageID != "m2" {
		t.Fatalf("persisted order/content mismatch: %+v", got)
	}
}

// TestFileBackedRetryQueue_EmptyPathFallsBackToMemory 校验路径为空时退化为纯内存，不 panic。
// 测试场景或开发模式不需要持久化文件。
func TestFileBackedRetryQueue_EmptyPathFallsBackToMemory(t *testing.T) {
	q, err := NewFileBackedRetryQueue("", 0, zap.NewNop())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer q.Stop()
	if err := q.Enqueue(RetryItem{MessageID: "m", Reason: RetryReasonHandlerError, EnqueuedAt: time.Now()}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if q.Len() != 1 {
		t.Fatalf("len = %d", q.Len())
	}
}

