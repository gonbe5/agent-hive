package feishu

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestLongConnWatchdog_BecomesReconnectingWhenStale(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	var reconnectCalls atomic.Int32

	client.initWatchdog(longConnWatchdogConfig{
		enabled:           true,
		staleAfter:        50 * time.Millisecond,
		checkInterval:     10 * time.Millisecond,
		onReconnectNeeded: func() { reconnectCalls.Add(1) },
		now:               time.Now,
	})
	client.setLastEventAt(time.Now().Add(-time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.startWatchdog(ctx)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if client.IsReconnecting() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !client.IsReconnecting() {
		t.Fatalf("watchdog did not enter reconnecting state")
	}
	if got := reconnectCalls.Load(); got != 1 {
		t.Fatalf("watchdog should trigger reconnect callback once, got %d", got)
	}
}

func TestLongConnWatchdog_RecoveryClearsReconnectingAndCallsRecoveredOnce(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	var recoveredCalls atomic.Int32

	client.initWatchdog(longConnWatchdogConfig{
		enabled:       true,
		staleAfter:    40 * time.Millisecond,
		checkInterval: 10 * time.Millisecond,
		onRecovered: func() {
			recoveredCalls.Add(1)
		},
		now: time.Now,
	})
	client.setLastEventAt(time.Now().Add(-time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.startWatchdog(ctx)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if client.IsReconnecting() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !client.IsReconnecting() {
		t.Fatalf("watchdog did not enter reconnecting state before recovery")
	}

	client.markEventObserved(time.Now())

	deadline = time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !client.IsReconnecting() && recoveredCalls.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if client.IsReconnecting() {
		t.Fatalf("watchdog did not clear reconnecting state after recovery event")
	}
	if got := recoveredCalls.Load(); got != 1 {
		t.Fatalf("recovery callback should fire once, got %d", got)
	}

	time.Sleep(60 * time.Millisecond)
	if got := recoveredCalls.Load(); got != 1 {
		t.Fatalf("recovery callback should not repeat without state change, got %d", got)
	}
}

func TestLongConnWatchdog_RecoveryPreparesGapFetchWindow(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	base := time.Unix(1710000600, 0)

	client.ConfigureReliability(45*time.Second, 10*time.Minute, true)
	client.initWatchdog(longConnWatchdogConfig{
		enabled:       true,
		staleAfter:    45 * time.Second,
		checkInterval: time.Second,
		now: func() time.Time {
			return base
		},
	})
	client.setLastEventAt(base.Add(-2 * time.Minute))

	client.runWatchdogCheck(client.getWatchdogConfig())
	if !client.IsReconnecting() {
		t.Fatal("watchdog should enter reconnecting before recovery")
	}

	recoveredAt := base.Add(30 * time.Second)
	client.markEventObserved(recoveredAt)

	window, ok := client.PendingGapFetchWindow()
	if !ok {
		t.Fatal("expected pending gap fetch window after recovery")
	}
	if got := window.StartTime; !got.Equal(base.Add(-2 * time.Minute)) {
		t.Fatalf("window.StartTime = %v, want %v", got, base.Add(-2*time.Minute))
	}
	if got := window.EndTime; !got.Equal(recoveredAt) {
		t.Fatalf("window.EndTime = %v, want %v", got, recoveredAt)
	}

	consumed, ok := client.ConsumePendingGapFetchWindow()
	if !ok {
		t.Fatal("expected consume pending gap fetch window to succeed")
	}
	if consumed != window {
		t.Fatalf("consumed window = %+v, want %+v", consumed, window)
	}
	if _, ok := client.PendingGapFetchWindow(); ok {
		t.Fatal("pending gap fetch window should be cleared after consume")
	}
}

func TestLongConnWatchdog_RecoveryCapsGapFetchWindow(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	base := time.Unix(1710000600, 0)

	client.ConfigureReliability(30*time.Second, 90*time.Second, true)
	client.initWatchdog(longConnWatchdogConfig{
		enabled:       true,
		staleAfter:    30 * time.Second,
		checkInterval: time.Second,
		now: func() time.Time {
			return base
		},
	})
	client.setLastEventAt(base.Add(-10 * time.Minute))

	client.runWatchdogCheck(client.getWatchdogConfig())
	if !client.IsReconnecting() {
		t.Fatal("watchdog should enter reconnecting before recovery")
	}

	recoveredAt := base.Add(15 * time.Second)
	client.markEventObserved(recoveredAt)

	window, ok := client.PendingGapFetchWindow()
	if !ok {
		t.Fatal("expected capped gap fetch window after recovery")
	}
	wantStart := recoveredAt.Add(-90 * time.Second)
	if got := window.StartTime; !got.Equal(wantStart) {
		t.Fatalf("window.StartTime = %v, want %v", got, wantStart)
	}
	if got := window.EndTime; !got.Equal(recoveredAt) {
		t.Fatalf("window.EndTime = %v, want %v", got, recoveredAt)
	}
	status := client.ReliabilityStatus()
	if !status.PendingGapFetchWindowCapped {
		t.Fatal("PendingGapFetchWindowCapped = false, want true")
	}
}

func TestLongConnClient_ReliabilityStatusReflectsPendingGapFetch(t *testing.T) {
	t.Parallel()

	client := NewLongConnClient("", "", nil, nil, zap.NewNop())
	now := time.Unix(1710000600, 0)
	client.ConfigureReliability(75*time.Second, 4*time.Minute, true)
	client.setLastEventAt(now)
	client.setLastTenantKey("tenant-a")
	client.setReconnecting(true)
	client.pendingGapFetch = &GapFetchWindow{
		StartTime: now.Add(-time.Minute),
		EndTime:   now,
	}

	status := client.ReliabilityStatus()
	if !status.LastEventAt.Equal(now) {
		t.Fatalf("LastEventAt = %v, want %v", status.LastEventAt, now)
	}
	if status.LastTenantKey != "tenant-a" {
		t.Fatalf("LastTenantKey = %q, want tenant-a", status.LastTenantKey)
	}
	if !status.Reconnecting {
		t.Fatal("Reconnecting = false, want true")
	}
	if !status.GapFetchEnabled {
		t.Fatal("GapFetchEnabled = false, want true")
	}
	if !status.GapFetchPending {
		t.Fatal("GapFetchPending = false, want true")
	}
	if got := status.HeartbeatStaleWindow; got != 75*time.Second {
		t.Fatalf("HeartbeatStaleWindow = %v, want %v", got, 75*time.Second)
	}
	if got := status.GapFetchMaxWindow; got != 4*time.Minute {
		t.Fatalf("GapFetchMaxWindow = %v, want %v", got, 4*time.Minute)
	}
	if got := status.PendingGapFetchWindow.StartTime; !got.Equal(now.Add(-time.Minute)) {
		t.Fatalf("PendingGapFetchWindow.StartTime = %v, want %v", got, now.Add(-time.Minute))
	}
}
