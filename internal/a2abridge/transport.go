package a2abridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// InProcessTransport implements A2A-style communication using in-process channels.
type InProcessTransport struct {
	mu      sync.RWMutex
	agents  map[string]subagent.Agent
	logger  *zap.Logger
	counter uint64
}

// NewInProcessTransport creates a new in-process transport.
func NewInProcessTransport(logger *zap.Logger) *InProcessTransport {
	return &InProcessTransport{
		agents: make(map[string]subagent.Agent),
		logger: logger,
	}
}

// RegisterAgent makes an agent available for communication.
func (t *InProcessTransport) RegisterAgent(agent subagent.Agent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agents[agent.ID()] = agent
	t.logger.Debug("已注册 agent 到传输层", zap.String("id", agent.ID()))
}

// UnregisterAgent removes an agent from the transport.
func (t *InProcessTransport) UnregisterAgent(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.agents, id)
}

// SendMessage sends a message to an agent using A2A semantics but channel transport.
func (t *InProcessTransport) SendMessage(ctx context.Context, agentID string, msg Message) (*Task, error) {
	t.mu.RLock()
	agent, ok := t.agents[agentID]
	t.mu.RUnlock()

	if !ok {
		return nil, errs.New(errs.CodeAgentNotFound, fmt.Sprintf("agent %q not found in transport", agentID))
	}

	// Convert A2A message to TaskRequest
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "marshal message", err)
	}

	t.mu.Lock()
	t.counter++
	taskID := fmt.Sprintf("task-%d-%d", time.Now().UnixMilli(), t.counter)
	t.mu.Unlock()

	req := subagent.TaskRequest{
		ID:      taskID,
		Type:    "a2a_message",
		Payload: payload,
	}

	// Send through channel and wait for response
	resp, err := agent.SendTask(ctx, req)
	if err != nil {
		return nil, err
	}

	// Build A2A Task from response
	task := &Task{
		ID:      taskID,
		History: []Message{msg},
	}

	if resp.Error != "" {
		task.Status = TaskStatus{State: "failed", Message: resp.Error}
	} else {
		task.Status = TaskStatus{State: "completed"}
		task.Result = &TaskResult{
			Parts: []Part{NewDataPart(resp.Result)},
		}
	}

	return task, nil
}

// GetAgent returns an agent by ID.
func (t *InProcessTransport) GetAgent(id string) (subagent.Agent, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	agent, ok := t.agents[id]
	if !ok {
		return nil, errs.New(errs.CodeAgentNotFound, fmt.Sprintf("agent %q not found", id))
	}
	return agent, nil
}

// ListAgents returns all registered agent IDs.
func (t *InProcessTransport) ListAgents() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ids := make([]string, 0, len(t.agents))
	for id := range t.agents {
		ids = append(ids, id)
	}
	return ids
}
