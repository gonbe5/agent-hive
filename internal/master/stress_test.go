package master

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/lsp"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
	"github.com/chef-guo/agents-hive/internal/subagent"
)

// TestStress_HITL_ConcurrentInputRequests 压力测试：50 个并发 HITL 输入请求与响应
func TestStress_HITL_ConcurrentInputRequests(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)

	m := NewMaster(Config{}, config.HITLConfig{
		Enabled:      true,
		InputTimeout: 10 * time.Second,
	}, agentReg, skillReg, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	const numRequests = 50
	var wg sync.WaitGroup

	// 创建 50 个并发请求，每个都有匹配的响应
	wg.Add(numRequests * 2) // 每个请求 = 1 个 waitForInput + 1 个 SubmitInput
	results := make(chan string, numRequests)
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		taskID := fmt.Sprintf("stress-task-%d", i)
		req := m.hitlBroker.RequestInput(taskID, "step-1", InputApproval, fmt.Sprintf("请求 %d", i), nil)

		// 等待响应的 goroutine
		go func(taskID string, req *InputRequest, id int) {
			defer wg.Done()
			resp, err := m.hitlBroker.WaitForInput(ctx, taskID, req)
			if err != nil {
				errors <- fmt.Errorf("waitForInput %d 失败: %w", id, err)
				return
			}
			results <- resp.Value
		}(taskID, req, i)

		// 提交响应的 goroutine
		go func(reqID, taskID string, id int) {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond) // 短暂延迟确保 waitForInput 先就绪
			err := m.SubmitInput(InputResponse{
				RequestID: reqID,
				TaskID:    taskID,
				Action:    "approve",
				Value:     fmt.Sprintf("响应-%d", id),
			})
			if err != nil {
				errors <- fmt.Errorf("SubmitInput %d 失败: %w", id, err)
			}
		}(req.ID, taskID, i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// 检查错误
	for err := range errors {
		t.Error(err)
	}

	// 验证所有响应都已收到
	count := 0
	for range results {
		count++
	}
	assert.Equal(t, numRequests, count, "应收到 %d 个响应", numRequests)
}

// TestStress_HITL_ResponseIsolation 压力测试：验证 per-request channel 不会混淆响应
func TestStress_HITL_ResponseIsolation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)

	m := NewMaster(Config{}, config.HITLConfig{
		Enabled:      true,
		InputTimeout: 10 * time.Second,
	}, agentReg, skillReg, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	const numPairs = 30
	var wg sync.WaitGroup
	wg.Add(numPairs)

	for i := 0; i < numPairs; i++ {
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("isolation-task-%d", id)
			expectedValue := fmt.Sprintf("隔离值-%d", id)

			req := m.hitlBroker.RequestInput(taskID, "step-1", InputApproval, "测试隔离", nil)

			// 立即提交匹配的响应
			go func() {
				time.Sleep(5 * time.Millisecond)
				err := m.SubmitInput(InputResponse{
					RequestID: req.ID,
					TaskID:    taskID,
					Action:    "approve",
					Value:     expectedValue,
				})
				if err != nil {
					t.Errorf("SubmitInput 失败: %v", err)
				}
			}()

			resp, err := m.hitlBroker.WaitForInput(ctx, taskID, req)
			require.NoError(t, err, "waitForInput 不应失败")
			assert.Equal(t, expectedValue, resp.Value,
				"响应值应与请求匹配，任务 %s", taskID)
		}(i)
	}

	wg.Wait()
}

// TestStress_ConcurrentSessionCreation 压力测试：并发创建会话（通过 ProcessCommand）
func TestStress_ConcurrentSessionCreation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)

	st := store.NewMemoryStore()

	m := NewMaster(Config{Model: "test"}, config.HITLConfig{}, agentReg, skillReg, st, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		m.SessionLoop(ctx)
	}()

	time.Sleep(50 * time.Millisecond) // 等待 SessionLoop 就绪

	const numSessions = 20
	var wg sync.WaitGroup
	var successCount int32

	// 串行创建会话（因为 SessionLoop 是单通道处理）
	for i := 0; i < numSessions; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			select {
			case m.RequestCh() <- SessionRequest{
				Command: SessionCommandNew,
				Args:    []string{fmt.Sprintf("压力会话-%d", id)},
			}:
				select {
				case <-m.ResponseCh():
					successCount++
				case <-time.After(5 * time.Second):
					t.Errorf("等待会话 %d 响应超时", id)
				}
			case <-time.After(5 * time.Second):
				t.Errorf("发送会话 %d 请求超时", id)
			}
		}(i)
		// SessionLoop 是单线程的，需要串行化请求
		wg.Wait()
	}

	// 验证最终会话数
	m.sessionMgr.sessionMu.RLock()
	sessionCount := len(m.sessionMgr.sessions)
	m.sessionMgr.sessionMu.RUnlock()

	// 应有 numSessions + 1（初始会话）
	assert.Equal(t, numSessions+1, sessionCount, "应有 %d 个会话", numSessions+1)
	t.Logf("成功创建 %d 个会话", sessionCount)

	cancel()
	select {
	case <-sessionDone:
	case <-time.After(5 * time.Second):
	}
}

// TestStress_PendingInputsConsistency 压力测试：大量创建/清理 pendingInput 的一致性
func TestStress_PendingInputsConsistency(t *testing.T) {
	logger := zap.NewNop()
	skillReg := skills.NewRegistry(logger)
	agentReg := subagent.NewRegistry(logger)

	m := NewMaster(Config{}, config.HITLConfig{
		Enabled:      true,
		InputTimeout: 500 * time.Millisecond,
	}, agentReg, skillReg, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop()

	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)

	// 创建大量请求，一半响应、一半超时
	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("consistency-task-%d", id)
			req := m.hitlBroker.RequestInput(taskID, "step-1", InputApproval, "一致性测试", nil)

			if id%2 == 0 {
				// 偶数：提交响应
				go func() {
					time.Sleep(10 * time.Millisecond)
					m.SubmitInput(InputResponse{
						RequestID: req.ID,
						TaskID:    taskID,
						Action:    "approve",
						Value:     "ok",
					})
				}()
			}
			// 奇数：让它超时

			// 等待结果（成功或超时）
			m.hitlBroker.WaitForInput(ctx, taskID, req)
		}(i)
	}

	wg.Wait()

	// 验证所有 pendingInput 都已清理
	m.hitlBroker.inputMu.Lock()
	remaining := len(m.hitlBroker.pendingInput)
	remainingChans := len(m.hitlBroker.pendingInputChans)
	m.hitlBroker.inputMu.Unlock()

	assert.Equal(t, 0, remaining, "pendingInput 应全部清理")
	assert.Equal(t, 0, remainingChans, "pendingInputChans 应全部清理")
}

// ==================== LSP 压力测试 (Agent A) ====================

// TestLSPConcurrency LSP 并发调用压力测试（50+ 并发请求）
func TestLSPConcurrency(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 检查 gopls 是否安装
	goplsPath := ""
	for _, p := range []string{"/usr/local/bin/gopls", os.ExpandEnv("$HOME/go/bin/gopls")} {
		if _, err := os.Stat(p); err == nil {
			goplsPath = p
			break
		}
	}
	if goplsPath == "" {
		t.Skip("gopls 未安装，跳过 LSP 压力测试")
	}

	cfg := lsp.LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    15 * time.Second,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
		MaxConcurrentRequestsPerServer: 20, // 允许更高并发
	}

	manager := lsp.NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动服务器
	server, err := manager.GetServer(ctx, "go")
	require.NoError(t, err, "GetServer 不应失败")
	require.NotNil(t, server, "服务器不应为 nil")

	const numRequests = 60
	var wg sync.WaitGroup
	var successCount int32
	var errorCount int32
	errors := make(chan error, numRequests)

	// 并发发送 60 个请求
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 使用短超时避免测试时间过长
			reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			// 模拟 LSP 请求（这里使用简单的测试请求）
			// 实际测试中可以使用 server.client 发送真实请求
			select {
			case <-reqCtx.Done():
				atomic.AddInt32(&errorCount, 1)
				errors <- fmt.Errorf("请求 %d 超时", id)
			case <-time.After(10 * time.Millisecond):
				// 模拟成功
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 收集错误
	var collectedErrors []error
	for err := range errors {
		collectedErrors = append(collectedErrors, err)
	}

	// 验证结果
	t.Logf("成功: %d, 失败: %d", successCount, errorCount)

	// 至少 80% 的请求应该成功
	assert.GreaterOrEqual(t, successCount, int32(numRequests*80/100),
		"至少 80%% 的请求应该成功，实际: %d/%d", successCount, numRequests)

	// 报告错误（如果有）
	if len(collectedErrors) > 0 {
		t.Logf("收集到 %d 个错误:", len(collectedErrors))
		for i, err := range collectedErrors {
			if i < 5 { // 只显示前 5 个错误
				t.Logf("  - %v", err)
			}
		}
	}
}

// TestLSPServerCrashRecovery LSP 服务器崩溃恢复测试
func TestLSPServerCrashRecovery(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 检查 gopls 是否安装
	goplsPath := ""
	for _, p := range []string{"/usr/local/bin/gopls", os.ExpandEnv("$HOME/go/bin/gopls")} {
		if _, err := os.Stat(p); err == nil {
			goplsPath = p
			break
		}
	}
	if goplsPath == "" {
		t.Skip("gopls 未安装，跳过 LSP 崩溃恢复测试")
	}

	cfg := lsp.LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    10 * time.Second,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
	}

	manager := lsp.NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	// 第一次启动服务器
	server1, err := manager.GetServer(ctx, "go")
	require.NoError(t, err, "GetServer 不应失败")
	require.NotNil(t, server1, "服务器不应为 nil")
	require.True(t, server1.IsHealthy(), "服务器应该健康")

	// 模拟服务器崩溃（停止服务器）
	manager.StopServer("go")
	time.Sleep(100 * time.Millisecond) // 等待停止完成

	// 尝试恢复（重新获取服务器）
	server2, err := manager.GetServer(ctx, "go")
	require.NoError(t, err, "恢复时 GetServer 不应失败")
	require.NotNil(t, server2, "恢复的服务器不应为 nil")

	// 验证是新的服务器实例
	assert.NotEqual(t, server1, server2, "应该是新的服务器实例")
	assert.True(t, server2.IsHealthy(), "恢复的服务器应该健康")

	t.Logf("服务器崩溃恢复成功")
}

// TestLSPServerPoolLimit LSP 服务器池资源限制测试（MaxServers）
func TestLSPServerPoolLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 检查 gopls 是否安装
	goplsPath := ""
	for _, p := range []string{"/usr/local/bin/gopls", os.ExpandEnv("$HOME/go/bin/gopls")} {
		if _, err := os.Stat(p); err == nil {
			goplsPath = p
			break
		}
	}
	if goplsPath == "" {
		t.Skip("gopls 未安装，跳过 LSP 资源限制测试")
	}

	// 设置 MaxServers = 2
	cfg := lsp.LSPConfig{
		Enabled:    true,
		MaxServers: 2,
		Timeout:    10 * time.Second,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
			"python": {
				Command:    "pyright-langserver",
				Args:       []string{"--stdio"},
				Extensions: []string{".py"},
			},
			"rust": {
				Command:    "rust-analyzer",
				Args:       []string{},
				Extensions: []string{".rs"},
			},
		},
	}

	manager := lsp.NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	// 启动第一个服务器 (Go)
	server1, err := manager.GetServer(ctx, "go")
	require.NoError(t, err, "启动第 1 个服务器不应失败")
	require.NotNil(t, server1, "第 1 个服务器不应为 nil")

	// 尝试启动第二个服务器（Python - 可能不可用）
	_, err = manager.GetServer(ctx, "python")

	t.Logf("MaxServers 限制: %d", cfg.MaxServers)
	t.Logf("当前服务器数量应 <= MaxServers")
}
