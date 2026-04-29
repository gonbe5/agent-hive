// Package observability 提供统一的可观测性支持：Traces、Metrics、Logs 写入 PostgreSQL。
// 设计原则：nil 安全，所有写入异步 fire-and-forget，不阻塞主路径。
package observability

import (
	"context"
	"time"
)

// Tracer 写入 Span 到 hive_traces 表
type Tracer interface {
	// StartSpan 开始一个 Span，返回 SpanContext 用于结束时写入
	StartSpan(ctx context.Context, traceID, spanID, parentSpanID, operation, service, sessionID string) SpanContext
	// RecordSpan 直接写入一条完整 Span（已知 duration）
	RecordSpan(ctx context.Context, span Span) error
}

// MetricsWriter 写入指标到 hive_metrics 表
type MetricsWriter interface {
	// Record 写入一条指标
	Record(ctx context.Context, metric Metric) error
}

// LogWriter 写入日志到 hive_logs 表
type LogWriter interface {
	// Write 写入一条日志
	Write(ctx context.Context, entry LogEntry) error
}

// SpanContext 持有一个进行中的 Span 的上下文，用于结束时写入
type SpanContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Operation    string
	Service      string
	SessionID    string
	UserID       string // Phase 5: 用户级 trace
	StartTime    time.Time
	tracer       Tracer
}

// End 结束 Span 并写入 PG
func (sc *SpanContext) End(ctx context.Context, status string, attrs map[string]any) {
	if sc == nil || sc.tracer == nil {
		return
	}
	durationMs := int(time.Since(sc.StartTime).Milliseconds())
	_ = sc.tracer.RecordSpan(ctx, Span{
		TraceID:      sc.TraceID,
		SpanID:       sc.SpanID,
		ParentSpanID: sc.ParentSpanID,
		Operation:    sc.Operation,
		Service:      sc.Service,
		SessionID:    sc.SessionID,
		UserID:       sc.UserID,
		DurationMs:   durationMs,
		Status:       status,
		Attributes:   attrs,
		Ts:           sc.StartTime,
	})
}

// Span 一条 trace span 记录
type Span struct {
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Operation    string         `json:"operation"`
	Service      string         `json:"service"`
	SessionID    string         `json:"session_id,omitempty"`
	UserID       string         `json:"user_id,omitempty"` // Phase 5: 用户级 trace
	DurationMs   int            `json:"duration_ms"`
	Status       string         `json:"status"` // "ok" | "error"
	Attributes   map[string]any `json:"attributes,omitempty"`
	Ts           time.Time      `json:"ts"`
}

// Metric 一条指标记录
type Metric struct {
	Name   string         `json:"name"`
	Value  float64        `json:"value"`
	Labels map[string]any `json:"labels,omitempty"`
	Ts     time.Time      `json:"ts"`
}

// LogEntry 一条日志记录
type LogEntry struct {
	Level      string         `json:"level"` // "debug" | "info" | "warn" | "error"
	Message    string         `json:"message"`
	TraceID    string         `json:"trace_id,omitempty"`
	SpanID     string         `json:"span_id,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Ts         time.Time      `json:"ts"`
}

// NoopTracer 空实现，DB 不可用时使用
type NoopTracer struct{}

func (n *NoopTracer) StartSpan(_ context.Context, traceID, spanID, parentSpanID, operation, service, sessionID string) SpanContext {
	return SpanContext{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Operation:    operation,
		Service:      service,
		SessionID:    sessionID,
		StartTime:    time.Now(),
		tracer:       nil,
	}
}

func (n *NoopTracer) RecordSpan(_ context.Context, _ Span) error { return nil }

// NoopMetricsWriter 空实现
type NoopMetricsWriter struct{}

func (n *NoopMetricsWriter) Record(_ context.Context, _ Metric) error { return nil }

// NoopLogWriter 空实现
type NoopLogWriter struct{}

func (n *NoopLogWriter) Write(_ context.Context, _ LogEntry) error { return nil }
