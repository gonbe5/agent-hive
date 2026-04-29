package subagent

import (
	"context"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry(testLogger())
	card := AgentCard{ID: "agent-1", Name: "Agent 1"}
	agent := NewBaseAgent(card, echoHandler, testSkillReg(), testLogger())

	if err := reg.Register(agent); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	got, err := reg.Get("agent-1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.ID() != "agent-1" {
		t.Errorf("expected ID agent-1, got %s", got.ID())
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	reg := NewRegistry(testLogger())
	agent := NewBaseAgent(AgentCard{ID: "dup"}, echoHandler, testSkillReg(), testLogger())
	reg.Register(agent)

	err := reg.Register(agent)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := NewRegistry(testLogger())
	_, err := reg.Get("missing")
	if !errs.IsCode(err, errs.CodeAgentNotFound) {
		t.Errorf("expected CodeAgentNotFound, got %v", err)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry(testLogger())
	agent := NewBaseAgent(AgentCard{ID: "rm-agent"}, echoHandler, testSkillReg(), testLogger())
	reg.Register(agent)

	if err := reg.Unregister("rm-agent"); err != nil {
		t.Fatalf("Unregister error: %v", err)
	}

	_, err := reg.Get("rm-agent")
	if !errs.IsCode(err, errs.CodeAgentNotFound) {
		t.Errorf("expected not found after unregister, got %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry(testLogger())
	reg.Register(NewBaseAgent(AgentCard{ID: "a", Name: "A"}, echoHandler, testSkillReg(), testLogger()))
	reg.Register(NewBaseAgent(AgentCard{ID: "b", Name: "B"}, echoHandler, testSkillReg(), testLogger()))

	cards := reg.List()
	if len(cards) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(cards))
	}
}

func TestRegistry_HealthCheckAll(t *testing.T) {
	reg := NewRegistry(testLogger())
	a1 := NewBaseAgent(AgentCard{ID: "h1"}, echoHandler, testSkillReg(), testLogger())
	a2 := NewBaseAgent(AgentCard{ID: "h2"}, echoHandler, testSkillReg(), testLogger())
	reg.Register(a1)
	reg.Register(a2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.StartAll(ctx)
	time.Sleep(20 * time.Millisecond)

	results := reg.HealthCheckAll(ctx)
	if len(results) != 2 {
		t.Fatalf("expected 2 health results, got %d", len(results))
	}

	for _, s := range results {
		if s.Status != StatusRunning {
			t.Errorf("expected running status for %s, got %s", s.AgentID, s.Status.String())
		}
	}
}
