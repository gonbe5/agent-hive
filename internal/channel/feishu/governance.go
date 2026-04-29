package feishu

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/imctx"
	"go.uber.org/zap"
)

var ErrOutboundSuppressed = errors.New("feishu outbound suppressed")

type GovernanceService struct {
	repo              ChatStateRepo
	terminator        SessionTerminator
	acl               CommandACL
	rollout           DeterministicRollout
	models            map[string]struct{}
	debugEnabled      bool
	multiAgentEnabled bool
	auditStore        AuditStore
	logger            *zap.Logger
}

func NewGovernanceService(repo ChatStateRepo, logger *zap.Logger) *GovernanceService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GovernanceService{
		repo:   repo,
		logger: logger,
	}
}

func (g *GovernanceService) WithTerminator(terminator SessionTerminator) *GovernanceService {
	g.terminator = terminator
	return g
}

func (g *GovernanceService) WithACL(acl CommandACL) *GovernanceService {
	g.acl = acl
	return g
}

func (g *GovernanceService) WithModelAllowlist(models []string) *GovernanceService {
	allowed := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		allowed[strings.ToLower(model)] = struct{}{}
	}
	g.models = allowed
	return g
}

func (g *GovernanceService) WithDebugEnabled(enabled bool) *GovernanceService {
	g.debugEnabled = enabled
	return g
}

func (g *GovernanceService) WithMultiAgentEnabled(enabled bool) *GovernanceService {
	g.multiAgentEnabled = enabled
	return g
}

func (g *GovernanceService) WithAuditStore(store AuditStore) *GovernanceService {
	g.auditStore = store
	return g
}

func (g *GovernanceService) CheckOutbound(ctx context.Context, tenantKey, chatID string) error {
	if g == nil || g.repo == nil {
		return nil
	}
	state, err := g.repo.Get(ctx, lifecyclePlatform, tenantKey, chatID)
	if err != nil {
		return err
	}
	if state != nil && state.SuppressOutbound {
		g.logger.Warn("飞书出站消息被 suppression 拦截",
			zap.String("tenant_key", tenantKey),
			zap.String("chat_id", chatID))
		return ErrOutboundSuppressed
	}
	return nil
}

func (g *GovernanceService) CheckInbound(ctx context.Context, tenantKey, chatID string) (*ChatStateRecord, error) {
	if g == nil || g.repo == nil {
		return nil, nil
	}
	return g.repo.Get(ctx, lifecyclePlatform, tenantKey, chatID)
}

func (g *GovernanceService) ExecuteCommand(ctx context.Context, msg channel.InboundMessage, currentSessionID string, cmd ParsedCommand) (string, string, bool, error) {
	reply := func(response, nextSessionID string, handled bool, outcome string, err error) (string, string, bool, error) {
		if handled {
			g.writeCommandAudit(ctx, msg, cmd, outcome, err)
		}
		return response, nextSessionID, handled, err
	}
	switch cmd.Name {
	case "help":
		return reply("可用命令: /help /status /reset /mute /unmute /model /audit", "", true, "ok", nil)
	case "status":
		response, nextSessionID, handled, err := g.renderStatus(ctx, msg, currentSessionID)
		if err != nil {
			return reply(response, nextSessionID, handled, "error", err)
		}
		return reply(response, nextSessionID, handled, "ok", nil)
	case "reset":
		if denied, err := g.checkAdminCommand(ctx, msg, "reset"); err != nil || denied {
			if err != nil {
				return reply("", "", true, "error", err)
			}
			return reply("你没有权限执行 /reset", "", true, "denied", nil)
		}
		sessionID, err := g.ResetChatSession(ctx, msg, currentSessionID)
		if err != nil {
			return reply("", "", true, "error", err)
		}
		return reply("会话已重置: "+sessionID, sessionID, true, "ok", nil)
	case "mute":
		if denied, err := g.checkAdminCommand(ctx, msg, "mute"); err != nil || denied {
			if err != nil {
				return reply("", "", true, "error", err)
			}
			return reply("你没有权限执行 /mute", "", true, "denied", nil)
		}
		until, err := parseMuteDuration(cmd.Arg)
		if err != nil {
			return reply("mute 参数无效，示例: /mute 15m", "", true, "invalid", nil)
		}
		if err := g.SetMute(ctx, msg, until); err != nil {
			return reply("", "", true, "error", err)
		}
		return reply("已静默到: "+until.UTC().Format(time.RFC3339), "", true, "ok", nil)
	case "unmute":
		if denied, err := g.checkAdminCommand(ctx, msg, "unmute"); err != nil || denied {
			if err != nil {
				return reply("", "", true, "error", err)
			}
			return reply("你没有权限执行 /unmute", "", true, "denied", nil)
		}
		if err := g.ClearMute(ctx, msg); err != nil {
			return reply("", "", true, "error", err)
		}
		return reply("已取消静默", "", true, "ok", nil)
	case "model":
		if denied, err := g.checkAdminCommand(ctx, msg, "model"); err != nil || denied {
			if err != nil {
				return reply("", "", true, "error", err)
			}
			return reply("你没有权限执行 /model", "", true, "denied", nil)
		}
		model := strings.TrimSpace(cmd.Arg)
		if model == "" {
			return reply("model 参数无效，示例: /model gpt-5.2", "", true, "invalid", nil)
		}
		if !g.modelAllowed(model) {
			return reply(fmt.Sprintf("模型 %q 不在白名单内", model), "", true, "invalid", nil)
		}
		if err := g.SetModelOverride(ctx, msg, model); err != nil {
			return reply("", "", true, "error", err)
		}
		return reply("已切换本群模型: "+model, "", true, "ok", nil)
	case "audit":
		if denied, err := g.checkAdminCommand(ctx, msg, "audit"); err != nil || denied {
			if err != nil {
				return reply("", "", true, "error", err)
			}
			return reply("你没有权限执行 /audit", "", true, "denied", nil)
		}
		if len(cmd.Args) != 2 || cmd.Args[0] != "last" {
			return reply("audit 参数无效，示例: /audit last 10", "", true, "invalid", nil)
		}
		limit, err := strconv.Atoi(cmd.Args[1])
		if err != nil || limit <= 0 {
			return reply("audit 参数无效，示例: /audit last 10", "", true, "invalid", nil)
		}
		if limit > 20 {
			limit = 20
		}
		response, err := g.renderAudit(ctx, msg, limit)
		if err != nil {
			return reply("", "", true, "error", err)
		}
		return reply(response, "", true, "ok", nil)
	case "debug":
		if !g.debugEnabled {
			return reply("/debug 仍处于 Phase 2，当前版本未开放", "", true, "rejected", nil)
		}
		return reply("/debug 尚未完成安全约束，暂不可用", "", true, "rejected", nil)
	case "agent":
		if !g.multiAgentEnabled {
			return reply("/agent 仍处于 Phase 2，当前版本未开放", "", true, "rejected", nil)
		}
		return reply("/agent 尚未完成 profile 路由与隔离，暂不可用", "", true, "rejected", nil)
	default:
		return "", "", false, nil
	}
}

func (g *GovernanceService) IsMuted(now time.Time, state *ChatStateRecord) bool {
	if state == nil || state.MuteUntil == nil {
		return false
	}
	return state.MuteUntil.After(now)
}

func (g *GovernanceService) ShouldDropNormalMessage(now time.Time, state *ChatStateRecord) bool {
	if state == nil {
		return false
	}
	if state.State == ChatStateEvicted {
		return true
	}
	if !g.rollout.Allow(state.RolloutMode) {
		return true
	}
	return g.IsMuted(now, state)
}

func (g *GovernanceService) renderStatus(ctx context.Context, msg channel.InboundMessage, currentSessionID string) (string, string, bool, error) {
	state, err := g.CheckInbound(ctx, msg.TenantKey, msg.ChatID)
	if err != nil {
		return "", "", true, err
	}
	lifecycleState := "missing"
	rolloutMode := string(RolloutModeAllow)
	mutedUntil := "off"
	suppressed := false
	if state != nil {
		lifecycleState = string(state.State)
		if state.RolloutMode != "" {
			rolloutMode = string(state.RolloutMode)
		}
		if state.MuteUntil != nil {
			mutedUntil = state.MuteUntil.UTC().Format(time.RFC3339)
		}
		suppressed = state.SuppressOutbound
		if state.SessionID != "" {
			currentSessionID = state.SessionID
		}
	}
	return fmt.Sprintf("session=%s\nstate=%s\nrollout=%s\nmute_until=%s\nsuppress_outbound=%t",
		currentSessionID, lifecycleState, rolloutMode, mutedUntil, suppressed), "", true, nil
}

func (g *GovernanceService) renderAudit(ctx context.Context, msg channel.InboundMessage, limit int) (string, error) {
	if g == nil || g.auditStore == nil {
		return "audit 未启用", nil
	}
	records, err := g.auditStore.ReadRecent(ctx, AuditQuery{
		Platform:  lifecyclePlatform,
		TenantKey: msg.TenantKey,
		ChatID:    msg.ChatID,
		Limit:     limit,
	})
	if err != nil {
		return "", err
	}
	return formatAuditRecords(records), nil
}

func (g *GovernanceService) ResetChatSession(ctx context.Context, msg channel.InboundMessage, currentSessionID string) (string, error) {
	if currentSessionID != "" && g.terminator != nil {
		if err := g.terminator.TerminateSession(currentSessionID, "feishu reset"); err != nil {
			return "", err
		}
	}
	sessionID := buildResetSessionID(msg)
	if g.repo != nil {
		record := ChatStateRecord{
			Platform:         lifecyclePlatform,
			TenantKey:        msg.TenantKey,
			ChatID:           msg.ChatID,
			SessionID:        sessionID,
			State:            ChatStateActive,
			RolloutMode:      RolloutModeAllow,
			SuppressOutbound: false,
			UpdatedBy:        "feishu.command.reset",
		}
		if err := g.repo.Upsert(ctx, record); err != nil && !errors.Is(err, ErrChatStateRepoNotImplemented) {
			return "", err
		}
	}
	return sessionID, nil
}

func (g *GovernanceService) SetMute(ctx context.Context, msg channel.InboundMessage, until time.Time) error {
	if g == nil || g.repo == nil {
		return nil
	}
	return g.repo.SetMuteUntil(ctx, lifecyclePlatform, msg.TenantKey, msg.ChatID, &until, "feishu.command.mute")
}

func (g *GovernanceService) ClearMute(ctx context.Context, msg channel.InboundMessage) error {
	if g == nil || g.repo == nil {
		return nil
	}
	return g.repo.SetMuteUntil(ctx, lifecyclePlatform, msg.TenantKey, msg.ChatID, nil, "feishu.command.unmute")
}

func (g *GovernanceService) SetModelOverride(ctx context.Context, msg channel.InboundMessage, model string) error {
	if g == nil || g.repo == nil {
		return nil
	}
	return g.repo.SetModelOverride(ctx, lifecyclePlatform, msg.TenantKey, msg.ChatID, model, "feishu.command.model")
}

func (g *GovernanceService) checkAdminCommand(ctx context.Context, msg channel.InboundMessage, command string) (bool, error) {
	if g == nil || g.acl == nil {
		return false, nil
	}
	allowed, err := g.acl.CanExecute(ctx, msg.TenantKey, msg.ChatID, msg.SenderID, msg.ChatType == channel.ChatDirect, command)
	if err != nil {
		return false, err
	}
	return !allowed, nil
}

func (g *GovernanceService) modelAllowed(model string) bool {
	if len(g.models) == 0 {
		return false
	}
	_, ok := g.models[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

func (g *GovernanceService) writeCommandAudit(ctx context.Context, msg channel.InboundMessage, cmd ParsedCommand, outcome string, commandErr error) {
	if g == nil || g.auditStore == nil {
		return
	}
	record := AuditRecord{
		TS:        time.Now().UTC(),
		Platform:  lifecyclePlatform,
		Action:    "command.execute",
		Outcome:   outcome,
		TenantKey: msg.TenantKey,
		Actor: map[string]any{
			"type":      "im_user",
			"sender_id": SafeSenderID(msg.SenderID),
		},
		Target: map[string]any{
			"chat_id": msg.ChatID,
			"command": cmd.Raw,
		},
	}
	if len(cmd.Args) > 0 {
		record.Target["args"] = append([]string(nil), cmd.Args...)
	}
	if commandErr != nil {
		record.Error = commandErr.Error()
	}
	_ = g.auditStore.Write(ctx, record)
}

func parseMuteDuration(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("missing mute duration")
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return time.Now().Add(d), nil
	}
	if mins, err := strconv.Atoi(raw); err == nil && mins > 0 {
		return time.Now().Add(time.Duration(mins) * time.Minute), nil
	}
	return time.Time{}, errors.New("invalid mute duration")
}

// buildResetSessionID 通过 imctx.BuildSessionID 唯一入口构造 session_id。
//
// Phase 0 P0-#10/#14 不变式:所有 IM session_id 必须经 BuildSessionID 4 段格式
// `im-{platform}-{tenantKey}-{chatID}` 构造,禁止 fmt.Sprintf 自拼。
//
// reset 前后 session_id 保持一致:旧 session 由 terminator 杀掉,新消息进来时
// master 用同一 sessionID lazy-create 新 SessionState。router.go:614 看到
// SessionIDOverride == currentSessionID 时 no-op,master 自然衔接。
//
// tenantKey 为空时 fallback 到 DefaultTenantKey,与 tenant.go 单租户路径一致。
// platform 为空时默认 PlatformFeishu(本函数仅飞书路径调用)。
// chatID 为空属上游错误,BuildSessionID 返 ErrEmptySessionPart,本函数返空串
// 让 ResetChatSession 调用方感知降级。
func buildResetSessionID(msg channel.InboundMessage) string {
	tenantKey := msg.TenantKey
	if tenantKey == "" {
		tenantKey = DefaultTenantKey
	}
	platform := msg.Platform
	if platform == "" {
		platform = channel.PlatformFeishu
	}
	sessionID, err := imctx.BuildSessionID(imctx.Platform(platform), tenantKey, msg.ChatID)
	if err != nil {
		return ""
	}
	return sessionID
}
