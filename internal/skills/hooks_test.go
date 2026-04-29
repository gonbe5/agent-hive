package skills

import (
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

type hookTestExecutor struct {
	calls []string
	fail  map[string]bool
}

func (e *hookTestExecutor) Execute(command string) (string, string, error) {
	e.calls = append(e.calls, command)
	if e.fail != nil && e.fail[command] {
		return "", "", errs.New(errs.CodeSkillHookFailed, "command failed: "+command)
	}
	return "ok", "", nil
}

func TestHookRunner_PreInvoke(t *testing.T) {
	exec := &hookTestExecutor{}
	runner := NewHookRunner(exec, zap.NewNop())

	hooks := &HookConfig{
		PreInvoke: []string{"echo pre1", "echo pre2"},
	}

	err := runner.RunPreInvoke(hooks, "/skill/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(exec.calls))
	}
}

func TestHookRunner_PostInvoke(t *testing.T) {
	exec := &hookTestExecutor{}
	runner := NewHookRunner(exec, zap.NewNop())

	hooks := &HookConfig{
		PostInvoke: []string{"echo post"},
	}

	err := runner.RunPostInvoke(hooks, "/skill/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(exec.calls))
	}
}

func TestHookRunner_PreInvoke_NilHooks(t *testing.T) {
	exec := &hookTestExecutor{}
	runner := NewHookRunner(exec, zap.NewNop())

	err := runner.RunPreInvoke(nil, "/skill/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(exec.calls))
	}
}

func TestHookRunner_PreInvoke_EmptyHooks(t *testing.T) {
	exec := &hookTestExecutor{}
	runner := NewHookRunner(exec, zap.NewNop())

	hooks := &HookConfig{}
	err := runner.RunPreInvoke(hooks, "/skill/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(exec.calls))
	}
}

func TestHookRunner_PreInvoke_StopsOnError(t *testing.T) {
	exec := &hookTestExecutor{
		fail: map[string]bool{
			"cd \"/skill/dir\" && fail-cmd": true,
		},
	}
	runner := NewHookRunner(exec, zap.NewNop())

	hooks := &HookConfig{
		PreInvoke: []string{"fail-cmd", "never-reached"},
	}

	err := runner.RunPreInvoke(hooks, "/skill/dir")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsCode(err, errs.CodeSkillHookFailed) {
		t.Errorf("expected CodeSkillHookFailed, got %v", err)
	}
	if len(exec.calls) != 1 {
		t.Errorf("expected 1 call (stopped early), got %d", len(exec.calls))
	}
}

func TestHookRunner_ExecutionOrder(t *testing.T) {
	exec := &hookTestExecutor{}
	runner := NewHookRunner(exec, zap.NewNop())

	hooks := &HookConfig{
		PreInvoke:  []string{"echo pre"},
		PostInvoke: []string{"echo post"},
	}

	if err := runner.RunPreInvoke(hooks, "/dir"); err != nil {
		t.Fatal(err)
	}
	if err := runner.RunPostInvoke(hooks, "/dir"); err != nil {
		t.Fatal(err)
	}

	if len(exec.calls) != 2 {
		t.Fatalf("expected 2 total calls, got %d", len(exec.calls))
	}
}
