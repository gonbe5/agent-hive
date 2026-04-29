// Package acpserver 实现 ACP (Agent Client Protocol) 协议服务器
package acpserver

import (
	"context"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// connectSessionMCPServers 为指定会话连接 MCP 服务端
// ACP 模式下每个会话可独立配置 MCP 服务端（通过 NewSessionRequest 传入）
// 成功连接的客户端列表返回给调用方管理生命周期
func connectSessionMCPServers(
	ctx context.Context,
	host *mcphost.Host,
	servers map[string]config.MCPServerConfig,
	logger *zap.Logger,
) []*mcphost.RemoteMCPClient {
	var clients []*mcphost.RemoteMCPClient

	for name, serverCfg := range servers {
		spec := mcphost.MCPServerSpec{
			Name:      name,
			Command:   serverCfg.Command,
			Args:      serverCfg.Args,
			Transport: serverCfg.Transport,
			URL:       serverCfg.URL,
			Headers:   serverCfg.Headers,
		}

		transport, err := mcphost.BuildTransport(spec, nil, logger)
		if err != nil {
			logger.Warn("创建会话 MCP 传输失败，跳过",
				zap.String("服务端", name),
				zap.Error(err))
			continue
		}

		client, err := mcphost.ConnectRemoteMCP(ctx, transport, host, name, logger)
		if err != nil {
			logger.Warn("连接会话 MCP 服务端失败，跳过",
				zap.String("服务端", name),
				zap.Error(err))
			continue
		}

		clients = append(clients, client)
		logger.Info("会话 MCP 服务端已连接", zap.String("服务端", name))
	}

	return clients
}

// closeSessionMCPClients 关闭会话级 MCP 客户端
// 应在会话结束时调用，以释放底层连接资源
func closeSessionMCPClients(clients []*mcphost.RemoteMCPClient) {
	for _, c := range clients {
		if c != nil {
			_ = c.Close()
		}
	}
}
