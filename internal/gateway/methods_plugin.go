package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/errs"
)

func registerPluginMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "plugin.list",
		Description: "列出所有已加载插件",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(deps.PluginLoader.ListPlugins())
		},
	})

	gw.Register(MethodDef{
		Name:        "plugin.load",
		Description: "加载指定插件",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if err := deps.PluginLoader.Reload(ctx, p.ID); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "ok", "plugin": p.ID})
		},
	})

	gw.Register(MethodDef{
		Name:        "plugin.unload",
		Description: "卸载指定插件",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if err := deps.PluginLoader.Unload(ctx, p.ID); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "ok", "plugin": p.ID})
		},
	})

	gw.Register(MethodDef{
		Name:        "plugin.reload",
		Description: "重载指定插件",
		AuthScope:   "admin",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if err := deps.PluginLoader.Reload(ctx, p.ID); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "ok", "plugin": p.ID})
		},
	})
}
