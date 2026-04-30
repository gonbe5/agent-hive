package master

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/agentquality"
	"github.com/chef-guo/agents-hive/internal/journal"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/toolctx"
	"github.com/chef-guo/agents-hive/internal/tools"
)

func qualitySessionHash(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(sessionID))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func routeFromSession(session *SessionState) string {
	if session == nil {
		return "unknown"
	}
	return routeFromSessionID(session.ID)
}

func routeFromSessionID(sessionID string) string {
	if sessionID == "" {
		return "unknown"
	}
	if strings.HasPrefix(sessionID, "im-") {
		return "im"
	}
	if strings.HasPrefix(sessionID, "acp-") || strings.HasPrefix(sessionID, "acp_") {
		return "acp"
	}
	return "web"
}

func hashToolArgs(args json.RawMessage) string {
	raw := []byte(args)
	var normalized any
	if json.Unmarshal(args, &normalized) == nil {
		if b, err := json.Marshal(normalized); err == nil {
			raw = b
		}
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func (m *Master) emitQualityEvent(traceID, spanID, sessionID string, ev agentquality.Event) {
	if ev.Ts.IsZero() {
		ev.Ts = time.Now()
	}
	if ev.SessionIDHash == "" {
		ev.SessionIDHash = qualitySessionHash(sessionID)
	}
	raw, _ := json.Marshal(ev)
	labels := agentquality.MetricLabels(ev)
	if ev.Name == agentquality.EventDelegation && ev.Delegation.StopReason != "" {
		labels["stop_reason"] = ev.Delegation.StopReason
	}
	m.enqueueMetric(observability.Metric{
		Name:   string(ev.Name),
		Value:  1,
		Labels: labels,
		Ts:     ev.Ts,
	})
	m.enqueueLog(observability.LogEntry{
		Level:     "info",
		Message:   string(ev.Name),
		TraceID:   traceID,
		SpanID:    spanID,
		SessionID: sessionID,
		Attributes: map[string]any{
			"quality_event": json.RawMessage(raw),
		},
		Ts: ev.Ts,
	})
	m.enqueueQualityJournalDecision(sessionID, ev, raw)
}

func (m *Master) enqueueQualityJournalDecision(sessionID string, ev agentquality.Event, raw []byte) {
	if m.journal == nil || m.journalCh == nil || sessionID == "" {
		return
	}
	entry := journal.DecisionEntry{
		SessionID: sessionID,
		Decision:  string(ev.Name),
		Reason:    string(raw),
		AgentID:   "quality",
		Timestamp: ev.Ts,
	}
	select {
	case m.journalCh <- journalEntry{decision: &entry}:
	default:
	}
}

func (m *Master) RecordDelegation(ctx context.Context, ev tools.DelegationEvent) {
	sessionID := toolctx.GetSessionID(ctx)
	if ev.SessionID != "" {
		sessionID = ev.SessionID
	}

	status := agentquality.StatusPass
	failureType := agentquality.FailureNone
	if ev.Status == "failed" {
		status = agentquality.StatusFail
		failureType = agentquality.FailureRuntime
	}
	if ev.FailureType != "" {
		failureType = agentquality.FailureType(ev.FailureType)
	}

	attrs := map[string]any{}
	if ev.Error != "" {
		attrs["error"] = ev.Error
	}
	if ev.StopReason != "" {
		attrs["stop_reason"] = ev.StopReason
	}

	m.emitQualityEvent("", "", sessionID, agentquality.Event{
		Name:        agentquality.EventDelegation,
		Route:       routeFromSessionID(sessionID),
		FailureType: failureType,
		FinalStatus: status,
		Delegation: agentquality.Delegation{
			ParentTraceID: ev.ParentTraceID,
			ChildTraceID:  ev.ChildTraceID,
			AgentID:       ev.AgentID,
			AgentType:     ev.AgentType,
			GroupID:       ev.GroupID,
			ToolWhitelist: append([]string(nil), ev.ToolWhitelist...),
			SpawnDepth:    ev.SpawnDepth,
			MaxTurns:      ev.MaxTurns,
			StopReason:    ev.StopReason,
		},
		Attributes: attrs,
	})
}

func (m *Master) RecordACPPermissionDecision(ctx context.Context, sessionID string, req skills.PermissionRequest, decision string, granted bool, remember bool, errText string) {
	status := agentquality.StatusBlocked
	if granted {
		status = agentquality.StatusPass
	} else if decision == "cancelled" {
		status = agentquality.StatusNeedsUser
	}
	attrs := map[string]any{
		"tool_name": req.ToolName,
		"decision":  decision,
		"remember":  remember,
		"bridge":    "acp",
	}
	if errText != "" {
		attrs["error"] = errText
	}
	m.emitQualityEvent("", "", sessionID, agentquality.Event{
		Name:        agentquality.EventPermissionDecision,
		Route:       routeFromSessionID(sessionID),
		FailureType: agentquality.FailurePermission,
		FinalStatus: status,
		ToolDecision: agentquality.ToolDecision{
			Actual:   req.ToolName,
			Decision: agentquality.Decision(decision),
		},
		Attributes: attrs,
	})
}
