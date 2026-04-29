package observability

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// --- NoopTracer 测试 ---

func TestNoopTracer_RecordSpan_ReturnsNil(t *testing.T) {
	tr := &NoopTracer{}
	err := tr.RecordSpan(context.Background(), Span{
		TraceID:   "abc",
		SpanID:    "def",
		Operation: "test",
		Service:   "svc",
		Status:    "ok",
	})
	assert.NoError(t, err)
}

func TestNoopTracer_StartSpan_ReturnsNilTracer(t *testing.T) {
	tr := &NoopTracer{}
	sc := tr.StartSpan(context.Background(), "t1", "s1", "p1", "op", "svc", "sess")
	assert.Equal(t, "t1", sc.TraceID)
	assert.Equal(t, "s1", sc.SpanID)
	assert.Equal(t, "p1", sc.ParentSpanID)
	assert.Equal(t, "op", sc.Operation)
	assert.Equal(t, "svc", sc.Service)
	assert.Equal(t, "sess", sc.SessionID)
	assert.Nil(t, sc.tracer, "NoopTracer 返回的 SpanContext.tracer 应为 nil")
	assert.False(t, sc.StartTime.IsZero(), "StartTime 应被设置")
}

// --- NoopMetricsWriter 测试 ---

func TestNoopMetricsWriter_Record_ReturnsNil(t *testing.T) {
	w := &NoopMetricsWriter{}
	err := w.Record(context.Background(), Metric{Name: "test", Value: 1.0})
	assert.NoError(t, err)
}

// --- NoopLogWriter 测试 ---

func TestNoopLogWriter_Write_ReturnsNil(t *testing.T) {
	w := &NoopLogWriter{}
	err := w.Write(context.Background(), LogEntry{Level: "info", Message: "hello"})
	assert.NoError(t, err)
}

// --- SpanContext.End nil 安全测试 ---

func TestSpanContext_End_NilReceiver(t *testing.T) {
	var sc *SpanContext
	// 不应 panic
	sc.End(context.Background(), "ok", nil)
}

func TestSpanContext_End_NilTracer(t *testing.T) {
	sc := &SpanContext{
		TraceID:   "t1",
		SpanID:    "s1",
		Operation: "op",
		StartTime: time.Now(),
		tracer:    nil,
	}
	// 不应 panic
	sc.End(context.Background(), "ok", map[string]any{"key": "val"})
}

// --- NewTraceID / NewSpanID 测试 ---

func TestNewTraceID_Returns32CharHex(t *testing.T) {
	id := NewTraceID()
	assert.Len(t, id, 32, "TraceID 应为 32 字符 hex")
	// 验证是合法 hex
	for _, c := range id {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"TraceID 应只包含 hex 字符，发现: %c", c)
	}
}

func TestNewTraceID_Unique(t *testing.T) {
	id1 := NewTraceID()
	id2 := NewTraceID()
	assert.NotEqual(t, id1, id2, "两次生成的 TraceID 应不同")
}

func TestNewSpanID_Returns16CharHex(t *testing.T) {
	id := NewSpanID()
	assert.Len(t, id, 16, "SpanID 应为 16 字符 hex")
	for _, c := range id {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"SpanID 应只包含 hex 字符，发现: %c", c)
	}
}

func TestNewSpanID_Unique(t *testing.T) {
	id1 := NewSpanID()
	id2 := NewSpanID()
	assert.NotEqual(t, id1, id2, "两次生成的 SpanID 应不同")
}

// --- JSON 序列化 roundtrip 测试 ---

func TestSpan_JSONRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := Span{
		TraceID:      "aaaa",
		SpanID:       "bbbb",
		ParentSpanID: "cccc",
		Operation:    "llm_call",
		Service:      "master",
		SessionID:    "sess-1",
		DurationMs:   42,
		Status:       "ok",
		Attributes:   map[string]any{"model": "gpt-4", "tokens": float64(100)},
		Ts:           now,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded Span
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.TraceID, decoded.TraceID)
	assert.Equal(t, original.SpanID, decoded.SpanID)
	assert.Equal(t, original.ParentSpanID, decoded.ParentSpanID)
	assert.Equal(t, original.Operation, decoded.Operation)
	assert.Equal(t, original.Service, decoded.Service)
	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.DurationMs, decoded.DurationMs)
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.Attributes["model"], decoded.Attributes["model"])
	assert.Equal(t, original.Attributes["tokens"], decoded.Attributes["tokens"])
	assert.True(t, original.Ts.Equal(decoded.Ts), "Ts 应一致")
}

func TestSpan_JSONOmitsEmpty(t *testing.T) {
	s := Span{
		TraceID:   "t",
		SpanID:    "s",
		Operation: "op",
		Service:   "svc",
		Status:    "ok",
		Ts:        time.Now(),
	}
	data, err := json.Marshal(s)
	assert.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	assert.NoError(t, err)
	assert.NotContains(t, raw, "parent_span_id", "空 ParentSpanID 应被 omitempty 省略")
	assert.NotContains(t, raw, "session_id", "空 SessionID 应被 omitempty 省略")
	assert.NotContains(t, raw, "attributes", "nil Attributes 应被 omitempty 省略")
}

func TestMetric_JSONRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := Metric{
		Name:   "llm_latency_ms",
		Value:  123.45,
		Labels: map[string]any{"model": "gpt-4"},
		Ts:     now,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded Metric
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Value, decoded.Value)
	assert.Equal(t, original.Labels["model"], decoded.Labels["model"])
	assert.True(t, original.Ts.Equal(decoded.Ts))
}

func TestMetric_JSONOmitsEmpty(t *testing.T) {
	m := Metric{Name: "counter", Value: 1.0, Ts: time.Now()}
	data, err := json.Marshal(m)
	assert.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	assert.NoError(t, err)
	assert.NotContains(t, raw, "labels", "nil Labels 应被 omitempty 省略")
}

func TestLogEntry_JSONRoundtrip(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := LogEntry{
		Level:      "error",
		Message:    "something broke",
		TraceID:    "t1",
		SpanID:     "s1",
		SessionID:  "sess",
		Attributes: map[string]any{"code": float64(500)},
		Ts:         now,
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded LogEntry
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	assert.Equal(t, original.Level, decoded.Level)
	assert.Equal(t, original.Message, decoded.Message)
	assert.Equal(t, original.TraceID, decoded.TraceID)
	assert.Equal(t, original.SpanID, decoded.SpanID)
	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.Attributes["code"], decoded.Attributes["code"])
	assert.True(t, original.Ts.Equal(decoded.Ts))
}

func TestLogEntry_JSONOmitsEmpty(t *testing.T) {
	entry := LogEntry{Level: "info", Message: "hi", Ts: time.Now()}
	data, err := json.Marshal(entry)
	assert.NoError(t, err)

	var raw map[string]any
	err = json.Unmarshal(data, &raw)
	assert.NoError(t, err)
	assert.NotContains(t, raw, "trace_id")
	assert.NotContains(t, raw, "span_id")
	assert.NotContains(t, raw, "session_id")
	assert.NotContains(t, raw, "attributes")
}
