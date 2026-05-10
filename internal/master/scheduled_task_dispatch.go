package master

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/chef-guo/agents-hive/internal/store"
)

var scheduledTaskRetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

type scheduledTaskUserResolver interface {
	GetUserByID(ctx context.Context, userID string) (*auth.User, error)
}

type scheduledTaskPushService interface {
	DispatchScheduledPrompt(ctx context.Context, prompt string) error
	DispatchScheduledConfig(ctx context.Context, platform string, targetConfig map[string]any, prompt string) error
}

func (m *Master) SetScheduledTaskUserResolver(resolver scheduledTaskUserResolver) {
	if m == nil {
		return
	}
	m.cronMu.Lock()
	m.scheduledTaskUserResolver = resolver
	m.cronMu.Unlock()
}

func (m *Master) SetScheduledTaskPushService(service scheduledTaskPushService) {
	if m == nil {
		return
	}
	m.cronMu.Lock()
	m.scheduledTaskPushService = service
	m.cronMu.Unlock()
}

func (m *Master) RecordScheduledTaskMetric(name string, labels map[string]any) {
	if m == nil {
		return
	}
	m.enqueueMetric(observability.Metric{
		Name:   name,
		Value:  1,
		Labels: labels,
		Ts:     time.Now().UTC(),
	})
}

func (m *Master) DispatchScheduledTask(ctx context.Context, task store.ScheduledTask, runID string) (string, string, error) {
	ctx, err := m.scheduledTaskOwnerContext(ctx, task)
	if err != nil {
		return "", "", err
	}
	switch task.TargetType {
	case "im_push":
		return m.dispatchScheduledIMPush(ctx, task)
	case "session":
		return m.dispatchScheduledSession(ctx, task, runID)
	default:
		return "", "", fmt.Errorf("unsupported scheduled task target_type: %s", task.TargetType)
	}
}

func (m *Master) DispatchScheduledTaskWithRetry(ctx context.Context, task store.ScheduledTask, runID string) (string, string, int, error) {
	return m.dispatchScheduledTaskWithRetry(ctx, task, runID, scheduledTaskRetryDelays)
}

func (m *Master) dispatchScheduledTaskWithRetry(ctx context.Context, task store.ScheduledTask, runID string, retryDelays []time.Duration) (string, string, int, error) {
	var lastSessionID string
	var lastOutput string
	var lastErr error
	attempts := 0
	for {
		attempts++
		sessionID, output, err := m.DispatchScheduledTask(ctx, task, runID)
		if sessionID != "" {
			lastSessionID = sessionID
		}
		if output != "" {
			lastOutput = output
		}
		if err == nil {
			return sessionID, output, attempts, nil
		}
		lastErr = err
		if attempts > len(retryDelays) {
			return lastSessionID, lastOutput, attempts, lastErr
		}
		delay := retryDelays[attempts-1]
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastSessionID, lastOutput, attempts, lastErr
		case <-timer.C:
		}
	}
}

func (m *Master) dispatchScheduledIMPush(ctx context.Context, task store.ScheduledTask) (string, string, error) {
	if m == nil {
		return "", "", fmt.Errorf("master not initialized")
	}
	m.cronMu.Lock()
	service := m.scheduledTaskPushService
	m.cronMu.Unlock()
	if service == nil {
		return "", "", fmt.Errorf("scheduled task push service not configured")
	}
	if strings.HasPrefix(strings.TrimSpace(task.Prompt), "scheduled_push:") && len(task.TargetConfig) == 0 {
		if err := service.DispatchScheduledPrompt(ctx, task.Prompt); err != nil {
			return "", "", err
		}
		return "", "push dispatched", nil
	}
	if err := service.DispatchScheduledConfig(ctx, task.Platform, task.TargetConfig, task.Prompt); err != nil {
		return "", "", err
	}
	return "", "push dispatched", nil
}

func (m *Master) scheduledTaskOwnerContext(ctx context.Context, task store.ScheduledTask) (context.Context, error) {
	if m == nil {
		return ctx, fmt.Errorf("master not initialized")
	}
	m.cronMu.Lock()
	resolver := m.scheduledTaskUserResolver
	m.cronMu.Unlock()
	var user *auth.User
	if resolver == nil {
		if task.CreatedBy != "" {
			return ctx, fmt.Errorf("scheduled task user resolver not configured")
		}
		user = &auth.User{ID: "", Role: "admin", Status: "active", DisplayName: "Local Admin"}
	} else {
		resolved, err := resolver.GetUserByID(ctx, task.CreatedBy)
		if err != nil {
			return ctx, fmt.Errorf("scheduled task %s owner lookup failed: %w", task.ID, err)
		}
		user = resolved
	}
	if user == nil || user.Status != "active" {
		return ctx, fmt.Errorf("scheduled task %s owner is missing or inactive", task.ID)
	}
	return auth.WithUser(auth.WithAuthEnabled(ctx), user), nil
}

func (m *Master) dispatchScheduledSession(ctx context.Context, task store.ScheduledTask, runID string) (string, string, error) {
	sessionID := fmt.Sprintf("scheduled-%s-%s", task.ID, runID)
	resp, err := m.sessionMgr.ProcessRequestWithResponse(ctx, SessionRequest{
		SessionID: sessionID,
		Input:     task.Prompt,
	})
	if err != nil {
		return sessionID, "", err
	}
	return sessionID, resp.Content, nil
}
