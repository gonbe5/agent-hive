package master

import (
	"context"
	"strings"
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

func TestValidateScheduleSpec_RejectsHighFrequencyForUser(t *testing.T) {
	err := ValidateScheduleSpec(ScheduleSpec{Interval: 30 * time.Second, Timezone: "UTC"}, ScheduledTaskDefaultMinInterval)
	if err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("ValidateScheduleSpec error = %v, want min interval error", err)
	}
}

func TestValidateScheduleSpec_AllowsCronWithTimezone(t *testing.T) {
	err := ValidateScheduleSpec(ScheduleSpec{CronExpr: "0 9 * * *", Timezone: "Asia/Shanghai"}, ScheduledTaskDefaultMinInterval)
	if err != nil {
		t.Fatalf("ValidateScheduleSpec cron error = %v", err)
	}
	next, err := NextScheduledRun(ScheduleSpec{CronExpr: "0 9 * * *", Timezone: "Asia/Shanghai"}, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NextScheduledRun error = %v", err)
	}
	want := time.Date(2026, 5, 10, 1, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %v, want %v", next, want)
	}
}

func TestValidateScheduleSpec_RejectsInvalidCronAndTimezone(t *testing.T) {
	if err := ValidateScheduleSpec(ScheduleSpec{CronExpr: "bad", Timezone: "UTC"}, ScheduledTaskDefaultMinInterval); err == nil {
		t.Fatal("expected invalid cron error")
	}
	if err := ValidateScheduleSpec(ScheduleSpec{CronExpr: "0 9 * * *", Timezone: "No/SuchZone"}, ScheduledTaskDefaultMinInterval); err == nil {
		t.Fatal("expected invalid timezone error")
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
