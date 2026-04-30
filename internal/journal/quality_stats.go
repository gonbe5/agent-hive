package journal

import "encoding/json"

type qualityDecisionStats struct {
	QualityError     bool
	Dangerous        bool
	Delegation       bool
	ACP              bool
	ContextPollution bool
}

type qualityEventLite struct {
	Name         string `json:"name"`
	FailureType  string `json:"failure_type"`
	FinalStatus  string `json:"final_status"`
	ContextBuild struct {
		ContaminationCheck string `json:"contamination_check"`
	} `json:"context_build"`
	Delegation struct {
		AgentID    string `json:"agent_id"`
		AgentType  string `json:"agent_type"`
		StopReason string `json:"stop_reason"`
	} `json:"delegation"`
}

func qualityDecisionStatsFromReason(reason string) qualityDecisionStats {
	var ev qualityEventLite
	if err := json.Unmarshal([]byte(reason), &ev); err != nil {
		return qualityDecisionStats{}
	}
	if len(ev.Name) < len("quality.") || ev.Name[:len("quality.")] != "quality." {
		return qualityDecisionStats{}
	}

	stats := qualityDecisionStats{}
	if ev.FailureType != "" && ev.FailureType != "none" {
		stats.QualityError = true
	}
	switch ev.FinalStatus {
	case "fail", "blocked", "needs_user":
		stats.QualityError = true
	}
	if ev.Name == "quality.permission_decision" {
		stats.Dangerous = true
	}
	if ev.Name == "quality.delegation" || ev.Delegation.AgentID != "" || ev.Delegation.AgentType != "" {
		stats.Delegation = true
	}
	if ev.Delegation.AgentType == "acp" {
		stats.ACP = true
	}
	if ev.ContextBuild.ContaminationCheck == "filtered" {
		stats.ContextPollution = true
	}
	return stats
}
