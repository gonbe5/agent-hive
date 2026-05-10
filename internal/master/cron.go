package master

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// CronJob 是当前进程内的最小定时任务定义。
type CronJob struct {
	Name     string
	ID       string
	Interval time.Duration
	Schedule ScheduleSpec
	Prompt   string
	Callback func(context.Context) error
}

type ScheduleSpec struct {
	Interval time.Duration
	CronExpr string
	Timezone string
}

type cronJobState struct {
	job    CronJob
	cancel context.CancelFunc
}

const (
	ScheduledTaskDefaultMinInterval = time.Minute
	ScheduledTaskAdminMinInterval   = 10 * time.Second
)

func (m *Master) CronCreate(job CronJob) error {
	if m == nil {
		return fmt.Errorf("master not initialized")
	}
	if job.Name == "" {
		return fmt.Errorf("cron job name is required")
	}
	spec := job.Schedule
	if spec.Interval == 0 && job.Interval > 0 {
		spec.Interval = job.Interval
	}
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	if err := ValidateScheduleSpec(spec, time.Nanosecond); err != nil {
		return err
	}
	job.Schedule = spec
	if job.Interval == 0 {
		job.Interval = spec.Interval
	}
	callback := job.Callback
	if callback == nil && job.Prompt != "" && m.scheduledPromptDispatcher != nil {
		callback = func(ctx context.Context) error {
			return m.scheduledPromptDispatcher(ctx, job.Prompt)
		}
	}
	if callback == nil {
		return fmt.Errorf("cron job callback is required")
	}

	m.cronMu.Lock()
	defer m.cronMu.Unlock()
	if m.cronJobs == nil {
		m.cronJobs = make(map[string]*cronJobState)
	}
	if _, exists := m.cronJobs[job.Name]; exists {
		return fmt.Errorf("cron job already exists: %s", job.Name)
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &cronJobState{job: job, cancel: cancel}
	m.cronJobs[job.Name] = state
	go m.runCronJob(ctx, job, callback)
	return nil
}

func (m *Master) runCronJob(ctx context.Context, job CronJob, callback func(context.Context) error) {
	next := func(now time.Time) (time.Time, error) {
		return NextScheduledRun(job.Schedule, now)
	}
	for {
		nextRun, err := next(time.Now())
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("计算定时任务下一次运行时间失败", zap.String("cron_name", job.Name), zap.Error(err))
			}
			return
		}
		delay := time.Until(nextRun)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			if err := callback(context.Background()); err != nil && m.logger != nil {
				m.logger.Warn("定时任务执行失败", zap.String("cron_name", job.Name), zap.Error(err))
			}
		}
	}
}

func (m *Master) StopCron(name string) {
	if m == nil {
		return
	}
	m.cronMu.Lock()
	defer m.cronMu.Unlock()
	if state, ok := m.cronJobs[name]; ok {
		state.cancel()
		delete(m.cronJobs, name)
	}
}

func (m *Master) ListCrons() []CronJob {
	if m == nil {
		return nil
	}
	m.cronMu.Lock()
	defer m.cronMu.Unlock()
	out := make([]CronJob, 0, len(m.cronJobs))
	for _, state := range m.cronJobs {
		out = append(out, state.job)
	}
	return out
}

func (m *Master) SetScheduledPromptDispatcher(fn func(context.Context, string) error) {
	if m == nil {
		return
	}
	m.cronMu.Lock()
	m.scheduledPromptDispatcher = fn
	m.cronMu.Unlock()
}

func ValidateScheduleSpec(spec ScheduleSpec, minInterval time.Duration) error {
	hasInterval := spec.Interval > 0
	hasCron := strings.TrimSpace(spec.CronExpr) != ""
	if hasInterval == hasCron {
		return fmt.Errorf("schedule requires exactly one of interval or cron_expr")
	}
	if minInterval <= 0 {
		minInterval = time.Second
	}
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(spec.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q: %w", spec.Timezone, err)
	}
	if hasInterval {
		if spec.Interval < minInterval {
			return fmt.Errorf("schedule interval must be at least %s", minInterval)
		}
		return nil
	}
	next, second, err := nextTwoCronRuns(spec, time.Now())
	if err != nil {
		return err
	}
	if second.Sub(next) < minInterval {
		return fmt.Errorf("cron schedule interval must be at least %s", minInterval)
	}
	return nil
}

func NextScheduledRun(spec ScheduleSpec, after time.Time) (time.Time, error) {
	if spec.Timezone == "" {
		spec.Timezone = "UTC"
	}
	if spec.Interval > 0 {
		return after.UTC().Add(spec.Interval), nil
	}
	next, _, err := nextTwoCronRuns(spec, after)
	return next, err
}

func nextTwoCronRuns(spec ScheduleSpec, after time.Time) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(spec.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timezone %q: %w", spec.Timezone, err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(strings.TrimSpace(spec.CronExpr))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid cron_expr: %w", err)
	}
	localAfter := after.In(loc)
	next := schedule.Next(localAfter)
	second := schedule.Next(next)
	return next.UTC(), second.UTC(), nil
}
