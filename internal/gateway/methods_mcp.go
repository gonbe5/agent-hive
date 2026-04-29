package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// registerMCPMethods 注册 MCP 资源和提示相关的 RPC 方法
func registerMCPMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "mcp.resources.list",
		Description: "列出所有已注册的 MCP 资源",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(deps.MCPHost.ListResources())
		},
	})

	gw.Register(MethodDef{
		Name:        "mcp.resources.read",
		Description: "读取指定 MCP 资源",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				URI string `json:"uri"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.URI == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "缺少必需参数 uri")
			}
			content, err := deps.MCPHost.ReadResource(ctx, p.URI)
			if err != nil {
				return nil, err
			}
			return json.Marshal(content)
		},
	})

	gw.Register(MethodDef{
		Name:        "mcp.prompts.list",
		Description: "列出所有已注册的 MCP 提示",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(deps.MCPHost.ListPrompts())
		},
	})

	gw.Register(MethodDef{
		Name:        "mcp.prompts.get",
		Description: "获取并执行指定 MCP 提示",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Name string            `json:"name"`
				Args map[string]string `json:"args"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.Name == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "缺少必需参数 name")
			}
			messages, err := deps.MCPHost.GetPrompt(ctx, p.Name, p.Args)
			if err != nil {
				return nil, err
			}
			return json.Marshal(messages)
		},
	})
}
