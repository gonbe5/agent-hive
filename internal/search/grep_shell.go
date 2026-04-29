package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/sandbox"
)

// shellQuote 对字符串做 shell 单引号转义，防止命令注入。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
type ShellGrep struct {
	exec sandbox.Executor
}

func NewShellGrep(exec sandbox.Executor) *ShellGrep {
	return &ShellGrep{exec: exec}
}

func (s *ShellGrep) Grep(ctx context.Context, req GrepRequest) (*GrepResult, error) {
	// ShellGrep 不支持 multiline 和 type filter，明确报错而非静默忽略
	if req.Multiline {
		return nil, fmt.Errorf("系统 grep 不支持跨行匹配（multiline），请安装 ripgrep (rg)")
	}
	if req.TypeFilter != "" {
		return nil, fmt.Errorf("系统 grep 不支持按文件类型过滤（type filter），请安装 ripgrep (rg)")
	}

	args := []string{"-rn", "--color=never"}

	if req.GlobFilter != "" {
		args = append(args, "--include="+shellQuote(req.GlobFilter))
	}
	if req.Context > 0 {
		args = append(args, fmt.Sprintf("-C%d", req.Context))
	}
	if req.Before > 0 {
		args = append(args, fmt.Sprintf("-B%d", req.Before))
	}
	if req.After > 0 {
		args = append(args, fmt.Sprintf("-A%d", req.After))
	}

	args = append(args, "--", shellQuote(req.Pattern))

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}
	args = append(args, shellQuote(searchPath))

	fullCommand := fmt.Sprintf("grep %s", strings.Join(args, " "))

	result, err := s.exec.Execute(ctx, sandbox.ExecRequest{
		Command:   fullCommand,
		SessionID: "grep",
		Timeout:   60 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("grep 执行失败: %w", err)
	}
	if result.ExitCode == 1 {
		return &GrepResult{}, nil // 未找到匹配
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("grep 退出码 %d: %s", result.ExitCode, result.Stderr)
	}

	return parseGrepOutput(result.Stdout), nil
}

// parseGrepOutput 解析 grep/rg 输出为结构化结果。
// 支持两种行格式：
//   - 匹配行: file:line:content（冒号分隔）
//   - 上下文行: file-line-content（连字符分隔，-B/-A/-C 产生）
//   - 分组分隔符: -- （直接跳过）
func parseGrepOutput(output string) *GrepResult {
	output = strings.TrimSpace(output)
	if output == "" {
		return &GrepResult{}
	}

	lines := strings.Split(output, "\n")
	var matches []GrepMatch
	for _, line := range lines {
		// 跳过分组分隔符
		if line == "--" {
			continue
		}

		// 优先尝试匹配行格式: file:line:content
		if m, ok := parseGrepLine(line, ":"); ok {
			matches = append(matches, m)
			continue
		}

		// 尝试上下文行格式: file-line-content
		// 注意：文件名本身可能包含 -，所以从右侧查找
		if m, ok := parseContextLine(line); ok {
			matches = append(matches, m)
		}
	}
	return &GrepResult{Matches: matches, Total: len(matches)}
}

// mergeMultilineMatches 合并 multiline 模式下同一文件连续行号的匹配为一条。
// rg -U 输出跨行匹配时，每行仍然是 file:line:content 格式，
// 同一个匹配的多行之间行号连续。合并后 Content 用 \n 拼接，Line 取首行行号。
func mergeMultilineMatches(result *GrepResult) *GrepResult {
	if len(result.Matches) <= 1 {
		return result
	}

	var merged []GrepMatch
	cur := result.Matches[0]

	for i := 1; i < len(result.Matches); i++ {
		m := result.Matches[i]
		// 同文件、行号连续 → 属于同一个跨行匹配
		if m.File == cur.File && m.Line == cur.Line+strings.Count(cur.Content, "\n")+1 {
			cur.Content += "\n" + m.Content
		} else {
			merged = append(merged, cur)
			cur = m
		}
	}
	merged = append(merged, cur)

	return &GrepResult{Matches: merged, Total: len(merged)}
}

// parseGrepLine 尝试用指定分隔符解析 grep 输出行。
func parseGrepLine(line, sep string) (GrepMatch, bool) {
	parts := strings.SplitN(line, sep, 3)
	if len(parts) < 3 {
		return GrepMatch{}, false
	}
	lineNum, err := strconv.Atoi(parts[1])
	if err != nil {
		return GrepMatch{}, false
	}
	return GrepMatch{
		File:    parts[0],
		Line:    lineNum,
		Content: parts[2],
	}, true
}

// parseContextLine 解析上下文行（file-line-content 格式）。
// 上下文行用 - 分隔，但文件名也可能包含 -，
// 所以从右向左扫描，找到最后一个 "-纯数字-" 模式。
// 这样 foo-2024-bar.go-17-text 会正确解析为 file=foo-2024-bar.go, line=17。
func parseContextLine(line string) (GrepMatch, bool) {
	// 从右向左扫描，找到最后一个 "-数字-" 模式
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] != '-' {
			continue
		}
		// 向左找前一个 -
		left := strings.LastIndex(line[:i], "-")
		if left < 0 {
			continue
		}
		numStr := line[left+1 : i]
		lineNum, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		file := line[:left]
		if file == "" {
			continue
		}
		content := line[i+1:]
		return GrepMatch{
			File:    file,
			Line:    lineNum,
			Content: content,
		}, true
	}
	return GrepMatch{}, false
}
