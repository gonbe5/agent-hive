package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/sandbox"
)

// mockExecutor 记录收到的命令，返回预设结果。
type mockExecutor struct {
	lastCommand string
	result      sandbox.ExecResult
	err         error
}

func (m *mockExecutor) Execute(_ context.Context, req sandbox.ExecRequest) (sandbox.ExecResult, error) {
	m.lastCommand = req.Command
	return m.result, m.err
}

func (m *mockExecutor) Close() error { return nil }

// --- Glob 测试 ---

func setupGlobTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// 创建测试目录结构
	dirs := []string{
		"src",
		"src/components",
		"src/utils",
		"pkg",
	}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}

	files := []string{
		"main.go",
		"README.md",
		"src/app.ts",
		"src/index.tsx",
		"src/components/Button.tsx",
		"src/components/Modal.tsx",
		"src/utils/helper.ts",
		"pkg/lib.go",
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(root, f), []byte("// "+f), 0o644)
	}
	return root
}

func TestBasicGlob_BasenameMatch(t *testing.T) {
	root := setupGlobTestDir(t)
	g := NewBasicGlob()

	matches, err := g.Glob(context.Background(), "*.go", root)
	if err != nil {
		t.Fatal(err)
	}

	// BasicGlob 只做 basename 匹配，应该匹配所有 .go 文件
	var names []string
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	sort.Strings(names)

	if len(names) != 2 || names[0] != "lib.go" || names[1] != "main.go" {
		t.Errorf("expected [lib.go main.go], got %v", names)
	}
}

func TestDoublestarGlob_RecursiveMatch(t *testing.T) {
	root := setupGlobTestDir(t)
	g := NewDoublestarGlob()

	// ** 递归匹配所有 .tsx 文件
	matches, err := g.Glob(context.Background(), "**/*.tsx", root)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, m := range matches {
		// 去掉 root 前缀，只保留相对路径
		rel := strings.TrimPrefix(m, root+"/")
		names = append(names, rel)
	}
	sort.Strings(names)

	expected := []string{"src/components/Button.tsx", "src/components/Modal.tsx", "src/index.tsx"}
	if len(names) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, names)
	}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("index %d: expected %s, got %s", i, expected[i], n)
		}
	}
}

func TestDoublestarGlob_SubdirPattern(t *testing.T) {
	root := setupGlobTestDir(t)
	g := NewDoublestarGlob()

	// 只匹配 src/components 下的文件
	matches, err := g.Glob(context.Background(), "src/components/*.tsx", root)
	if err != nil {
		t.Fatal(err)
	}

	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestDoublestarGlob_NoMatch(t *testing.T) {
	root := setupGlobTestDir(t)
	g := NewDoublestarGlob()

	matches, err := g.Glob(context.Background(), "**/*.py", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("expected no matches, got %v", matches)
	}
}

func TestDoublestarGlob_DefaultRoot(t *testing.T) {
	g := NewDoublestarGlob()

	// 空 root 应该默认为 "."
	matches, err := g.Glob(context.Background(), "*.go", "")
	if err != nil {
		t.Fatal(err)
	}
	// 不检查具体结果，只确保不报错
	_ = matches
}

// --- shellQuote 测试 ---

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\\''s'"},
		{"$(curl evil)", "'$(curl evil)'"},
		{"`whoami`", "'`whoami`'"},
		{"; rm -rf /", "'; rm -rf /'"},
		{"", "''"},
	}
	for _, tt := range tests {
		result := shellQuote(tt.input)
		if result != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, result, tt.want)
		}
	}
}

// --- Grep 输出解析测试 ---

func TestParseGrepOutput_Standard(t *testing.T) {
	output := `main.go:10:func main() {
main.go:15:	fmt.Println("hello")
src/app.ts:3:export function app() {`

	result := parseGrepOutput(output)
	if result.Total != 3 {
		t.Errorf("expected 3 matches, got %d", result.Total)
	}
	if result.Matches[0].File != "main.go" || result.Matches[0].Line != 10 {
		t.Errorf("unexpected first match: %+v", result.Matches[0])
	}
	if result.Matches[2].File != "src/app.ts" || result.Matches[2].Line != 3 {
		t.Errorf("unexpected third match: %+v", result.Matches[2])
	}
}

func TestParseGrepOutput_Empty(t *testing.T) {
	result := parseGrepOutput("")
	if result.Total != 0 || len(result.Matches) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

func TestParseGrepOutput_MalformedLines(t *testing.T) {
	output := `main.go:10:valid line
--
not a match line
main.go:abc:invalid line number`

	result := parseGrepOutput(output)
	// 只有第一行是有效的
	if result.Total != 1 {
		t.Errorf("expected 1 match, got %d", result.Total)
	}
}

func TestParseGrepOutput_ContextLines(t *testing.T) {
	// grep -C1 输出格式：匹配行用 :，上下文行用 -
	output := `main.go-9-// before context
main.go:10:func main() {
main.go-11-	// after context
--
src/app.ts:20:export default app`

	result := parseGrepOutput(output)
	if result.Total != 4 {
		t.Errorf("expected 4 matches (2 match + 2 context), got %d", result.Total)
	}
	// 第一条是上下文行
	if result.Matches[0].File != "main.go" || result.Matches[0].Line != 9 {
		t.Errorf("context line parse failed: %+v", result.Matches[0])
	}
	// 第二条是匹配行
	if result.Matches[1].File != "main.go" || result.Matches[1].Line != 10 {
		t.Errorf("match line parse failed: %+v", result.Matches[1])
	}
	// 第三条是上下文行
	if result.Matches[2].File != "main.go" || result.Matches[2].Line != 11 {
		t.Errorf("after context line parse failed: %+v", result.Matches[2])
	}
}

func TestParseGrepOutput_GroupSeparator(t *testing.T) {
	output := `a.go:1:first
--
b.go:5:second`

	result := parseGrepOutput(output)
	if result.Total != 2 {
		t.Errorf("expected 2 matches (-- separator skipped), got %d", result.Total)
	}
}

func TestParseContextLine_NumericFilename(t *testing.T) {
	// 文件名含数字和连字符：foo-2024-bar.go-17-text
	// 应解析为 file=foo-2024-bar.go, line=17, content=text
	output := `foo-2024-bar.go-17-some context text`

	result := parseGrepOutput(output)
	if result.Total != 1 {
		t.Fatalf("expected 1 match, got %d", result.Total)
	}
	m := result.Matches[0]
	if m.File != "foo-2024-bar.go" {
		t.Errorf("expected file 'foo-2024-bar.go', got %q", m.File)
	}
	if m.Line != 17 {
		t.Errorf("expected line 17, got %d", m.Line)
	}
	if m.Content != "some context text" {
		t.Errorf("expected content 'some context text', got %q", m.Content)
	}
}

func TestShellGrep_MultilineError(t *testing.T) {
	// ShellGrep 不支持 multiline，应返回明确错误
	g := NewShellGrep(nil) // executor 不会被调用
	_, err := g.Grep(context.Background(), GrepRequest{
		Pattern:   "test",
		Multiline: true,
	})
	if err == nil {
		t.Fatal("expected error for multiline on ShellGrep")
	}
	if !strings.Contains(err.Error(), "跨行匹配") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestShellGrep_TypeFilterError(t *testing.T) {
	g := NewShellGrep(nil)
	_, err := g.Grep(context.Background(), GrepRequest{
		Pattern:    "test",
		TypeFilter: "go",
	})
	if err == nil {
		t.Fatal("expected error for type filter on ShellGrep")
	}
	if !strings.Contains(err.Error(), "文件类型过滤") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

// --- ShellGrep 集成测试（mock executor）---

func TestShellGrep_CommandBuilding(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{Stdout: "main.go:1:hello\n", ExitCode: 0},
	}
	g := NewShellGrep(mock)

	_, err := g.Grep(context.Background(), GrepRequest{
		Pattern:    "hello",
		Path:       "/tmp/test",
		GlobFilter: "*.go",
		Context:    3,
		Before:     2,
		After:      1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证命令包含所有参数
	cmd := mock.lastCommand
	if !strings.Contains(cmd, "-rn") {
		t.Error("missing -rn flag")
	}
	if !strings.Contains(cmd, "--color=never") {
		t.Error("missing --color=never")
	}
	if !strings.Contains(cmd, "--include='*.go'") {
		t.Errorf("missing or unquoted --include, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-C3") {
		t.Error("missing -C3")
	}
	if !strings.Contains(cmd, "-B2") {
		t.Error("missing -B2")
	}
	if !strings.Contains(cmd, "-A1") {
		t.Error("missing -A1")
	}
	// pattern 应该被 shell 转义
	if !strings.Contains(cmd, "'hello'") {
		t.Errorf("pattern not shell-quoted, got: %s", cmd)
	}
	// -- 分隔符防止 pattern 被解释为 flag
	if !strings.Contains(cmd, "-- ") {
		t.Errorf("missing -- separator, got: %s", cmd)
	}
	if !strings.Contains(cmd, "'/tmp/test'") {
		t.Errorf("search path not shell-quoted, got: %s", cmd)
	}
}

func TestShellGrep_PathInjection(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{ExitCode: 1},
	}
	g := NewShellGrep(mock)

	_, _ = g.Grep(context.Background(), GrepRequest{
		Pattern: "test",
		Path:    "/tmp/; rm -rf /",
	})

	// 验证路径中的注入 payload 被转义
	if !strings.Contains(mock.lastCommand, "'/tmp/; rm -rf /'") {
		t.Errorf("path injection not escaped, got: %s", mock.lastCommand)
	}
}

func TestShellGrep_PatternInjection(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{ExitCode: 1}, // no match
	}
	g := NewShellGrep(mock)

	_, err := g.Grep(context.Background(), GrepRequest{
		Pattern: "$(rm -rf /)",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证注入 payload 被转义
	if !strings.Contains(mock.lastCommand, "'$(rm -rf /)'") {
		t.Errorf("injection not escaped, got: %s", mock.lastCommand)
	}
}

func TestShellGrep_ExecutorError(t *testing.T) {
	mock := &mockExecutor{
		err: fmt.Errorf("connection refused"),
	}
	g := NewShellGrep(mock)

	_, err := g.Grep(context.Background(), GrepRequest{Pattern: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "grep 执行失败") {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestShellGrep_NonZeroExitCode(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{ExitCode: 2, Stderr: "invalid regex"},
	}
	g := NewShellGrep(mock)

	_, err := g.Grep(context.Background(), GrepRequest{Pattern: "[invalid"})
	if err == nil {
		t.Fatal("expected error for exit code 2")
	}
	if !strings.Contains(err.Error(), "grep 退出码 2") {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestShellGrep_NoMatch(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{ExitCode: 1},
	}
	g := NewShellGrep(mock)

	result, err := g.Grep(context.Background(), GrepRequest{Pattern: "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Errorf("expected empty result, got %d matches", len(result.Matches))
	}
}

func TestShellGrep_DefaultPath(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{ExitCode: 1},
	}
	g := NewShellGrep(mock)

	_, _ = g.Grep(context.Background(), GrepRequest{Pattern: "test"})
	// 空 Path 应该默认为 "."（shell 转义后为 '.'）
	if !strings.HasSuffix(mock.lastCommand, "'.'") {
		t.Errorf("expected default path \"'.'\", got: %s", mock.lastCommand)
	}
}

// --- RipgrepEngine 集成测试（mock executor）---

func TestRipgrepEngine_CommandBuilding(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{Stdout: "main.go:1:hello\n", ExitCode: 0},
	}
	rg := &RipgrepEngine{exec: mock, fallback: NewShellGrep(mock), hasRg: true}

	_, err := rg.Grep(context.Background(), GrepRequest{
		Pattern:    "hello",
		Path:       "/tmp/test",
		GlobFilter: "*.go",
		TypeFilter: "go",
		Context:    3,
		Before:     2,
		After:      1,
		Multiline:  true,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := mock.lastCommand
	if !strings.HasPrefix(cmd, "rg ") {
		t.Errorf("expected rg command, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--no-heading") {
		t.Error("missing --no-heading")
	}
	if !strings.Contains(cmd, "--glob='*.go'") {
		t.Errorf("missing or unquoted --glob, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--type='go'") {
		t.Errorf("missing or unquoted --type, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-C3") {
		t.Error("missing -C3")
	}
	if !strings.Contains(cmd, "-B2") {
		t.Error("missing -B2")
	}
	if !strings.Contains(cmd, "-A1") {
		t.Error("missing -A1")
	}
	if !strings.Contains(cmd, "-U") || !strings.Contains(cmd, "--multiline-dotall") {
		t.Error("missing multiline flags")
	}
	if !strings.Contains(cmd, "--max-count=10") {
		t.Error("missing --max-count")
	}
	if !strings.Contains(cmd, "'hello'") {
		t.Errorf("pattern not shell-quoted, got: %s", cmd)
	}
	if !strings.Contains(cmd, "-- ") {
		t.Errorf("missing -- separator, got: %s", cmd)
	}
}

func TestRipgrepEngine_FallbackToShellGrep(t *testing.T) {
	mock := &mockExecutor{
		result: sandbox.ExecResult{Stdout: "a.go:1:test\n", ExitCode: 0},
	}
	rg := &RipgrepEngine{exec: mock, fallback: NewShellGrep(mock), hasRg: false}

	result, err := rg.Grep(context.Background(), GrepRequest{Pattern: "test"})
	if err != nil {
		t.Fatal(err)
	}

	// fallback 应该使用 grep 而非 rg
	if !strings.HasPrefix(mock.lastCommand, "grep ") {
		t.Errorf("expected grep fallback, got: %s", mock.lastCommand)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 match, got %d", result.Total)
	}
}

func TestRipgrepEngine_FallbackMultilineError(t *testing.T) {
	mock := &mockExecutor{}
	rg := &RipgrepEngine{exec: mock, fallback: NewShellGrep(mock), hasRg: false}

	// fallback 到 ShellGrep 时，Multiline 应该报错
	_, err := rg.Grep(context.Background(), GrepRequest{
		Pattern:   "test",
		Multiline: true,
	})
	if err == nil {
		t.Fatal("expected error for multiline on fallback")
	}
	if !strings.Contains(err.Error(), "跨行匹配") {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

func TestRipgrepEngine_HasRipgrep(t *testing.T) {
	rg := &RipgrepEngine{hasRg: true}
	if !rg.HasRipgrep() {
		t.Error("expected HasRipgrep() == true")
	}
	rg2 := &RipgrepEngine{hasRg: false}
	if rg2.HasRipgrep() {
		t.Error("expected HasRipgrep() == false")
	}
}

// --- mergeMultilineMatches 测试 ---

func TestMergeMultilineMatches_ConsecutiveLines(t *testing.T) {
	// 模拟 rg -U 跨行匹配输出：同文件连续行号应合并
	result := &GrepResult{
		Matches: []GrepMatch{
			{File: "main.go", Line: 10, Content: "type Foo struct {"},
			{File: "main.go", Line: 11, Content: "    Name string"},
			{File: "main.go", Line: 12, Content: "}"},
		},
		Total: 3,
	}

	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 1 {
		t.Fatalf("expected 1 merged match, got %d", len(merged.Matches))
	}
	if merged.Matches[0].Line != 10 {
		t.Errorf("expected line 10, got %d", merged.Matches[0].Line)
	}
	expected := "type Foo struct {\n    Name string\n}"
	if merged.Matches[0].Content != expected {
		t.Errorf("expected merged content %q, got %q", expected, merged.Matches[0].Content)
	}
}

func TestMergeMultilineMatches_DifferentFiles(t *testing.T) {
	// 不同文件的连续行号不应合并
	result := &GrepResult{
		Matches: []GrepMatch{
			{File: "a.go", Line: 5, Content: "line a"},
			{File: "b.go", Line: 6, Content: "line b"},
		},
		Total: 2,
	}

	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 2 {
		t.Fatalf("expected 2 matches (different files), got %d", len(merged.Matches))
	}
}

func TestMergeMultilineMatches_NonConsecutive(t *testing.T) {
	// 同文件但行号不连续，不应合并
	result := &GrepResult{
		Matches: []GrepMatch{
			{File: "main.go", Line: 10, Content: "first match"},
			{File: "main.go", Line: 20, Content: "second match"},
		},
		Total: 2,
	}

	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 2 {
		t.Fatalf("expected 2 matches (non-consecutive), got %d", len(merged.Matches))
	}
}

func TestMergeMultilineMatches_Mixed(t *testing.T) {
	// 混合场景：两个跨行匹配 + 一个单行匹配
	result := &GrepResult{
		Matches: []GrepMatch{
			{File: "a.go", Line: 1, Content: "func foo() {"},
			{File: "a.go", Line: 2, Content: "    return"},
			{File: "a.go", Line: 3, Content: "}"},
			{File: "b.go", Line: 10, Content: "single line"},
			{File: "a.go", Line: 20, Content: "func bar() {"},
			{File: "a.go", Line: 21, Content: "}"},
		},
		Total: 6,
	}

	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 3 {
		t.Fatalf("expected 3 merged matches, got %d", len(merged.Matches))
	}
	if merged.Matches[0].Content != "func foo() {\n    return\n}" {
		t.Errorf("first match content wrong: %q", merged.Matches[0].Content)
	}
	if merged.Matches[1].Content != "single line" {
		t.Errorf("second match content wrong: %q", merged.Matches[1].Content)
	}
	if merged.Matches[2].Content != "func bar() {\n}" {
		t.Errorf("third match content wrong: %q", merged.Matches[2].Content)
	}
}

func TestMergeMultilineMatches_SingleMatch(t *testing.T) {
	result := &GrepResult{
		Matches: []GrepMatch{
			{File: "a.go", Line: 1, Content: "only one"},
		},
		Total: 1,
	}
	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(merged.Matches))
	}
}

func TestMergeMultilineMatches_Empty(t *testing.T) {
	result := &GrepResult{}
	merged := mergeMultilineMatches(result)
	if len(merged.Matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(merged.Matches))
	}
}
