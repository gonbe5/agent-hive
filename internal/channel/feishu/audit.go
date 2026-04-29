package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/channel"
)

type AuditRecord struct {
	TS         time.Time       `json:"ts"`
	Platform   string          `json:"platform"`
	Action     string          `json:"action"`
	Tool       string          `json:"tool,omitempty"`
	Outcome    string          `json:"outcome"`
	DurationMS int64           `json:"duration_ms,omitempty"`
	TenantKey  string          `json:"tenant_key,omitempty"`
	TenantHash string          `json:"tenant_key_hash,omitempty"`
	Actor      map[string]any  `json:"actor,omitempty"`
	Target     map[string]any  `json:"target,omitempty"`
	Error      string          `json:"error,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type AuditSink interface {
	Write(context.Context, any) error
}

type AuditQuery struct {
	Platform  string
	TenantKey string
	ChatID    string
	Limit     int
}

type AuditReader interface {
	ReadRecent(context.Context, AuditQuery) ([]AuditRecord, error)
}

type AuditStore interface {
	AuditSink
	AuditReader
}

type JSONLAuditSink struct {
	path string
	mu   sync.Mutex
}

func NewJSONLAuditSink(path string) *JSONLAuditSink {
	if path == "" {
		path = filepath.Join(os.TempDir(), "agents-hive-feishu-audit.jsonl")
	}
	return &JSONLAuditSink{path: path}
}

func (s *JSONLAuditSink) Write(_ context.Context, record any) error {
	if s == nil {
		return nil
	}
	if typed, ok := record.(AuditRecord); ok {
		if typed.TS.IsZero() {
			typed.TS = time.Now().UTC()
		}
		if typed.Platform == "" {
			typed.Platform = "feishu"
		}
		if typed.TenantHash == "" {
			typed.TenantHash = channel.TenantKeyHashLabel(typed.TenantKey)
		}
		typed.TenantKey = ""
		record = typed
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(record)
}

func (s *JSONLAuditSink) ReadRecent(_ context.Context, query AuditQuery) ([]AuditRecord, error) {
	if s == nil {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := bytes.Split(data, []byte{'\n'})
	records := make([]AuditRecord, 0, limit)
	for i := len(lines) - 1; i >= 0 && len(records) < limit; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var record AuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if !auditRecordMatches(record, query) {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func auditRecordMatches(record AuditRecord, query AuditQuery) bool {
	if query.Platform != "" && record.Platform != query.Platform {
		return false
	}
	if query.TenantKey != "" && record.TenantHash != channel.TenantKeyHashLabel(query.TenantKey) {
		return false
	}
	if query.ChatID != "" && auditRecordChatID(record) != query.ChatID {
		return false
	}
	return true
}

func auditRecordChatID(record AuditRecord) string {
	if record.Target == nil {
		return ""
	}
	if v, ok := record.Target["chat_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func formatAuditRecords(records []AuditRecord) string {
	if len(records) == 0 {
		return "最近没有审计记录"
	}
	var b strings.Builder
	for i, record := range records {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		if !record.TS.IsZero() {
			b.WriteString(record.TS.UTC().Format(time.RFC3339))
			b.WriteByte(' ')
		}
		action := strings.TrimSpace(record.Action)
		if action == "" {
			action = "unknown"
		}
		b.WriteString(action)
		outcome := strings.TrimSpace(record.Outcome)
		if outcome == "" {
			outcome = "unknown"
		}
		b.WriteString(" [")
		b.WriteString(outcome)
		b.WriteString("]")
		if actor := formatAuditActor(record.Actor); actor != "" {
			b.WriteString(" actor=")
			b.WriteString(actor)
		}
		if target := formatAuditTarget(record.Target); target != "" {
			b.WriteString(" target=")
			b.WriteString(target)
		}
		if record.Error != "" {
			b.WriteString(" err=")
			b.WriteString(record.Error)
		}
	}
	return b.String()
}

func formatAuditActor(actor map[string]any) string {
	if len(actor) == 0 {
		return ""
	}
	if senderID, ok := actor["sender_id"].(string); ok && senderID != "" {
		return senderID
	}
	if userID, ok := actor["user_id"].(string); ok && userID != "" {
		return userID
	}
	if typ, ok := actor["type"].(string); ok && typ != "" {
		return typ
	}
	return ""
}

func formatAuditTarget(target map[string]any) string {
	if len(target) == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if chatID, ok := target["chat_id"].(string); ok && chatID != "" {
		parts = append(parts, "chat="+chatID)
	}
	if command, ok := target["command"].(string); ok && command != "" {
		parts = append(parts, "cmd="+command)
	}
	if openID, ok := target["open_id"].(string); ok && openID != "" {
		parts = append(parts, "open="+SafeSenderID(openID))
	}
	if msgType, ok := target["msg_type"].(string); ok && msgType != "" {
		parts = append(parts, "msg_type="+msgType)
	}
	return strings.Join(parts, ",")
}

func AuditActorFromContext(ctx context.Context) map[string]any {
	if user := auth.UserFrom(ctx); user != nil {
		return map[string]any{
			"type":    "user",
			"user_id": user.ID,
			"role":    user.Role,
		}
	}
	return map[string]any{"type": "system"}
}
