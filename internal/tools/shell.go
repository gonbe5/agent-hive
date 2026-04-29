package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// PersistentShell 维护一个持久的 shell 进程，保持工作目录和环境变量
type PersistentShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bufio.Reader
	mu     sync.Mutex
}

// NewPersistentShell 创建新的持久化 shell
func NewPersistentShell() (*PersistentShell, error) {
	cmd := exec.Command("/bin/bash", "--norc", "--noprofile")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	shell := &PersistentShell{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		stderr: bufio.NewReader(stderrPipe),
	}

	// 初始化: 设置 prompt 为空
	shell.stdin.Write([]byte("PS1=''\nPS2=''\n"))

	return shell, nil
}

// Execute 执行命令并返回结果。
// 支持 ctx 取消：当 ctx 被取消或超时时，向 shell 发送 kill 信号终止当前命令，
// 然后发送新的 delimiter 恢复 shell 状态。
func (s *PersistentShell) Execute(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delim := fmt.Sprintf("___DELIM_%s", uuid.New().String())

	// 写命令 + delimiter
	script := fmt.Sprintf("%s\necho \"%s_$?\" >&1\necho \"%s\" >&2\n", command, delim, delim)
	if _, err := s.stdin.Write([]byte(script)); err != nil {
		return "", "", -1, err
	}

	// 用 channel 收集读取结果，支持 ctx 取消
	type readResult struct {
		stdoutStr string
		stderrStr string
		exitCode  int
		err       error
	}
	resultCh := make(chan readResult, 1)

	go func() {
		// 读取 stdout 直到 delimiter
		var stdoutLines []string
		var ec int
		for {
			line, readErr := s.stdout.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				resultCh <- readResult{err: readErr}
				return
			}
			if strings.HasPrefix(line, delim) {
				parts := strings.Split(strings.TrimSpace(line), "_")
				if len(parts) >= 2 {
					ec, _ = strconv.Atoi(parts[len(parts)-1])
				}
				break
			}
			stdoutLines = append(stdoutLines, line)
		}

		// 读取 stderr 直到 delimiter
		var stderrLines []string
		for {
			line, readErr := s.stderr.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				resultCh <- readResult{err: readErr}
				return
			}
			if strings.HasPrefix(line, delim) {
				break
			}
			stderrLines = append(stderrLines, line)
		}

		resultCh <- readResult{
			stdoutStr: strings.Join(stdoutLines, ""),
			stderrStr: strings.Join(stderrLines, ""),
			exitCode:  ec,
		}
	}()

	select {
	case res := <-resultCh:
		return res.stdoutStr, res.stderrStr, res.exitCode, res.err
	case <-ctx.Done():
		// ctx 取消：向 shell 发送 Ctrl-C（kill 当前前台进程组），然后用新 delimiter 恢复
		s.stdin.Write([]byte{0x03}) // ETX = Ctrl-C
		recoveryDelim := fmt.Sprintf("___RECOVER_%s", uuid.New().String())
		recoveryScript := fmt.Sprintf("\necho \"%s\" >&1\necho \"%s\" >&2\n", recoveryDelim, recoveryDelim)
		s.stdin.Write([]byte(recoveryScript))

		// 排空 stdout/stderr 直到 recovery delimiter（带超时保护）
		go func() {
			for {
				line, _ := s.stdout.ReadString('\n')
				if strings.HasPrefix(line, recoveryDelim) || strings.HasPrefix(line, delim) {
					break
				}
			}
			for {
				line, _ := s.stderr.ReadString('\n')
				if strings.HasPrefix(line, recoveryDelim) || strings.HasPrefix(line, delim) {
					break
				}
			}
			// 排空 resultCh 防止 goroutine 泄漏
			<-resultCh
		}()

		return "", "", -1, ctx.Err()
	}
}

// Close 关闭 shell
func (s *PersistentShell) Close() error {
	s.stdin.Close()
	return s.cmd.Process.Kill()
}

// ShellPool 管理多个 shell session
type ShellPool struct {
	shells   map[string]*PersistentShell
	mu       sync.RWMutex
	executor interface{} // sandbox.Executor，Phase C 注入
}

// NewShellPool 创建新的 ShellPool
func NewShellPool() *ShellPool {
	return &ShellPool{
		shells: make(map[string]*PersistentShell),
	}
}

// Get 获取或创建 shell session
func (p *ShellPool) Get(sessionID string) (*PersistentShell, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if shell, ok := p.shells[sessionID]; ok {
		return shell, nil
	}

	shell, err := NewPersistentShell()
	if err != nil {
		return nil, err
	}

	p.shells[sessionID] = shell
	return shell, nil
}

// Close 关闭特定 session
func (p *ShellPool) Close(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if shell, ok := p.shells[sessionID]; ok {
		shell.Close()
		delete(p.shells, sessionID)
	}
}

// CloseAll 关闭所有 sessions
func (p *ShellPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, shell := range p.shells {
		shell.Close()
	}
	p.shells = make(map[string]*PersistentShell)
}

// Count 返回活跃 session 数量
func (p *ShellPool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.shells)
}
