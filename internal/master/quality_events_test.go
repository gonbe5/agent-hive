package master

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	"github.com/chef-guo/agents-hive/internal/journal"
	"github.com/chef-guo/agents-hive/internal/observability"
)

func TestMaster_EnqueueLog_NilSafe(t *testing.T) {
	m := &Master{}
	assert.NotPanics(t, func() {
		m.enqueueLog(observability.LogEntry{Level: "info", Message: "x"})
	})
}

func TestMaster_SetObservabilityWriters_DoNotResetQueue(t *testing.T) {
	m := &Master{}
	m.SetLogWriter(&observability.NoopLogWriter{})
	ch := m.obsCh
	m.SetTracer(&observability.NoopTracer{})
	m.SetMetricsWriter(&observability.NoopMetricsWriter{})
	assert.True(t, ch == m.obsCh)
}

func TestEmitQualityEvent_EnqueuesMetricAndLog(t *testing.T) {
	m := &Master{obsCh: make(chan observabilityEntry, 4)}
	m.emitQualityEvent("trace", "span", "session-1", agentquality.Event{
		Name:        agentquality.EventToolDecision,
		Route:       "web",
		FailureType: agentquality.FailureNone,
		FinalStatus: agentquality.StatusPass,
		ToolDecision: agentquality.ToolDecision{
			Actual:   "grep",
			Decision: agentquality.DecisionExpected,
		},
	})

	first := <-m.obsCh
	second := <-m.obsCh
	require.NotNil(t, first.metric)
	require.NotNil(t, second.log)
	assert.Equal(t, "quality.tool_decision", first.metric.Name)
	assert.NotContains(t, first.metric.Labels, "session_id")
	assert.Equal(t, "session-1", second.log.SessionID)
}

func TestHashToolArgs_StableForJSONKeyOrder(t *testing.T) {
	a := json.RawMessage(`{"b":2,"a":1}`)
	b := json.RawMessage(`{"a":1,"b":2}`)
	assert.Equal(t, hashToolArgs(a), hashToolArgs(b))
	assert.NotEmpty(t, hashToolArgs(a))
}

func TestEmitQualityEvent_EnqueuesJournalDecision(t *testing.T) {
	m := &Master{
		journal:   journal.NoopJournal{},
		journalCh: make(chan journalEntry, 1),
	}
	m.emitQualityEvent("trace", "span", "session-1", agentquality.Event{
		Name:        agentquality.EventAgentTurn,
		Route:       "web",
		FailureType: agentquality.FailureTool,
		FinalStatus: agentquality.StatusFail,
	})

	got := <-m.journalCh
	require.NotNil(t, got.decision)
	assert.Equal(t, "quality.agent_turn", got.decision.Decision)
	assert.Contains(t, got.decision.Reason, `"failure_type":"tool"`)
}
