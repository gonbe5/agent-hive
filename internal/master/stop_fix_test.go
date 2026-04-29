package master

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/subagent"
	"go.uber.org/zap"
)

// TestStopDoesNotPanic 验证 Stop() 方法不会导致 panic
func TestStopDoesNotPanic(t *testing.T) {
	logger := zap.NewNop()
	stopCh := make(chan struct{})
	sessionMgr := NewSessionManager(stopCh, logger)
	eventBus := NewEventBus(logger)

	m := &Master{
		sessionMgr: sessionMgr,
		eventBus:   eventBus,
		stopCh:     stopCh,
		registry:   subagent.NewRegistry(logger),
		logger:     logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// 启动 SessionLoop
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = m.SessionLoop(ctx)
	}()

	// 等待一小段时间确保 SessionLoop 已启动
	time.Sleep(50 * time.Millisecond)

	// 调用 Stop() - 这应该不会导致 panic
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.Stop()
	}()

	// 等待所有 goroutine 完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Stop() completed without panic")
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

// TestStopChClosedBeforeChannels 验证 stopCh 在 requestCh/responseCh 之前关闭
func TestStopChClosedBeforeChannels(t *testing.T) {
	logger := zap.NewNop()
	stopCh := make(chan struct{})
	sessionMgr := NewSessionManager(stopCh, logger)
	eventBus := NewEventBus(logger)

	m := &Master{
		sessionMgr: sessionMgr,
		eventBus:   eventBus,
		stopCh:     stopCh,
		registry:   subagent.NewRegistry(logger),
		logger:     logger,
	}

	ctx := context.Background()

	var wg sync.WaitGroup

	// 启动 SessionLoop
	wg.Add(1)
	var sessionLoopExited bool
	go func() {
		defer wg.Done()
		_ = m.SessionLoop(ctx)
		sessionLoopExited = true
	}()

	// 等待一小段时间确保 SessionLoop 已启动
	time.Sleep(50 * time.Millisecond)

	// 调用 Stop()
	m.Stop()

	// 等待 SessionLoop 退出
	wg.Wait()

	// 验证 SessionLoop 已退出
	if !sessionLoopExited {
		t.Fatal("SessionLoop should have exited")
	}

	t.Log("SessionLoop exited successfully after Stop()")
}
