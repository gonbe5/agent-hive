package agentquality

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryGate_BlocksRequiredFailure(t *testing.T) {
	cases := []LoadedCase{
		{Case: Case{ID: "required", Required: true}},
		{Case: Case{ID: "optional", Required: false}},
	}
	s := Summarize(cases, []Result{{CaseID: "optional", Passed: true}})

	require.Error(t, s.Gate())
	assert.Equal(t, []string{"required"}, s.RequiredFailed)
	assert.Equal(t, 1, s.RequiredTotal)
	assert.Equal(t, 0, s.RequiredPassed)
}

func TestSummaryGate_AllRequiredPass(t *testing.T) {
	cases := []LoadedCase{{Case: Case{ID: "required", Required: true}}}
	s := Summarize(cases, []Result{{CaseID: "required", Passed: true}})

	require.NoError(t, s.Gate())
	assert.Equal(t, 1, s.Passed)
	assert.Equal(t, 1, s.RequiredPassed)
}
