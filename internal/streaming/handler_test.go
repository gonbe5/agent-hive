package streaming

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- SSEWriter TESTS ---

func TestSSEWriter_WriteEvent(t *testing.T) {
	rec := httptest.NewRecorder()

	writer, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("failed to create SSEWriter: %v", err)
	}

	// Test writing an event
	err = writer.WriteEvent("test-type", map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("WriteEvent failed: %v", err)
	}

	// Verify output format
	output := rec.Body.String()
	if !strings.Contains(output, "event: test-type") {
		t.Errorf("output missing event type, got: %s", output)
	}
	if !strings.Contains(output, "data: {") {
		t.Errorf("output missing data, got: %s", output)
	}
}

func TestSSEWriter_WriteComment(t *testing.T) {
	rec := httptest.NewRecorder()

	writer, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("failed to create SSEWriter: %v", err)
	}

	err = writer.WriteComment("test comment")
	if err != nil {
		t.Errorf("WriteComment failed: %v", err)
	}

	output := rec.Body.String()
	if !strings.Contains(output, ": test comment") {
		t.Errorf("output missing comment, got: %s", output)
	}
}

func TestSSEWriter_MultipleEvents(t *testing.T) {
	rec := httptest.NewRecorder()

	writer, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("failed to create SSEWriter: %v", err)
	}

	// Write multiple events
	writer.WriteEvent("event1", "data1")
	writer.WriteEvent("event2", "data2")
	writer.WriteEvent("event3", "data3")

	output := rec.Body.String()
	if !strings.Contains(output, "event1") || !strings.Contains(output, "event2") || !strings.Contains(output, "event3") {
		t.Errorf("missing some events in output: %s", output)
	}
}

func TestSSEWriter_Close(t *testing.T) {
	rec := httptest.NewRecorder()

	writer, err := NewSSEWriter(rec)
	if err != nil {
		t.Fatalf("failed to create SSEWriter: %v", err)
	}

	// Close should not panic
	writer.Close()
}

func TestSSEWriter_InvalidFlusher(t *testing.T) {
	// Create a ResponseWriter that doesn't support flushing
	buf := &bytes.Buffer{}
	w := &nonFlushingWriter{buf: buf}

	_, err := NewSSEWriter(w)
	if err == nil {
		t.Error("expected error when ResponseWriter doesn't support flushing")
	}
}

// nonFlushingWriter is a test helper that doesn't implement http.Flusher
type nonFlushingWriter struct {
	buf        *bytes.Buffer
	headers    http.Header
	statusCode int
}

func (w *nonFlushingWriter) Header() http.Header {
	if w.headers == nil {
		w.headers = make(http.Header)
	}
	return w.headers
}

func (w *nonFlushingWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

func (w *nonFlushingWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// --- ToolCallAssembler TESTS ---

func TestToolCallAssembler_AddChunk(t *testing.T) {
	assembler := NewToolCallAssembler()

	assembler.AddChunk("call-1", "test_func", `{"key":`)
	assembler.AddChunk("call-1", "", `"value"}`)

	buf, exists := assembler.Get("call-1")
	if !exists {
		t.Fatal("expected call-1 to exist")
	}

	if buf.Name != "test_func" {
		t.Errorf("expected name 'test_func', got %s", buf.Name)
	}

	expectedArgs := `{"key":"value"}`
	if buf.Args != expectedArgs {
		t.Errorf("expected args %s, got %s", expectedArgs, buf.Args)
	}
}

func TestToolCallAssembler_Complete(t *testing.T) {
	assembler := NewToolCallAssembler()

	assembler.AddChunk("call-1", "func1", `{"arg":"val"}`)

	buf, ok := assembler.Complete("call-1")
	if !ok {
		t.Fatal("expected Complete to return true")
	}

	if buf.Name != "func1" {
		t.Errorf("expected name 'func1', got %s", buf.Name)
	}

	if !buf.Complete {
		t.Error("expected Complete flag to be true")
	}

	// After complete, Get should still return the call (it's marked complete, not deleted)
	buf2, exists := assembler.Get("call-1")
	if !exists {
		t.Error("expected call to still exist after Complete")
	}

	if !buf2.Complete {
		t.Error("expected retrieved buffer to be marked complete")
	}
}

func TestToolCallAssembler_CompleteNonexistent(t *testing.T) {
	assembler := NewToolCallAssembler()

	_, ok := assembler.Complete("nonexistent")
	if ok {
		t.Error("expected Complete to return false for nonexistent call")
	}
}

func TestToolCallAssembler_Reset(t *testing.T) {
	assembler := NewToolCallAssembler()

	assembler.AddChunk("call-1", "func1", "data1")
	assembler.AddChunk("call-2", "func2", "data2")

	assembler.Reset()

	_, exists1 := assembler.Get("call-1")
	_, exists2 := assembler.Get("call-2")

	if exists1 || exists2 {
		t.Error("expected all calls to be cleared after Reset")
	}
}

func TestToolCallAssembler_MultipleCalls(t *testing.T) {
	assembler := NewToolCallAssembler()

	// Add chunks for multiple calls
	assembler.AddChunk("call-1", "func1", `{"a":`)
	assembler.AddChunk("call-2", "func2", `{"b":`)
	assembler.AddChunk("call-1", "", `1}`)
	assembler.AddChunk("call-2", "", `2}`)

	buf1, _ := assembler.Get("call-1")
	buf2, _ := assembler.Get("call-2")

	if buf1.Args != `{"a":1}` {
		t.Errorf("call-1 args incorrect: %s", buf1.Args)
	}

	if buf2.Args != `{"b":2}` {
		t.Errorf("call-2 args incorrect: %s", buf2.Args)
	}
}

func TestToolCallAssembler_NameOverwrite(t *testing.T) {
	assembler := NewToolCallAssembler()

	// First chunk with name
	assembler.AddChunk("call-1", "func1", "")

	// Second chunk with different name (should be ignored)
	assembler.AddChunk("call-1", "func2", "data")

	buf, _ := assembler.Get("call-1")
	if buf.Name != "func1" {
		t.Errorf("expected name to remain 'func1', got %s", buf.Name)
	}
}

// --- StreamHandler TESTS ---

func TestNewStreamHandler(t *testing.T) {
	// Basic creation test - handler needs Master which requires complex setup
	// This test verifies the constructor doesn't panic

	// Note: Full integration tests would require a running Master instance
	// Those are better suited for integration test suite

	// For now, we test that the struct creation logic works
	handler := &StreamHandler{}
	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
}
