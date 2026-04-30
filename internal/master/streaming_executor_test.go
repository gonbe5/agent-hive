package master

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

func TestStreamingExecutor_AddTool_SafeConcurrent(t *testing.T) {
	var count atomic.Int32
	var last atomic.Int32

	exec := func(ctx context.Context, name string, input json.RawMessage) (*mcphost.ToolResult, error) {
		last.Add(1)
		count.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &mcphost.ToolResult{Content: json.RawMessage(`"` + name + `"`)}, nil
	}

	// read_file is safe → should run concurrently
	tools := []mcphost.ToolDefinition{
		{Name: "read_file", IsConcurrencySafe: true},
	}

	se := NewStreamingExecutor(tools, exec)
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		go func(id string) {
			defer wg.Done()
			se.AddTool(context.Background(), id, "read_file", nil)
		}(string(rune('a' + i)))
	}

	// Wait for all to be added
	time.Sleep(50 * time.Millisecond)

	// Safe tools should all be running concurrently
	if count.Load() != 3 {
		t.Errorf("expected all 3 safe tools running, got %d", count.Load())
	}

	// Wait for completion
	results := se.GetResults()
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	wg.Wait()
}

func TestStreamingExecutor_AddTool_UnsafeSerialized(t *testing.T) {
	var active atomic.Int32
	var maxConcurrent atomic.Int32

	exec := func(ctx context.Context, name string, input json.RawMessage) (*mcphost.ToolResult, error) {
		n := active.Add(1)
		for {
			m := maxConcurrent.Load()
			if n > m {
				maxConcurrent.Store(n)
			}
			if active.Load() == 1 {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return &mcphost.ToolResult{Content: json.RawMessage(`"` + name + `"`)}, nil
	}

	// bash is unsafe → should be serialized
	tools := []mcphost.ToolDefinition{
		{Name: "bash", IsConcurrencySafe: false},
	}

	se := NewStreamingExecutor(tools, exec)
	for i := 0; i < 3; i++ {
		se.AddTool(context.Background(), string(rune('a'+i)), "bash", nil)
	}

	time.Sleep(50 * time.Millisecond)

	// Unsafe tools should never run more than 1 concurrently
	if maxConcurrent.Load() > 1 {
		t.Errorf("expected max 1 unsafe tool concurrent, got %d", maxConcurrent.Load())
	}

	results := se.GetResults()
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestStreamingExecutor_MixedSafeAndUnsafe(t *testing.T) {
	order := []string{}
	var mu sync.Mutex

	exec := func(ctx context.Context, name string, input json.RawMessage) (*mcphost.ToolResult, error) {
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
		return &mcphost.ToolResult{Content: json.RawMessage(`"` + name + `"`)}, nil
	}

	tools := []mcphost.ToolDefinition{
		{Name: "read_file", IsConcurrencySafe: true},
		{Name: "bash", IsConcurrencySafe: false},
	}

	se := NewStreamingExecutor(tools, exec)

	// Add in mixed order: safe first, then unsafe
	se.AddTool(context.Background(), "s1", "read_file", nil)
	se.AddTool(context.Background(), "u1", "bash", nil)
	se.AddTool(context.Background(), "s2", "read_file", nil)

	se.GetResults()

	// read_file can interleave with bash
	// bash must not overlap with itself
	if len(order) != 3 {
		t.Errorf("expected 3 executions, got %v", order)
	}
}

func TestStreamingExecutor_CanExecute(t *testing.T) {
	tools := []mcphost.ToolDefinition{
		{Name: "read_file", IsConcurrencySafe: true},
		{Name: "bash", IsConcurrencySafe: false},
	}
	se := NewStreamingExecutor(tools, nil)

	// Safe tool always executable
	safe := &TrackedTool{IsSafe: true}
	if !se.canExecute(safe) {
		t.Error("safe tool should always be executable")
	}

	// Unsafe tool with no other unsafe running
	unsafe := &TrackedTool{IsSafe: false}
	if !se.canExecute(unsafe) {
		t.Error("unsafe tool should be executable when unsafeCount=0")
	}
}

func TestStreamingExecutor_AbortAllReturnsSyntheticResultsByID(t *testing.T) {
	release := make(chan struct{})
	execStarted := make(chan struct{})
	exec := func(ctx context.Context, name string, input json.RawMessage) (*mcphost.ToolResult, error) {
		close(execStarted)
		<-release
		return &mcphost.ToolResult{Content: json.RawMessage(`"late"`), IsError: false}, nil
	}
	tools := []mcphost.ToolDefinition{
		{Name: "bash", IsConcurrencySafe: false},
	}
	se := NewStreamingExecutor(tools, exec)
	se.AddTool(context.Background(), "call-1", "bash", json.RawMessage(`{}`))
	se.AddTool(context.Background(), "call-2", "bash", json.RawMessage(`{}`))
	<-execStarted

	se.AbortAll("safe sibling failed")
	results := se.GetResultsByID()
	close(release)

	for _, id := range []string{"call-1", "call-2"} {
		result, ok := results[id]
		if !ok {
			t.Fatalf("aborted tool %s must still produce a synthetic tool result", id)
		}
		if result == nil || !result.IsError {
			t.Fatalf("aborted tool %s result must be an error: %+v", id, result)
		}
		if got := result.DecodeContent(); got == "" || got == "late" {
			t.Fatalf("aborted tool %s should use synthetic cancellation content, got %q", id, got)
		}
	}
}
