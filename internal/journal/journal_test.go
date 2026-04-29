package journal

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoopJournal_AllMethodsNilSafe(t *testing.T) {
	var j NoopJournal
	ctx := context.Background()

	if err := j.StartSession(ctx, "s1", "task"); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := j.LogToolCall(ctx, ToolCallEntry{SessionID: "s1", ToolName: "bash"}); err != nil {
		t.Fatalf("LogToolCall: %v", err)
	}
	if err := j.LogFileChange(ctx, FileChangeEntry{SessionID: "s1", FilePath: "/tmp/x"}); err != nil {
		t.Fatalf("LogFileChange: %v", err)
	}
	if err := j.LogDecision(ctx, DecisionEntry{SessionID: "s1", Decision: "use Go"}); err != nil {
		t.Fatalf("LogDecision: %v", err)
	}
	if err := j.EndSession(ctx, "s1", "done"); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if err := j.DeleteSession(ctx, "s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	sj, err := j.GetJournal(ctx, "s1", 0)
	if err != nil {
		t.Fatalf("GetJournal: %v", err)
	}
	if sj != nil {
		t.Fatalf("expected nil journal, got %+v", sj)
	}

	// GetJournalEvents 应返回 ErrJournalNotAvailable
	events, err := j.GetJournalEvents(ctx, "s1", 0, time.Time{})
	if !errors.Is(err, ErrJournalNotAvailable) {
		t.Fatalf("GetJournalEvents: expected ErrJournalNotAvailable, got %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events, got %+v", events)
	}

	// GetJournalStats 应返回 ErrJournalNotAvailable
	stats, err := j.GetJournalStats(ctx, []string{"s1"})
	if !errors.Is(err, ErrJournalNotAvailable) {
		t.Fatalf("GetJournalStats: expected ErrJournalNotAvailable, got %v", err)
	}
	if stats != nil {
		t.Fatalf("expected nil stats, got %+v", stats)
	}
}

func TestNoopJournal_ImplementsInterface(t *testing.T) {
	// 编译期验证 NoopJournal 实现 Journal 接口
	var _ Journal = NoopJournal{}
	var _ Journal = (*NoopJournal)(nil)
}

func TestPGJournal_ImplementsInterface(t *testing.T) {
	// 编译期验证 PGJournal 实现 Journal 接口
	var _ Journal = (*PGJournal)(nil)
}

func TestToolCallEntry_ZeroTimestamp(t *testing.T) {
	e := ToolCallEntry{
		SessionID: "s1",
		ToolName:  "bash",
		Duration:  500 * time.Millisecond,
	}
	if !e.Timestamp.IsZero() {
		t.Fatal("expected zero timestamp")
	}
}
