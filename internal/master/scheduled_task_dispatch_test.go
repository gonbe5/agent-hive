package master

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/store"
)

type scheduledTaskUserResolverFunc func(context.Context, string) (*auth.User, error)

func (f scheduledTaskUserResolverFunc) GetUserByID(ctx context.Context, userID string) (*auth.User, error) {
	return f(ctx, userID)
}

type scheduledTaskPushStub struct {
	promptCalled bool
	configCalled bool
	platform     string
	prompt       string
	cfg          map[string]any
	failuresLeft int
	calls        int
}

func (s *scheduledTaskPushStub) DispatchScheduledPrompt(_ context.Context, prompt string) error {
	s.calls++
	s.promptCalled = true
	s.prompt = prompt
	if s.failuresLeft > 0 {
		s.failuresLeft--
		return errors.New("temporary push failure")
	}
	return nil
}

func (s *scheduledTaskPushStub) DispatchScheduledConfig(_ context.Context, platform string, cfg map[string]any, prompt string) error {
	s.calls++
	s.configCalled = true
	s.platform = platform
	s.cfg = cfg
	s.prompt = prompt
	if s.failuresLeft > 0 {
		s.failuresLeft--
		return errors.New("temporary push failure")
	}
	return nil
}

func TestDispatchScheduledTask_IMPushUsesLegacyPrompt(t *testing.T) {
	pushSvc := &scheduledTaskPushStub{}
	m := &Master{}
	m.SetScheduledTaskPushService(pushSvc)

	_, out, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{
		ID:         "task-1",
		TargetType: "im_push",
		Prompt:     "scheduled_push:task_done:chat_id=oc_1:title=ok:summary=done",
	}, "run-1")
	if err != nil {
		t.Fatalf("DispatchScheduledTask error = %v", err)
	}
	if out != "push dispatched" || !pushSvc.promptCalled || pushSvc.configCalled {
		t.Fatalf("unexpected push dispatch state: out=%q stub=%+v", out, pushSvc)
	}
}

func TestDispatchScheduledTask_IMPushChecksOwner(t *testing.T) {
	pushSvc := &scheduledTaskPushStub{}
	m := &Master{}
	m.SetScheduledTaskPushService(pushSvc)
	m.SetScheduledTaskUserResolver(scheduledTaskUserResolverFunc(func(context.Context, string) (*auth.User, error) {
		return &auth.User{ID: "u1", Status: "disabled"}, nil
	}))

	_, _, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{
		ID:         "task-im-owner",
		TargetType: "im_push",
		CreatedBy:  "u1",
		Prompt:     "scheduled_push:task_done:chat_id=oc_1:title=ok:summary=done",
	}, "run-1")
	if err == nil {
		t.Fatal("expected inactive owner error")
	}
	if pushSvc.calls != 0 {
		t.Fatalf("push should not run when owner is inactive, calls=%d", pushSvc.calls)
	}
}

func TestDispatchScheduledTask_IMPushUsesTargetConfig(t *testing.T) {
	pushSvc := &scheduledTaskPushStub{}
	m := &Master{}
	m.SetScheduledTaskPushService(pushSvc)

	_, _, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{
		ID:           "task-1",
		TargetType:   "im_push",
		Platform:     "feishu",
		Prompt:       "hello",
		TargetConfig: map[string]any{"chat_id": "oc_1", "content": "hello"},
	}, "run-1")
	if err != nil {
		t.Fatalf("DispatchScheduledTask error = %v", err)
	}
	if !pushSvc.configCalled || pushSvc.platform != "feishu" || pushSvc.cfg["chat_id"] != "oc_1" {
		t.Fatalf("unexpected config dispatch: %+v", pushSvc)
	}
}

func TestDispatchScheduledTask_SessionOwnerMissingOrInactive(t *testing.T) {
	m := &Master{}
	m.SetScheduledTaskUserResolver(scheduledTaskUserResolverFunc(func(context.Context, string) (*auth.User, error) {
		return nil, nil
	}))
	_, _, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{ID: "task-1", TargetType: "session", CreatedBy: "u1"}, "run-1")
	if err == nil {
		t.Fatal("expected missing owner error")
	}

	m.SetScheduledTaskUserResolver(scheduledTaskUserResolverFunc(func(context.Context, string) (*auth.User, error) {
		return &auth.User{ID: "u1", Status: "disabled"}, nil
	}))
	_, _, err = m.DispatchScheduledTask(context.Background(), store.ScheduledTask{ID: "task-1", TargetType: "session", CreatedBy: "u1"}, "run-1")
	if err == nil {
		t.Fatal("expected inactive owner error")
	}
}

func TestDispatchScheduledTask_ResolverErrorAndUnsupportedTarget(t *testing.T) {
	m := &Master{}
	m.SetScheduledTaskUserResolver(scheduledTaskUserResolverFunc(func(context.Context, string) (*auth.User, error) {
		return nil, errors.New("db down")
	}))
	_, _, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{ID: "task-1", TargetType: "session", CreatedBy: "u1"}, "run-1")
	if err == nil {
		t.Fatal("expected resolver error")
	}
	_, _, err = m.DispatchScheduledTask(context.Background(), store.ScheduledTask{ID: "task-1", TargetType: "webhook"}, "run-1")
	if err == nil {
		t.Fatal("expected unsupported target error")
	}
}

func TestDispatchScheduledTaskWithRetryCountsAttempts(t *testing.T) {
	pushSvc := &scheduledTaskPushStub{failuresLeft: 2}
	m := &Master{}
	m.SetScheduledTaskPushService(pushSvc)

	_, output, attempts, err := m.dispatchScheduledTaskWithRetry(context.Background(), store.ScheduledTask{
		ID:         "task-retry",
		TargetType: "im_push",
		Prompt:     "scheduled_push:task_done:chat_id=oc_1:title=ok:summary=done",
	}, "run-1", []time.Duration{0, 0, 0})
	if err != nil {
		t.Fatalf("dispatchScheduledTaskWithRetry error = %v", err)
	}
	if attempts != 3 || pushSvc.calls != 3 || output != "push dispatched" {
		t.Fatalf("unexpected retry result: attempts=%d calls=%d output=%q", attempts, pushSvc.calls, output)
	}
}

func TestDispatchScheduledTaskWithRetryReturnsLastError(t *testing.T) {
	pushSvc := &scheduledTaskPushStub{failuresLeft: 4}
	m := &Master{}
	m.SetScheduledTaskPushService(pushSvc)

	_, _, attempts, err := m.dispatchScheduledTaskWithRetry(context.Background(), store.ScheduledTask{
		ID:         "task-retry-fail",
		TargetType: "im_push",
		Prompt:     "scheduled_push:task_done:chat_id=oc_1:title=ok:summary=done",
	}, "run-1", []time.Duration{0, 0, 0})
	if err == nil {
		t.Fatal("expected retry failure")
	}
	if attempts != 4 || pushSvc.calls != 4 {
		t.Fatalf("unexpected retry attempts=%d calls=%d", attempts, pushSvc.calls)
	}
}

func TestDispatchScheduledTask_NoAuthSessionUsesSyntheticOwner(t *testing.T) {
	stopCh := make(chan struct{})
	defer close(stopCh)
	sm := NewSessionManager(stopCh, nil)
	m := &Master{sessionMgr: sm}
	done := make(chan SessionRequest, 1)
	go func() {
		req := <-sm.requestCh
		done <- req
		sm.SendResponse(req.ResponseID, TaskResponse{Content: "ok", Status: string(TaskStatusCompleted)})
	}()

	sessionID, output, err := m.DispatchScheduledTask(context.Background(), store.ScheduledTask{
		ID:         "task-local",
		TargetType: "session",
		Prompt:     "run",
		CreatedBy:  "",
	}, "run-1")
	if err != nil {
		t.Fatalf("DispatchScheduledTask error = %v", err)
	}
	if output != "ok" || sessionID != "scheduled-task-local-run-1" {
		t.Fatalf("unexpected session dispatch result sessionID=%q output=%q", sessionID, output)
	}
	select {
	case req := <-done:
		user := auth.UserFrom(req.Ctx)
		if user == nil || user.Status != "active" || user.Role != "admin" {
			t.Fatalf("synthetic user not injected: %+v", user)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for scheduled session request")
	}
}
