package feishu

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type LongConnReliabilityStatus struct {
	LastEventAt                 time.Time
	LastTenantKey               string
	Reconnecting                bool
	ReliabilityLeader           bool
	GapFetchEnabled             bool
	GapFetchPending             bool
	PendingGapFetchWindowCapped bool
	PendingGapFetchWindow       GapFetchWindow
	HeartbeatStaleWindow        time.Duration
	GapFetchMaxWindow           time.Duration
	LastGapFetchAt              time.Time
	LastGapFetchChatIDs         []string
	LastGapFetchError           string
}

type longConnWatchdogConfig struct {
	enabled           bool
	staleAfter        time.Duration
	checkInterval     time.Duration
	onReconnectNeeded func()
	onRecovered       func()
	now               func() time.Time
}

func newLongConnWatchdogConfig(logger *zap.Logger) longConnWatchdogConfig {
	if logger == nil {
		logger = zap.NewNop()
	}
	return normalizeLongConnWatchdogConfig(longConnWatchdogConfig{
		enabled:       true,
		staleAfter:    2 * time.Minute,
		checkInterval: 15 * time.Second,
		onReconnectNeeded: func() {
			logger.Warn("飞书 longconn watchdog 检测到事件停滞，进入 reconnecting 状态")
		},
		onRecovered: func() {
			logger.Info("飞书 longconn watchdog 检测到事件恢复，退出 reconnecting 状态")
		},
		now: time.Now,
	})
}

func normalizeLongConnWatchdogConfig(cfg longConnWatchdogConfig) longConnWatchdogConfig {
	if cfg.staleAfter <= 0 {
		cfg.staleAfter = 2 * time.Minute
	}
	if cfg.checkInterval <= 0 {
		cfg.checkInterval = 15 * time.Second
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	return cfg
}

func (c *LongConnClient) initWatchdog(cfg longConnWatchdogConfig) {
	c.watchdogMu.Lock()
	c.watchdogCfg = normalizeLongConnWatchdogConfig(cfg)
	c.watchdogMu.Unlock()
}

func (c *LongConnClient) ConfigureReliability(staleAfter, gapFetchMaxWindow time.Duration, gapFetchEnabled bool) {
	cfg := c.getWatchdogConfig()
	cfg.staleAfter = staleAfter
	c.initWatchdog(cfg)

	c.watchdogStateMu.Lock()
	c.gapFetchEnabled = gapFetchEnabled
	if gapFetchMaxWindow > 0 {
		c.gapFetchMaxWindow = gapFetchMaxWindow
	}
	c.watchdogStateMu.Unlock()
}

// WithReconnectWatchdogHooks 注入 watchdog 的状态切换回调，为后续 gap fetch / reconnect 编排预留 hook 点。
func (c *LongConnClient) WithReconnectWatchdogHooks(onReconnectNeeded, onRecovered func()) *LongConnClient {
	cfg := c.getWatchdogConfig()
	cfg.onReconnectNeeded = onReconnectNeeded
	cfg.onRecovered = onRecovered
	c.initWatchdog(cfg)
	return c
}

func (c *LongConnClient) startWatchdog(ctx context.Context) {
	cfg := c.getWatchdogConfig()
	if !cfg.enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(cfg.checkInterval)
		defer ticker.Stop()

		c.runWatchdogCheck(cfg)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.runWatchdogCheck(cfg)
			}
		}
	}()
}

func (c *LongConnClient) runWatchdogCheck(cfg longConnWatchdogConfig) {
	now := cfg.now()
	lastEventAt := c.LastEventAt()
	if lastEventAt.IsZero() {
		return
	}

	if now.Sub(lastEventAt) >= cfg.staleAfter {
		if c.setReconnecting(true) && cfg.onReconnectNeeded != nil {
			cfg.onReconnectNeeded()
		}
		return
	}

	if c.setReconnecting(false) && cfg.onRecovered != nil {
		cfg.onRecovered()
	}
}

func (c *LongConnClient) markEventObserved(at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	previousLastEventAt := c.LastEventAt()
	c.setLastEventAt(at)

	cfg := c.getWatchdogConfig()
	if c.IsReconnecting() {
		c.prepareGapFetchWindow(previousLastEventAt, at)
	}
	if c.setReconnecting(false) && cfg.onRecovered != nil {
		cfg.onRecovered()
	}
}

func (c *LongConnClient) setLastEventAt(at time.Time) {
	c.watchdogStateMu.Lock()
	c.lastEventAt = at
	c.watchdogStateMu.Unlock()
}

func (c *LongConnClient) setLastTenantKey(tenantKey string) {
	c.watchdogStateMu.Lock()
	c.lastTenantKey = tenantKey
	c.watchdogStateMu.Unlock()
}

func (c *LongConnClient) LastTenantKey() string {
	c.watchdogStateMu.RLock()
	defer c.watchdogStateMu.RUnlock()
	return c.lastTenantKey
}

func (c *LongConnClient) LastEventAt() time.Time {
	c.watchdogStateMu.RLock()
	defer c.watchdogStateMu.RUnlock()
	return c.lastEventAt
}

func (c *LongConnClient) setReconnecting(next bool) bool {
	c.watchdogStateMu.Lock()
	defer c.watchdogStateMu.Unlock()
	if c.reconnecting == next {
		return false
	}
	c.reconnecting = next
	return true
}

func (c *LongConnClient) IsReconnecting() bool {
	c.watchdogStateMu.RLock()
	defer c.watchdogStateMu.RUnlock()
	return c.reconnecting
}

func (c *LongConnClient) PendingGapFetchWindow() (GapFetchWindow, bool) {
	c.watchdogStateMu.RLock()
	defer c.watchdogStateMu.RUnlock()
	if c.pendingGapFetch == nil {
		return GapFetchWindow{}, false
	}
	return *c.pendingGapFetch, true
}

func (c *LongConnClient) ConsumePendingGapFetchWindow() (GapFetchWindow, bool) {
	c.watchdogStateMu.Lock()
	defer c.watchdogStateMu.Unlock()
	if c.pendingGapFetch == nil {
		return GapFetchWindow{}, false
	}
	window := *c.pendingGapFetch
	c.pendingGapFetch = nil
	c.pendingGapFetchWindowCapped = false
	return window, true
}

func (c *LongConnClient) ReplayGapFetchWindow(ctx context.Context, tenantKey, chatID string) error {
	return c.ReplayGapFetchWindows(ctx, tenantKey, []string{chatID})
}

func (c *LongConnClient) ReplayGapFetchWindows(ctx context.Context, tenantKey string, chatIDs []string) error {
	if c == nil || c.gapFetchRunner == nil {
		return nil
	}
	window, ok := c.PendingGapFetchWindow()
	if !ok {
		return nil
	}
	var replayedChatIDs []string
	for _, chatID := range chatIDs {
		if chatID == "" {
			continue
		}
		if err := c.gapFetchRunner.ReplayWindow(ctx, tenantKey, GapFetchRequest{
			ContainerIDType: GapFetchContainerIDTypeChat,
			ContainerID:     chatID,
			Window:          window,
		}); err != nil {
			c.recordGapFetchReplay(replayedChatIDs, err)
			return err
		}
		replayedChatIDs = append(replayedChatIDs, chatID)
	}
	_, _ = c.ConsumePendingGapFetchWindow()
	c.recordGapFetchReplay(replayedChatIDs, nil)
	return nil
}

func (c *LongConnClient) ReliabilityStatus() LongConnReliabilityStatus {
	if c == nil {
		return LongConnReliabilityStatus{}
	}

	cfg := c.getWatchdogConfig()

	c.watchdogStateMu.RLock()
	defer c.watchdogStateMu.RUnlock()

	status := LongConnReliabilityStatus{
		LastEventAt:          c.lastEventAt,
		LastTenantKey:        c.lastTenantKey,
		Reconnecting:         c.reconnecting,
		ReliabilityLeader:    c.reliabilityLeader,
		GapFetchEnabled:      c.gapFetchEnabled,
		HeartbeatStaleWindow: cfg.staleAfter,
		GapFetchMaxWindow:    c.gapFetchMaxWindow,
		LastGapFetchAt:       c.lastGapFetchAt,
		LastGapFetchError:    c.lastGapFetchError,
	}
	if len(c.lastGapFetchChatIDs) > 0 {
		status.LastGapFetchChatIDs = append([]string(nil), c.lastGapFetchChatIDs...)
	}
	if c.pendingGapFetch != nil {
		status.GapFetchPending = true
		status.PendingGapFetchWindowCapped = c.pendingGapFetchWindowCapped
		status.PendingGapFetchWindow = *c.pendingGapFetch
	}
	return status
}

func (c *LongConnClient) setReliabilityLeader(isLeader bool) {
	c.watchdogStateMu.Lock()
	c.reliabilityLeader = isLeader
	c.watchdogStateMu.Unlock()
}

func (c *LongConnClient) getWatchdogConfig() longConnWatchdogConfig {
	c.watchdogMu.RLock()
	defer c.watchdogMu.RUnlock()
	return c.watchdogCfg
}

func (c *LongConnClient) prepareGapFetchWindow(lastEventAt, recoveredAt time.Time) {
	if lastEventAt.IsZero() || recoveredAt.IsZero() || !recoveredAt.After(lastEventAt) {
		return
	}

	c.watchdogStateMu.Lock()
	defer c.watchdogStateMu.Unlock()
	if !c.gapFetchEnabled {
		return
	}

	start := lastEventAt
	capped := false
	if c.gapFetchMaxWindow > 0 {
		cutoff := recoveredAt.Add(-c.gapFetchMaxWindow)
		if start.Before(cutoff) {
			start = cutoff
			capped = true
		}
	}
	if !recoveredAt.After(start) {
		return
	}
	c.pendingGapFetch = &GapFetchWindow{
		StartTime: start,
		EndTime:   recoveredAt,
	}
	c.pendingGapFetchWindowCapped = capped
	if capped && c.logger != nil {
		c.logger.Warn("飞书 longconn gap fetch 窗口被截断",
			zap.Time("last_event_at", lastEventAt),
			zap.Time("recovered_at", recoveredAt),
			zap.Duration("gap_fetch_max_window", c.gapFetchMaxWindow),
			zap.Time("effective_start_time", start))
	}
}

func (c *LongConnClient) recordGapFetchReplay(chatIDs []string, err error) {
	c.watchdogStateMu.Lock()
	defer c.watchdogStateMu.Unlock()
	c.lastGapFetchAt = time.Now()
	c.lastGapFetchChatIDs = append([]string(nil), chatIDs...)
	if err != nil {
		c.lastGapFetchError = err.Error()
		return
	}
	c.lastGapFetchError = ""
}
