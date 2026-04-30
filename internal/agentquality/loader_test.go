package agentquality

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCases_AndValidate(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(cases), 7)

	required := 0
	for _, lc := range cases {
		require.NoError(t, ValidateCase(lc.Case), lc.Path)
		if lc.Case.Required {
			required++
		}
	}
	assert.GreaterOrEqual(t, required, 7)
}

func TestValidateCase_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		c       Case
		wantErr bool
	}{
		{name: "valid", c: Case{ID: "x", Name: "n", Route: "web", Input: "i", ExpectedStatus: StatusPass}, wantErr: false},
		{name: "missing id", c: Case{Name: "n", Route: "web", Input: "i", ExpectedStatus: StatusPass}, wantErr: true},
		{name: "invalid status", c: Case{ID: "x", Name: "n", Route: "web", Input: "i", ExpectedStatus: "bad"}, wantErr: true},
		{name: "exclusive tools", c: Case{ID: "x", Name: "n", Route: "web", Input: "i", ExpectedTools: []string{"grep"}, AllowedTools: []string{"bash"}, ExpectedStatus: StatusPass}, wantErr: true},
		{name: "safe needs user", c: Case{ID: "x", Name: "n", Route: "web", Input: "i", ExpectedStatus: StatusNeedsUser, Risk: "safe"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCase(tt.c)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
