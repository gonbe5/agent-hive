package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// newTestHostWithLS 创建包含 ls 工具的测试 host
func newTestHostWithLS(t *testing.T) *mcphost.Host {
	t.Helper()
	// 临时放宽路径校验（测试使用临时目录，不在工作目录内）
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	logger, _ := zap.NewDevelopment()
	host := mcphost.NewHost(logger)
	registerLS(host, logger)
	return host
}

// setupTestDir 创建测试目录结构
func setupTestDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// 创建目录结构:
	// tmpDir/
	//   file1.txt
	//   file2.go
	//   subdir1/
	//     file3.txt
	//     subdir2/
	//       file4.txt
	//       subdir3/
	//         file5.txt

	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test1"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("test2"), 0o644)

	subdir1 := filepath.Join(tmpDir, "subdir1")
	os.Mkdir(subdir1, 0o755)
	os.WriteFile(filepath.Join(subdir1, "file3.txt"), []byte("test3"), 0o644)

	subdir2 := filepath.Join(subdir1, "subdir2")
	os.Mkdir(subdir2, 0o755)
	os.WriteFile(filepath.Join(subdir2, "file4.txt"), []byte("test4"), 0o644)

	subdir3 := filepath.Join(subdir2, "subdir3")
	os.Mkdir(subdir3, 0o755)
	os.WriteFile(filepath.Join(subdir3, "file5.txt"), []byte("test5"), 0o644)

	return tmpDir
}

func TestLS_SingleFile(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := setupTestDir(t)

	testFile := filepath.Join(tmpDir, "file1.txt")
	input, _ := json.Marshal(map[string]string{"path": testFile})
	result, err := host.ExecuteTool(context.Background(), "ls", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var output string
	json.Unmarshal(result.Content, &output)

	// 检查输出包含文件名
	if !strings.Contains(output, "file1.txt") {
		t.Errorf("output should contain 'file1.txt', got: %s", output)
	}
}

func TestLS_DirectoryFlat(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := setupTestDir(t)

	tests := []struct {
		name     string
		input    map[string]any
		expected []string // 预期输出包含的字符串
	}{
		{
			name:  "默认路径（当前目录）",
			input: map[string]any{},
			expected: []string{
				"internal/", // 假设当前目录是项目根目录
			},
		},
		{
			name:  "指定目录",
			input: map[string]any{"path": tmpDir},
			expected: []string{
				"file1.txt",
				"file2.go",
				"subdir1/",
			},
		},
		{
			name:  "非递归（默认）",
			input: map[string]any{"path": tmpDir, "recursive": false},
			expected: []string{
				"file1.txt",
				"file2.go",
				"subdir1/",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 跳过默认路径测试（依赖当前工作目录）
			if tt.name == "默认路径（当前目录）" {
				t.Skip("跳过依赖当前工作目录的测试")
			}

			input, _ := json.Marshal(tt.input)
			result, err := host.ExecuteTool(context.Background(), "ls", input)
			if err != nil {
				t.Fatalf("ExecuteTool: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}

			var output string
			json.Unmarshal(result.Content, &output)

			for _, exp := range tt.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("output should contain %q, got: %s", exp, output)
				}
			}

			// 非递归模式不应包含深层文件
			if recursive, ok := tt.input["recursive"].(bool); !ok || !recursive {
				if strings.Contains(output, "file3.txt") {
					t.Errorf("non-recursive mode should not contain 'file3.txt', got: %s", output)
				}
			}
		})
	}
}

func TestLS_DirectoryRecursive(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := setupTestDir(t)

	tests := []struct {
		name          string
		input         map[string]any
		shouldInclude []string // 应该包含的文件
		shouldExclude []string // 不应该包含的文件
	}{
		{
			name: "递归深度1（默认）",
			input: map[string]any{
				"path":      tmpDir,
				"recursive": true,
			},
			shouldInclude: []string{
				"file1.txt",
				"file2.go",
				"subdir1/",
			},
			shouldExclude: []string{
				"file3.txt", // 深度2
			},
		},
		{
			name: "递归深度2",
			input: map[string]any{
				"path":      tmpDir,
				"recursive": true,
				"max_depth": 2,
			},
			shouldInclude: []string{
				"file1.txt",
				"subdir1/",
				"file3.txt",
				"subdir2/",
			},
			shouldExclude: []string{
				"file4.txt", // 深度3
			},
		},
		{
			name: "递归深度3",
			input: map[string]any{
				"path":      tmpDir,
				"recursive": true,
				"max_depth": 3,
			},
			shouldInclude: []string{
				"file1.txt",
				"file3.txt",
				"file4.txt",
				"subdir3/",
			},
			shouldExclude: []string{
				"file5.txt", // 深度4（超过最大限制3）
			},
		},
		{
			name: "递归深度超过最大限制（应限制到3）",
			input: map[string]any{
				"path":      tmpDir,
				"recursive": true,
				"max_depth": 10,
			},
			shouldInclude: []string{
				"file1.txt",
				"file3.txt",
				"file4.txt",
				"subdir3/",
			},
			shouldExclude: []string{
				"file5.txt", // 深度4（超过最大限制3）
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := json.Marshal(tt.input)
			result, err := host.ExecuteTool(context.Background(), "ls", input)
			if err != nil {
				t.Fatalf("ExecuteTool: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", result.Content)
			}

			var output string
			json.Unmarshal(result.Content, &output)

			for _, exp := range tt.shouldInclude {
				if !strings.Contains(output, exp) {
					t.Errorf("output should include %q, got: %s", exp, output)
				}
			}

			for _, exp := range tt.shouldExclude {
				if strings.Contains(output, exp) {
					t.Errorf("output should exclude %q, got: %s", exp, output)
				}
			}
		})
	}
}

func TestLS_NonexistentPath(t *testing.T) {
	host := newTestHostWithLS(t)

	input, _ := json.Marshal(map[string]string{
		"path": "/nonexistent/path/that/does/not/exist",
	})
	result, err := host.ExecuteTool(context.Background(), "ls", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent path")
	}

	var errMsg string
	json.Unmarshal(result.Content, &errMsg)
	if !strings.Contains(errMsg, "路径不存在") {
		t.Errorf("error message should contain '路径不存在', got: %s", errMsg)
	}
}

func TestLS_InvalidInput(t *testing.T) {
	host := newTestHostWithLS(t)

	// 无效的 JSON
	result, err := host.ExecuteTool(context.Background(), "ls", []byte("invalid json"))
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid input")
	}

	var errMsg string
	json.Unmarshal(result.Content, &errMsg)
	if !strings.Contains(errMsg, "输入无效") {
		t.Errorf("error message should contain '输入无效', got: %s", errMsg)
	}
}

func TestLS_SymbolicLink(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := t.TempDir()

	// 创建一个文件
	realFile := filepath.Join(tmpDir, "real.txt")
	os.WriteFile(realFile, []byte("test"), 0o644)

	// 创建符号链接
	linkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("无法创建符号链接（可能是权限问题）: %v", err)
	}

	input, _ := json.Marshal(map[string]string{"path": tmpDir})
	result, err := host.ExecuteTool(context.Background(), "ls", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var output string
	json.Unmarshal(result.Content, &output)

	// 符号链接应该标记为 @
	if !strings.Contains(output, "link.txt@") {
		t.Errorf("symbolic link should be marked with @, got: %s", output)
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0B"},
		{100, "100B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 1024, "1.0GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSize(tt.size)
			if result != tt.expected {
				t.Errorf("formatSize(%d) = %s, expected %s", tt.size, result, tt.expected)
			}
		})
	}
}

func TestLS_EmptyDirectory(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := t.TempDir()

	input, _ := json.Marshal(map[string]string{"path": tmpDir})
	result, err := host.ExecuteTool(context.Background(), "ls", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var output string
	json.Unmarshal(result.Content, &output)

	// 应该显示 0 项
	if !strings.Contains(output, "(0 项)") {
		t.Errorf("output should indicate 0 items for empty directory, got: %s", output)
	}
}

func TestLS_OutputFormat(t *testing.T) {
	host := newTestHostWithLS(t)
	tmpDir := setupTestDir(t)

	input, _ := json.Marshal(map[string]string{"path": tmpDir})
	result, err := host.ExecuteTool(context.Background(), "ls", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var output string
	json.Unmarshal(result.Content, &output)

	// 检查输出格式包含必要元素
	requiredElements := []string{
		tmpDir,      // 目录路径
		"(3 项)",     // 文件数量（file1.txt, file2.go, subdir1/）
		"file1.txt", // 文件名
		"-rw-",      // 文件权限
		"B",         // 文件大小单位
		"subdir1/",  // 目录标记
	}

	for _, elem := range requiredElements {
		if !strings.Contains(output, elem) {
			t.Errorf("output should contain %q, got: %s", elem, output)
		}
	}
}
