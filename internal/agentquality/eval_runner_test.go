package agentquality

import (
	"path/filepath"
	"testing"
)

func TestStaticEvalRunnerBuildsGateableSummaryFromFixtures(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata"))
	if err != nil {
		t.Fatalf("load cases: %v", err)
	}

	input := StaticEvalSummary(cases)
	metrics := ComputeGateMetrics(GateInput{
		Cases:              cases,
		Results:            input.Results,
		Events:             input.Events,
		EventsByCase:       input.EventsByCase,
		CandidateByCaseID:  input.CandidateByCaseID,
		ToolActualByCaseID: input.ToolActualByCaseID,
		ReplayRefByCaseID:  input.ReplayRefByCaseID,
	})

	if len(input.Results) != len(cases) {
		t.Fatalf("results count = %d, want %d", len(input.Results), len(cases))
	}
	if err := EvaluateGate(metrics, DefaultGateThresholds()); err != nil {
		t.Fatalf("static eval summary must pass gate: %v", err)
	}
}
