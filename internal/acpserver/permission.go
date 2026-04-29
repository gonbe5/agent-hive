package acpserver

import (
	"context"
	"fmt"

	acp "github.com/coder/acp-go-sdk"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/skills"
)

// createACPPermissionFn 创建一个桥接 ACP 权限请求的 promptFn
// 当 Master 需要请求工具执行权限时，通过 ACP 协议转发给 IDE 客户端
func createACPPermissionFn(
	conn *acp.AgentSideConnection,
	sessionID string,
	logger *zap.Logger,
) func(ctx context.Context, req skills.PermissionRequest) (skills.PermissionResponse, error) {
	return func(ctx context.Context, req skills.PermissionRequest) (skills.PermissionResponse, error) {
		logger.Debug("ACP 权限请求",
			zap.String("session_id", sessionID),
			zap.String("tool", req.ToolName))

		// 构建 ACP 权限请求
		permResp, err := conn.RequestPermission(ctx, acp.RequestPermissionRequest{
			SessionId: acp.SessionId(sessionID),
			ToolCall: acp.RequestPermissionToolCall{
				ToolCallId: acp.ToolCallId(fmt.Sprintf("perm_%s", req.ToolName)),
				Title:      acp.Ptr(req.Description),
				Kind:       acp.Ptr(toolKindFromName(req.ToolName)),
				Status:     acp.Ptr(acp.ToolCallStatusPending),
			},
			Options: []acp.PermissionOption{
				{
					Kind:     acp.PermissionOptionKindAllowOnce,
					Name:     "允许此次操作",
					OptionId: acp.PermissionOptionId("allow_once"),
				},
				{
					Kind:     acp.PermissionOptionKindAllowAlways,
					Name:     "本次会话始终允许",
					OptionId: acp.PermissionOptionId("allow_session"),
				},
				{
					Kind:     acp.PermissionOptionKindRejectOnce,
					Name:     "拒绝此次操作",
					OptionId: acp.PermissionOptionId("reject"),
				},
			},
		})
		if err != nil {
			logger.Warn("ACP 权限请求失败，默认拒绝",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return skills.PermissionResponse{Granted: false}, nil
		}

		// 处理权限结果
		if permResp.Outcome.Cancelled != nil {
			return skills.PermissionResponse{Granted: false}, nil
		}
		if permResp.Outcome.Selected == nil {
			return skills.PermissionResponse{Granted: false}, nil
		}

		switch string(permResp.Outcome.Selected.OptionId) {
		case "allow_once":
			return skills.PermissionResponse{Granted: true, Remember: false}, nil
		case "allow_session":
			return skills.PermissionResponse{Granted: true, Remember: true}, nil
		default: // reject
			return skills.PermissionResponse{Granted: false}, nil
		}
	}
}
