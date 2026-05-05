package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/sandbox"
)

func newTestHost(t *testing.T) *mcphost.Host {
	t.Helper()
	orig := allowAllPaths
	allowAllPaths = true
	t.Cleanup(func() { allowAllPaths = orig })

	// 注入测试用 executor（SafeExecutorWrapper + LocalExecutor）
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("创建 PersistentShell 失败: %v", err)
	}
	inner := sandbox.NewLocalExecutor(shell, nil)
	wrapper := sandbox.NewSafeExecutorWrapper(inner, nil)
	oldExec := globalExecutor
	globalExecutor = wrapper
	t.Cleanup(func() {
		globalExecutor = oldExec
		shell.Close()
	})

	logger, _ := zap.NewDevelopment()
	host := mcphost.NewHost(logger)
	RegisterBuiltinTools(host, logger, nil, nil, nil, "", nil, nil, nil, nil, nil)
	return host
}

func TestReadFile(t *testing.T) {
	host := newTestHost(t)

	// Create a temp file
	tmp, err := os.CreateTemp("", "test-read-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString("line1\nline2\nline3\n")
	tmp.Close()
	defer os.Remove(tmp.Name())

	input, _ := json.Marshal(map[string]string{"path": tmp.Name()})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)
	if content == "" {
		t.Error("expected non-empty content")
	}
}

func TestReadFile_WorkspaceRootSlashPath(t *testing.T) {
	origAllowAll := allowAllPaths
	origWorkDir := os.Getenv("TOOLS_WORK_DIR")
	allowAllPaths = false
	root := t.TempDir()
	t.Setenv("TOOLS_WORK_DIR", root)
	t.Cleanup(func() {
		allowAllPaths = origAllowAll
		if origWorkDir == "" {
			os.Unsetenv("TOOLS_WORK_DIR")
		} else {
			os.Setenv("TOOLS_WORK_DIR", origWorkDir)
		}
	})

	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("workspace readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	host := newTestHost(t)
	allowAllPaths = false
	input, _ := json.Marshal(map[string]string{"path": "/README.md"})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		var msg string
		_ = json.Unmarshal(result.Content, &msg)
		t.Fatalf("unexpected error: %s", msg)
	}

	var content string
	_ = json.Unmarshal(result.Content, &content)
	if !strings.Contains(content, "workspace readme") {
		t.Fatalf("expected workspace readme, got %q", content)
	}
}

func TestReadFile_SystemAbsolutePathStillRejected(t *testing.T) {
	origAllowAll := allowAllPaths
	origWorkDir := os.Getenv("TOOLS_WORK_DIR")
	allowAllPaths = false
	root := t.TempDir()
	t.Setenv("TOOLS_WORK_DIR", root)
	t.Cleanup(func() {
		allowAllPaths = origAllowAll
		if origWorkDir == "" {
			os.Unsetenv("TOOLS_WORK_DIR")
		} else {
			os.Setenv("TOOLS_WORK_DIR", origWorkDir)
		}
	})

	host := newTestHost(t)
	allowAllPaths = false
	input, _ := json.Marshal(map[string]string{"path": "/etc/passwd"})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected /etc/passwd to remain rejected")
	}
	var msg string
	_ = json.Unmarshal(result.Content, &msg)
	if !strings.Contains(msg, "超出允许的工作目录") {
		t.Fatalf("expected workspace rejection, got %q", msg)
	}
}

func TestWriteFile(t *testing.T) {
	host := newTestHost(t)

	path := filepath.Join(t.TempDir(), "sub", "test.txt")
	input, _ := json.Marshal(map[string]string{
		"path":    path,
		"content": "hello world",
	})
	result, err := host.ExecuteTool(context.Background(), "write_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestEditFile(t *testing.T) {
	host := newTestHost(t)

	path := filepath.Join(t.TempDir(), "edit-test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	// 首先读取文件 (满足 ReadTracker 要求)
	readInput, _ := json.Marshal(map[string]string{"path": path})
	_, err := host.ExecuteTool(context.Background(), "read_file", readInput)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}

	input, _ := json.Marshal(map[string]any{
		"path":       path,
		"old_string": "world",
		"new_string": "go",
	})
	result, err := host.ExecuteTool(context.Background(), "edit", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello go" {
		t.Errorf("expected 'hello go', got %q", string(data))
	}
}

func TestBash(t *testing.T) {
	host := newTestHost(t)

	input, _ := json.Marshal(map[string]string{"command": "echo hello"})
	result, err := host.ExecuteTool(context.Background(), "bash", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var output string
	json.Unmarshal(result.Content, &output)
	if output != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", output)
	}
}

func TestAllToolsRegistered(t *testing.T) {
	host := newTestHost(t)

	tools := host.ListTools()
	expected := map[string]bool{
		"read_file":  false,
		"write_file": false,
		"glob":       false,
		"grep":       false,
		"bash":       false,
		"edit":       false,
		"ls":         false,
	}

	for _, tool := range tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// detectBinaryFile 测试
// ---------------------------------------------------------------------------

func TestDetectBinaryFile(t *testing.T) {
	t.Run("纯文本文件检测为非二进制", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "text.txt")
		os.WriteFile(tmp, []byte("这是一个纯文本文件\n包含中文和 English\n第三行内容\n"), 0o644)

		isBinary, _ := detectBinaryFile(tmp)
		if isBinary {
			t.Error("纯文本文件不应被检测为二进制")
		}
	})

	t.Run("含null字节的文件检测为二进制", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "binary.bin")
		data := []byte("hello\x00world\x00binary\x00content")
		os.WriteFile(tmp, data, 0o644)

		isBinary, _ := detectBinaryFile(tmp)
		if !isBinary {
			t.Error("含 null 字节的文件应被检测为二进制")
		}
	})

	t.Run("高比例非打印字符检测为二进制", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "mostly_binary.dat")
		// 创建 70% 控制字符的内容
		data := make([]byte, 100)
		for i := range data {
			if i < 70 {
				data[i] = byte(i%30 + 1) // 控制字符 (1-30, 排开 0)
			} else {
				data[i] = 'a'
			}
		}
		os.WriteFile(tmp, data, 0o644)

		isBinary, _ := detectBinaryFile(tmp)
		if !isBinary {
			t.Error("高比例非打印字符的文件应被检测为二进制")
		}
	})

	t.Run("空文件检测为非二进制", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "empty.txt")
		os.WriteFile(tmp, []byte{}, 0o644)

		isBinary, _ := detectBinaryFile(tmp)
		if isBinary {
			t.Error("空文件不应被检测为二进制")
		}
	})

	t.Run("含常见控制字符的文本文件为非二进制", func(t *testing.T) {
		tmp := filepath.Join(t.TempDir(), "with_tabs.txt")
		// 包含制表符、换行、回车的正常文本
		content := "line1\tcolumn2\r\nline2\tcolumn2\nline3\tend\n"
		os.WriteFile(tmp, []byte(content), 0o644)

		isBinary, _ := detectBinaryFile(tmp)
		if isBinary {
			t.Error("含制表符和换行符的文本不应被检测为二进制")
		}
	})

	t.Run("不存在的文件返回非二进制", func(t *testing.T) {
		isBinary, _ := detectBinaryFile("/nonexistent/path/file.txt")
		if isBinary {
			t.Error("不存在的文件应返回非二进制")
		}
	})
}

// ---------------------------------------------------------------------------
// truncateLongLines 测试
// ---------------------------------------------------------------------------
// detectMIMEType 测试
// ---------------------------------------------------------------------------

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantMIME  string
		wantImage bool
		wantPDF   bool
	}{
		{"PNG 文件", "/tmp/test.png", "image/png", true, false},
		{"JPG 文件", "/tmp/test.jpg", "image/jpeg", true, false},
		{"JPEG 文件", "/tmp/test.jpeg", "image/jpeg", true, false},
		{"GIF 文件", "/tmp/test.gif", "image/gif", true, false},
		{"WebP 文件", "/tmp/test.webp", "image/webp", true, false},
		{"SVG 文件", "/tmp/test.svg", "image/svg+xml", true, false},
		{"BMP 文件", "/tmp/test.bmp", "image/bmp", true, false},
		{"ICO 文件", "/tmp/test.ico", "image/x-icon", true, false},
		{"PDF 文件", "/tmp/test.pdf", "application/pdf", false, true},
		{"大写扩展名", "/tmp/test.PNG", "image/png", true, false},
		{"混合大小写", "/tmp/test.Jpg", "image/jpeg", true, false},
		{"Go 文件", "/tmp/test.go", "", false, false},
		{"文本文件", "/tmp/test.txt", "", false, false},
		{"无扩展名", "/tmp/testfile", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, isImage, isPDF := detectMIMEType(tt.path)
			if mime != tt.wantMIME {
				t.Errorf("MIME 类型 = %q, 期望 %q", mime, tt.wantMIME)
			}
			if isImage != tt.wantImage {
				t.Errorf("isImage = %v, 期望 %v", isImage, tt.wantImage)
			}
			if isPDF != tt.wantPDF {
				t.Errorf("isPDF = %v, 期望 %v", isPDF, tt.wantPDF)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// isBinaryByExtension 测试
// ---------------------------------------------------------------------------

func TestIsBinaryByExtension(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantBin bool
	}{
		{"EXE 文件", "/tmp/app.exe", true},
		{"ZIP 文件", "/tmp/archive.zip", true},
		{"SO 文件", "/tmp/lib.so", true},
		{"WASM 文件", "/tmp/module.wasm", true},
		{"MP4 文件", "/tmp/video.mp4", true},
		{"TTF 字体", "/tmp/font.ttf", true},
		{"Go 源码", "/tmp/main.go", false},
		{"文本文件", "/tmp/note.txt", false},
		{"PNG 图片不在此列", "/tmp/pic.png", false},
		{"PDF 不在此列", "/tmp/doc.pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBinaryByExtension(tt.path)
			if got != tt.wantBin {
				t.Errorf("isBinaryByExtension(%q) = %v, 期望 %v", tt.path, got, tt.wantBin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReadFile 图片/PDF base64 附件测试
// ---------------------------------------------------------------------------

func TestReadFile_ImageBase64(t *testing.T) {
	host := newTestHost(t)

	// 创建一个假 PNG 文件（真实 PNG 头 + 少量数据）
	tmp := filepath.Join(t.TempDir(), "test.png")
	// PNG 文件签名 + 少量数据
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01, 0x02, 0x03}
	os.WriteFile(tmp, pngData, 0o644)

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应返回错误: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)

	// 验证返回 data URI 格式
	expectedPrefix := "data:image/png;base64,"
	if !strings.HasPrefix(content, expectedPrefix) {
		t.Errorf("返回内容应以 %q 开头, 实际: %q", expectedPrefix, content[:min(len(content), 50)])
	}

	// 验证 base64 解码后与原始数据一致
	b64Part := strings.TrimPrefix(content, expectedPrefix)
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if string(decoded) != string(pngData) {
		t.Error("解码后的数据与原始数据不一致")
	}
}

func TestReadFile_PDFBase64(t *testing.T) {
	host := newTestHost(t)

	// 创建一个假 PDF 文件
	tmp := filepath.Join(t.TempDir(), "test.pdf")
	pdfData := []byte("%PDF-1.4 test content")
	os.WriteFile(tmp, pdfData, 0o644)

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应返回错误: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)

	expectedPrefix := "data:application/pdf;base64,"
	if !strings.HasPrefix(content, expectedPrefix) {
		t.Errorf("返回内容应以 %q 开头", expectedPrefix)
	}
}

func TestReadFile_JPEGBase64(t *testing.T) {
	host := newTestHost(t)

	tmp := filepath.Join(t.TempDir(), "photo.jpg")
	// JPEG 文件签名
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	os.WriteFile(tmp, jpegData, 0o644)

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应返回错误: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)

	if !strings.HasPrefix(content, "data:image/jpeg;base64,") {
		t.Errorf("JPEG 文件应返回 image/jpeg data URI")
	}
}

func TestReadFile_BinaryByExtension(t *testing.T) {
	host := newTestHost(t)

	// 创建一个 .zip 文件
	tmp := filepath.Join(t.TempDir(), "archive.zip")
	os.WriteFile(tmp, []byte{0x50, 0x4B, 0x03, 0x04, 0x00}, 0o644)

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应返回错误: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)

	if !strings.Contains(content, "二进制文件") {
		t.Error("ZIP 文件应被检测为二进制文件")
	}
	if !strings.Contains(content, ".zip") {
		t.Error("应显示扩展名 .zip")
	}
}

func TestReadFile_ImageTooLarge(t *testing.T) {
	host := newTestHost(t)

	// 创建一个假的大图片文件（使用稀疏写入模拟）
	tmp := filepath.Join(t.TempDir(), "huge.png")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	// 写入超过 10MB 的数据
	bigData := make([]byte, 11*1024*1024)
	bigData[0] = 0x89 // PNG 签名首字节
	f.Write(bigData)
	f.Close()

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if !result.IsError {
		t.Error("超大图片应返回错误")
	}

	var content string
	json.Unmarshal(result.Content, &content)
	if !strings.Contains(content, "文件过大") {
		t.Errorf("应包含文件过大提示, 实际: %s", content)
	}
}

func TestReadFile_SVGAsImage(t *testing.T) {
	host := newTestHost(t)

	// SVG 是文本格式的图片，但应按图片处理
	tmp := filepath.Join(t.TempDir(), "icon.svg")
	svgContent := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`
	os.WriteFile(tmp, []byte(svgContent), 0o644)

	input, _ := json.Marshal(map[string]string{"path": tmp})
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应返回错误: %s", result.Content)
	}

	var content string
	json.Unmarshal(result.Content, &content)

	if !strings.HasPrefix(content, "data:image/svg+xml;base64,") {
		t.Error("SVG 文件应返回 image/svg+xml data URI")
	}

	// 验证解码后内容一致
	b64Part := strings.TrimPrefix(content, "data:image/svg+xml;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	if string(decoded) != svgContent {
		t.Error("解码后的 SVG 内容与原始不一致")
	}
}

// --- read_file offset/limit/show_line_numbers 测试 ---

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "test-read-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString(content)
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })
	return tmp.Name()
}

func readFileResult(t *testing.T, host *mcphost.Host, params map[string]any) string {
	t.Helper()
	input, _ := json.Marshal(params)
	result, err := host.ExecuteTool(context.Background(), "read_file", input)
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	var content string
	json.Unmarshal(result.Content, &content)
	return content
}

func TestReadFileOffset(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "line1\nline2\nline3\nline4\nline5\n")

	content := readFileResult(t, host, map[string]any{"path": path, "offset": 2})
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if lines[0] != "line3" {
		t.Errorf("offset=2 应从 line3 开始，got %q", lines[0])
	}
	if len(lines) != 3 {
		t.Errorf("offset=2 应返回 3 行，got %d", len(lines))
	}
}

func TestReadFileLimit(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "line1\nline2\nline3\nline4\nline5\n")

	content := readFileResult(t, host, map[string]any{"path": path, "limit": 2})
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("limit=2 应返回 2 行，got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" {
		t.Errorf("limit=2 应返回 line1/line2，got %v", lines)
	}
}

func TestReadFileOffsetAndLimit(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "line1\nline2\nline3\nline4\nline5\n")

	content := readFileResult(t, host, map[string]any{"path": path, "offset": 1, "limit": 2})
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("offset=1,limit=2 应返回 2 行，got %d", len(lines))
	}
	if lines[0] != "line2" || lines[1] != "line3" {
		t.Errorf("offset=1,limit=2 应返回 line2/line3，got %v", lines)
	}
}

func TestReadFileOffsetBeyondEnd(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "line1\nline2\n")

	content := readFileResult(t, host, map[string]any{"path": path, "offset": 100})
	if content != "" && strings.TrimSpace(content) != "" {
		t.Errorf("offset 超出文件行数应返回空内容，got %q", content)
	}
}

func TestReadFileShowLineNumbers(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "alpha\nbeta\ngamma\n")

	content := readFileResult(t, host, map[string]any{"path": path, "show_line_numbers": true})
	if !strings.Contains(content, "\t") {
		t.Error("show_line_numbers=true 应包含 tab 分隔的行号")
	}
	// 第一行行号应为 1
	firstLine := strings.SplitN(content, "\n", 2)[0]
	if !strings.HasSuffix(strings.TrimSpace(strings.SplitN(firstLine, "\t", 2)[0]), "1") {
		t.Errorf("第一行行号应为 1，got %q", firstLine)
	}
}

func TestReadFileShowLineNumbersWithOffset(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "line1\nline2\nline3\nline4\nline5\n")

	// offset=2 时，第一行显示的行号应为 3（原始文件行号）
	content := readFileResult(t, host, map[string]any{"path": path, "offset": 2, "show_line_numbers": true})
	firstLine := strings.SplitN(content, "\n", 2)[0]
	numStr := strings.TrimSpace(strings.SplitN(firstLine, "\t", 2)[0])
	if numStr != "3" {
		t.Errorf("offset=2 时第一行行号应为 3，got %q", numStr)
	}
}

func TestReadFileDefaultNoLineNumbers(t *testing.T) {
	host := newTestHost(t)
	path := writeTestFile(t, "hello\nworld\n")

	// 默认不加行号，输出与原始内容一致
	content := readFileResult(t, host, map[string]any{"path": path})
	if strings.Contains(content, "\t") {
		t.Error("默认输出不应包含行号 tab 分隔符")
	}
	if !strings.Contains(content, "hello") || !strings.Contains(content, "world") {
		t.Error("默认输出应包含原始内容")
	}
}

func TestReadFileGBK(t *testing.T) {
	host := newTestHost(t)

	// GBK 编码的"你好世界"
	gbkBytes := []byte{0xC4, 0xE3, 0xBA, 0xC3, 0xCA, 0xC0, 0xBD, 0xE7}
	tmp, err := os.CreateTemp("", "test-gbk-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Write(gbkBytes)
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	result, err := host.ExecuteTool(context.Background(), "read_file",
		func() json.RawMessage { b, _ := json.Marshal(map[string]string{"path": tmp.Name()}); return b }())
	if err != nil {
		t.Fatalf("ExecuteTool: %v", err)
	}
	// GBK 文件不应被识别为二进制
	if result.IsError {
		t.Fatalf("GBK 文件不应返回错误: %s", result.Content)
	}
	var content string
	json.Unmarshal(result.Content, &content)
	if strings.Contains(content, "二进制") {
		t.Errorf("GBK 文件不应被识别为二进制，got %q", content)
	}
	// 解码后的内容应为"你好世界"
	if !strings.Contains(content, "你好世界") {
		t.Errorf("GBK 文件应解码为\"你好世界\"，got %q", content)
	}
}
