package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

// registerHITLMethods 注册人机交互（HITL）相关 RPC 方法
func registerHITLMethods(gw *Gateway, deps Deps) {
	// hitl.submit — 提交用户输入响应
	gw.Register(MethodDef{
		Name:        "hitl.submit",
		Description: "提交人机交互输入响应",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				RequestID string `json:"request_id"`
				TaskID    string `json:"task_id"`
				Action    string `json:"action"`
				Value     string `json:"value"`
				Remember  bool   `json:"remember"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.RequestID == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "request_id 不能为空")
			}
			// task_id 可为空：权限请求的 TaskID 为空，master.SubmitInput 已处理匹配逻辑
			err := deps.Master.SubmitInput(master.InputResponse{
				RequestID: p.RequestID,
				TaskID:    p.TaskID,
				Action:    p.Action,
				Value:     p.Value,
				Remember:  p.Remember,
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "submitted"})
		},
	})

	// hitl.command — 发送用户控制命令（暂停/恢复/取消）
	gw.Register(MethodDef{
		Name:        "hitl.command",
		Description: "发送控制命令（暂停/恢复/取消）",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				TaskID string `json:"task_id"`
				Type   string `json:"type"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.TaskID == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "task_id 不能为空")
			}
			if p.Type == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "type 不能为空")
			}
			err := deps.Master.SendCommand(master.UserCommand{
				TaskID: p.TaskID,
				Type:   master.UserCommandType(p.Type),
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "sent"})
		},
	})

	// hitl.pending — 获取待处理的输入请求
	gw.Register(MethodDef{
		Name:        "hitl.pending",
		Description: "获取待处理的人机交互输入请求",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				TaskID string `json:"task_id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			// task_id 可为空：为空时返回所有待处理请求
			pending := deps.Master.PendingInputs(p.TaskID)
			return json.Marshal(pending)
		},
	})
}
