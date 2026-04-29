package master

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCronCreate_ExecutesScheduledCallback(t *testing.T) {
	m := &Master{
		logger: zap.NewNop(),
		stopCh: make(chan struct{}),
	}

	var calls atomic.Int32
	err := m.CronCreate(CronJob{
		Name:     "push-test",
		Interval: 20 * time.Millisecond,
		Callback: func(context.Context) error {
			calls.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("CronCreate() error = %v", err)
	}
	defer m.StopCron("push-test")

	time.Sleep(70 * time.Millisecond)
	if calls.Load() < 2 {
		t.Fatalf("calls = %d, want >= 2", calls.Load())
	}
}

func TestCronCreate_DispatchesPromptToScheduledDispatcher(t *testing.T) {
	m := &Master{
		logger:   zap.NewNop(),
		stopCh:   make(chan struct{}),
		cronJobs: make(map[string]*cronJobState),
	}

	var calls atomic.Int32
	var lastPrompt atomic.Value
	m.SetScheduledPromptDispatcher(func(_ context.Context, prompt string) error {
		lastPrompt.Store(prompt)
		calls.Add(1)
		return nil
	})

	err := m.CronCreate(CronJob{
		Name:     "scheduled-prompt",
		Interval: 20 * time.Millisecond,
		Prompt:   "scheduled_push:task_done:chat_id=oc_sched:title=日报生成完成:summary=请查收",
	})
	if err != nil {
		t.Fatalf("CronCreate() error = %v", err)
	}
	defer m.StopCron("scheduled-prompt")

	time.Sleep(70 * time.Millisecond)
	if calls.Load() < 2 {
		t.Fatalf("calls = %d, want >= 2", calls.Load())
	}
	if got, _ := lastPrompt.Load().(string); got != "scheduled_push:task_done:chat_id=oc_sched:title=日报生成完成:summary=请查收" {
		t.Fatalf("prompt = %q, want scheduled push prompt", got)
	}
}
