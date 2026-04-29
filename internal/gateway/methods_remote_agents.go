package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/acpclient"
)

// registerRemoteAgentMethods 注册远程 ACP Agent 管理 API
func registerRemoteAgentMethods(gw *Gateway, pool *acpclient.ACPClientPool) {
	// remote_agents.list — 列出所有远程 Agent 及状态
	gw.Register(MethodDef{
		Name:        "remote_agents.list",
		Description: "列出所有远程 ACP Agent",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			configs := pool.List()
			return json.Marshal(configs)
		},
	})

	// remote_agents.connect — 动态连接远程 ACP Agent
	gw.Register(MethodDef{
		Name:        "remote_agents.connect",
		Description: "动态连接远程 ACP Agent",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var cfg acpclient.RemoteAgentConfig
			if err := json.Unmarshal(params, &cfg); err != nil {
				return nil, err
			}
			agent, err := pool.Connect(ctx, cfg)
			if err != nil {
				return nil, err
			}
			// 启动 agent 事件循环
			go agent.Run(ctx)
			return json.Marshal(map[string]string{
				"name":   cfg.Name,
				"status": "connected",
			})
		},
	})

	// remote_agents.disconnect — 断开连接
	gw.Register(MethodDef{
		Name:        "remote_agents.disconnect",
		Description: "断开远程 ACP Agent 连接",
		AuthScope:   "admin",
		Handler: func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
			var req struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, err
			}
			if err := pool.Disconnect(req.Name); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{
				"name":   req.Name,
				"status": "disconnected",
			})
		},
	})

	// remote_agents.health — 健康检查
	gw.Register(MethodDef{
		Name:        "remote_agents.health",
		Description: "检查所有远程 ACP Agent 健康状态",
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			results := pool.HealthCheckAll(ctx)
			return json.Marshal(results)
		},
	})
}
