package acpserver

import (
	"context"
	"errors"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

type acpMetricRecorder struct {
	metrics []observability.Metric
	ch      chan observability.Metric
}

func (r *acpMetricRecorder) Record(_ context.Context, metric observability.Metric) error {
	r.metrics = append(r.metrics, metric)
	if r.ch != nil {
		r.ch <- metric
	}
	return nil
}

func newACPQualityMaster(t *testing.T) (*master.Master, *acpMetricRecorder, context.CancelFunc) {
	t.Helper()
	logger := zaptest.NewLogger(t)
	m := master.NewMaster(
		master.Config{},
		config.HITLConfig{Enabled: false},
		subagent.NewRegistry(logger),
		skills.NewRegistry(logger),
		store.NewMemoryStore(),
		logger,
	)
	rec := &acpMetricRecorder{ch: make(chan observability.Metric, 8)}
	m.SetMetricsWriter(rec)
	ctx, cancel := context.WithCancel(context.Background())
	m.StartObsWorker(ctx)
	return m, rec, cancel
}

func waitACPMetrics(t *testing.T, rec *acpMetricRecorder, count int) []observability.Metric {
	t.Helper()
	got := make([]observability.Metric, 0, count)
	timeout := time.After(2 * time.Second)
	for len(got) < count {
		select {
		case metric := <-rec.ch:
			got = append(got, metric)
		case <-timeout:
			t.Fatalf("等待 ACP quality metric 超时，已收到 %d/%d", len(got), count)
		}
	}
	return got
}

func TestClawAgentNewSessionAndCancelEmitDelegationQualityEvents(t *testing.T) {
	m, rec, stop := newACPQualityMaster(t)
	agent := NewClawAgent(m, defaultTestACPConfig(), zap.NewNop(), nil, nil)

	resp, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	require.NoError(t, err)
	sid := string(resp.SessionId)
	agent.Cancel(context.Background(), acp.CancelNotification{SessionId: resp.SessionId})

	metrics := waitACPMetrics(t, rec, 2)
	stop()

	assert.Equal(t, "quality.delegation", metrics[0].Name)
	assert.Equal(t, "acp", metrics[0].Labels["route"])
	assert.Equal(t, "pass", metrics[0].Labels["status"])
	assert.Equal(t, "quality.delegation", metrics[1].Name)
	assert.Equal(t, "acp", metrics[1].Labels["route"])
	assert.Equal(t, "fail", metrics[1].Labels["status"])
	assert.Equal(t, "runtime", metrics[1].Labels["failure_type"])
	assert.NotEmpty(t, sid)
}

func TestClawAgentPromptErrorEmitsDelegationQualityEvent(t *testing.T) {
	m, rec, stop := newACPQualityMaster(t)
	agent := NewClawAgent(m, defaultTestACPConfig(), zap.NewNop(), nil, nil)

	_, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: "acp-missing",
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	})
	require.Error(t, err)

	metrics := waitACPMetrics(t, rec, 1)
	stop()

	assert.Equal(t, "quality.delegation", metrics[0].Name)
	assert.Equal(t, "acp", metrics[0].Labels["route"])
	assert.Equal(t, "fail", metrics[0].Labels["status"])
	assert.Equal(t, "runtime", metrics[0].Labels["failure_type"])
	assert.Equal(t, "session_not_found", metrics[0].Labels["stop_reason"])
}

func TestClawAgentPromptCancelledEmitsDelegationQualityEvent(t *testing.T) {
	m, rec, stop := newACPQualityMaster(t)
	agent := NewClawAgent(m, defaultTestACPConfig(), zap.NewNop(), nil, nil)

	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	require.NoError(t, err)
	_ = waitACPMetrics(t, rec, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := agent.Prompt(ctx, acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	})
	require.NoError(t, err)
	assert.Equal(t, acp.StopReasonCancelled, resp.StopReason)

	metrics := waitACPMetrics(t, rec, 1)
	stop()

	assert.Equal(t, "quality.delegation", metrics[0].Name)
	assert.Equal(t, "acp", metrics[0].Labels["route"])
	assert.Equal(t, "fail", metrics[0].Labels["status"])
	assert.Equal(t, "runtime", metrics[0].Labels["failure_type"])
	assert.Equal(t, "cancelled", metrics[0].Labels["stop_reason"])
}

func TestClawAgentPromptEmptyEmitsDelegationQualityEvent(t *testing.T) {
	m, rec, stop := newACPQualityMaster(t)
	agent := NewClawAgent(m, defaultTestACPConfig(), zap.NewNop(), nil, nil)

	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	require.NoError(t, err)
	_ = waitACPMetrics(t, rec, 1)

	resp, err := agent.Prompt(context.Background(), acp.PromptRequest{SessionId: session.SessionId})
	require.NoError(t, err)
	assert.Equal(t, acp.StopReasonEndTurn, resp.StopReason)

	metrics := waitACPMetrics(t, rec, 1)
	stop()

	assert.Equal(t, "quality.delegation", metrics[0].Name)
	assert.Equal(t, "acp", metrics[0].Labels["route"])
	assert.Equal(t, "pass", metrics[0].Labels["status"])
	assert.Equal(t, "end_turn", metrics[0].Labels["stop_reason"])
}

type fakeACPPermissionRequester struct {
	resp acp.RequestPermissionResponse
	err  error
}

func (f fakeACPPermissionRequester) RequestPermission(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return f.resp, f.err
}

func TestCreateACPPermissionFnRecordsDecisionWithoutChangingSemantics(t *testing.T) {
	tests := []struct {
		name         string
		resp         acp.RequestPermissionResponse
		err          error
		wantGranted  bool
		wantRemember bool
		wantDecision string
	}{
		{
			name:         "allow once",
			resp:         acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("allow_once")},
			wantGranted:  true,
			wantDecision: "allow_once",
		},
		{
			name:         "allow session",
			resp:         acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("allow_session")},
			wantGranted:  true,
			wantRemember: true,
			wantDecision: "allow_session",
		},
		{
			name:         "reject",
			resp:         acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("reject")},
			wantDecision: "reject",
		},
		{
			name:         "cancelled",
			resp:         acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()},
			wantDecision: "cancelled",
		},
		{
			name:         "request error",
			err:          errors.New("client gone"),
			wantDecision: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &recordingPermissionRecorder{}
			fn := createACPPermissionFnWithRequester(
				fakeACPPermissionRequester{resp: tt.resp, err: tt.err},
				"acp-test",
				zap.NewNop(),
				recorder,
			)

			got, err := fn(context.Background(), skills.PermissionRequest{ToolName: "bash", Description: "danger"})
			require.NoError(t, err)
			assert.Equal(t, tt.wantGranted, got.Granted)
			assert.Equal(t, tt.wantRemember, got.Remember)

			records := recorder.snapshot()
			require.Len(t, records, 1)
			assert.Equal(t, "acp-test", records[0].sessionID)
			assert.Equal(t, "bash", records[0].req.ToolName)
			assert.Equal(t, tt.wantDecision, records[0].decision)
			assert.Equal(t, tt.wantGranted, records[0].granted)
			assert.Equal(t, tt.wantRemember, records[0].remember)
		})
	}
}

func TestACPPermissionBridgeEmitsQualityPermissionDecision(t *testing.T) {
	m, rec, stop := newACPQualityMaster(t)
	fn := createACPPermissionFnWithRequester(
		fakeACPPermissionRequester{resp: acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected("allow_once")}},
		"acp-permission",
		zap.NewNop(),
		m,
	)

	got, err := fn(context.Background(), skills.PermissionRequest{ToolName: "bash", Description: "danger"})
	require.NoError(t, err)
	assert.True(t, got.Granted)

	metrics := waitACPMetrics(t, rec, 1)
	stop()

	assert.Equal(t, "quality.permission_decision", metrics[0].Name)
	assert.Equal(t, "acp", metrics[0].Labels["route"])
	assert.Equal(t, "permission", metrics[0].Labels["failure_type"])
	assert.Equal(t, "pass", metrics[0].Labels["status"])
	assert.Equal(t, "bash", metrics[0].Labels["tool_name"])
	assert.Equal(t, "allow_once", string(metrics[0].Labels["decision"].(agentquality.Decision)))
}

type permissionRecord struct {
	sessionID string
	req       skills.PermissionRequest
	decision  string
	granted   bool
	remember  bool
	errText   string
}

type recordingPermissionRecorder struct {
	records []permissionRecord
}

func (r *recordingPermissionRecorder) RecordACPPermissionDecision(_ context.Context, sessionID string, req skills.PermissionRequest, decision string, granted bool, remember bool, errText string) {
	r.records = append(r.records, permissionRecord{
		sessionID: sessionID,
		req:       req,
		decision:  decision,
		granted:   granted,
		remember:  remember,
		errText:   errText,
	})
}

func (r *recordingPermissionRecorder) snapshot() []permissionRecord {
	out := make([]permissionRecord, len(r.records))
	copy(out, r.records)
	return out
}
