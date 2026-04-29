package feishu

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/chef-guo/agents-hive/internal/observability"
)

type HealthStatus struct {
	Platform               string `json:"platform"`
	Status                 string `json:"status"`
	Degraded               bool   `json:"degraded"`
	BotOpenID              string `json:"bot_open_id,omitempty"`
	TokenConfigured        bool   `json:"token_configured"`
	EncryptKeyConfigured   bool   `json:"encrypt_key_configured"`
	VerificationConfigured bool   `json:"verification_configured"`
	PermissionDeniedCount  int    `json:"permission_denied_count,omitempty"`
	LastAPIError           string `json:"last_api_error,omitempty"`
}

const permissionDeniedWindow = 5 * time.Minute

type clientHealthTracker struct {
	mu                         sync.Mutex
	lastAPIError               error
	permissionDeniedAt         []time.Time
	permissionDegradeThreshold int
	tokenConfigured            bool
	encryptKeyConfigured       bool
	verificationConfigured     bool
	degradeMetricEmitted       bool
}

func (c *Client) HealthStatus(ctx context.Context) HealthStatus {
	status := HealthStatus{
		Platform: "feishu",
		Status:   "healthy",
		Degraded: false,
	}
	if c == nil {
		status.Status = "disabled"
		return status
	}
	if c.health == nil {
		c.health = &clientHealthTracker{permissionDegradeThreshold: 5}
	}
	now := time.Now()
	c.health.mu.Lock()
	c.health.permissionDeniedAt = prunePermissionDeniedLocked(c.health.permissionDeniedAt, now)
	status.PermissionDeniedCount = len(c.health.permissionDeniedAt)
	status.TokenConfigured = c.health.tokenConfigured
	status.EncryptKeyConfigured = c.health.encryptKeyConfigured
	status.VerificationConfigured = c.health.verificationConfigured
	if c.health.lastAPIError != nil {
		status.LastAPIError = c.health.lastAPIError.Error()
	}
	threshold := c.health.permissionDegradeThreshold
	if threshold <= 0 {
		threshold = 5
	}
	if threshold > 0 && len(c.health.permissionDeniedAt) >= threshold {
		status.Status = "degraded"
		status.Degraded = true
	}
	c.health.mu.Unlock()
	if c.larkClient != nil {
		status.BotOpenID = c.BotOpenID()
	}
	return status
}

func (c *Client) ApplySecurityConfig(permissionDegradeThreshold int) {
	if c == nil {
		return
	}
	if permissionDegradeThreshold <= 0 {
		permissionDegradeThreshold = 5
	}
	if c.health == nil {
		c.health = &clientHealthTracker{}
	}
	c.health.mu.Lock()
	c.health.permissionDegradeThreshold = permissionDegradeThreshold
	c.health.mu.Unlock()
}

func (c *Client) ApplyHealthConfig(appID, appSecret, verificationToken, encryptKey string) {
	if c == nil {
		return
	}
	if c.health == nil {
		c.health = &clientHealthTracker{permissionDegradeThreshold: 5}
	}
	c.health.mu.Lock()
	c.health.tokenConfigured = strings.TrimSpace(appID) != "" && strings.TrimSpace(appSecret) != ""
	c.health.encryptKeyConfigured = strings.TrimSpace(encryptKey) != ""
	c.health.verificationConfigured = strings.TrimSpace(verificationToken) != ""
	c.health.mu.Unlock()
}

func (c *Client) observeAPIError(err error, now time.Time) {
	if c == nil || err == nil {
		return
	}
	if c.health == nil {
		c.health = &clientHealthTracker{permissionDegradeThreshold: 5}
	}
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	c.health.lastAPIError = err
	if isPermissionDeniedError(err) {
		c.health.permissionDeniedAt = append(c.health.permissionDeniedAt, now)
		c.health.permissionDeniedAt = prunePermissionDeniedLocked(c.health.permissionDeniedAt, now)
		threshold := c.health.permissionDegradeThreshold
		if threshold <= 0 {
			threshold = 5
		}
		if threshold > 0 && len(c.health.permissionDeniedAt) >= threshold && !c.health.degradeMetricEmitted {
			c.emitBotDegradedMetricLocked()
			c.health.degradeMetricEmitted = true
		}
	}
}

func (c *Client) ObserveAPIErrorForTest(err error, now time.Time) {
	c.observeAPIError(err, now)
}

func (c *Client) permissionDeniedCount(now time.Time) int {
	if c == nil || c.health == nil {
		return 0
	}
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	c.health.permissionDeniedAt = prunePermissionDeniedLocked(c.health.permissionDeniedAt, now)
	return len(c.health.permissionDeniedAt)
}

func (c *Client) permissionDegradeThreshold() int {
	if c == nil || c.health == nil {
		return 5
	}
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	if c.health.permissionDegradeThreshold <= 0 {
		return 5
	}
	return c.health.permissionDegradeThreshold
}

func (c *Client) degraded(now time.Time) bool {
	threshold := c.permissionDegradeThreshold()
	return threshold > 0 && c.permissionDeniedCount(now) >= threshold
}

func (c *Client) emitBotDegradedMetricLocked() {
	if c == nil || c.health == nil || c.metricsWriter == nil {
		return
	}
	_ = c.metricsWriter.Record(context.Background(), observability.Metric{
		Name:  MetricBotDegraded,
		Value: 1,
		Ts:    time.Now(),
	})
}

func prunePermissionDeniedLocked(events []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-permissionDeniedWindow)
	kept := events[:0]
	for _, ts := range events {
		if !ts.Before(cutoff) {
			kept = append(kept, ts)
		}
	}
	return kept
}

func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return errors.Is(err, ErrPermissionDenied) ||
		strings.Contains(msg, "code=99991663") ||
		strings.Contains(msg, "code=10013") ||
		strings.Contains(strings.ToLower(msg), "permission denied")
}
