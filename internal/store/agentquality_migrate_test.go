package store

import (
	"strings"
	"testing"
)

func TestPGInitSQLIncludesAgentQualityCandidates(t *testing.T) {
	sql := strings.Join(strings.Fields(pgInitSQL), " ")
	required := []string{
		"CREATE TABLE IF NOT EXISTS agentquality_candidates",
		"id TEXT PRIMARY KEY",
		"case_json JSONB NOT NULL DEFAULT '{}'",
		"source_event JSONB NOT NULL DEFAULT '{}'",
		"suggestions_json JSONB NOT NULL DEFAULT '[]'",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_agentquality_candidates_fingerprint",
		"WHERE status IN ('new', 'reviewing', 'approved')",
		"CREATE INDEX IF NOT EXISTS idx_agentquality_candidates_status_created",
		"CREATE INDEX IF NOT EXISTS idx_agentquality_candidates_session",
	}

	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("pgInitSQL missing %q", needle)
		}
	}
}
