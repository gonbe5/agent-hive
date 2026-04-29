package mcphost

import (
	"encoding/json"
	"sync"
)

// ToolSet manages a named collection of tools for a specific agent or context.
type ToolSet struct {
	mu    sync.RWMutex
	name  string
	tools map[string]ToolDefinition
}

// NewToolSet creates a new named tool set.
func NewToolSet(name string) *ToolSet {
	return &ToolSet{
		name:  name,
		tools: make(map[string]ToolDefinition),
	}
}

// Name returns the tool set name.
func (ts *ToolSet) Name() string { return ts.name }

// Add adds a tool definition to the set.
func (ts *ToolSet) Add(tool ToolDefinition) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tools[tool.Name] = tool
}

// Remove removes a tool from the set.
func (ts *ToolSet) Remove(name string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tools, name)
}

// Get returns a tool definition by name.
func (ts *ToolSet) Get(name string) (ToolDefinition, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	t, ok := ts.tools[name]
	return t, ok
}

// List returns all tools in the set.
func (ts *ToolSet) List() []ToolDefinition {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(ts.tools))
	for _, t := range ts.tools {
		defs = append(defs, t)
	}
	return defs
}

// ToJSON serializes the tool set.
func (ts *ToolSet) ToJSON() (json.RawMessage, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return json.Marshal(ts.tools)
}

// Count returns the number of tools in the set.
func (ts *ToolSet) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.tools)
}
