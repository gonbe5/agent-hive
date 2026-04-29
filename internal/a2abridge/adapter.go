package a2abridge

import (
	"encoding/json"
	"time"
)

// Message represents an A2A protocol message.
type Message struct {
	Role    string          `json:"role"` // "user" or "agent"
	Parts   []Part          `json:"parts"`
	Created time.Time       `json:"created"`
}

// Part is a content part within a message.
type Part struct {
	Type    string          `json:"type"` // "text", "data"
	Content json.RawMessage `json:"content"`
}

// NewTextPart creates a text part.
func NewTextPart(text string) Part {
	data, _ := json.Marshal(text)
	return Part{Type: "text", Content: data}
}

// NewDataPart creates a data part.
func NewDataPart(data any) Part {
	raw, _ := json.Marshal(data)
	return Part{Type: "data", Content: raw}
}

// Task represents an A2A task with lifecycle.
type Task struct {
	ID      string       `json:"id"`
	Status  TaskStatus   `json:"status"`
	History []Message    `json:"history,omitempty"`
	Result  *TaskResult  `json:"result,omitempty"`
}

// TaskStatus represents the state of a task.
type TaskStatus struct {
	State   string `json:"state"` // "submitted", "working", "completed", "failed", "canceled"
	Message string `json:"message,omitempty"`
}

// TaskResult contains the output artifacts of a completed task.
type TaskResult struct {
	Parts []Part `json:"parts"`
}
