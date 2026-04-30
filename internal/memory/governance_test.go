package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGovernanceInjectable(t *testing.T) {
	now := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		governance    Governance
		minConfidence float64
		want          bool
	}{
		{
			name:          "低置信度不可注入",
			governance:    Governance{Confidence: 0.2},
			minConfidence: 0.5,
			want:          false,
		},
		{
			name:          "过期记忆不可注入",
			governance:    Governance{ExpiresAt: now.Add(-time.Hour)},
			minConfidence: 0.5,
			want:          false,
		},
		{
			name:          "高置信且未过期可注入",
			governance:    Governance{Confidence: 0.9, ExpiresAt: now.Add(time.Hour)},
			minConfidence: 0.5,
			want:          true,
		},
		{
			name:          "未设置治理字段默认兼容可注入",
			governance:    Governance{},
			minConfidence: 0.5,
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.governance.Injectable(now, tt.minConfidence); got != tt.want {
				t.Fatalf("Injectable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeGovernance(t *testing.T) {
	raw := json.RawMessage(`{"existing":true}`)
	expiresAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	encoded := EncodeGovernance(raw, Governance{
		Source:       "summary",
		Evidence:     "用户明确说偏好 Go",
		Confidence:   0.8,
		ExpiresAt:    expiresAt,
		ExtractedBy:   "compaction",
		SourceMessage: "msg-1",
	})

	got := DecodeGovernance(encoded)
	if got.Source != "summary" {
		t.Fatalf("Source = %q, want summary", got.Source)
	}
	if got.Confidence != 0.8 {
		t.Fatalf("Confidence = %v, want 0.8", got.Confidence)
	}
	if !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %v, want %v", got.ExpiresAt, expiresAt)
	}
	if !json.Valid(encoded) {
		t.Fatalf("encoded metadata is not valid JSON: %s", string(encoded))
	}
	if !containsJSONFragment(encoded, `"existing":true`) {
		t.Fatalf("encoded metadata lost existing fields: %s", string(encoded))
	}
}

func containsJSONFragment(raw json.RawMessage, fragment string) bool {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	b, _ := json.Marshal(m)
	return stringContains(string(b), fragment)
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
