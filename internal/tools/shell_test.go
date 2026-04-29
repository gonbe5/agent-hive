package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPersistentShell_Execute(t *testing.T) {
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("failed to create shell: %v", err)
	}
	defer shell.Close()

	ctx := context.Background()

	// 测试简单命令
	stdout, stderr, exitCode, err := shell.Execute(ctx, "echo hello")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got: %s", stdout)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got: %s", stderr)
	}
}

func TestPersistentShell_WorkingDirectory(t *testing.T) {
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("failed to create shell: %v", err)
	}
	defer shell.Close()

	ctx := context.Background()

	// cd 到 /tmp
	stdout, _, exitCode, err := shell.Execute(ctx, "cd /tmp && pwd")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "/tmp") {
		t.Errorf("expected /tmp in output, got: %s", stdout)
	}

	// 再次执行 pwd，应该还在 /tmp
	stdout, _, exitCode, err = shell.Execute(ctx, "pwd")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "/tmp") {
		t.Errorf("expected to still be in /tmp, got: %s", stdout)
	}
}

func TestPersistentShell_EnvironmentVariables(t *testing.T) {
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("failed to create shell: %v", err)
	}
	defer shell.Close()

	ctx := context.Background()

	// 设置环境变量
	_, _, exitCode, err := shell.Execute(ctx, "export FOO=bar")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}

	// 读取环境变量
	stdout, _, exitCode, err := shell.Execute(ctx, "echo $FOO")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "bar") {
		t.Errorf("expected 'bar' in output, got: %s", stdout)
	}
}

func TestPersistentShell_ExitCode(t *testing.T) {
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("failed to create shell: %v", err)
	}
	defer shell.Close()

	ctx := context.Background()

	// 失败的命令
	_, _, exitCode, err := shell.Execute(ctx, "false")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code for 'false'")
	}
}

func TestShellPool_GetAndClose(t *testing.T) {
	pool := NewShellPool()

	shell1, err := pool.Get("session1")
	if err != nil {
		t.Fatalf("failed to get shell: %v", err)
	}

	// 再次获取相同 session 应该返回同一个 shell
	shell2, err := pool.Get("session1")
	if err != nil {
		t.Fatalf("failed to get shell: %v", err)
	}

	if shell1 != shell2 {
		t.Error("expected same shell for same session ID")
	}

	if pool.Count() != 1 {
		t.Errorf("expected 1 shell, got %d", pool.Count())
	}

	pool.Close("session1")

	if pool.Count() != 0 {
		t.Errorf("expected 0 shells after close, got %d", pool.Count())
	}
}

func TestShellPool_MultipleSessions(t *testing.T) {
	pool := NewShellPool()
	defer pool.CloseAll()

	ctx := context.Background()

	// Session 1: cd 到 /tmp
	shell1, _ := pool.Get("s1")
	shell1.Execute(ctx, "cd /tmp")

	// Session 2: cd 到 /var
	shell2, _ := pool.Get("s2")
	shell2.Execute(ctx, "cd /var")

	// 验证 session 1 还在 /tmp
	stdout, _, _, _ := shell1.Execute(ctx, "pwd")
	if !strings.Contains(stdout, "/tmp") {
		t.Errorf("session 1 should be in /tmp, got: %s", stdout)
	}

	// 验证 session 2 还在 /var
	stdout, _, _, _ = shell2.Execute(ctx, "pwd")
	if !strings.Contains(stdout, "/var") {
		t.Errorf("session 2 should be in /var, got: %s", stdout)
	}

	if pool.Count() != 2 {
		t.Errorf("expected 2 sessions, got %d", pool.Count())
	}
}

func TestShellPool_CloseAll(t *testing.T) {
	pool := NewShellPool()

	pool.Get("s1")
	pool.Get("s2")
	pool.Get("s3")

	if pool.Count() != 3 {
		t.Errorf("expected 3 sessions, got %d", pool.Count())
	}

	pool.CloseAll()

	if pool.Count() != 0 {
		t.Errorf("expected 0 sessions after CloseAll, got %d", pool.Count())
	}
}

func TestPersistentShell_Concurrency(t *testing.T) {
	shell, err := NewPersistentShell()
	if err != nil {
		t.Fatalf("failed to create shell: %v", err)
	}
	defer shell.Close()

	ctx := context.Background()

	// 并发执行（shell 内部有 mutex 保护）
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(n int) {
			_, _, _, err := shell.Execute(ctx, "sleep 0.01")
			if err != nil {
				t.Errorf("concurrent execute %d failed: %v", n, err)
			}
			done <- true
		}(i)
	}

	timeout := time.After(2 * time.Second)
	for i := 0; i < 5; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("concurrent execution timed out")
		}
	}
}
