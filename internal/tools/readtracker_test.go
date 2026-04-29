package tools

import (
	"fmt"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func TestReadTracker_RecordAndCheck(t *testing.T) {
	tracker := NewReadTracker(5 * time.Minute)

	// 未读取的文件应该失败
	err := tracker.CheckRead("/tmp/test.txt")
	if err == nil {
		t.Error("expected error for unread file")
	}

	// 记录读取
	tracker.RecordRead("/tmp/test.txt")

	// 现在应该通过
	err = tracker.CheckRead("/tmp/test.txt")
	if err != nil {
		t.Errorf("expected no error after read, got: %v", err)
	}
}

func TestReadTracker_Stale(t *testing.T) {
	tracker := NewReadTracker(100 * time.Millisecond)

	tracker.RecordRead("/tmp/stale.txt")

	// 立即检查应该通过
	err := tracker.CheckRead("/tmp/stale.txt")
	if err != nil {
		t.Errorf("immediate check should pass: %v", err)
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 现在应该失败（过期）
	err = tracker.CheckRead("/tmp/stale.txt")
	if err == nil {
		t.Error("expected error for stale read")
	}

	if e, ok := err.(*errs.Error); ok {
		if e.Code != errs.CodeInvalidInput {
			t.Errorf("expected CodeInvalidInput, got %d", e.Code)
		}
	}
}

func TestReadTracker_Clear(t *testing.T) {
	tracker := NewReadTracker(5 * time.Minute)

	tracker.RecordRead("/tmp/a.txt")
	tracker.RecordRead("/tmp/b.txt")

	reads := tracker.GetReads()
	if len(reads) != 2 {
		t.Errorf("expected 2 reads, got %d", len(reads))
	}

	tracker.Clear()

	reads = tracker.GetReads()
	if len(reads) != 0 {
		t.Errorf("expected 0 reads after clear, got %d", len(reads))
	}
}

func TestReadTracker_GetReads(t *testing.T) {
	tracker := NewReadTracker(5 * time.Minute)

	tracker.RecordRead("/tmp/x.txt")
	tracker.RecordRead("/tmp/y.txt")

	reads := tracker.GetReads()

	if len(reads) != 2 {
		t.Errorf("expected 2 reads, got %d", len(reads))
	}

	if _, ok := reads["/tmp/x.txt"]; !ok {
		t.Error("expected /tmp/x.txt in reads")
	}

	if _, ok := reads["/tmp/y.txt"]; !ok {
		t.Error("expected /tmp/y.txt in reads")
	}
}

func TestReadTracker_RemoveRead(t *testing.T) {
	tracker := NewReadTracker(5 * time.Minute)

	tracker.RecordRead("/tmp/remove.txt")

	err := tracker.CheckRead("/tmp/remove.txt")
	if err != nil {
		t.Errorf("check before remove should pass: %v", err)
	}

	tracker.RemoveRead("/tmp/remove.txt")

	err = tracker.CheckRead("/tmp/remove.txt")
	if err == nil {
		t.Error("check after remove should fail")
	}
}

func TestReadTracker_DefaultStaleTime(t *testing.T) {
	tracker := NewReadTracker(0) // 应该使用默认 5 分钟

	tracker.RecordRead("/tmp/default.txt")

	// 立即检查应该通过
	err := tracker.CheckRead("/tmp/default.txt")
	if err != nil {
		t.Errorf("check with default stale time should pass: %v", err)
	}
}

func TestReadTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewReadTracker(5 * time.Minute)

	// 并发写入
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			tracker.RecordRead(fmt.Sprintf("/tmp/file%d.txt", n))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	reads := tracker.GetReads()
	if len(reads) != 10 {
		t.Errorf("expected 10 reads, got %d", len(reads))
	}
}
