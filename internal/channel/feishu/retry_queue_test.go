package feishu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/observability"
)

func TestPlanRetryFailure_BackoffAndExhaustion(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)

	count, nextAt, exhausted := planRetryFailure(0, 5, now)
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if exhausted {
		t.Fatal("exhausted = true, want false")
	}
	if got := nextAt.Sub(now); got != time.Minute {
		t.Fatalf("backoff = %v, want 1m", got)
	}

	count, nextAt, exhausted = planRetryFailure(3, 5, now)
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	if exhausted {
		t.Fatal("exhausted = true, want false")
	}
	if got := nextAt.Sub(now); got != 8*time.Minute {
		t.Fatalf("backoff = %v, want 8m", got)
	}

	count, nextAt, exhausted = planRetryFailure(4, 5, now)
	if count != 5 {
		t.Fatalf("count = %d, want 5", count)
	}
	if !exhausted {
		t.Fatal("exhausted = false, want true")
	}
	if !nextAt.Equal(now) {
		t.Fatalf("nextAt = %v, want %v", nextAt, now)
	}
}

func TestNormalizeRetryTenantKey_DefaultFallback(t *testing.T) {
	if got := normalizeRetryTenantKey(""); got != defaultRetryTenantKey {
		t.Fatalf("normalizeRetryTenantKey(\"\") = %q, want %q", got, defaultRetryTenantKey)
	}
	if got := normalizeRetryTenantKey("tenant-a"); got != "tenant-a" {
		t.Fatalf("normalizeRetryTenantKey(tenant-a) = %q, want tenant-a", got)
	}
}

func TestTruncateRetryError_CapsLargeMessages(t *testing.T) {
	msg := make([]byte, 1100)
	for i := range msg {
		msg[i] = 'a'
	}
	got := truncateRetryError(errors.New(string(msg)))
	if len(got) != 1024 {
		t.Fatalf("len = %d, want 1024", len(got))
	}
}

func TestRetryQueueWorker_EmitDeadLetterMetricOnExhausted(t *testing.T) {
	writer := &retryQueueMetricWriter{}
	worker := NewRetryQueueWorker(nil, nil).WithMetricsWriter(writer)

	worker.emitDeadLetterMetric(retryQueueDBItem{
		Reason:    string(channel.RetryReasonPushSend),
		TenantKey: "tenant-a",
	})

	if len(writer.items) != 1 {
		t.Fatalf("metric count = %d, want 1", len(writer.items))
	}
	metric := writer.items[0]
	if metric.Name != MetricOutboundDeadLetter {
		t.Fatalf("metric name = %q, want %q", metric.Name, MetricOutboundDeadLetter)
	}
	if got := metric.Labels["reason"]; got != string(channel.RetryReasonPushSend) {
		t.Fatalf("metric reason = %v, want %q", got, channel.RetryReasonPushSend)
	}
	if got := metric.Labels["tenant_key_hash"]; got != "tk_80a707af" {
		t.Fatalf("metric tenant_key_hash = %v, want tk_80a707af", got)
	}
}

type retryQueueMetricWriter struct {
	items []observability.Metric
}

func (w *retryQueueMetricWriter) Record(_ context.Context, metric observability.Metric) error {
	w.items = append(w.items, metric)
	return nil
}
