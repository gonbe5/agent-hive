package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// allowAllPathsForTest 临时放开路径校验（测试用临时目录不在工作目录内）
func allowAllPathsForTest(t *testing.T) {
	t.Helper()
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })
}

func TestApplyPatch_Simple(t *testing.T) {
	allowAllPathsForTest(t)
	// 创建临时文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	original := `line1
line2
line3`

	if err := os.WriteFile(testFile, []byte(original), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 创建补丁
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3
`

	// 替换路径为实际路径
	patch = strings.ReplaceAll(patch, "test.txt", testFile)

	// 创建 MCP Host
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	// 准备输入
	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": false, // 不创建备份
	})

	// 执行工具
	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var msg string
		json.Unmarshal(result.Content, &msg)
		t.Fatalf("工具返回错误: %s", msg)
	}

	// 验证文件内容
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}

	expected := `line1
line2_modified
line3`

	if string(data) != expected {
		t.Errorf("文件内容不匹配\n期望:\n%s\n实际:\n%s", expected, string(data))
	}
}

func TestApplyPatch_MultipleFiles(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	os.WriteFile(file1, []byte("hello\nworld"), 0o644)
	os.WriteFile(file2, []byte("first line"), 0o644)

	patch := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,2 @@
 hello
-world
+universe
--- a/file2.txt
+++ b/file2.txt
@@ -1,1 +1,2 @@
 first line
+second line
`

	patch = strings.ReplaceAll(patch, "file1.txt", file1)
	patch = strings.ReplaceAll(patch, "file2.txt", file2)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": false,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var msg string
		json.Unmarshal(result.Content, &msg)
		t.Fatalf("工具返回错误: %s", msg)
	}

	// 验证 file1
	data1, _ := os.ReadFile(file1)
	if string(data1) != "hello\nuniverse" {
		t.Errorf("file1 内容不匹配: %q", string(data1))
	}

	// 验证 file2
	data2, _ := os.ReadFile(file2)
	if string(data2) != "first line\nsecond line" {
		t.Errorf("file2 内容不匹配: %q", string(data2))
	}
}

func TestApplyPatch_Reverse(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// 先写入修改后的内容
	modified := `line1
line2_modified
line3`

	os.WriteFile(testFile, []byte(modified), 0o644)

	// 创建补丁（正向）
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3
`

	patch = strings.ReplaceAll(patch, "test.txt", testFile)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	// 反向应用补丁
	inputJSON, _ := json.Marshal(map[string]any{
		"patch":   patch,
		"reverse": true,
		"backup":  false,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var msg string
		json.Unmarshal(result.Content, &msg)
		t.Fatalf("工具返回错误: %s", msg)
	}

	// 验证文件内容恢复
	data, _ := os.ReadFile(testFile)
	expected := `line1
line2
line3`

	if string(data) != expected {
		t.Errorf("反向应用后内容不匹配\n期望:\n%s\n实际:\n%s", expected, string(data))
	}
}

func TestApplyPatch_DryRun(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	original := `line1
line2
line3`

	os.WriteFile(testFile, []byte(original), 0o644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3
`

	patch = strings.ReplaceAll(patch, "test.txt", testFile)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	// Dry run
	inputJSON, _ := json.Marshal(map[string]any{
		"patch":   patch,
		"dry_run": true,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var msg string
		json.Unmarshal(result.Content, &msg)
		t.Fatalf("工具返回错误: %s", msg)
	}

	// 验证文件未被修改
	data, _ := os.ReadFile(testFile)
	if string(data) != original {
		t.Errorf("Dry run 不应该修改文件")
	}
}

func TestApplyPatch_Backup(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	original := `line1
line2
line3`

	os.WriteFile(testFile, []byte(original), 0o644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3
`

	patch = strings.ReplaceAll(patch, "test.txt", testFile)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": true,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	if result.IsError {
		var msg string
		json.Unmarshal(result.Content, &msg)
		t.Fatalf("工具返回错误: %s", msg)
	}

	// 验证备份文件存在
	backupFile := testFile + backupSuffix
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		t.Errorf("备份文件不存在: %s", backupFile)
	}

	// 验证备份内容
	backupData, _ := os.ReadFile(backupFile)
	if string(backupData) != original {
		t.Errorf("备份内容不匹配")
	}
}

func TestApplyPatch_ContextMismatch(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// 写入不匹配的内容
	os.WriteFile(testFile, []byte("wrong\ncontent\nhere"), 0o644)

	// 补丁期望不同的内容
	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3
`

	patch = strings.ReplaceAll(patch, "test.txt", testFile)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": false,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 应该返回错误
	if !result.IsError {
		t.Errorf("期望上下文不匹配返回错误")
	}
}

func TestApplyPatch_PathSafety(t *testing.T) {
	// 注意：这里不调用 allowAllPathsForTest，因为要测试路径校验生效
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "subdir", "test.txt")
	os.MkdirAll(filepath.Dir(testFile), 0o755)
	os.WriteFile(testFile, []byte("content"), 0o644)

	// 尝试使用 .. 路径遍历
	patch := `--- a/../../../etc/passwd
+++ b/../../../etc/passwd
@@ -1,1 +1,1 @@
-original
+hacked
`

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": false,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 应该返回错误
	if !result.IsError {
		t.Errorf("期望路径安全检查返回错误")
	}

	var msg string
	json.Unmarshal(result.Content, &msg)
	if !strings.Contains(msg, "路径安全校验失败") {
		t.Errorf("错误消息应包含'路径安全校验失败'，实际: %s", msg)
	}
}

func TestApplyPatch_SizeLimit(t *testing.T) {
	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	// 创建超大补丁
	hugePatch := "--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n" + strings.Repeat("A", maxPatchSize+1000)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch": hugePatch,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 应该返回错误
	if !result.IsError {
		t.Errorf("期望补丁大小限制返回错误")
	}

	var msg string
	json.Unmarshal(result.Content, &msg)
	if !strings.Contains(msg, "补丁过大") {
		t.Errorf("错误消息应包含'补丁过大'，实际: %s", msg)
	}
}

func TestApplyPatch_Rollback(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	os.WriteFile(file1, []byte("line1\nline2"), 0o644)
	os.WriteFile(file2, []byte("wrong content"), 0o644) // 故意不匹配

	// 补丁会应用到 file1，但在 file2 失败
	patch := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,2 @@
 line1
-line2
+line2_modified
--- a/file2.txt
+++ b/file2.txt
@@ -1,1 +1,1 @@
-expected content
+new content
`

	patch = strings.ReplaceAll(patch, "file1.txt", file1)
	patch = strings.ReplaceAll(patch, "file2.txt", file2)

	logger := zap.NewNop()
	host := mcphost.NewHost(logger)
	registerApplyPatch(host, logger)

	inputJSON, _ := json.Marshal(map[string]any{
		"patch":  patch,
		"backup": true,
	})

	result, err := host.ExecuteTool(context.Background(), "apply_patch", inputJSON)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 应该返回错误
	if !result.IsError {
		t.Errorf("期望应用失败返回错误")
	}

	// 验证 file1 被回滚（内容应该保持原样）
	data1, _ := os.ReadFile(file1)
	if string(data1) != "line1\nline2" {
		t.Errorf("file1 应该被回滚，实际内容: %q", string(data1))
	}

	// 验证备份文件被清理
	backupFile := file1 + backupSuffix
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Errorf("备份文件应该被清理")
	}
}

func TestApplyPatch_ConcurrentFileLock(t *testing.T) {
	allowAllPathsForTest(t)
	tmpDir := t.TempDir()

	// 创建共享文件，初始内容为 3 行
	sharedFile := filepath.Join(tmpDir, "shared.txt")
	initialContent := "line1\nline2\nline3"
	os.WriteFile(sharedFile, []byte(initialContent), 0o644)

	logger := zap.NewNop()
	goroutines := 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0

	// 多个 goroutine 并发 applypatch 同一文件
	// 只有第一个能成功（后续的 old content 不匹配），但不应出现数据损坏
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// 每个 goroutine 尝试把 line2 改为不同的值
			patch := fmt.Sprintf(`--- a/%s
+++ b/%s
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified_%d
 line3`, sharedFile, sharedFile, idx)

			result, err := applyPatch(
				mustParsePatch(t, patch),
				false, false, false, logger,
			)
			if err == nil && result != "" {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 验证文件内容完整性：不应出现部分写入或损坏
	finalContent, err := os.ReadFile(sharedFile)
	if err != nil {
		t.Fatalf("读取最终文件失败: %v", err)
	}

	content := string(finalContent)
	lines := strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Errorf("文件应有 3 行，实际 %d 行: %q", len(lines), content)
	}
	if lines[0] != "line1" {
		t.Errorf("第一行应为 line1，实际: %q", lines[0])
	}
	if lines[2] != "line3" {
		t.Errorf("第三行应为 line3，实际: %q", lines[2])
	}
	// 第二行要么是原始的 line2（全部失败），要么是某个 goroutine 的修改
	if !strings.HasPrefix(lines[1], "line2") {
		t.Errorf("第二行应以 line2 开头，实际: %q", lines[1])
	}
}

// mustParsePatch 解析补丁，失败时 t.Fatal
func mustParsePatch(t *testing.T, patchStr string) *Patch {
	t.Helper()
	patch, err := ParsePatch(patchStr)
	if err != nil {
		t.Fatalf("解析补丁失败: %v", err)
	}
	return patch
}
