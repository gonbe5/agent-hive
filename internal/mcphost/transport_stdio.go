package mcphost

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// StdioTransportConfig stdio 传输配置
type StdioTransportConfig struct {
	Command string            // 可执行命令
	Args    []string          // 命令参数
	Env     map[string]string // 附加环境变量（叠加在父进程环境之上）
}

// StdioTransport 通过子进程 stdin/stdout 与 MCP 服务端通信
// MCP stdio 协议：每条消息为一行换行符分隔的 JSON
type StdioTransport struct {
	cfg    StdioTransportConfig
	logger *zap.Logger

	mu     sync.Mutex
	cmd    *exec.Cmd
	writer *json.Encoder
	reader *bufio.Scanner
	closed bool
}

// NewStdioTransport 创建 stdio 传输实例
func NewStdioTransport(cfg StdioTransportConfig, logger *zap.Logger) *StdioTransport {
	return &StdioTransport{cfg: cfg, logger: logger}
}

// Connect 启动子进程、建立 stdio 通信管道，并发送 MCP initialize 请求
// client.Connect() 会直接调用 Receive() 读取 initialize 响应，所以此处必须发送请求
func (t *StdioTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return errs.New(errs.CodeMCPTransportClosed, "stdio 传输已关闭")
	}
	if t.cmd != nil {
		return nil // 已连接
	}

	// 使用独立的后台 context 启动子进程，避免与握手超时 context 耦合
	// 子进程生命周期由 Close() 管理
	cmd := exec.Command(t.cfg.Command, t.cfg.Args...) //nolint:gosec

	// 构建环境变量：继承父进程环境，再叠加自定义变量
	env := os.Environ()
	for k, v := range t.cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	cmd.Stderr = os.Stderr // 将子进程的 stderr 输出到父进程 stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "获取 stdin pipe 失败", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "获取 stdout pipe 失败", err)
	}

	if err := cmd.Start(); err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed,
			fmt.Sprintf("启动 stdio MCP 子进程失败: %s", t.cfg.Command), err)
	}

	t.cmd = cmd
	t.writer = json.NewEncoder(stdin)
	t.reader = bufio.NewScanner(stdout)
	t.reader.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB 缓冲，防止大响应截断

	// 记录传入的环境变量 key（不记录 value，避免泄露密钥）
	envKeys := make([]string, 0, len(t.cfg.Env))
	for k := range t.cfg.Env {
		envKeys = append(envKeys, k)
	}

	t.logger.Info("stdio MCP 子进程已启动",
		zap.String("command", t.cfg.Command),
		zap.Strings("args", t.cfg.Args),
		zap.Int("pid", cmd.Process.Pid),
		zap.Strings("custom_env_keys", envKeys),
	)

	// 发送 MCP initialize 请求（协议要求客户端先发起）
	// client.Connect() 随后会调用 Receive() 读取响应
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "agents-hive",
				"version": "1.0",
			},
		},
	}
	if err := t.writer.Encode(initReq); err != nil {
		_ = cmd.Process.Kill()
		return errs.Wrap(errs.CodeMCPTransportFailed, "发送 MCP initialize 请求失败", err)
	}

	return nil
}

// Send 将 JSON-RPC 消息写入子进程 stdin（单行 JSON）
func (t *StdioTransport) Send(_ context.Context, msg json.RawMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed || t.writer == nil {
		return errs.New(errs.CodeMCPTransportClosed, "stdio 传输未连接或已关闭")
	}
	if err := t.writer.Encode(msg); err != nil {
		return errs.Wrap(errs.CodeMCPTransportFailed, "向 stdio MCP 子进程写入消息失败", err)
	}
	return nil
}

// Receive 从子进程 stdout 读取一行 JSON-RPC 响应（阻塞）
func (t *StdioTransport) Receive(_ context.Context) (json.RawMessage, error) {
	// Receive 不加锁：bufio.Scanner 是单线程读取，client 串行调用 Send+Receive
	if t.reader == nil {
		return nil, errs.New(errs.CodeMCPTransportClosed, "stdio 传输未连接")
	}
	if !t.reader.Scan() {
		if err := t.reader.Err(); err != nil {
			return nil, errs.Wrap(errs.CodeMCPTransportFailed, "从 stdio MCP 子进程读取响应失败", err)
		}
		return nil, errs.New(errs.CodeMCPTransportClosed, "stdio MCP 子进程已关闭输出流")
	}
	// 必须拷贝：bufio.Scanner.Bytes() 返回内部缓冲区引用，下次 Scan() 会覆盖
	raw := t.reader.Bytes()
	result := make([]byte, len(raw))
	copy(result, raw)
	return json.RawMessage(result), nil
}

// Close 终止子进程并释放资源
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
		t.logger.Info("stdio MCP 子进程已终止",
			zap.String("command", t.cfg.Command),
			zap.Int("pid", t.cmd.Process.Pid),
		)
	}
	return nil
}
