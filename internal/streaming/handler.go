package streaming

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// SSEWriter writes Server-Sent Events to an HTTP response.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
}

// NewSSEWriter creates a new SSE writer.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errs.New(errs.CodeUnavailable, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	return &SSEWriter{w: w, flusher: flusher}, nil
}

// WriteEvent sends a named event with JSON data.
func (s *SSEWriter) WriteEvent(eventType string, data any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errs.New(errs.CodeFailedPrecondition, "writer closed")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, "marshal event data", err)
	}

	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	if err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteComment sends an SSE comment (keep-alive).
func (s *SSEWriter) WriteComment(comment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errs.New(errs.CodeFailedPrecondition, "writer closed")
	}
	_, err := fmt.Fprintf(s.w, ": %s\n\n", comment)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// Close marks the writer as closed.
func (s *SSEWriter) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

// StreamHandler manages streaming events from the master to SSE clients.
type StreamHandler struct {
	master *master.Master
	logger *zap.Logger
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(m *master.Master, logger *zap.Logger) *StreamHandler {
	return &StreamHandler{master: m, logger: logger}
}

// ToolCallAssembler incrementally assembles tool call arguments from streaming chunks.
type ToolCallAssembler struct {
	mu    sync.Mutex
	calls map[string]*ToolCallBuffer
}

// ToolCallBuffer accumulates argument chunks for a single tool call.
type ToolCallBuffer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Args     string `json:"arguments"`
	Complete bool   `json:"complete"`
}

// NewToolCallAssembler creates a new assembler.
func NewToolCallAssembler() *ToolCallAssembler {
	return &ToolCallAssembler{calls: make(map[string]*ToolCallBuffer)}
}

// AddChunk appends an argument chunk to a tool call buffer.
func (a *ToolCallAssembler) AddChunk(callID, name, argChunk string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	buf, ok := a.calls[callID]
	if !ok {
		buf = &ToolCallBuffer{ID: callID, Name: name}
		a.calls[callID] = buf
	}
	buf.Args += argChunk
}

// Complete marks a tool call as complete and returns its buffer.
func (a *ToolCallAssembler) Complete(callID string) (*ToolCallBuffer, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	buf, ok := a.calls[callID]
	if !ok {
		return nil, false
	}
	buf.Complete = true
	return buf, true
}

// Get returns a tool call buffer.
func (a *ToolCallAssembler) Get(callID string) (*ToolCallBuffer, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	buf, ok := a.calls[callID]
	return buf, ok
}

// Reset clears all buffers.
func (a *ToolCallAssembler) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = make(map[string]*ToolCallBuffer)
}
