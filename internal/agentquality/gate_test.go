package agentquality

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateGate_AllPass(t *testing.T) {
	err := EvaluateGate(GateMetrics{
		RequiredTotal:               7,
		RequiredPassed:              7,
		DangerousMisallowCount:      0,
		FailureAttributionRate:      0.95,
		ToolChoiceAccuracy:          0.90,
		ReplayLocatableRate:         0.92,
		RegressionCandidateRate:     0.85,
		RequiredZeroToolRegression:  0,
		DelegationTraceCoverageRate: 1.0,
	}, DefaultGateThresholds())
	require.NoError(t, err)
}

func TestEvaluateGate_BlocksAnyHardFailure(t *testing.T) {
	err := EvaluateGate(GateMetrics{
		RequiredTotal:               7,
		RequiredPassed:              6,
		DangerousMisallowCount:      1,
		FailureAttributionRate:      0.95,
		ToolChoiceAccuracy:          0.90,
		ReplayLocatableRate:         0.92,
		RegressionCandidateRate:     0.85,
		DelegationTraceCoverageRate: 1.0,
	}, DefaultGateThresholds())
	require.Error(t, err)
}
