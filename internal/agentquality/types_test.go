package agentquality

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualityEvent_JSONStable(t *testing.T) {
	ev := Event{
		Name:          EventAgentTurn,
		CaseID:        "aq01",
		SessionIDHash: "sha256:abc",
		Route:         "web",
		Prompt: PromptRef{
			Key:     "system/base",
			Version: "sha256:1111",
			Source:  "db",
		},
		FailureType: FailureTool,
		FinalStatus: StatusFail,
		Attributes:  map[string]any{"trace_id": "trace-1"},
	}

	b, err := json.Marshal(ev)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"name":"quality.agent_turn"`)
	assert.Contains(t, string(b), `"failure_type":"tool"`)
	assert.Contains(t, string(b), `"final_status":"fail"`)
}

func TestMetricLabels_AreLowCardinality(t *testing.T) {
	labels := MetricLabels(Event{
		Name:         EventToolDecision,
		Route:        "im",
		ToolDecision: ToolDecision{Actual: "grep", Decision: DecisionExpected},
		FailureType:  FailureNone,
		FinalStatus:  StatusPass,
	})

	assert.Equal(t, "im", labels["route"])
	assert.Equal(t, "grep", labels["tool_name"])
	assert.Equal(t, DecisionExpected, labels["decision"])
	assert.NotContains(t, labels, "session_id")
	assert.NotContains(t, labels, "user_id")
	assert.NotContains(t, labels, "trace_id")
}
