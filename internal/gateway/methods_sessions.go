package gateway

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/master"
)

func registerSessionMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "sessions.list",
		Description: "列出所有会话",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			sessions, err := deps.Master.ListAllSessions(ctx)
			if err != nil {
				return nil, err
			}
			return json.Marshal(sessions)
		},
	})

	gw.Register(MethodDef{
		Name:        "sessions.get",
		Description: "获取会话详情",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			session, err := deps.Master.GetSessionByID(ctx, p.ID)
			if err != nil {
				return nil, err
			}
			return json.Marshal(session)
		},
	})

	gw.Register(MethodDef{
		Name:        "sessions.message",
		Description: "向会话发送消息",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				SessionID string `json:"session_id"`
				Input     string `json:"input"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			resp, err := deps.Master.ProcessMessage(ctx, p.SessionID, p.Input)
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.create — 创建新会话
	gw.Register(MethodDef{
		Name:        "sessions.create",
		Description: "创建新会话",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandNew,
				Args:    []string{p.Name},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.update — 更新会话（重命名）
	gw.Register(MethodDef{
		Name:        "sessions.update",
		Description: "更新会话（重命名/标签）",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.Name == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "名称不能为空")
			}
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandRename,
				Args:    []string{p.Name},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.delete — 删除会话
	gw.Register(MethodDef{
		Name:        "sessions.delete",
		Description: "删除指定会话",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.ID == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "会话 ID 不能为空")
			}
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandDelete,
				Args:    []string{p.ID},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.messages — 获取会话消息历史
	gw.Register(MethodDef{
		Name:        "sessions.messages",
		Description: "获取会话消息历史",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				ID    string `json:"id"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			if p.ID == "" {
				return nil, errs.New(errs.CodeInvalidArgument, "会话 ID 不能为空")
			}
			messages, err := deps.Master.GetSessionMessages(ctx, p.ID, p.Limit)
			if err != nil {
				return nil, err
			}
			return json.Marshal(messages)
		},
	})

	// sessions.clear — 清空会话消息（回滚到索引 0）
	gw.Register(MethodDef{
		Name:        "sessions.clear",
		Description: "清空会话消息",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandRevert,
				Args:    []string{"0"},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.fork — 分叉会话
	gw.Register(MethodDef{
		Name:        "sessions.fork",
		Description: "从指定位置分叉会话",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Name      string `json:"name"`
				ForkPoint string `json:"fork_point"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandFork,
				Args:    []string{p.Name, p.ForkPoint},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})

	// sessions.revert — 回滚会话到指定消息索引
	gw.Register(MethodDef{
		Name:        "sessions.revert",
		Description: "回滚会话到指定消息索引",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Index int `json:"index"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			resp, err := deps.Master.ProcessCommand(ctx, master.SessionRequest{
				Command: master.SessionCommandRevert,
				Args:    []string{strconv.Itoa(p.Index)},
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(resp)
		},
	})
}
