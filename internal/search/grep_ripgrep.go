package search

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/sandbox"
)

// detectRipgrep 检测系统中是否安装了 ripgrep。
func detectRipgrep() bool {
	_, err := exec.LookPath("rg")
	return err == nil
}

// RipgrepEngine 使用 ripgrep (rg) 实现高级搜索。
// 支持 multiline、context、type filter 等高级特性。
// 运行时检测 rg 是否存在，不存在则 fallback 到 ShellGrep。
type RipgrepEngine struct {
	exec     sandbox.Executor
	fallback *ShellGrep
	hasRg    bool
}

func NewRipgrepEngine(exec sandbox.Executor) *RipgrepEngine {
	return &RipgrepEngine{
		exec:     exec,
		fallback: NewShellGrep(exec),
		hasRg:    detectRipgrep(),
	}
}

func (r *RipgrepEngine) Grep(ctx context.Context, req GrepRequest) (*GrepResult, error) {
	if !r.hasRg {
		return r.fallback.Grep(ctx, req)
	}

	args := []string{"--no-heading", "--line-number", "--color=never"}

	if req.GlobFilter != "" {
		args = append(args, "--glob="+shellQuote(req.GlobFilter))
	}
	if req.TypeFilter != "" {
		args = append(args, "--type="+shellQuote(req.TypeFilter))
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
	if req.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if req.MaxResults > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", req.MaxResults))
	}

	args = append(args, "--", shellQuote(req.Pattern))

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}
	args = append(args, shellQuote(searchPath))
	fullCommand := fmt.Sprintf("rg %s", strings.Join(args, " "))

	result, err := r.exec.Execute(ctx, sandbox.ExecRequest{
		Command:   fullCommand,
		SessionID: "rg",
		Timeout:   60 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("rg 执行失败: %w", err)
	}
	if result.ExitCode == 1 {
		return &GrepResult{}, nil
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("rg 退出码 %d: %s", result.ExitCode, result.Stderr)
	}

	parsed := parseGrepOutput(result.Stdout)
	if req.Multiline {
		parsed = mergeMultilineMatches(parsed)
	}
	return parsed, nil
}

// HasRipgrep 返回是否检测到 ripgrep。
func (r *RipgrepEngine) HasRipgrep() bool {
	return r.hasRg
}
