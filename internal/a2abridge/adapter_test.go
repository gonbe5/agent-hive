package a2abridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

func echoHandler(ctx context.Context, req subagent.TaskRequest) subagent.TaskResponse {
	return subagent.TaskResponse{
		Status: "completed",
		Result: req.Payload,
	}
}

func TestInProcessTransport_SendMessage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)

	agent := subagent.NewBaseAgent(
		subagent.AgentCard{ID: "test-agent", Name: "Test"},
		echoHandler, skillReg, logger,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go agent.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	transport := NewInProcessTransport(logger)
	transport.RegisterAgent(agent)

	msg := Message{
		Role:  "user",
		Parts: []Part{NewTextPart("hello")},
	}

	task, err := transport.SendMessage(ctx, "test-agent", msg)
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}
	if task.Status.State != "completed" {
		t.Errorf("expected completed, got %s", task.Status.State)
	}
	if task.ID == "" {
		t.Error("expected non-empty task ID")
	}
}

func TestInProcessTransport_AgentNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	transport := NewInProcessTransport(logger)

	_, err := transport.SendMessage(context.Background(), "nonexistent", Message{})
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestInProcessTransport_ListAgents(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	transport := NewInProcessTransport(logger)

	a1 := subagent.NewBaseAgent(subagent.AgentCard{ID: "a1"}, echoHandler, skillReg, logger)
	a2 := subagent.NewBaseAgent(subagent.AgentCard{ID: "a2"}, echoHandler, skillReg, logger)
	transport.RegisterAgent(a1)
	transport.RegisterAgent(a2)

	agents := transport.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestInProcessTransport_UnregisterAgent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	transport := NewInProcessTransport(logger)

	agent := subagent.NewBaseAgent(subagent.AgentCard{ID: "rm"}, echoHandler, skillReg, logger)
	transport.RegisterAgent(agent)
	transport.UnregisterAgent("rm")

	if len(transport.ListAgents()) != 0 {
		t.Error("expected 0 agents after unregister")
	}
}

func TestNewTextPart(t *testing.T) {
	part := NewTextPart("hello")
	if part.Type != "text" {
		t.Errorf("expected type text, got %s", part.Type)
	}
	var text string
	json.Unmarshal(part.Content, &text)
	if text != "hello" {
		t.Errorf("expected hello, got %s", text)
	}
}
