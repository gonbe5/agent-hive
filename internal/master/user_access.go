package master

import (
	"context"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/errs"
	"github.com/chef-guo/agents-hive/internal/store"
)

// checkSessionAccess 统一的会话访问权限检查
// 返回 session record 或权限错误
// 规则：
//   - auth 未启用 → 放行（session 不存在时也放行，让 session_loop 自行创建）
//   - auth 启用 + 未登录 → 拒绝
//   - 所有用户（包括 admin）：只能访问自己的 session 或遗留无主 session（user_id=""）
//
// C4: 直接调 m.store.LoadSession，绕过 SessionManager 内存缓存，
// 确保读取到持久化的 user_id（内存缓存中的 UserID 可能为空）
//
// Phase 4 fix: IM 路径第一条消息时 session 尚未持久化到 DB，
// LoadSession 返回"未找到"错误。auth 未启用时应放行，让 session_loop 创建 session。
func (m *Master) checkSessionAccess(ctx context.Context, sessionID string) (*store.SessionRecord, error) {
	if m.store == nil {
		return nil, errs.New(errs.CodeInternal, "存储未初始化")
	}
	record, err := m.store.LoadSession(ctx, sessionID)
	if err != nil {
		// auth 未启用时，session 不存在不是权限问题（IM 路径第一条消息时 session 尚未持久化）
		// 放行让 session_loop 自行创建 session
		// 只对 ErrNotFound 放行；真实 DB 故障（连接超时等）仍然返回错误
		if !auth.IsAuthEnabled(ctx) && err == store.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	if record == nil {
		return nil, errs.New(errs.CodeNotFound, "会话不存在")
	}

	// auth 未启用 → 放行
	if !auth.IsAuthEnabled(ctx) {
		return record, nil
	}

	user := auth.UserFrom(ctx)
	if user == nil {
		// C5: 记录权限拒绝日志，便于运维定位
		m.logger.Info("checkSessionAccess: auth 启用但未登录，拒绝访问",
			zap.String("session_id", sessionID),
		)
		return nil, errs.New(errs.CodePermissionDenied, "未登录")
	}

	// 所有用户（包括 admin）：只能访问自己的 session，遗留无主 session 也不可见
	if record.UserID != user.ID {
		// C5: 记录越权访问日志；返回 NotFound 避免泄露 session 存在性
		m.logger.Info("checkSessionAccess: 越权访问，拒绝",
			zap.String("user_id", user.ID),
			zap.String("session_id", sessionID),
			zap.String("session_owner", record.UserID),
		)
		return nil, errs.New(errs.CodeNotFound, "会话不存在")
	}

	return record, nil
}
