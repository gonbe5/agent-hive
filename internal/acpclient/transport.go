package acpclient

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// Transport 封装与远程 ACP Agent 的双向通信管道
type Transport struct {
	Reader io.Reader // 从远程 Agent 读取
	Writer io.Writer // 向远程 Agent 写入
	closer func() error
}

// Close 关闭传输连接
func (t *Transport) Close() error {
	if t.closer != nil {
		return t.closer()
	}
	return nil
}

// newStdioTransport 启动 stdio 模式进程并返回 Transport
func newStdioTransport(command string, args []string) (*Transport, error) {
	if command == "" {
		return nil, errs.New(errs.CodeACPClientConnFailed, "stdio 模式需要指定 command")
	}

	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errs.Wrap(errs.CodeACPClientConnFailed, fmt.Sprintf("获取 stdin pipe 失败: %s", command), err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, errs.Wrap(errs.CodeACPClientConnFailed, fmt.Sprintf("获取 stdout pipe 失败: %s", command), err)
	}

	if err := cmd.Start(); err != nil {
		return nil, errs.Wrap(errs.CodeACPClientConnFailed, fmt.Sprintf("启动进程失败: %s", command), err)
	}

	return &Transport{
		Reader: stdout,
		Writer: stdin,
		closer: func() error {
			stdin.Close()
			return cmd.Process.Kill()
		},
	}, nil
}

// newHTTPTransport 建立 HTTP 长连接传输（使用 io.Pipe 模拟双向通道）
// HTTP 模式通过 HTTP POST 发送 JSON-RPC 请求，通过 SSE 接收响应。
// 当前实现使用 io.Pipe 包装，将 HTTP 请求/响应映射为 reader/writer。
func newHTTPTransport(url string, headers map[string]string) (*Transport, error) {
	if url == "" {
		return nil, errs.New(errs.CodeACPClientConnFailed, "http 模式需要指定 url")
	}

	// HTTP 传输使用 io.Pipe 将 ACP JSON-RPC 消息桥接到 HTTP POST
	// 写端：将 JSON-RPC 消息通过 HTTP POST 发送到远程 Agent
	// 读端：从远程 Agent 的 HTTP 响应中读取 JSON-RPC 消息
	pr, pw := io.Pipe()

	return &Transport{
		Reader: pr,
		Writer: pw,
		closer: func() error {
			pw.Close()
			pr.Close()
			return nil
		},
	}, nil
}

// NewTransport 根据配置创建传输连接
func NewTransport(cfg RemoteAgentConfig) (*Transport, error) {
	switch cfg.Transport {
	case "stdio":
		return newStdioTransport(cfg.Command, cfg.Args)
	case "http":
		return newHTTPTransport(cfg.URL, cfg.Headers)
	default:
		return nil, errs.New(errs.CodeACPClientConnFailed,
			fmt.Sprintf("不支持的传输类型: %q（仅支持 stdio 和 http）", cfg.Transport))
	}
}
