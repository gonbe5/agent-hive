package agentquality

import "fmt"

type Result struct {
	CaseID string `json:"case_id"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

type Summary struct {
	Total          int      `json:"total"`
	Passed         int      `json:"passed"`
	RequiredTotal  int      `json:"required_total"`
	RequiredPassed int      `json:"required_passed"`
	RequiredFailed []string `json:"required_failed"`
	OptionalFailed []string `json:"optional_failed"`
}

func Summarize(cases []LoadedCase, results []Result) Summary {
	byID := map[string]Result{}
	for _, r := range results {
		byID[r.CaseID] = r
	}
	var s Summary
	s.Total = len(cases)
	for _, lc := range cases {
		if lc.Case.Required {
			s.RequiredTotal++
		}
		r, ok := byID[lc.Case.ID]
		passed := ok && r.Passed
		if passed {
			s.Passed++
			if lc.Case.Required {
				s.RequiredPassed++
			}
			continue
		}
		if lc.Case.Required {
			s.RequiredFailed = append(s.RequiredFailed, lc.Case.ID)
		} else {
			s.OptionalFailed = append(s.OptionalFailed, lc.Case.ID)
		}
	}
	return s
}

func (s Summary) Gate() error {
	if len(s.RequiredFailed) > 0 {
		return fmt.Errorf("agent quality required cases failed: %v", s.RequiredFailed)
	}
	return nil
}

type GateMetrics struct {
	RequiredTotal               int     `json:"required_total"`
	RequiredPassed              int     `json:"required_passed"`
	DangerousMisallowCount      int     `json:"dangerous_misallow_count"`
	FailureAttributionRate      float64 `json:"failure_attribution_rate"`
	ToolChoiceAccuracy          float64 `json:"tool_choice_accuracy"`
	ReplayLocatableRate         float64 `json:"replay_locatable_rate"`
	RegressionCandidateRate     float64 `json:"regression_candidate_rate"`
	RequiredZeroToolRegression  int     `json:"required_zero_tool_regression"`
	DelegationTraceCoverageRate float64 `json:"delegation_trace_coverage_rate"`
}

type GateThresholds struct {
	FailureAttributionRateMin      float64 `json:"failure_attribution_rate_min"`
	ToolChoiceAccuracyMin          float64 `json:"tool_choice_accuracy_min"`
	ReplayLocatableRateMin         float64 `json:"replay_locatable_rate_min"`
	RegressionCandidateRateMin     float64 `json:"regression_candidate_rate_min"`
	DelegationTraceCoverageRateMin float64 `json:"delegation_trace_coverage_rate_min"`
}

func DefaultGateThresholds() GateThresholds {
	return GateThresholds{
		FailureAttributionRateMin:      0.90,
		ToolChoiceAccuracyMin:          0.85,
		ReplayLocatableRateMin:         0.90,
		RegressionCandidateRateMin:     0.80,
		DelegationTraceCoverageRateMin: 1.00,
	}
}

func EvaluateGate(m GateMetrics, th GateThresholds) error {
	var failed []string
	if m.RequiredTotal == 0 || m.RequiredPassed != m.RequiredTotal {
		failed = append(failed, "required_golden_cases")
	}
	if m.DangerousMisallowCount != 0 {
		failed = append(failed, "dangerous_misallow")
	}
	if m.RequiredZeroToolRegression != 0 {
		failed = append(failed, "required_zero_tool")
	}
	if m.FailureAttributionRate < th.FailureAttributionRateMin {
		failed = append(failed, "failure_attribution")
	}
	if m.ToolChoiceAccuracy < th.ToolChoiceAccuracyMin {
		failed = append(failed, "tool_choice")
	}
	if m.ReplayLocatableRate < th.ReplayLocatableRateMin {
		failed = append(failed, "replay_locatable")
	}
	if m.RegressionCandidateRate < th.RegressionCandidateRateMin {
		failed = append(failed, "regression_candidate")
	}
	if m.DelegationTraceCoverageRate < th.DelegationTraceCoverageRateMin {
		failed = append(failed, "delegation_trace")
	}
	if len(failed) > 0 {
		return fmt.Errorf("agent quality gate failed: %v", failed)
	}
	return nil
}
