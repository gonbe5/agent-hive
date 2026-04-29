package subagent

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// InheritUserIDFromParent 是 §9.1 的 helper：在 SubAgent spawn 时，确保 child
// ctx 能从 parent 获取 userID，否则返回 CodeUnauthenticated 供 spawn 入口拒绝。
//
// 契约：
//  1. parentCtx 必须带有 userID（通过 auth.WithUser），否则判定为缺失继承 → 失败
//  2. childCtx 返回一个与 parentCtx 同 userID 的派生 ctx（auth.WithUser 幂等）
//  3. 空字符串 userID 等价于缺失 → 失败（保持与 skill_install anon personal 的一致拒绝语义）
//
// 调用方（见 factory.CreateAgent / spawn_agent 路径）在 ctx 经过 InheritUserIDFromParent
// 后，再走 NewAgentLoop + SetUserID，就能保证 AgentLoop.userID 与 parent 一致；
// 在此之上，agentloop.go 的 toolCtx 注入逻辑会把 userID 贯通到工具层。
func InheritUserIDFromParent(parentCtx context.Context) (context.Context, string, error) {
	if parentCtx == nil {
		return nil, "", errs.New(errs.CodeInvalidInput, "InheritUserIDFromParent: parentCtx 为 nil")
	}
	uid := auth.UserIDFrom(parentCtx)
	if uid == "" {
		return parentCtx, "", errs.New(errs.CodeFailedPrecondition, "SubAgent spawn 缺失 userID 继承：parent ctx 无 auth.User")
	}
	if u := auth.UserFrom(parentCtx); u != nil {
		return auth.WithUser(parentCtx, u), uid, nil
	}
	return auth.WithUser(parentCtx, &auth.User{ID: uid, Role: "user", Status: "active"}), uid, nil
}
