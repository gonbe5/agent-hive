package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPgInitSQLScheduledTaskSchema(t *testing.T) {
	sql := strings.Join(strings.Fields(pgInitSQL), " ")
	needles := []string{
		"ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS target_type TEXT NOT NULL DEFAULT 'im_push'",
		"ADD COLUMN IF NOT EXISTS target_config JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN IF NOT EXISTS cron_expr TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC'",
		"ADD COLUMN IF NOT EXISTS active_run_id TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ",
		"conname = 'scheduled_pushes_target_type_check' AND conrelid = 'scheduled_pushes'::regclass",
		"conname = 'scheduled_pushes_schedule_check' AND conrelid = 'scheduled_pushes'::regclass",
		"CREATE INDEX IF NOT EXISTS idx_scheduled_pushes_user_enabled ON scheduled_pushes(created_by, enabled, next_run_at)",
		"CREATE TABLE IF NOT EXISTS scheduled_task_runs",
		"PARTITION BY RANGE (scheduled_at)",
	}
	for _, needle := range needles {
		if !strings.Contains(sql, needle) {
			t.Fatalf("pgInitSQL missing %q", needle)
		}
	}
	if strings.Contains(sql, "HAVING COUNT(*) > 1; CREATE UNIQUE INDEX") {
		t.Fatalf("pgInitSQL must not include a bare duplicate-name preflight SELECT before CREATE UNIQUE INDEX")
	}
	if !strings.Contains(strings.Join(strings.Fields(pgScheduledTaskUniqueNameIndexSQL), " "), "CREATE UNIQUE INDEX IF NOT EXISTS idx_scheduled_pushes_user_name ON scheduled_pushes(created_by, name)") {
		t.Fatalf("scheduled task unique-name index helper missing CREATE UNIQUE INDEX")
	}
}

func TestMemoryStoreScheduledTaskCRUDFiltersByUser(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	next := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)

	task := &ScheduledTask{
		ID:           "task-1",
		Name:         "daily",
		Description:  "daily report",
		TargetType:   "session",
		TargetConfig: map[string]any{"session_name": "report"},
		Prompt:       "make report",
		CronExpr:     "0 9 * * *",
		Timezone:     "Asia/Shanghai",
		Enabled:      true,
		CreatedBy:    "u1",
		NextRunAt:    &next,
	}
	if err := m.SaveScheduledTask(ctx, task); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
	task.TargetConfig["session_name"] = "mutated"

	got, err := m.GetScheduledTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetScheduledTask: %v", err)
	}
	if got.TargetConfig["session_name"] != "report" {
		t.Fatalf("TargetConfig was not copied: %#v", got.TargetConfig)
	}

	if err := m.SaveScheduledTask(ctx, &ScheduledTask{ID: "task-2", Name: "other", TargetType: "im_push", IntervalSec: 60, Enabled: true, CreatedBy: "u2"}); err != nil {
		t.Fatalf("SaveScheduledTask task-2: %v", err)
	}
	listed, err := m.ListScheduledTasksByUser(ctx, "u1")
	if err != nil {
		t.Fatalf("ListScheduledTasksByUser: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "task-1" {
		t.Fatalf("unexpected user-filtered tasks: %#v", listed)
	}
	enabled, err := m.ListEnabledScheduledTasks(ctx)
	if err != nil {
		t.Fatalf("ListEnabledScheduledTasks: %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("enabled task count = %d, want 2", len(enabled))
	}
	if err := m.DeleteScheduledTask(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteScheduledTask: %v", err)
	}
	if _, err := m.GetScheduledTask(ctx, "task-1"); err != ErrNotFound {
		t.Fatalf("GetScheduledTask after delete error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreScheduledTaskDeleteDoesNotFilterTargetType(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:          "task-session-delete",
		Name:        "session-delete",
		TargetType:  "session",
		Prompt:      "run",
		IntervalSec: 60,
		Timezone:    "UTC",
		Enabled:     true,
		CreatedBy:   "u1",
	}); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
	if err := m.DeleteScheduledTask(ctx, "task-session-delete"); err != nil {
		t.Fatalf("DeleteScheduledTask session task: %v", err)
	}
	if _, err := m.GetScheduledTask(ctx, "task-session-delete"); err != ErrNotFound {
		t.Fatalf("GetScheduledTask after delete error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreScheduledTaskSavePreservesRuntimeLease(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	leaseUntil := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	lastRunAt := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:             "task-lease",
		Name:           "lease",
		TargetType:     "session",
		Prompt:         "run",
		IntervalSec:    60,
		Timezone:       "UTC",
		Enabled:        true,
		CreatedBy:      "u1",
		ActiveRunID:    "run-1",
		LeaseExpiresAt: &leaseUntil,
		LastRunAt:      &lastRunAt,
		LastError:      "running",
	}); err != nil {
		t.Fatalf("SaveScheduledTask initial: %v", err)
	}
	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:          "task-lease",
		Name:        "lease-renamed",
		TargetType:  "session",
		Prompt:      "run updated",
		IntervalSec: 60,
		Timezone:    "UTC",
		Enabled:     false,
		CreatedBy:   "u1",
	}); err != nil {
		t.Fatalf("SaveScheduledTask update: %v", err)
	}
	got, err := m.GetScheduledTask(ctx, "task-lease")
	if err != nil {
		t.Fatalf("GetScheduledTask: %v", err)
	}
	if got.ActiveRunID != "run-1" || got.LeaseExpiresAt == nil || !got.LeaseExpiresAt.Equal(leaseUntil) {
		t.Fatalf("SaveScheduledTask must preserve runtime lease: %+v", got)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(lastRunAt) || got.LastError != "running" {
		t.Fatalf("SaveScheduledTask must preserve runtime status: %+v", got)
	}
	if got.Enabled {
		t.Fatalf("config fields should still update: %+v", got)
	}
}

func TestMemoryStoreScheduledTaskClaimAndFinish(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	dueAt := now.Add(-time.Minute)
	leaseUntil := now.Add(30 * time.Minute)

	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:          "task-claim",
		Name:        "claim",
		TargetType:  "im_push",
		IntervalSec: 3600,
		Platform:    "feishu",
		Prompt:      "run",
		Timezone:    "UTC",
		Enabled:     true,
		CreatedBy:   "u1",
		NextRunAt:   &dueAt,
	}); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}

	run, err := m.ClaimDueScheduledTaskRun(ctx, "task-claim", now, "run-1", leaseUntil, next, "worker-1")
	if err != nil {
		t.Fatalf("ClaimDueScheduledTaskRun: %v", err)
	}
	if run.Status != "running" || run.ScheduledAt != dueAt {
		t.Fatalf("unexpected run after claim: %#v", run)
	}
	if _, err := m.ClaimDueScheduledTaskRun(ctx, "task-claim", now, "run-2", leaseUntil, next.Add(time.Hour), "worker-2"); err != ErrNotFound {
		t.Fatalf("second claim error = %v, want ErrNotFound", err)
	}
	task, err := m.GetScheduledTask(ctx, "task-claim")
	if err != nil {
		t.Fatalf("GetScheduledTask after claim: %v", err)
	}
	if task.ActiveRunID != "run-1" || task.LeaseExpiresAt == nil || !task.NextRunAt.Equal(next) {
		t.Fatalf("task lease/next not updated: %#v", task)
	}

	run.Status = "succeeded"
	run.Output = "ok"
	run.AttemptCount = 1
	if err := m.FinishScheduledTaskRun(ctx, run); err != nil {
		t.Fatalf("FinishScheduledTaskRun: %v", err)
	}
	task, err = m.GetScheduledTask(ctx, "task-claim")
	if err != nil {
		t.Fatalf("GetScheduledTask after finish: %v", err)
	}
	if task.ActiveRunID != "" || task.LeaseExpiresAt != nil || task.LastError != "" {
		t.Fatalf("task lease not cleared: %#v", task)
	}

	manual, err := m.ClaimManualScheduledTaskRun(ctx, "task-claim", now.Add(time.Minute), "run-manual", leaseUntil.Add(time.Minute), "worker-1")
	if err != nil {
		t.Fatalf("ClaimManualScheduledTaskRun: %v", err)
	}
	task, err = m.GetScheduledTask(ctx, "task-claim")
	if err != nil {
		t.Fatalf("GetScheduledTask after manual claim: %v", err)
	}
	if !task.NextRunAt.Equal(next) {
		t.Fatalf("manual claim changed next_run_at: got %v want %v", task.NextRunAt, next)
	}
	manual.Status = "failed"
	manual.Error = "boom"
	if err := m.FinishScheduledTaskRun(ctx, manual); err != nil {
		t.Fatalf("FinishScheduledTaskRun manual: %v", err)
	}
	task, err = m.GetScheduledTask(ctx, "task-claim")
	if err != nil {
		t.Fatalf("GetScheduledTask after manual finish: %v", err)
	}
	if task.LastError != "boom" {
		t.Fatalf("LastError = %q, want boom", task.LastError)
	}
}

func TestMemoryStoreBulkMarkScheduledTaskReloadFailures(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	for _, id := range []string{"bad-task", "good-task"} {
		if err := m.SaveScheduledTask(ctx, &ScheduledTask{
			ID:          id,
			Name:        id,
			TargetType:  "session",
			Prompt:      "run",
			IntervalSec: 60,
			Timezone:    "UTC",
			Enabled:     true,
			CreatedBy:   "u1",
		}); err != nil {
			t.Fatalf("SaveScheduledTask %s: %v", id, err)
		}
	}
	if err := m.BulkMarkScheduledTaskReloadFailures(ctx, map[string]string{"bad-task": "bad cron"}); err != nil {
		t.Fatalf("BulkMarkScheduledTaskReloadFailures: %v", err)
	}
	bad, err := m.GetScheduledTask(ctx, "bad-task")
	if err != nil {
		t.Fatalf("GetScheduledTask bad: %v", err)
	}
	if bad.Enabled || bad.LastError != "bad cron" {
		t.Fatalf("bad task not marked as reload failure: %+v", bad)
	}
	good, err := m.GetScheduledTask(ctx, "good-task")
	if err != nil {
		t.Fatalf("GetScheduledTask good: %v", err)
	}
	if !good.Enabled || good.LastError != "" {
		t.Fatalf("good task must remain untouched: %+v", good)
	}
}

func TestMemoryStoreScheduledTaskAutoDisableAfterFiveFailures(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:          "task-fail",
		Name:        "fail",
		TargetType:  "session",
		IntervalSec: 60,
		Prompt:      "run",
		Timezone:    "UTC",
		Enabled:     true,
		CreatedBy:   "u1",
	}); err != nil {
		t.Fatalf("SaveScheduledTask: %v", err)
	}
	for i := 0; i < 4; i++ {
		run := &ScheduledTaskRun{
			ScheduledAt: now.Add(time.Duration(i) * time.Minute),
			ID:          "run-pre-" + string(rune('a'+i)),
			TaskID:      "task-fail",
			StartedAt:   now,
			Status:      "failed",
			Error:       "boom",
		}
		m.taskRuns[scheduledTaskRunKey(run.ScheduledAt, run.ID)] = cloneScheduledTaskRun(run)
		if err := m.FinishScheduledTaskRun(ctx, run); err != nil {
			t.Fatalf("FinishScheduledTaskRun %d: %v", i, err)
		}
		task, err := m.GetScheduledTask(ctx, "task-fail")
		if err != nil {
			t.Fatalf("GetScheduledTask: %v", err)
		}
		if !task.Enabled {
			t.Fatalf("task disabled after %d failures, want still enabled", i+1)
		}
	}
	finalRun := &ScheduledTaskRun{
		ScheduledAt: now.Add(5 * time.Minute),
		ID:          "run-final",
		TaskID:      "task-fail",
		StartedAt:   now,
		Status:      "timeout",
		Error:       "timeout",
	}
	m.taskRuns[scheduledTaskRunKey(finalRun.ScheduledAt, finalRun.ID)] = cloneScheduledTaskRun(finalRun)
	if err := m.FinishScheduledTaskRun(ctx, finalRun); err != nil {
		t.Fatalf("FinishScheduledTaskRun final: %v", err)
	}
	task, err := m.GetScheduledTask(ctx, "task-fail")
	if err != nil {
		t.Fatalf("GetScheduledTask after final: %v", err)
	}
	if task.Enabled {
		t.Fatal("task still enabled after 5 consecutive failures")
	}
	if task.LastError != "最近 5 次执行均失败,已自动停用" {
		t.Fatalf("LastError = %q", task.LastError)
	}
}

func TestMemoryStoreScheduledPushFiltersOutSessionTasks(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	if err := m.SaveScheduledTask(ctx, &ScheduledTask{
		ID:          "task-session",
		Name:        "session",
		TargetType:  "session",
		Platform:    "feishu",
		Prompt:      "run",
		IntervalSec: 60,
		Timezone:    "UTC",
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveScheduledTask session: %v", err)
	}
	if err := m.SaveScheduledPush(ctx, &ScheduledPushRecord{
		ID:          "task-push",
		Name:        "push",
		Platform:    "feishu",
		Prompt:      "scheduled_push:task_done:chat_id=oc_1:title=ok:summary=done",
		IntervalSec: 60,
		Enabled:     true,
	}); err != nil {
		t.Fatalf("SaveScheduledPush: %v", err)
	}
	if _, err := m.GetScheduledPush(ctx, "task-session"); err != ErrNotFound {
		t.Fatalf("GetScheduledPush session error = %v, want ErrNotFound", err)
	}
	listed, err := m.ListScheduledPushes(ctx, "feishu")
	if err != nil {
		t.Fatalf("ListScheduledPushes: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "task-push" {
		t.Fatalf("unexpected scheduled pushes: %#v", listed)
	}
	if err := m.DeleteScheduledPush(ctx, "task-session"); err != ErrNotFound {
		t.Fatalf("DeleteScheduledPush session error = %v, want ErrNotFound", err)
	}
	if _, err := m.GetScheduledTask(ctx, "task-session"); err != nil {
		t.Fatalf("session task was deleted by legacy push path: %v", err)
	}
}

func TestScheduledTaskPartitionStart(t *testing.T) {
	got, ok := scheduledTaskPartitionStart("scheduled_task_runs_2026_w19")
	if !ok {
		t.Fatal("expected valid partition name")
	}
	want := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("partition start = %v, want %v", got, want)
	}
	if _, ok := scheduledTaskPartitionStart("scheduled_task_runs_2026_w54"); ok {
		t.Fatal("week 54 must be invalid")
	}
}
