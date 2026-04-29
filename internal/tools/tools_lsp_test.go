package tools

import (
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/lsp"
)

// TestFetchLSPDiagnostics_LSP未启用 验证 LSP 未启用时返回空字符串
func TestFetchLSPDiagnostics_LSP未启用(t *testing.T) {
	// 确保 globalLSPManager 为 nil
	saved := globalLSPManager
	globalLSPManager = nil
	defer func() { globalLSPManager = saved }()

	result := fetchLSPDiagnostics("/tmp/test.go", 2*time.Second)
	if result != "" {
		t.Errorf("LSP 未启用时应返回空字符串, 实际: %q", result)
	}
}

// TestFetchLSPDiagnostics_不支持的文件类型 验证不支持的文件扩展名返回空字符串
func TestFetchLSPDiagnostics_不支持的文件类型(t *testing.T) {
	// 创建一个有配置但不包含 .xyz 扩展名的 ServerManager
	cfg := lsp.LSPConfig{
		Enabled: true,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Extensions: []string{".go"},
			},
		},
	}
	mgr := lsp.NewServerManager(cfg, "/tmp", nil)

	saved := globalLSPManager
	globalLSPManager = mgr
	defer func() { globalLSPManager = saved }()

	result := fetchLSPDiagnostics("/tmp/test.xyz", 2*time.Second)
	if result != "" {
		t.Errorf("不支持的文件类型应返回空字符串, 实际: %q", result)
	}
}

// TestFetchLSPDiagnostics_超时不阻塞 验证超时情况下不会长时间阻塞
func TestFetchLSPDiagnostics_超时不阻塞(t *testing.T) {
	// 创建一个配置了 go 语言但无法真正启动服务器的 ServerManager
	cfg := lsp.LSPConfig{
		Enabled: true,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "nonexistent-lsp-binary-12345",
				Extensions: []string{".go"},
			},
		},
	}
	mgr := lsp.NewServerManager(cfg, "/tmp", nil)

	saved := globalLSPManager
	globalLSPManager = mgr
	defer func() { globalLSPManager = saved }()

	start := time.Now()
	result := fetchLSPDiagnostics("/tmp/test.go", 500*time.Millisecond)
	elapsed := time.Since(start)

	if result != "" {
		t.Errorf("无法启动服务器时应返回空字符串, 实际: %q", result)
	}

	// 确保不会阻塞超过合理时间（超时 + 1 秒缓冲）
	if elapsed > 3*time.Second {
		t.Errorf("fetchLSPDiagnostics 应在超时内返回, 实际耗时: %v", elapsed)
	}
}

// TestFetchLSPDiagnostics_无扩展名文件 验证无扩展名文件返回空字符串
func TestFetchLSPDiagnostics_无扩展名文件(t *testing.T) {
	cfg := lsp.LSPConfig{
		Enabled: true,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Extensions: []string{".go"},
			},
		},
	}
	mgr := lsp.NewServerManager(cfg, "/tmp", nil)

	saved := globalLSPManager
	globalLSPManager = mgr
	defer func() { globalLSPManager = saved }()

	result := fetchLSPDiagnostics("/tmp/Makefile", 2*time.Second)
	if result != "" {
		t.Errorf("无扩展名文件应返回空字符串, 实际: %q", result)
	}
}

// TestFetchLSPDiagnostics_禁用的语言 验证禁用的语言服务器返回空字符串
func TestFetchLSPDiagnostics_禁用的语言(t *testing.T) {
	cfg := lsp.LSPConfig{
		Enabled: true,
		Languages: map[string]lsp.LanguageSpec{
			"go": {
				Command:    "gopls",
				Extensions: []string{".go"},
				Disabled:   true,
			},
		},
	}
	mgr := lsp.NewServerManager(cfg, "/tmp", nil)

	saved := globalLSPManager
	globalLSPManager = mgr
	defer func() { globalLSPManager = saved }()

	result := fetchLSPDiagnostics("/tmp/test.go", 2*time.Second)
	if result != "" {
		t.Errorf("禁用的语言服务器应返回空字符串, 实际: %q", result)
	}
}
