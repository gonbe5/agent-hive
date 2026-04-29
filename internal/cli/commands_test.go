package cli

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantRequest     string
		wantInteractive bool
		wantModel       string
		wantConfigPath  string
		wantBaseURL     string
	}{
		{
			name:            "nil args returns interactive mode",
			args:            nil,
			wantRequest:     "",
			wantInteractive: true,
		},
		{
			name:            "empty slice returns interactive mode",
			args:            []string{},
			wantRequest:     "",
			wantInteractive: true,
		},
		{
			name:            "single arg returns request",
			args:            []string{"hello"},
			wantRequest:     "hello",
			wantInteractive: false,
		},
		{
			name:            "multiple args are joined with space",
			args:            []string{"hello", "world"},
			wantRequest:     "hello world",
			wantInteractive: false,
		},
		{
			name:            "short interactive flag",
			args:            []string{"-i"},
			wantRequest:     "",
			wantInteractive: true,
		},
		{
			name:            "long interactive flag",
			args:            []string{"--interactive"},
			wantRequest:     "",
			wantInteractive: true,
		},
		{
			name:            "model flag short",
			args:            []string{"-m", "deepseek-chat", "hello"},
			wantRequest:     "hello",
			wantInteractive: false,
			wantModel:       "deepseek-chat",
		},
		{
			name:            "model flag long",
			args:            []string{"--model", "gpt-5.2", "analyze code"},
			wantRequest:     "analyze code",
			wantInteractive: false,
			wantModel:       "gpt-5.2",
		},
		{
			name:            "model flag equals syntax",
			args:            []string{"--model=claude-3-5-sonnet", "review this"},
			wantRequest:     "review this",
			wantInteractive: false,
			wantModel:       "claude-3-5-sonnet",
		},
		{
			name:            "config flag short",
			args:            []string{"-c", "myconfig.json", "-i"},
			wantRequest:     "",
			wantInteractive: true,
			wantConfigPath:  "myconfig.json",
		},
		{
			name:            "config flag long",
			args:            []string{"--config", "/etc/claw.json", "hello"},
			wantRequest:     "hello",
			wantInteractive: false,
			wantConfigPath:  "/etc/claw.json",
		},
		{
			name:            "base-url flag",
			args:            []string{"--base-url", "https://api.deepseek.com/v1", "hello"},
			wantRequest:     "hello",
			wantInteractive: false,
			wantBaseURL:     "https://api.deepseek.com/v1",
		},
		{
			name:            "base-url equals syntax",
			args:            []string{"--base-url=https://custom.api.com/v1", "hello"},
			wantRequest:     "hello",
			wantInteractive: false,
			wantBaseURL:     "https://custom.api.com/v1",
		},
		{
			name:            "all flags combined",
			args:            []string{"-m", "gpt-5.2", "-c", "config.json", "--base-url", "https://api.example.com", "do something"},
			wantRequest:     "do something",
			wantInteractive: false,
			wantModel:       "gpt-5.2",
			wantConfigPath:  "config.json",
			wantBaseURL:     "https://api.example.com",
		},
		{
			name:            "model with interactive mode",
			args:            []string{"-m", "deepseek-chat", "-i"},
			wantRequest:     "",
			wantInteractive: true,
			wantModel:       "deepseek-chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseArgs(tt.args)
			if got.Request != tt.wantRequest {
				t.Errorf("ParseArgs(%v) request = %q, want %q", tt.args, got.Request, tt.wantRequest)
			}
			if got.Interactive != tt.wantInteractive {
				t.Errorf("ParseArgs(%v) interactive = %v, want %v", tt.args, got.Interactive, tt.wantInteractive)
			}
			if got.Model != tt.wantModel {
				t.Errorf("ParseArgs(%v) model = %q, want %q", tt.args, got.Model, tt.wantModel)
			}
			if got.ConfigPath != tt.wantConfigPath {
				t.Errorf("ParseArgs(%v) configPath = %q, want %q", tt.args, got.ConfigPath, tt.wantConfigPath)
			}
			if got.BaseURL != tt.wantBaseURL {
				t.Errorf("ParseArgs(%v) baseURL = %q, want %q", tt.args, got.BaseURL, tt.wantBaseURL)
			}
		})
	}
}

func TestParseArgs_HITL(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantHITL bool
		wantReq  string
	}{
		{
			name:     "hitl flag with request",
			args:     []string{"--hitl", "review code"},
			wantHITL: true,
			wantReq:  "review code",
		},
		{
			name:     "hitl flag only triggers interactive",
			args:     []string{"--hitl"},
			wantHITL: true,
			wantReq:  "",
		},
		{
			name:     "no hitl flag",
			args:     []string{"hello"},
			wantHITL: false,
			wantReq:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseArgs(tt.args)
			if got.HITL != tt.wantHITL {
				t.Errorf("ParseArgs(%v) HITL = %v, want %v", tt.args, got.HITL, tt.wantHITL)
			}
			if got.Request != tt.wantReq {
				t.Errorf("ParseArgs(%v) Request = %q, want %q", tt.args, got.Request, tt.wantReq)
			}
		})
	}
}

func TestCommand_FieldsSettable(t *testing.T) {
	called := false
	cmd := Command{
		Name:        "test-cmd",
		Description: "A test command",
		Run: func(args []string) error {
			called = true
			return nil
		},
	}

	if cmd.Run == nil {
		t.Fatal("Run should not be nil")
	}
	if err := cmd.Run([]string{"arg1"}); err != nil {
		t.Errorf("Run() returned unexpected error: %v", err)
	}
	if !called {
		t.Error("Run() was not invoked")
	}
}

func TestCommand_RunReturnsError(t *testing.T) {
	expectedErr := errors.New("command failed")
	cmd := Command{
		Run: func(args []string) error {
			return expectedErr
		},
	}

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("Run() should have returned an error")
	}
	if err != expectedErr {
		t.Errorf("Run() error = %v, want %v", err, expectedErr)
	}
}

func TestNewApp(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()

	app := NewApp(cfg, logger)
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.master == nil {
		t.Error("expected non-nil master")
	}
	if app.logger == nil {
		t.Error("expected non-nil logger")
	}
	if app.config != cfg {
		t.Error("expected config to match")
	}
}

func TestApp_RunOnce(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := app.RunOnce(ctx, "test request")
	// RunOnce may succeed or fail depending on agent timing, but it should not panic
	_ = err
}

func TestApp_RunOnce_EmptyRequest(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := app.RunOnce(ctx, "")
	if err == nil {
		t.Error("expected error for empty request")
	}
}

func TestApp_RunInteractive_Quit(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	// Create a pipe to simulate stdin with "quit" command
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Replace os.Stdin
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// Write "quit" to the pipe
	go func() {
		w.WriteString("quit\n")
		w.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.RunInteractive(ctx)
	if err != nil {
		t.Errorf("RunInteractive returned error: %v", err)
	}
}

func TestApp_RunInteractive_Exit(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		w.WriteString("exit\n")
		w.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.RunInteractive(ctx)
	if err != nil {
		t.Errorf("RunInteractive returned error: %v", err)
	}
}

func TestApp_RunInteractive_EmptyLineAndQuit(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// Send empty line, then quit
	go func() {
		w.WriteString("\n")
		w.WriteString("quit\n")
		w.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.RunInteractive(ctx)
	if err != nil {
		t.Errorf("RunInteractive returned error: %v", err)
	}
}

func TestApp_RunInteractive_EOF(t *testing.T) {
	if os.Getenv("POSTGRES_TEST_DSN") == "" {
		t.Skip("跳过：需要 PostgreSQL 连接（设置 POSTGRES_TEST_DSN 环境变量）")
	}
	logger, _ := zap.NewDevelopment()
	cfg := config.Default()
	cfg.SessionsDir = t.TempDir()
	app := NewApp(cfg, logger)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// Close immediately to simulate EOF
	w.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = app.RunInteractive(ctx)
	if err != nil {
		t.Errorf("RunInteractive returned error: %v", err)
	}
}

func TestPrintUsage(t *testing.T) {
	// printUsage writes to stdout - just verify it doesn't panic
	printUsage()
}
