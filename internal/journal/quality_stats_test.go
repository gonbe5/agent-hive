package journal

import "testing"

func TestQualityDecisionStatsFromReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   qualityDecisionStats
	}{
		{
			name:   "non quality decision",
			reason: `{"name":"other.event"}`,
			want:   qualityDecisionStats{},
		},
		{
			name:   "context failure counts as quality error",
			reason: `{"name":"quality.context_build","failure_type":"context","final_status":"fail","context_build":{"contamination_check":"filtered"}}`,
			want:   qualityDecisionStats{QualityError: true, ContextPollution: true},
		},
		{
			name:   "dangerous permission needs user",
			reason: `{"name":"quality.permission_decision","failure_type":"permission","final_status":"needs_user","tool_decision":{"actual":"bash"}}`,
			want:   qualityDecisionStats{QualityError: true, Dangerous: true},
		},
		{
			name:   "delegation counts subagent",
			reason: `{"name":"quality.delegation","failure_type":"none","final_status":"pass","delegation":{"agent_type":"subagent","agent_id":"a1"}}`,
			want:   qualityDecisionStats{Delegation: true},
		},
		{
			name:   "acp transport failure",
			reason: `{"name":"quality.delegation","failure_type":"runtime","final_status":"fail","delegation":{"agent_type":"acp","stop_reason":"disconnect"}}`,
			want:   qualityDecisionStats{QualityError: true, Delegation: true, ACP: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := qualityDecisionStatsFromReason(tt.reason)
			if got != tt.want {
				t.Fatalf("qualityDecisionStatsFromReason() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
