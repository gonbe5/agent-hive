package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

func TestMultiEdit(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // 初始文件内容
		edits []struct {
			path       string
			oldString  string
			newString  string
			replaceAll bool
		}
		recordReads    []string // 需要记录读取的文件
		expectError    bool
		expectContains string            // 错误消息应包含的内容
		expectFiles    map[string]string // 期望的最终文件内容
	}{
		{
			name: "单文件单次替换成功",
			files: map[string]string{
				"file1.txt": "hello world\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "world", newString: "universe", replaceAll: false},
			},
			recordReads: []string{"file1.txt"},
			expectError: false,
			expectFiles: map[string]string{
				"file1.txt": "hello universe\n",
			},
		},
		{
			name: "多文件编辑成功",
			files: map[string]string{
				"file1.txt": "line 1\nline 2\n",
				"file2.txt": "foo bar\n",
				"file3.txt": "test content\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "line 2", newString: "line TWO", replaceAll: false},
				{path: "file2.txt", oldString: "bar", newString: "baz", replaceAll: false},
				{path: "file3.txt", oldString: "test", newString: "TEST", replaceAll: false},
			},
			recordReads: []string{"file1.txt", "file2.txt", "file3.txt"},
			expectError: false,
			expectFiles: map[string]string{
				"file1.txt": "line 1\nline TWO\n",
				"file2.txt": "foo baz\n",
				"file3.txt": "TEST content\n",
			},
		},
		{
			name: "replace_all 替换多次出现",
			files: map[string]string{
				"file1.txt": "apple apple apple\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "apple", newString: "orange", replaceAll: true},
			},
			recordReads: []string{"file1.txt"},
			expectError: false,
			expectFiles: map[string]string{
				"file1.txt": "orange orange orange\n",
			},
		},
		{
			name: "ReadTracker 未读取文件",
			files: map[string]string{
				"file1.txt": "content\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "content", newString: "new", replaceAll: false},
			},
			recordReads:    []string{}, // 不记录读取
			expectError:    true,
			expectContains: "has not been read",
		},
		{
			name: "old_string 不存在",
			files: map[string]string{
				"file1.txt": "hello world\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "nonexistent", newString: "new", replaceAll: false},
			},
			recordReads:    []string{"file1.txt"},
			expectError:    true,
			expectContains: "未找到 old_string",
		},
		{
			name: "old_string 多次出现但未设置 replace_all",
			files: map[string]string{
				"file1.txt": "foo foo foo\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "foo", newString: "bar", replaceAll: false},
			},
			recordReads:    []string{"file1.txt"},
			expectError:    true,
			expectContains: "出现 3 次",
		},
		{
			name: "部分编辑失败应全部回滚",
			files: map[string]string{
				"file1.txt": "content 1\n",
				"file2.txt": "content 2\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{
				{path: "file1.txt", oldString: "content 1", newString: "NEW 1", replaceAll: false},
				{path: "file2.txt", oldString: "nonexistent", newString: "NEW 2", replaceAll: false},
			},
			recordReads:    []string{"file1.txt", "file2.txt"},
			expectError:    true,
			expectContains: "未找到 old_string",
			expectFiles: map[string]string{
				"file1.txt": "content 1\n", // 应回滚
				"file2.txt": "content 2\n",
			},
		},
		{
			name: "空编辑列表",
			files: map[string]string{
				"file1.txt": "content\n",
			},
			edits: []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			}{},
			recordReads:    []string{},
			expectError:    true,
			expectContains: "编辑列表不能为空",
		},
		{
			name: "编辑操作数量超过限制",
			files: map[string]string{
				"file1.txt": "content\n",
			},
			edits: func() []struct {
				path       string
				oldString  string
				newString  string
				replaceAll bool
			} {
				edits := make([]struct {
					path       string
					oldString  string
					newString  string
					replaceAll bool
				}, 101)
				for i := range edits {
					edits[i] = struct {
						path       string
						oldString  string
						newString  string
						replaceAll bool
					}{path: "file1.txt", oldString: "content", newString: "new", replaceAll: false}
				}
				return edits
			}(),
			recordReads:    []string{},
			expectError:    true,
			expectContains: "编辑操作数量不能超过 100 个",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 临时放开路径校验（测试用临时目录不在工作目录内）
			orig := allowAllPaths
			allowAllPaths = true
			t.Cleanup(func() { allowAllPaths = orig })

			// 创建临时目录
			tmpDir := t.TempDir()

			// 创建初始文件
			for name, content := range tt.files {
				fullPath := filepath.Join(tmpDir, name)
				require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
			}

			// 创建 MCP host 和 tracker
			logger := zap.NewNop()
			host := mcphost.NewHost(logger)
			tracker := NewReadTracker(5 * time.Minute)

			// 注册工具
			registerMultiEdit(host, logger, tracker)

			// 记录文件读取
			for _, file := range tt.recordReads {
				tracker.RecordRead(filepath.Join(tmpDir, file))
			}

			// 构建编辑输入
			input := map[string]any{
				"edits": []map[string]any{},
			}
			for _, e := range tt.edits {
				input["edits"] = append(input["edits"].([]map[string]any), map[string]any{
					"path":        filepath.Join(tmpDir, e.path),
					"old_string":  e.oldString,
					"new_string":  e.newString,
					"replace_all": e.replaceAll,
				})
			}

			inputJSON, err := json.Marshal(input)
			require.NoError(t, err)

			// 调用工具
			result, err := host.ExecuteTool(context.Background(), "multiedit", inputJSON)
			require.NoError(t, err)

			// 验证结果
			if tt.expectError {
				assert.True(t, result.IsError, "期望错误但没有收到")
				if tt.expectContains != "" {
					var content string
					require.NoError(t, json.Unmarshal(result.Content, &content))
					assert.Contains(t, content, tt.expectContains)
				}
			} else {
				assert.False(t, result.IsError, "不期望错误但收到了: %v", result.Content)
			}

			// 验证最终文件内容
			if tt.expectFiles != nil {
				for name, expectedContent := range tt.expectFiles {
					fullPath := filepath.Join(tmpDir, name)
					actualContent, err := os.ReadFile(fullPath)
					require.NoError(t, err, "读取文件 %s 失败", name)
					assert.Equal(t, expectedContent, string(actualContent),
						"文件 %s 内容不匹配", name)
				}
			}
		})
	}
}

func TestMultiEditToolRegistration(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	tracker := NewReadTracker(5 * time.Minute)

	registerMultiEdit(host, logger, tracker)

	// 验证工具已注册
	tools := host.ListTools()
	found := false
	for _, tool := range tools {
		if tool.Name == "multiedit" {
			found = true
			assert.NotEmpty(t, tool.Description)
			assert.NotNil(t, tool.InputSchema)
			break
		}
	}
	assert.True(t, found, "multiedit 工具未注册")
}

func TestMultiEditLargeOperations(t *testing.T) {
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	tmpDir := t.TempDir()

	// 创建 50 个文件
	fileCount := 50
	for i := 0; i < fileCount; i++ {
		content := []string{
			"line 1\n",
			"line 2\n",
			"line 3\n",
		}
		filename := filepath.Join(tmpDir, "file"+string(rune('A'+i))+".txt")
		require.NoError(t, os.WriteFile(filename, []byte(strings.Join(content, "")), 0o644))
	}

	// 创建 MCP host 和 tracker
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	tracker := NewReadTracker(5 * time.Minute)
	registerMultiEdit(host, logger, tracker)

	// 构建编辑输入
	edits := []map[string]any{}
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('A'+i))+".txt")
		tracker.RecordRead(filename)
		edits = append(edits, map[string]any{
			"path":       filename,
			"old_string": "line 2",
			"new_string": "line TWO",
		})
	}

	input := map[string]any{"edits": edits}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	// 调用工具
	result, err := host.ExecuteTool(context.Background(), "multiedit", inputJSON)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// 验证所有文件都已更新
	for i := 0; i < fileCount; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('A'+i))+".txt")
		content, err := os.ReadFile(filename)
		require.NoError(t, err)
		assert.Contains(t, string(content), "line TWO")
		assert.NotContains(t, string(content), "line 2")
	}
}

func TestMultiEditRollbackOnWriteFailure(t *testing.T) {
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	tmpDir := t.TempDir()

	// 创建测试文件
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content 1\n"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("content 2\n"), 0o644))

	// 创建 tracker 和 operations
	tracker := NewReadTracker(5 * time.Minute)
	tracker.RecordRead(file1)
	tracker.RecordRead(file2)

	operations := []editOperation{
		{
			path:      file1,
			oldString: "content 1",
			newString: "NEW 1",
			backup:    "content 1\n",
		},
		{
			path:      "/nonexistent/dir/file2.txt", // 会导致写入失败
			oldString: "content 2",
			newString: "NEW 2",
			backup:    "content 2\n",
		},
	}

	// 准备备份
	for i := range operations {
		if i == 0 {
			data, _ := os.ReadFile(operations[i].path)
			operations[i].backup = string(data)
		}
	}

	logger := zap.NewNop()

	// 执行编辑（应该失败并回滚）
	_, err := executeMultiEdit(operations, tracker, logger)
	require.Error(t, err)

	// 验证 file1 保持原样（已回滚）
	content, err := os.ReadFile(file1)
	require.NoError(t, err)
	assert.Equal(t, "content 1\n", string(content))
}

func TestMultiEditConcurrentFileLock(t *testing.T) {
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	tmpDir := t.TempDir()

	// 创建一个共享文件，初始内容为 "AAA"
	sharedFile := filepath.Join(tmpDir, "shared.txt")
	require.NoError(t, os.WriteFile(sharedFile, []byte("AAA"), 0o644))

	logger := zap.NewNop()
	goroutines := 10
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	// 多个 goroutine 并发编辑同一文件
	// 每个 goroutine 读取文件 → 替换 → 写入，由 FileLock 保证序列化
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			tracker := NewReadTracker(5 * time.Minute)
			tracker.RecordRead(sharedFile)

			// 读取当前内容并做替换
			data, err := os.ReadFile(sharedFile)
			if err != nil {
				errCh <- err
				return
			}
			currentContent := string(data)

			ops := []editOperation{
				{
					path:       sharedFile,
					oldString:  currentContent,
					newString:  currentContent + "B",
					replaceAll: false,
				},
			}

			_, err = executeMultiEdit(ops, tracker, logger)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	// 验证：因为 goroutine 并发竞争，部分可能因 old_string 不匹配而失败，
	// 但不应出现数据损坏（文件内容始终是 "AAA" + 若干个 "B"）
	finalContent, err := os.ReadFile(sharedFile)
	require.NoError(t, err)

	content := string(finalContent)
	assert.True(t, strings.HasPrefix(content, "AAA"), "文件内容应以 AAA 开头，实际: %s", content)
	// 每个成功的 goroutine 追加一个 "B"
	for _, ch := range content[3:] {
		assert.Equal(t, 'B', ch, "文件内容应只包含 AAA 和 B，实际: %s", content)
	}
}

func TestMultiEditFileLockDoesNotAffectNormalFlow(t *testing.T) {
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "a.txt")
	file2 := filepath.Join(tmpDir, "b.txt")
	require.NoError(t, os.WriteFile(file1, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("world"), 0o644))

	logger := zap.NewNop()
	tracker := NewReadTracker(5 * time.Minute)
	tracker.RecordRead(file1)
	tracker.RecordRead(file2)

	ops := []editOperation{
		{path: file1, oldString: "hello", newString: "HELLO"},
		{path: file2, oldString: "world", newString: "WORLD"},
	}

	result, err := executeMultiEdit(ops, tracker, logger)
	require.NoError(t, err)
	assert.Contains(t, result, "成功编辑 2 个文件")

	c1, _ := os.ReadFile(file1)
	c2, _ := os.ReadFile(file2)
	assert.Equal(t, "HELLO", string(c1))
	assert.Equal(t, "WORLD", string(c2))
}
