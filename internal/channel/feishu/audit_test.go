package feishu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONLAuditSink_ScrubsPlainTenantKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink := NewJSONLAuditSink(path)

	err := sink.Write(context.Background(), AuditRecord{
		Action:    "tool.call",
		Outcome:   "ok",
		TenantKey: "tenant-a",
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var record AuditRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if record.TenantKey != "" {
		t.Fatalf("TenantKey = %q, want empty scrubbed", record.TenantKey)
	}
	if record.TenantHash != "tk_80a707af" {
		t.Fatalf("TenantHash = %q, want tk_80a707af", record.TenantHash)
	}
}

func TestJSONLAuditSink_ReadRecentFiltersByChatAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	sink := NewJSONLAuditSink(path)

	records := []AuditRecord{
		{TS: time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC), Action: "push.api", Outcome: "ok", TenantKey: "tenant-a", Target: map[string]any{"chat_id": "oc-1"}},
		{TS: time.Date(2026, 4, 26, 10, 1, 0, 0, time.UTC), Action: "command.execute", Outcome: "ok", TenantKey: "tenant-a", Target: map[string]any{"chat_id": "oc-2"}},
		{TS: time.Date(2026, 4, 26, 10, 2, 0, 0, time.UTC), Action: "command.execute", Outcome: "ok", TenantKey: "tenant-a", Target: map[string]any{"chat_id": "oc-1"}},
	}
	for _, record := range records {
		if err := sink.Write(context.Background(), record); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	got, err := sink.ReadRecent(context.Background(), AuditQuery{
		Platform:  "feishu",
		TenantKey: "tenant-a",
		ChatID:    "oc-1",
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("ReadRecent() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Action != "command.execute" {
		t.Fatalf("got[0].Action = %q, want command.execute", got[0].Action)
	}
	if got[1].Action != "push.api" {
		t.Fatalf("got[1].Action = %q, want push.api", got[1].Action)
	}
}
