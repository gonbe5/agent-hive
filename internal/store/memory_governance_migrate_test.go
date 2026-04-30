package store

import (
	"strings"
	"testing"
)

func TestPGInitSQLIncludesMemoryGovernanceIndexes(t *testing.T) {
	sql := strings.Join(strings.Fields(pgInitSQL), " ")
	required := []string{
		"CREATE INDEX IF NOT EXISTS idx_memories_governance_expires ON memories (((metadata->'governance'->>'expires_at')))",
		"CREATE INDEX IF NOT EXISTS idx_memories_governance_source ON memories (((metadata->'governance'->>'source')))",
	}

	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("pgInitSQL missing %q", needle)
		}
	}
}
