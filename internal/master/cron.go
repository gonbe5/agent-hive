package master

import (
	"context"
	"fmt"
	"time"
)

// CronJob 是当前进程内的最小定时任务定义。
// 一期只支持固定间隔调度；后续若仓内引入真正 cron parser，再扩展为 CronExpr。
type CronJob struct {
	Name     string
	ID       string
	Interval time.Duration
	Prompt   string
	Callback func(context.Context) error
}

type cronJobState struct {
	job    CronJob
	cancel context.CancelFunc
}

func (m *Master) CronCreate(job CronJob) error {
	if m == nil {
		return fmt.Errorf("master not initialized")
	}
	if job.Name == "" {
		return fmt.Errorf("cron job name is required")
	}
	if job.Interval <= 0 {
		return fmt.Errorf("cron job interval must be positive")
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
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			_ = callback(context.Background())
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
