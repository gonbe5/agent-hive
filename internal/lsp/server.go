package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// ServerManager 管理 LSP 服务器进程池
type ServerManager struct {
	servers   map[string]*Server // language -> Server
	serversMu sync.RWMutex

	config LSPConfig
	logger *zap.Logger

	rootPath string // 工作区根路径
}

// Server 表示一个 LSP 服务器实例
type Server struct {
	language string
	command  string
	args     []string

	client *Client
	cmd    *exec.Cmd

	initialized bool
	initMu      sync.Mutex

	// Agent E: 健康检查增强 - 心跳缓存
	healthCheckCache struct {
		sync.RWMutex
		lastCheck time.Time
		lastOk    bool
	}
	healthCheckInterval time.Duration

	// Agent B: 并发限制 - semaphore
	concurrencySem chan struct{} // 并发信号量

	logger *zap.Logger
}

// LSPConfig LSP 配置（简化版，避免循环依赖）
type LSPConfig struct {
	Enabled                        bool
	MaxServers                     int
	Timeout                        time.Duration
	Languages                      map[string]LanguageSpec
	HealthInterval                 time.Duration
	MaxConcurrentRequestsPerServer int // Agent B: 每个服务器的最大并发请求数（默认 10）
}

// LanguageSpec 语言配置
type LanguageSpec struct {
	Command    string
	Args       []string
	Extensions []string
	Disabled   bool
}

// NewServerManager 创建服务器管理器
func NewServerManager(config LSPConfig, rootPath string, logger *zap.Logger) *ServerManager {
	if rootPath == "" {
		rootPath, _ = os.Getwd()
	}

	return &ServerManager{
		servers:  make(map[string]*Server),
		config:   config,
		logger:   logger,
		rootPath: rootPath,
	}
}

// GetServer 获取或启动语言服务器（惰性启动）
func (m *ServerManager) GetServer(ctx context.Context, language string) (*Server, error) {
	m.serversMu.RLock()
	server, ok := m.servers[language]
	m.serversMu.RUnlock()

	if ok && server != nil {
		// 服务器已存在，检查健康状态
		if !server.IsHealthy() {
			m.logger.Warn("LSP 服务器不健康，尝试重启",
				zap.String("language", language))
			m.StopServer(language)
			// 继续创建新服务器
		} else {
			return server, nil
		}
	}

	// 检查服务器数量限制
	m.serversMu.RLock()
	serverCount := len(m.servers)
	m.serversMu.RUnlock()

	if m.config.MaxServers > 0 && serverCount >= m.config.MaxServers {
		return nil, errs.New(errs.CodeResourceExhausted,
			fmt.Sprintf("LSP 服务器数量已达上限 (%d)", m.config.MaxServers))
	}

	// 获取语言配置
	langSpec, ok := m.config.Languages[language]
	if !ok {
		return nil, errs.New(errs.CodeNotFound,
			fmt.Sprintf("不支持的语言: %s", language))
	}

	if langSpec.Disabled {
		return nil, errs.New(errs.CodeUnavailable,
			fmt.Sprintf("语言服务器已禁用: %s", language))
	}

	// 启动服务器
	server, err := m.startServer(ctx, language, langSpec)
	if err != nil {
		return nil, err
	}

	m.serversMu.Lock()
	m.servers[language] = server
	m.serversMu.Unlock()

	m.logger.Info("LSP 服务器已启动",
		zap.String("language", language),
		zap.String("command", langSpec.Command))

	return server, nil
}

// startServer 启动 LSP 服务器
func (m *ServerManager) startServer(ctx context.Context, language string, spec LanguageSpec) (*Server, error) {
	// 创建命令
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = m.rootPath

	// 获取 stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "创建 stdin pipe 失败", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, errs.Wrap(errs.CodeInternal, "创建 stdout pipe 失败", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, errs.Wrap(errs.CodeInternal, "创建 stderr pipe 失败", err)
	}

	// 启动进程
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, errs.Wrap(errs.CodeInternal,
			fmt.Sprintf("启动 %s 失败", spec.Command), err)
	}

	// 创建客户端
	client := NewClient(stdin, stdout, stderr, m.logger)

	// Agent B: 注册标准通知处理器
	registerStandardNotificationHandlers(client, m.logger)

	// Agent B: 设置并发限制（默认 10）
	maxConcurrent := m.config.MaxConcurrentRequestsPerServer
	if maxConcurrent <= 0 {
		maxConcurrent = 10 // 默认值
	}

	server := &Server{
		language:            language,
		command:             spec.Command,
		args:                spec.Args,
		client:              client,
		cmd:                 cmd,
		healthCheckInterval: m.config.HealthInterval,
		concurrencySem:      make(chan struct{}, maxConcurrent), // Agent B: 并发信号量
		logger:              m.logger,
	}

	// 初始化服务器
	if err := server.Initialize(ctx, m.rootPath); err != nil {
		client.Close()
		cmd.Process.Kill()
		return nil, err
	}

	return server, nil
}

// StopServer 停止语言服务器
func (m *ServerManager) StopServer(language string) {
	m.serversMu.Lock()
	server, ok := m.servers[language]
	if ok {
		delete(m.servers, language)
	}
	m.serversMu.Unlock()

	if server != nil {
		server.Shutdown()
		m.logger.Info("LSP 服务器已停止", zap.String("language", language))
	}
}

// StopAll 停止所有服务器
func (m *ServerManager) StopAll() {
	m.serversMu.Lock()
	servers := make([]*Server, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	m.servers = make(map[string]*Server)
	m.serversMu.Unlock()

	for _, server := range servers {
		server.Shutdown()
	}

	m.logger.Info("所有 LSP 服务器已停止")
}

// HasLanguageForFile 检查是否有对应文件类型的语言服务器配置（不启动服务器）
func (m *ServerManager) HasLanguageForFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return false
	}
	for _, spec := range m.config.Languages {
		if spec.Disabled {
			continue
		}
		for _, supportedExt := range spec.Extensions {
			if ext == supportedExt {
				return true
			}
		}
	}
	return false
}

// LanguageIDForFile 根据文件路径返回语言 ID（如 "go"、"python"）
// 如果文件类型不支持，返回空字符串
func (m *ServerManager) LanguageIDForFile(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return ""
	}
	for lang, spec := range m.config.Languages {
		if spec.Disabled {
			continue
		}
		for _, supportedExt := range spec.Extensions {
			if ext == supportedExt {
				return lang
			}
		}
	}
	return ""
}

// GetServerForFile 根据文件路径获取对应的语言服务器
func (m *ServerManager) GetServerForFile(ctx context.Context, filePath string) (*Server, error) {
	// 根据文件扩展名判断语言
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return nil, errs.New(errs.CodeInvalidArgument, "无法从文件路径判断语言类型")
	}

	// 查找匹配的语言
	for lang, spec := range m.config.Languages {
		for _, supportedExt := range spec.Extensions {
			if ext == supportedExt {
				return m.GetServer(ctx, lang)
			}
		}
	}

	return nil, errs.New(errs.CodeNotFound,
		fmt.Sprintf("不支持的文件类型: %s", ext))
}

// Initialize 初始化 LSP 服务器
func (s *Server) Initialize(ctx context.Context, rootPath string) error {
	s.initMu.Lock()
	defer s.initMu.Unlock()

	if s.initialized {
		return nil
	}

	// 转换为 file:// URI
	rootURI := pathToURI(rootPath)

	pid := os.Getpid()
	params := InitializeParams{
		ProcessID: &pid,
		RootURI:   rootURI,
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				Completion:     &CompletionCapability{},
				Hover:          &HoverCapability{},
				Definition:     &DefinitionCapability{},
				References:     &ReferencesCapability{},
				Rename:         &RenameCapability{},
				Formatting:     &FormattingCapability{},
				CodeAction:     &CodeActionCapability{},
				DocumentSymbol: &DocumentSymbolCapability{},
			},
			Workspace: &WorkspaceClientCapabilities{
				Symbol: &WorkspaceSymbolCapability{},
			},
		},
	}

	var result InitializeResult
	if err := s.client.Call(ctx, "initialize", params, &result); err != nil {
		return errs.Wrap(errs.CodeInternal, "LSP initialize 失败", err)
	}

	// 发送 initialized 通知
	if err := s.client.Notify("initialized", map[string]interface{}{}); err != nil {
		return errs.Wrap(errs.CodeInternal, "LSP initialized 通知失败", err)
	}

	s.initialized = true
	s.logger.Info("LSP 服务器初始化成功",
		zap.String("language", s.language))

	return nil
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultLSPTimeout)
	defer cancel()

	// 发送 shutdown 请求
	_ = s.client.Call(ctx, "shutdown", nil, nil)

	// 发送 exit 通知
	_ = s.client.Notify("exit", nil)

	// 关闭客户端
	s.client.Close()

	// 等待进程退出
	if s.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- s.cmd.Wait()
		}()

		select {
		case <-done:
			// 正常退出
		case <-time.After(3 * time.Second):
			// 强制杀死
			s.cmd.Process.Kill()
		}
	}

	s.logger.Debug("LSP 服务器已关闭", zap.String("language", s.language))
}

// IsHealthy 检查服务器是否健康（Agent E: 增强版 - 心跳缓存 + ping 验证）
func (s *Server) IsHealthy() bool {
	// 1. 检查缓存（避免频繁检查）
	s.healthCheckCache.RLock()
	if time.Since(s.healthCheckCache.lastCheck) < s.healthCheckInterval {
		ok := s.healthCheckCache.lastOk
		s.healthCheckCache.RUnlock()
		return ok
	}
	s.healthCheckCache.RUnlock()

	// 2. 执行实际检查
	ok := s.performHealthCheck()

	// 3. 更新缓存
	s.healthCheckCache.Lock()
	s.healthCheckCache.lastCheck = time.Now()
	s.healthCheckCache.lastOk = ok
	s.healthCheckCache.Unlock()

	return ok
}

// performHealthCheck 执行实际的健康检查（进程存活 + 心跳验证）
func (s *Server) performHealthCheck() bool {
	// 检查进程是否存在
	if s.cmd.Process == nil {
		return false
	}

	// 向进程发送信号 0 检查存活
	if err := s.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	// 心跳检测：发送 ping 请求验证响应（避免僵尸进程）
	// 注意：标准 LSP 协议没有 ping，这里使用 $/cancelRequest 作为轻量级探测
	// 尝试取消一个不存在的请求 ID，仅用于验证服务器连接正常
	err := s.client.Notify("$/cancelRequest", map[string]interface{}{
		"id": -1, // 取消一个不存在的请求
	})

	// 如果 Notify 失败，说明连接有问题
	return err == nil
}

// Call 调用 LSP 方法（Agent B: 增加并发控制）
func (s *Server) Call(ctx context.Context, method string, params, result interface{}) error {
	if !s.initialized {
		return errs.New(errs.CodeFailedPrecondition, "LSP 服务器未初始化")
	}

	// Agent B: 并发限制 - 获取信号量
	select {
	case s.concurrencySem <- struct{}{}:
		// 获取到信号量，继续执行
		defer func() {
			<-s.concurrencySem // 释放信号量
		}()
	case <-ctx.Done():
		return errs.New(errs.CodeTimeout, "等待并发槽位超时")
	}

	return s.client.Call(ctx, method, params, result)
}

// Notify 发送 LSP 通知（无需等待响应）
func (s *Server) Notify(method string, params interface{}) error {
	if !s.initialized {
		return errs.New(errs.CodeFailedPrecondition, "LSP 服务器未初始化")
	}
	return s.client.Notify(method, params)
}

// GetFileDiagnostics 获取文件的诊断信息
// 通过 textDocument/didOpen 触发诊断，等待 publishDiagnostics 通知
// timeout 控制最大等待时间
func (s *Server) GetFileDiagnostics(ctx context.Context, filePath string, languageID string) ([]Diagnostic, error) {
	if !s.initialized {
		return nil, errs.New(errs.CodeFailedPrecondition, "LSP 服务器未初始化")
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "读取文件内容失败", err)
	}

	uri := pathToURI(filePath)

	// 创建诊断接收通道
	diagChan := make(chan []Diagnostic, 1)

	// 临时注册诊断通知处理器
	handlerKey := "textDocument/publishDiagnostics"
	originalHandler := s.client.getNotificationHandler(handlerKey)

	s.client.RegisterNotificationHandler(handlerKey, func(method string, params json.RawMessage) {
		var diagParams PublishDiagnosticsParams
		if err := json.Unmarshal(params, &diagParams); err != nil {
			return
		}
		// 只接收目标文件的诊断
		if diagParams.URI == uri {
			select {
			case diagChan <- diagParams.Diagnostics:
			default:
			}
		}
		// 同时调用原始处理器
		if originalHandler != nil {
			originalHandler(method, params)
		}
	})

	// 结束时恢复原始处理器
	defer func() {
		if originalHandler != nil {
			s.client.RegisterNotificationHandler(handlerKey, originalHandler)
		} else {
			s.client.UnregisterNotificationHandler(handlerKey)
		}
	}()

	// 发送 textDocument/didOpen 通知触发诊断
	openParams := map[string]interface{}{
		"textDocument": TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       string(content),
		},
	}
	if err := s.client.Notify("textDocument/didOpen", openParams); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "发送 didOpen 通知失败", err)
	}

	// 等待诊断结果
	var diagnostics []Diagnostic
	select {
	case diags := <-diagChan:
		diagnostics = diags
	case <-ctx.Done():
		// 超时，返回空诊断（不视为错误）
		diagnostics = nil
	}

	// 发送 textDocument/didClose 通知
	closeParams := map[string]interface{}{
		"textDocument": TextDocumentIdentifier{URI: uri},
	}
	_ = s.client.Notify("textDocument/didClose", closeParams)

	return diagnostics, nil
}

// pathToURI 将文件路径转换为 file:// URI
func pathToURI(path string) string {
	// 确保是绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// 处理 Windows 路径
	absPath = filepath.ToSlash(absPath)

	// 构造 URI
	if !strings.HasPrefix(absPath, "/") {
		absPath = "/" + absPath
	}

	return "file://" + absPath
}

// registerStandardNotificationHandlers 注册标准 LSP 通知处理器
// Agent B: LSP 通知处理机制
func registerStandardNotificationHandlers(client *Client, logger *zap.Logger) {
	// 1. textDocument/publishDiagnostics - 诊断信息
	client.RegisterNotificationHandler("textDocument/publishDiagnostics", func(method string, params json.RawMessage) {
		var diagParams PublishDiagnosticsParams
		if err := json.Unmarshal(params, &diagParams); err != nil {
			logger.Warn("解析 publishDiagnostics 参数失败", zap.Error(err))
			return
		}

		if len(diagParams.Diagnostics) == 0 {
			return
		}

		// 记录诊断信息
		for _, diag := range diagParams.Diagnostics {
			severity := "未知"
			switch diag.Severity {
			case 1:
				severity = "错误"
			case 2:
				severity = "警告"
			case 3:
				severity = "信息"
			case 4:
				severity = "提示"
			}

			logger.Debug("LSP 诊断",
				zap.String("uri", diagParams.URI),
				zap.String("severity", severity),
				zap.String("message", diag.Message),
				zap.Int("line", diag.Range.Start.Line+1))
		}
	})

	// 2. window/logMessage - 日志消息
	client.RegisterNotificationHandler("window/logMessage", func(method string, params json.RawMessage) {
		var logParams LogMessageParams
		if err := json.Unmarshal(params, &logParams); err != nil {
			logger.Warn("解析 logMessage 参数失败", zap.Error(err))
			return
		}

		// 根据消息类型选择日志级别
		switch logParams.Type {
		case MessageTypeError:
			logger.Error("LSP 服务器日志", zap.String("message", logParams.Message))
		case MessageTypeWarning:
			logger.Warn("LSP 服务器日志", zap.String("message", logParams.Message))
		case MessageTypeInfo:
			logger.Info("LSP 服务器日志", zap.String("message", logParams.Message))
		case MessageTypeLog:
			logger.Debug("LSP 服务器日志", zap.String("message", logParams.Message))
		}
	})

	// 3. window/showMessage - 显示消息
	client.RegisterNotificationHandler("window/showMessage", func(method string, params json.RawMessage) {
		var showParams ShowMessageParams
		if err := json.Unmarshal(params, &showParams); err != nil {
			logger.Warn("解析 showMessage 参数失败", zap.Error(err))
			return
		}

		// 根据消息类型选择日志级别
		switch showParams.Type {
		case MessageTypeError:
			logger.Error("LSP 服务器消息", zap.String("message", showParams.Message))
		case MessageTypeWarning:
			logger.Warn("LSP 服务器消息", zap.String("message", showParams.Message))
		case MessageTypeInfo:
			logger.Info("LSP 服务器消息", zap.String("message", showParams.Message))
		case MessageTypeLog:
			logger.Debug("LSP 服务器消息", zap.String("message", showParams.Message))
		}
	})
}
