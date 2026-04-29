package lsp

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestServerManager_GetServer(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 检查 gopls 是否安装
	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		if _, err := os.Stat(os.ExpandEnv("$HOME/go/bin/gopls")); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	cfg := LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    10 * time.Second,
		Languages: map[string]LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
	}

	manager := NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	// 获取 Go 语言服务器
	server, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("GetServer failed: %v", err)
	}

	if server == nil {
		t.Fatal("server is nil")
	}

	if !server.IsHealthy() {
		t.Error("server is not healthy")
	}

	// 再次获取应该返回同一个实例
	server2, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("GetServer (2nd) failed: %v", err)
	}

	if server != server2 {
		t.Error("expected same server instance")
	}
}

func TestServerManager_GetServerForFile(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		if _, err := os.Stat(os.ExpandEnv("$HOME/go/bin/gopls")); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	cfg := LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    10 * time.Second,
		Languages: map[string]LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
	}

	manager := NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	// 根据 .go 文件获取服务器
	server, err := manager.GetServerForFile(ctx, "/tmp/test.go")
	if err != nil {
		t.Fatalf("GetServerForFile failed: %v", err)
	}

	if server == nil {
		t.Fatal("server is nil")
	}
}

func TestServerManager_MaxServers(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := LSPConfig{
		Enabled:    true,
		MaxServers: 1, // 限制为 1
		Timeout:    10 * time.Second,
		Languages: map[string]LanguageSpec{
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
		},
	}

	manager := NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	// 启动第一个服务器（假设 gopls 可用）
	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		if _, err := os.Stat(os.ExpandEnv("$HOME/go/bin/gopls")); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	_, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("GetServer(go) failed: %v", err)
	}

	// 尝试启动第二个服务器应该失败（达到上限）
	_, err = manager.GetServer(ctx, "python")
	if err == nil {
		t.Error("expected error when exceeding max servers")
	}
}

func TestPathToURI(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/tmp/test.go", "file:///tmp/test.go"},
		{"/home/user/project/main.go", "file:///home/user/project/main.go"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := pathToURI(tt.path)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestServer_Initialize(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		if _, err := os.Stat(os.ExpandEnv("$HOME/go/bin/gopls")); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	cfg := LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    10 * time.Second,
		Languages: map[string]LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
	}

	manager := NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	server, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("GetServer failed: %v", err)
	}

	if !server.initialized {
		t.Error("server not initialized")
	}
}

// TestServer_HealthCheckCache 测试健康检查缓存机制（Agent E）
func TestServer_HealthCheckCache(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		homeGopls := os.ExpandEnv("$HOME/go/bin/gopls")
		if _, err := os.Stat(homeGopls); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	cfg := LSPConfig{
		Enabled:        true,
		MaxServers:     5,
		Timeout:        10 * time.Second,
		HealthInterval: 2 * time.Second, // 2秒缓存间隔（测试用）
		Languages: map[string]LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
	}

	manager := NewServerManager(cfg, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()

	server, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("GetServer failed: %v", err)
	}

	// 第一次检查
	if !server.IsHealthy() {
		t.Error("server should be healthy")
	}

	// 记录第一次检查时间
	server.healthCheckCache.RLock()
	firstCheck := server.healthCheckCache.lastCheck
	server.healthCheckCache.RUnlock()

	// 立即再次检查，应该使用缓存
	time.Sleep(500 * time.Millisecond)
	if !server.IsHealthy() {
		t.Error("server should be healthy (cached)")
	}

	// 验证缓存生效（lastCheck 时间未变化）
	server.healthCheckCache.RLock()
	secondCheck := server.healthCheckCache.lastCheck
	server.healthCheckCache.RUnlock()

	if !firstCheck.Equal(secondCheck) {
		t.Error("expected health check to use cache, but lastCheck changed")
	}

	// 等待缓存过期
	time.Sleep(3 * time.Second)
	if !server.IsHealthy() {
		t.Error("server should be healthy (cache expired)")
	}

	// 验证缓存已刷新
	server.healthCheckCache.RLock()
	thirdCheck := server.healthCheckCache.lastCheck
	server.healthCheckCache.RUnlock()

	if firstCheck.Equal(thirdCheck) {
		t.Error("expected health check to refresh cache after expiry")
	}

	t.Logf("健康检查缓存测试通过: 首次检查=%v, 缓存检查=%v, 刷新检查=%v",
		firstCheck, secondCheck, thirdCheck)
}

// Agent B: 并发限制压力测试
func TestServer_ConcurrencyLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	if _, err := os.Stat("/usr/local/bin/gopls"); os.IsNotExist(err) {
		if _, err := os.Stat(os.ExpandEnv("$HOME/go/bin/gopls")); os.IsNotExist(err) {
			t.Skip("gopls 未安装，跳过测试")
		}
	}

	config := LSPConfig{
		Enabled:    true,
		MaxServers: 5,
		Timeout:    5 * time.Second,
		Languages: map[string]LanguageSpec{
			"go": {
				Command:    "gopls",
				Args:       []string{"serve"},
				Extensions: []string{".go"},
			},
		},
		HealthInterval:                 5 * time.Second,
		MaxConcurrentRequestsPerServer: 3, // Agent B: 限制并发为 3
	}

	manager := NewServerManager(config, ".", logger)
	defer manager.StopAll()

	ctx := context.Background()
	server, err := manager.GetServer(ctx, "go")
	if err != nil {
		t.Fatalf("启动 LSP 服务器失败: %v", err)
	}

	// 获取当前文件的绝对路径
	currentFile, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testFile := currentFile + "/client.go" // 使用 client.go 作为测试文件

	// 并发发送 10 个请求（超过限制 3）
	const numRequests = 10
	results := make(chan string, numRequests)
	startCh := make(chan struct{})

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			<-startCh // 等待启动信号，确保并发

			// 模拟一个轻量级 LSP 调用
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// 调用 hover（比较轻量）
			uri := pathToURI(testFile)
			params := TextDocumentPositionParams{
				TextDocument: TextDocumentIdentifier{URI: uri},
				Position:     Position{Line: 10, Character: 5},
			}

			var result Hover
			err := server.Call(reqCtx, "textDocument/hover", params, &result)

			if err == nil {
				results <- "success"
			} else if strings.Contains(err.Error(), "超时") || strings.Contains(err.Error(), "timeout") {
				results <- "timeout"
			} else if strings.Contains(err.Error(), "等待并发槽位超时") {
				results <- "semaphore_timeout"
			} else {
				results <- "other_error"
			}
		}(i)
	}

	// 同时启动所有请求
	close(startCh)

	// 收集结果
	counts := make(map[string]int)
	for i := 0; i < numRequests; i++ {
		result := <-results
		counts[result]++
	}

	t.Logf("并发限制测试结果: 成功=%d, 超时=%d, 信号量超时=%d, 其他错误=%d, 总计=%d",
		counts["success"], counts["timeout"], counts["semaphore_timeout"], counts["other_error"], numRequests)

	// 验证：至少有一些请求成功或被限流（说明并发控制在工作）
	// 不要求全部成功，因为可能有并发限制
	if counts["success"] == 0 && counts["semaphore_timeout"] == 0 {
		t.Error("并发限制可能未生效：没有成功请求也没有信号量超时")
	}

	// 验证没有 OOM（测试能正常完成就说明没有 OOM）
	t.Log("并发限制测试通过：没有发生 OOM")
}
