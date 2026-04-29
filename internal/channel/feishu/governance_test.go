package feishu

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGovernance_SuppressedChatBlocksOutbound(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:         "feishu",
			TenantKey:        "tenant-a",
			ChatID:           "oc_chat",
			State:            ChatStateEvicted,
			SuppressOutbound: true,
		},
	}, zap.NewNop())

	err := gov.CheckOutbound(context.Background(), "tenant-a", "oc_chat")

	require.ErrorIs(t, err, ErrOutboundSuppressed)
}

func TestGovernance_AllowsOutboundForActiveChat(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:         "feishu",
			TenantKey:        "tenant-a",
			ChatID:           "oc_chat",
			State:            ChatStateActive,
			SuppressOutbound: false,
		},
	}, zap.NewNop())

	err := gov.CheckOutbound(context.Background(), "tenant-a", "oc_chat")

	require.NoError(t, err)
}

func TestGovernance_ExecuteCommand_HelpAndStatus(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:         "feishu",
			TenantKey:        "tenant-a",
			ChatID:           "oc_chat",
			SessionID:        "sess-1",
			State:            ChatStateActive,
			SuppressOutbound: false,
			RolloutMode:      RolloutModeAllow,
		},
	}, zap.NewNop())

	resp, nextSessionID, handled, err := gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
	}, "sess-1", ParsedCommand{Name: "help", Raw: "/help"})
	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Contains(t, resp, "/help")
	require.Contains(t, resp, "/mute")
	require.Contains(t, resp, "/unmute")
	require.Contains(t, resp, "/model")
	require.Contains(t, resp, "/audit")

	resp, nextSessionID, handled, err = gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
	}, "sess-1", ParsedCommand{Name: "status", Raw: "/status"})
	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Contains(t, resp, "session=sess-1")
	require.Contains(t, resp, "state=active")
}

func TestGovernance_ResetChatSession_TerminatesAndReturnsNewSessionID(t *testing.T) {
	repo := &stubChatStateRepo{}
	terminator := &stubSessionTerminator{}
	gov := NewGovernanceService(repo, zap.NewNop()).WithTerminator(terminator)

	nextSessionID, err := gov.ResetChatSession(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
	}, "sess-old")

	require.NoError(t, err)
	require.NotEmpty(t, nextSessionID)
	require.NotEqual(t, "sess-old", nextSessionID)
	require.Len(t, terminator.calls, 1)
	require.Equal(t, "sess-old", terminator.calls[0].sessionID)
	require.Equal(t, "feishu reset", terminator.calls[0].reason)
}

func TestGovernance_ExecuteCommand_ResetDeniedByACL(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{}, zap.NewNop()).WithACL(NewStaticAllowlistACL(map[string][]string{
		"tenant-a": {"ou-admin"},
	}))

	resp, nextSessionID, handled, err := gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		SenderID:  "ou-user",
		ChatType:  channel.ChatGroup,
	}, "sess-1", ParsedCommand{Name: "reset", Raw: "/reset"})

	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Equal(t, "你没有权限执行 /reset", resp)
}

func TestGovernance_ExecuteCommand_ModelPersistsOverride(t *testing.T) {
	repo := &stubChatStateRepo{}
	gov := NewGovernanceService(repo, zap.NewNop()).
		WithACL(NewStaticAllowlistACL(map[string][]string{
			"tenant-a": {"ou-admin"},
		})).
		WithModelAllowlist([]string{"gpt-5.2"})

	resp, nextSessionID, handled, err := gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		SenderID:  "ou-admin",
		ChatType:  channel.ChatGroup,
	}, "sess-1", ParsedCommand{Name: "model", Raw: "/model", Arg: "gpt-5.2"})

	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Equal(t, "已切换本群模型: gpt-5.2", resp)
	require.True(t, repo.setModelOverrideCalled)
	require.Equal(t, "gpt-5.2", repo.lastModelOverride)
}

func TestGovernance_ExecuteCommand_DebugRejectedInPhaseZero(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{}, zap.NewNop())

	resp, nextSessionID, handled, err := gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		SenderID:  "ou-admin",
		ChatType:  channel.ChatGroup,
	}, "sess-1", ParsedCommand{Name: "debug", Raw: "/debug", Arg: "on"})

	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Equal(t, "/debug 仍处于 Phase 2，当前版本未开放", resp)
}

func TestGovernance_ShouldDropNormalMessage_EvictedChat(t *testing.T) {
	gov := NewGovernanceService(&stubChatStateRepo{}, zap.NewNop())

	drop := gov.ShouldDropNormalMessage(time.Now(), &ChatStateRecord{
		State:       ChatStateEvicted,
		RolloutMode: RolloutModeAllow,
	})

	require.True(t, drop)
}

func TestGovernance_ExecuteCommand_AuditReadsCurrentChatOnly(t *testing.T) {
	dir := t.TempDir()
	sink := NewJSONLAuditSink(filepath.Join(dir, "audit.jsonl"))
	require.NoError(t, sink.Write(context.Background(), AuditRecord{
		TS:        time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC),
		Platform:  "feishu",
		Action:    "push.api",
		Outcome:   "ok",
		TenantKey: "tenant-a",
		Target:    map[string]any{"chat_id": "oc_chat", "msg_type": "text"},
	}))
	require.NoError(t, sink.Write(context.Background(), AuditRecord{
		TS:        time.Date(2026, 4, 26, 9, 1, 0, 0, time.UTC),
		Platform:  "feishu",
		Action:    "push.api",
		Outcome:   "ok",
		TenantKey: "tenant-a",
		Target:    map[string]any{"chat_id": "oc_other", "msg_type": "text"},
	}))

	gov := NewGovernanceService(&stubChatStateRepo{}, zap.NewNop()).
		WithACL(NewStaticAllowlistACL(map[string][]string{
			"tenant-a": {"ou-admin"},
		})).
		WithAuditStore(sink)

	resp, nextSessionID, handled, err := gov.ExecuteCommand(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		SenderID:  "ou-admin",
		ChatType:  channel.ChatGroup,
	}, "sess-1", ParsedCommand{Name: "audit", Raw: "/audit", Arg: "last", Args: []string{"last", "10"}})

	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, nextSessionID)
	require.Contains(t, resp, "push.api")
	require.Contains(t, resp, "chat=oc_chat")
	require.NotContains(t, resp, "oc_other")

	records, err := sink.ReadRecent(context.Background(), AuditQuery{
		Platform:  "feishu",
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Limit:     5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, records)
	require.Equal(t, "command.execute", records[0].Action)
}
