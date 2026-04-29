package feishu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

type fakeSubmitter struct {
	calls []master.InputResponse
	err   error
}

func (f *fakeSubmitter) SubmitInput(resp master.InputResponse) error {
	f.calls = append(f.calls, resp)
	return f.err
}

func ptrString(s string) *string { return &s }

func makeEvent(action string, requestID string, openID string, tenant string, value string) *callback.CardActionTriggerEvent {
	val := map[string]any{
		"request_id": requestID,
		"action":     action,
		"task_id":    "task-1",
	}
	if value != "" {
		val["value"] = value
	}
	return &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Operator: &callback.Operator{
				OpenID:    openID,
				TenantKey: ptrString(tenant),
			},
			Action: &callback.CallBackAction{
				Tag:   "button",
				Value: val,
			},
			Context: &callback.Context{OpenMessageID: "om-123"},
		},
	}
}

func TestDecodeCardAction_HappyPath(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	ev := makeEvent("approve", "req-1", "ou_open_xyz", "tk-test", "yes")

	got, err := decodeCardAction(ev, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RequestID != "req-1" || got.Action != "approve" || got.Value != "yes" {
		t.Fatalf("unexpected fields: %+v", got)
	}
	if got.Tag != imctx.CardActionTagButton {
		t.Fatalf("expected button tag, got %q", got.Tag)
	}
	if got.Platform != imctx.PlatformFeishu {
		t.Fatalf("expected feishu platform, got %q", got.Platform)
	}
	if got.SafeOperatorID == "" || got.SafeOperatorID == "ou_open_xyz" {
		t.Fatalf("SafeOperatorID must be hashed, got %q", got.SafeOperatorID)
	}
	if got.TenantKey != "tk-test" || got.ChannelMessageID != "om-123" {
		t.Fatalf("unexpected metadata: tenant=%q msg=%q", got.TenantKey, got.ChannelMessageID)
	}
	if !got.ReceivedAt.Equal(now) {
		t.Fatalf("ReceivedAt must equal injected now")
	}
}

func TestDecodeCardAction_RejectsNoRequestID(t *testing.T) {
	ev := makeEvent("approve", "", "ou", "tk", "")
	_, err := decodeCardAction(ev, time.Now())
	if !errors.Is(err, ErrCardActionMissingReqID) {
		t.Fatalf("want ErrCardActionMissingReqID, got %v", err)
	}
}

func TestDecodeCardAction_RejectsNilEvent(t *testing.T) {
	_, err := decodeCardAction(nil, time.Now())
	if !errors.Is(err, ErrCardActionNilEvent) {
		t.Fatalf("want ErrCardActionNilEvent, got %v", err)
	}
	_, err = decodeCardAction(&callback.CardActionTriggerEvent{}, time.Now())
	if !errors.Is(err, ErrCardActionNilEvent) {
		t.Fatalf("want ErrCardActionNilEvent for empty Event, got %v", err)
	}
}

func TestSafeSenderID_NotPlaintext(t *testing.T) {
	if SafeSenderID("") != "" {
		t.Fatal("empty input should return empty")
	}
	out := SafeSenderID("ou_open_real_user")
	if out == "ou_open_real_user" {
		t.Fatal("must not return raw input")
	}
	if len(out) != 8 { // sha256[:4] = 4 bytes = 8 hex chars
		t.Fatalf("expected 8 hex chars, got %d (%q)", len(out), out)
	}
	if SafeSenderID("ou_open_real_user") != out {
		t.Fatal("must be deterministic")
	}
	if SafeSenderID("ou_other_user") == out {
		t.Fatal("different inputs must produce different hashes (collision risk too high if not)")
	}
}

func TestBridge_HandleHappyPath(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, nil, nil)

	resp, err := bridge.HandleCardActionTrigger(context.Background(),
		makeEvent("approve", "req-42", "ou_x", "tk", "ok"))
	if err != nil {
		t.Fatalf("handler MUST always return nil error, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "success" {
		t.Fatalf("expected success toast, got %+v", resp)
	}
	if len(sub.calls) != 1 {
		t.Fatalf("expected 1 submit, got %d", len(sub.calls))
	}
	if sub.calls[0].RequestID != "req-42" || sub.calls[0].Action != "approve" || sub.calls[0].Value != "ok" {
		t.Fatalf("unexpected submit payload: %+v", sub.calls[0])
	}
}

func TestBridge_NeverReturnsError_OnSubmitFailure(t *testing.T) {
	sub := &fakeSubmitter{err: errors.New("broker down")}
	bridge := NewFeishuHITLBridge(sub, nil, nil)

	resp, err := bridge.HandleCardActionTrigger(context.Background(),
		makeEvent("reject", "req-43", "ou_x", "tk", ""))
	if err != nil {
		t.Fatalf("handler must return nil even when submit fails, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "error" {
		t.Fatalf("expected error toast, got %+v", resp)
	}
}

func TestBridge_NonHITLCard_NoSubmitNoToast(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, nil, nil)

	// Card without request_id (e.g., a regular interactive card unrelated to HITL)
	resp, err := bridge.HandleCardActionTrigger(context.Background(),
		makeEvent("approve", "", "ou_x", "tk", ""))
	if err != nil {
		t.Fatalf("handler must return nil, got %v", err)
	}
	if resp != nil {
		t.Fatalf("non-HITL card should not produce toast, got %+v", resp)
	}
	if len(sub.calls) != 0 {
		t.Fatalf("non-HITL card must not submit, got %d submits", len(sub.calls))
	}
}

func TestBridge_RejectsBadAction(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, nil, nil)

	resp, err := bridge.HandleCardActionTrigger(context.Background(),
		makeEvent("delete_everything", "req-bad", "ou_x", "tk", ""))
	if err != nil {
		t.Fatalf("handler must return nil, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "error" {
		t.Fatalf("expected error toast for bad action")
	}
	if len(sub.calls) != 0 {
		t.Fatalf("bad action must not be submitted to broker, got %d", len(sub.calls))
	}
}

func TestBridge_NilSubmitter_ToastsNotPanic(t *testing.T) {
	bridge := NewFeishuHITLBridge(nil, nil, nil)
	resp, err := bridge.HandleCardActionTrigger(context.Background(),
		makeEvent("approve", "req-x", "ou", "tk", ""))
	if err != nil {
		t.Fatalf("handler must return nil even with nil submitter, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "error" {
		t.Fatalf("expected error toast")
	}
}

func TestBridge_EmitsCallbackStatusMetrics(t *testing.T) {
	writer := &hitlMetricCaptureWriter{}

	successBridge := NewFeishuHITLBridge(&fakeSubmitter{}, nil, nil).WithMetricsWriter(writer)
	resp, err := successBridge.HandleCardActionTrigger(context.Background(),
		makeEvent("approve", "req-ok", "ou_x", "tk-ok", "ok"))
	if err != nil {
		t.Fatalf("handler MUST always return nil error, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "success" {
		t.Fatalf("expected success toast, got %+v", resp)
	}

	failBridge := NewFeishuHITLBridge(&fakeSubmitter{err: errors.New("broker down")}, nil, nil).WithMetricsWriter(writer)
	resp, err = failBridge.HandleCardActionTrigger(context.Background(),
		makeEvent("reject", "req-fail", "ou_y", "tk-fail", ""))
	if err != nil {
		t.Fatalf("handler must return nil even when submit fails, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "error" {
		t.Fatalf("expected error toast, got %+v", resp)
	}

	nilBridge := NewFeishuHITLBridge(nil, nil, nil).WithMetricsWriter(writer)
	resp, err = nilBridge.HandleCardActionTrigger(context.Background(),
		makeEvent("approve", "req-nil", "ou_z", "tk-nil", ""))
	if err != nil {
		t.Fatalf("handler must return nil even with nil submitter, got %v", err)
	}
	if resp == nil || resp.Toast == nil || resp.Toast.Type != "error" {
		t.Fatalf("expected error toast")
	}

	if metric := writer.findWithLabel(MetricHITLCallbackStatus, "status", "submitted"); metric == nil {
		t.Fatalf("expected %s metric with submitted status", MetricHITLCallbackStatus)
	}
	if metric := writer.findWithLabel(MetricHITLCallbackStatus, "status", "submit_failed"); metric == nil {
		t.Fatalf("expected %s metric with submit_failed status", MetricHITLCallbackStatus)
	}
	if metric := writer.findWithLabel(MetricHITLCallbackStatus, "status", "no_submitter"); metric == nil {
		t.Fatalf("expected %s metric with no_submitter status", MetricHITLCallbackStatus)
	}
}

func TestBridge_submitFromCardAction_UsesAllowList(t *testing.T) {
	sub := &fakeSubmitter{}
	bridge := NewFeishuHITLBridge(sub, nil, nil)

	if err := bridge.submitFromCardAction(imctx.CardAction{RequestID: "r", Action: "approve"}); err != nil {
		t.Fatalf("approve should be accepted: %v", err)
	}
	if err := bridge.submitFromCardAction(imctx.CardAction{RequestID: "r", Action: "drop_table"}); !errors.Is(err, ErrHITLBadAction) {
		t.Fatalf("expected ErrHITLBadAction, got %v", err)
	}
}

type hitlMetricCaptureWriter struct {
	items []observability.Metric
}

func (w *hitlMetricCaptureWriter) Record(_ context.Context, metric observability.Metric) error {
	w.items = append(w.items, metric)
	return nil
}

func (w *hitlMetricCaptureWriter) findWithLabel(name, key string, value any) *observability.Metric {
	for i := range w.items {
		if w.items[i].Name == name && w.items[i].Labels[key] == value {
			return &w.items[i]
		}
	}
	return nil
}
