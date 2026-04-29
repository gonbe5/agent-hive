package gateway

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chef-guo/agents-hive/internal/command"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// registerCommandMethods 注册命令相关的 RPC 方法
func registerCommandMethods(gw *Gateway, cmdReg *command.Registry, deps Deps) {
	gw.Register(MethodDef{
		Name:        "commands.list",
		Description: "列出所有可用命令",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(cmdReg.List())
		},
	})

	// commands.execute — 执行斜杠命令
	gw.Register(MethodDef{
		Name:        "commands.execute",
		Description: "执行指定的斜杠命令",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				SessionID string `json:"session_id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.New(errs.CodeInvalidInput, "参数解析失败: "+err.Error())
			}
			if p.Name == "" {
				return nil, errs.New(errs.CodeInvalidInput, "name 不能为空")
			}

			// 查找命令
			cmd, err := cmdReg.Get(p.Name)
			if err != nil {
				return nil, err
			}

			// 渲染模板
			args := strings.Fields(p.Arguments)
			message := cmd.Render(args)
			if message == "" {
				message = p.Name
			}

			// 发送给 Master 处理
			req := master.SessionRequest{
				Input:     message,
				SessionID: p.SessionID,
			}
			if cmd.Model != "" {
				req.ModelOverride = cmd.Model
			}

			resp, err := deps.Master.ProcessCommand(ctx, req)
			if err != nil {
				return nil, err
			}

			return json.Marshal(resp)
		},
	})
}
