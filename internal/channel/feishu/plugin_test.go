package feishu

import (
	"context"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/observability"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPluginBuildMessageContent_ImageAndFile(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)

	msgType, content := p.buildMessageContent(channel.OutboundMessage{
		MsgType: channel.MsgTypeImage,
		Content: `{"image_key":"img_v3_123"}`,
	})
	assert.Equal(t, "image", msgType)
	assert.Equal(t, `{"image_key":"img_v3_123"}`, content)

	msgType, content = p.buildMessageContent(channel.OutboundMessage{
		MsgType: channel.MsgTypeFile,
		Content: `{"file_key":"file_v3_123"}`,
	})
	assert.Equal(t, "file", msgType)
	assert.Equal(t, `{"file_key":"file_v3_123"}`, content)
}

type staticReliabilityLeaderGate struct {
	leader bool
	err    error
}

func (g staticReliabilityLeaderGate) TryAcquire(context.Context) (bool, error) {
	return g.leader, g.err
}

func (g staticReliabilityLeaderGate) Close() error {
	return nil
}

func TestPluginPlatform(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	assert.Equal(t, channel.PlatformFeishu, p.Platform())
}

func TestPluginStart_SkipsLongconnWhenDisabled(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)

	err := p.Start()
	assert.NoError(t, err)

	assert.False(t, p.longConn.started, "longconn must stay disabled in Phase 0 default path")
}

func TestPluginStart_OnlyStartsLongconnInLongconnMode(t *testing.T) {
	logger := zap.NewNop()
	router := channel.NewRouter(nil, logger)

	webhookPlugin := New(config.FeishuConfig{
		IngressMode: config.FeishuIngressModeWebhook,
	}, router, logger)
	err := webhookPlugin.Start()
	assert.NoError(t, err)
	assert.False(t, webhookPlugin.longConn.started, "webhook mode must not start longconn")

	longconnPlugin := New(config.FeishuConfig{
		IngressMode: config.FeishuIngressModeLongconn,
	}, router, logger)
	err = longconnPlugin.Start()
	assert.Error(t, err)
	assert.False(t, longconnPlugin.longConn.started, "longconn startup must not claim success on handshake failure")
}

func TestPluginWithHITLBridge_WiresWebhookAndLongconn(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	bridge := NewFeishuHITLBridge(master.NewHITLBroker(config.HITLConfig{}, nil, make(chan struct{}), logger), logger, nil)

	p = p.WithHITLBridge(bridge)

	assert.Same(t, bridge, p.webhook.hitlBridge)
	assert.Same(t, bridge, p.longConn.hitlBridge)
}

func TestPluginSetMetricsWriter_WiresWebhookAndClient(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	writer := &webhookMetricCaptureWriter{}
	bridge := NewFeishuHITLBridge(&fakeSubmitter{}, logger, nil)
	lifecycle := NewLifecycleHandler(&stubChatStateRepo{}, &stubSessionTerminator{}, &stubWelcomeSender{}, logger)
	p = p.WithHITLBridge(bridge).WithLifecycleHandler(lifecycle)

	p.SetMetricsWriter(writer)

	if p.client.metricsWriter != writer {
		t.Fatal("client metrics writer not wired")
	}
	if p.webhook.metricsWriter != writer {
		t.Fatal("webhook metrics writer not wired")
	}
	if p.webhook.hitlBridge == nil || p.webhook.hitlBridge.metricsWriter != writer {
		t.Fatal("webhook hitl bridge metrics writer not wired")
	}
	if p.longConn.hitlBridge == nil || p.longConn.hitlBridge.metricsWriter != writer {
		t.Fatal("longconn hitl bridge metrics writer not wired")
	}
	if p.webhook.lifecycle == nil || p.webhook.lifecycle.metricsWriter != writer {
		t.Fatal("lifecycle handler metrics writer not wired")
	}
}

var _ observability.MetricsWriter = (*webhookMetricCaptureWriter)(nil)

func TestPluginNew_WiresReliabilityConfigToLongConn(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
			HeartbeatStaleWindow:    75 * time.Second,
			GapFetchMaxWindow:       3 * time.Minute,
		},
	}, channel.NewRouter(nil, logger), logger)

	assert.Equal(t, 75*time.Second, p.longConn.getWatchdogConfig().staleAfter)
	assert.True(t, p.longConn.gapFetchEnabled)
	assert.Equal(t, 3*time.Minute, p.longConn.gapFetchMaxWindow)
}

func TestPluginReplayPendingGapFetch_ConsumesWindowForExplicitChat(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_1",
		SessionID: "sess-1",
	})

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, router, logger)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, &gapFetchTestClient{
		pages: []GapFetchPageResponse{
			{
				Items: []GapFetchMessage{{
					MessageID: "om_gap_plugin",
					Raw: larkim.NewMessageBuilder().
						MessageId("om_gap_plugin").
						ChatId("oc_chat_1").
						MsgType("text").
						Body(larkim.NewMessageBodyBuilder().Content(`{"text":"plugin replay"}`).Build()).
						Build(),
				}},
			},
		},
	}, logger)
	p.longConn.pendingGapFetch = &GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}

	err := p.ReplayPendingGapFetch(context.Background(), "tenant-a", "oc_chat_1")
	assert.NoError(t, err)

	calls := proc.waitCalls(t, 1, time.Second)
	assert.Len(t, calls, 1)
	assert.Equal(t, "om_gap_plugin", calls[0].messageID)

	_, ok := p.longConn.PendingGapFetchWindow()
	assert.False(t, ok, "pending gap window should be consumed after replay")
}

func TestPluginReplayPendingGapFetches_AllChatsSucceededThenConsumesWindow(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_1",
		SessionID: "sess-1",
	})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_2",
		SessionID: "sess-2",
	})

	client := &gapFetchMultiChatClient{
		pagesByChat: map[string][]GapFetchPageResponse{
			"oc_chat_1": {
				{
					Items: []GapFetchMessage{{
						MessageID: "om_gap_1",
						Raw: larkim.NewMessageBuilder().
							MessageId("om_gap_1").
							ChatId("oc_chat_1").
							MsgType("text").
							Body(larkim.NewMessageBodyBuilder().Content(`{"text":"chat1"}`).Build()).
							Build(),
					}},
				},
			},
			"oc_chat_2": {
				{
					Items: []GapFetchMessage{{
						MessageID: "om_gap_2",
						Raw: larkim.NewMessageBuilder().
							MessageId("om_gap_2").
							ChatId("oc_chat_2").
							MsgType("text").
							Body(larkim.NewMessageBodyBuilder().Content(`{"text":"chat2"}`).Build()).
							Build(),
					}},
				},
			},
		},
	}

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, router, logger)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, client, logger)
	p.longConn.pendingGapFetch = &GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}

	err := p.ReplayPendingGapFetches(context.Background(), "tenant-a", []string{"oc_chat_1", "oc_chat_2"})
	assert.NoError(t, err)

	calls := proc.waitCalls(t, 2, time.Second)
	assert.Len(t, calls, 2)
	assert.ElementsMatch(t, []string{"om_gap_1", "om_gap_2"}, []string{calls[0].messageID, calls[1].messageID})

	_, ok := p.longConn.PendingGapFetchWindow()
	assert.False(t, ok, "pending gap window should be consumed after all chat replays succeed")
}

func TestPluginReplayPendingGapFetches_AnyChatFailureKeepsWindow(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_1",
		SessionID: "sess-1",
	})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_2",
		SessionID: "sess-2",
	})

	client := &gapFetchMultiChatClient{
		pagesByChat: map[string][]GapFetchPageResponse{
			"oc_chat_1": {
				{
					Items: []GapFetchMessage{{
						MessageID: "om_gap_ok",
						Raw: larkim.NewMessageBuilder().
							MessageId("om_gap_ok").
							ChatId("oc_chat_1").
							MsgType("text").
							Body(larkim.NewMessageBodyBuilder().Content(`{"text":"ok"}`).Build()).
							Build(),
					}},
				},
			},
		},
		errByChat: map[string]error{
			"oc_chat_2": assert.AnError,
		},
	}

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, router, logger)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, client, logger)
	wantWindow := GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}
	p.longConn.pendingGapFetch = &wantWindow

	err := p.ReplayPendingGapFetches(context.Background(), "tenant-a", []string{"oc_chat_1", "oc_chat_2"})
	assert.Error(t, err)

	gotWindow, ok := p.longConn.PendingGapFetchWindow()
	assert.True(t, ok, "pending gap window should remain after any chat replay fails")
	assert.Equal(t, wantWindow, gotWindow)

	status := p.ReliabilityStatus()
	assert.NotZero(t, status.LastGapFetchAt)
	assert.Equal(t, []string{"oc_chat_1"}, status.LastGapFetchChatIDs)
	assert.Equal(t, assert.AnError.Error(), status.LastGapFetchError)
}

func TestPluginReplayPendingGapFetchForActiveChats_UsesChatStateRepo(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{Platform: channel.PlatformFeishu, ChatID: "oc_chat_1", SessionID: "sess-1"})
	router.Bind(channel.Binding{Platform: channel.PlatformFeishu, ChatID: "oc_chat_2", SessionID: "sess-2"})

	client := &gapFetchMultiChatClient{
		pagesByChat: map[string][]GapFetchPageResponse{
			"oc_chat_1": {{
				Items: []GapFetchMessage{{
					MessageID: "om_gap_a",
					Raw: larkim.NewMessageBuilder().
						MessageId("om_gap_a").
						ChatId("oc_chat_1").
						MsgType("text").
						Body(larkim.NewMessageBodyBuilder().Content(`{"text":"a"}`).Build()).
						Build(),
				}},
			}},
			"oc_chat_2": {{
				Items: []GapFetchMessage{{
					MessageID: "om_gap_b",
					Raw: larkim.NewMessageBuilder().
						MessageId("om_gap_b").
						ChatId("oc_chat_2").
						MsgType("text").
						Body(larkim.NewMessageBodyBuilder().Content(`{"text":"b"}`).Build()).
						Build(),
				}},
			}},
		},
	}
	repo := &stubChatStateRepo{
		listActiveRecords: []ChatStateRecord{
			{Platform: "feishu", TenantKey: "tenant-a", ChatID: "oc_chat_1", State: ChatStateActive},
			{Platform: "feishu", TenantKey: "tenant-a", ChatID: "oc_chat_2", State: ChatStateActive},
			{Platform: "feishu", TenantKey: "tenant-a", ChatID: "oc_chat_1", State: ChatStateActive},
		},
	}

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{LongconnGapFetchEnabled: true},
	}, router, logger).WithChatStateRepo(repo)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, client, logger)
	p.longConn.pendingGapFetch = &GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}

	err := p.ReplayPendingGapFetchForActiveChats(context.Background(), "tenant-a")
	assert.NoError(t, err)

	calls := proc.waitCalls(t, 2, time.Second)
	assert.Len(t, calls, 2)
	assert.ElementsMatch(t, []string{"om_gap_a", "om_gap_b"}, []string{calls[0].messageID, calls[1].messageID})
}

func TestPluginReplayPendingGapFetchForActiveChats_RepoErrorKeepsWindow(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{LongconnGapFetchEnabled: true},
	}, channel.NewRouter(nil, logger), logger).WithChatStateRepo(&stubChatStateRepo{
		listActiveErr: assert.AnError,
	})
	wantWindow := GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}
	p.longConn.pendingGapFetch = &wantWindow

	err := p.ReplayPendingGapFetchForActiveChats(context.Background(), "tenant-a")
	assert.ErrorIs(t, err, assert.AnError)

	gotWindow, ok := p.longConn.PendingGapFetchWindow()
	assert.True(t, ok)
	assert.Equal(t, wantWindow, gotWindow)
}

func TestPluginReplayPendingGapFetch_FailureKeepsPendingWindow(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{
		Platform:  channel.PlatformFeishu,
		ChatID:    "oc_chat_1",
		SessionID: "sess-1",
	})

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, router, logger)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, &gapFetchFailingClient{}, logger)
	wantWindow := GapFetchWindow{
		StartTime: time.Unix(1710000000, 0),
		EndTime:   time.Unix(1710000300, 0),
	}
	p.longConn.pendingGapFetch = &wantWindow

	err := p.ReplayPendingGapFetch(context.Background(), "tenant-a", "oc_chat_1")
	assert.Error(t, err)

	gotWindow, ok := p.longConn.PendingGapFetchWindow()
	assert.True(t, ok, "pending gap window should remain after replay failure")
	assert.Equal(t, wantWindow, gotWindow)
}

func TestPluginReliabilityStatus_ExposesLongConnSnapshot(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
			HeartbeatStaleWindow:    42 * time.Second,
			GapFetchMaxWindow:       2 * time.Minute,
		},
	}, channel.NewRouter(nil, logger), logger)

	now := time.Unix(1710000600, 0)
	p.longConn.setLastEventAt(now)
	p.longConn.setLastTenantKey("tenant-a")
	p.longConn.setReconnecting(true)
	p.longConn.pendingGapFetch = &GapFetchWindow{
		StartTime: now.Add(-30 * time.Second),
		EndTime:   now,
	}

	status := p.ReliabilityStatus()
	assert.True(t, status.Reconnecting)
	assert.True(t, status.GapFetchEnabled)
	assert.True(t, status.GapFetchPending)
	assert.Equal(t, 42*time.Second, status.HeartbeatStaleWindow)
	assert.Equal(t, 2*time.Minute, status.GapFetchMaxWindow)
	assert.Equal(t, now, status.LastEventAt)
	assert.Equal(t, "tenant-a", status.LastTenantKey)
	assert.Equal(t, now.Add(-30*time.Second), status.PendingGapFetchWindow.StartTime)
}

func TestPluginReliabilityStatus_ExposesLastGapFetchReplayResult(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, channel.NewRouter(nil, logger), logger)

	p.longConn.recordGapFetchReplay([]string{"oc_chat_1", "oc_chat_2"}, assert.AnError)

	status := p.ReliabilityStatus()
	assert.NotZero(t, status.LastGapFetchAt)
	assert.Equal(t, []string{"oc_chat_1", "oc_chat_2"}, status.LastGapFetchChatIDs)
	assert.Equal(t, assert.AnError.Error(), status.LastGapFetchError)
}

func TestPluginRecoveredWatchdog_AutoReplaysActiveChats(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{Platform: channel.PlatformFeishu, ChatID: "oc_chat_1", SessionID: "sess-1"})

	repo := &stubChatStateRepo{
		listActiveRecords: []ChatStateRecord{
			{Platform: "feishu", TenantKey: "tenant-a", ChatID: "oc_chat_1", State: ChatStateActive},
		},
	}
	client := &gapFetchMultiChatClient{
		pagesByChat: map[string][]GapFetchPageResponse{
			"oc_chat_1": {{
				Items: []GapFetchMessage{{
					MessageID: "om_auto_1",
					Raw: larkim.NewMessageBuilder().
						MessageId("om_auto_1").
						ChatId("oc_chat_1").
						MsgType("text").
						Body(larkim.NewMessageBodyBuilder().Content(`{"text":"auto"}`).Build()).
						Build(),
				}},
			}},
		},
	}

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
			HeartbeatStaleWindow:    30 * time.Second,
		},
	}, router, logger).WithChatStateRepo(repo)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, client, logger)

	base := time.Unix(1710000600, 0)
	p.longConn.setLastTenantKey("tenant-a")
	p.longConn.setLastEventAt(base.Add(-2 * time.Minute))
	cfg := p.longConn.getWatchdogConfig()
	cfg.enabled = true
	cfg.staleAfter = 30 * time.Second
	cfg.checkInterval = time.Second
	cfg.now = func() time.Time { return base }
	p.longConn.initWatchdog(cfg)

	p.longConn.runWatchdogCheck(p.longConn.getWatchdogConfig())
	require.True(t, p.longConn.IsReconnecting())

	p.longConn.markEventObserved(base.Add(10 * time.Second))

	calls := proc.waitCalls(t, 1, time.Second)
	require.Len(t, calls, 1)
	assert.Equal(t, "om_auto_1", calls[0].messageID)

	status := p.ReliabilityStatus()
	assert.Equal(t, []string{"oc_chat_1"}, status.LastGapFetchChatIDs)
	assert.Empty(t, status.LastGapFetchError)

	_, ok := p.longConn.PendingGapFetchWindow()
	assert.False(t, ok)
}

func TestPluginRecoveredWatchdog_AutoReplaySkipsWhenNotLeader(t *testing.T) {
	logger := zap.NewNop()
	proc := &gapFetchCaptureProcessor{}
	router := channel.NewRouter(proc, logger)
	router.RegisterPlugin(&noopPlugin{platform: channel.PlatformFeishu})
	router.Bind(channel.Binding{Platform: channel.PlatformFeishu, ChatID: "oc_chat_1", SessionID: "sess-1"})

	repo := &stubChatStateRepo{
		listActiveRecords: []ChatStateRecord{
			{Platform: "feishu", TenantKey: "tenant-a", ChatID: "oc_chat_1", State: ChatStateActive},
		},
	}
	client := &gapFetchMultiChatClient{
		pagesByChat: map[string][]GapFetchPageResponse{
			"oc_chat_1": {{
				Items: []GapFetchMessage{{
					MessageID: "om_auto_skip",
					Raw: larkim.NewMessageBuilder().
						MessageId("om_auto_skip").
						ChatId("oc_chat_1").
						MsgType("text").
						Body(larkim.NewMessageBodyBuilder().Content(`{"text":"auto"}`).Build()).
						Build(),
				}},
			}},
		},
	}

	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
			HeartbeatStaleWindow:    30 * time.Second,
		},
	}, router, logger).WithChatStateRepo(repo)
	p.longConn.gapFetchRunner = newGapFetchRunner(router, client, logger)
	p.WithReliabilityLeaderGate(staticReliabilityLeaderGate{leader: false})

	base := time.Unix(1710000600, 0)
	p.longConn.setLastTenantKey("tenant-a")
	p.longConn.setLastEventAt(base.Add(-2 * time.Minute))
	cfg := p.longConn.getWatchdogConfig()
	cfg.enabled = true
	cfg.staleAfter = 30 * time.Second
	cfg.checkInterval = time.Second
	cfg.now = func() time.Time { return base }
	p.longConn.initWatchdog(cfg)

	p.longConn.runWatchdogCheck(p.longConn.getWatchdogConfig())
	require.True(t, p.longConn.IsReconnecting())

	p.longConn.markEventObserved(base.Add(10 * time.Second))
	time.Sleep(150 * time.Millisecond)

	status := p.ReliabilityStatus()
	assert.False(t, status.ReliabilityLeader)
	assert.True(t, status.GapFetchPending)
	assert.Empty(t, status.LastGapFetchChatIDs)
	assert.Empty(t, status.LastGapFetchError)

	gotWindow, ok := p.longConn.PendingGapFetchWindow()
	assert.True(t, ok)
	assert.Equal(t, base.Add(-2*time.Minute), gotWindow.StartTime)
	assert.Equal(t, base.Add(10*time.Second), gotWindow.EndTime)
}

func TestPluginReliabilityStatus_ExposesLeaderState(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{
		Reliability: config.FeishuReliabilityConfig{
			LongconnGapFetchEnabled: true,
		},
	}, channel.NewRouter(nil, logger), logger)

	p.WithReliabilityLeaderGate(staticReliabilityLeaderGate{leader: true})

	status := p.ReliabilityStatus()
	assert.True(t, status.ReliabilityLeader)
}

func TestPluginSend_SuppressedChatSkipsOutbound(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	p.governance = NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:         "feishu",
			TenantKey:        "tenant-a",
			ChatID:           "oc_chat",
			State:            ChatStateEvicted,
			SuppressOutbound: true,
		},
	}, logger)

	err := p.Send(context.Background(), channel.OutboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "should be suppressed",
	})

	assert.NoError(t, err)
}

func TestPluginControlInbound_HandlesResetCommand(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	p.governance = NewGovernanceService(&stubChatStateRepo{}, logger)

	result, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "/ReSeT\u200b ",
	}, "im-feishu-tenant-a-oc_chat")

	assert.NoError(t, err)
	assert.True(t, result.Handled)
	assert.NotEmpty(t, result.SessionIDOverride)
	assert.Contains(t, result.Response, "会话已重置")
}

func TestPluginControlInbound_DropsMutedOrDeniedNormalMessage(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	now := time.Now().Add(10 * time.Minute)
	p.governance = NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:    "feishu",
			TenantKey:   "tenant-a",
			ChatID:      "oc_chat",
			State:       ChatStateActive,
			RolloutMode: RolloutModeDeny,
			MuteUntil:   &now,
		},
	}, logger)

	result, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "hello",
	}, "sess-1")

	assert.NoError(t, err)
	assert.True(t, result.Drop)
	assert.False(t, result.Handled)
}

func TestPluginControlInbound_DropsEvictedNormalMessageButAllowsStatus(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	now := time.Now().Add(10 * time.Minute)
	p.governance = NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:         "feishu",
			TenantKey:        "tenant-a",
			ChatID:           "oc_chat",
			SessionID:        "sess-evicted",
			State:            ChatStateEvicted,
			RolloutMode:      RolloutModeDeny,
			MuteUntil:        &now,
			SuppressOutbound: true,
		},
	}, logger)

	dropResult, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "hello",
	}, "sess-evicted")

	assert.NoError(t, err)
	assert.True(t, dropResult.Drop)
	assert.False(t, dropResult.Handled)

	statusResult, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "/status",
	}, "sess-evicted")

	assert.NoError(t, err)
	assert.True(t, statusResult.Handled)
	assert.Contains(t, statusResult.Response, "state=evicted")
}

func TestPluginControlInbound_HandlesMuteCommand(t *testing.T) {
	logger := zap.NewNop()
	repo := &stubChatStateRepo{}
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	p.governance = NewGovernanceService(repo, logger)

	result, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "/mute 15m",
	}, "sess-1")

	assert.NoError(t, err)
	assert.True(t, result.Handled)
	assert.Contains(t, result.Response, "已静默")
	if assert.NotNil(t, repo.lastMuteUntil) {
		assert.WithinDuration(t, time.Now().Add(15*time.Minute), *repo.lastMuteUntil, 2*time.Second)
	}
}

func TestPluginControlInbound_HandlesUnmuteCommand(t *testing.T) {
	logger := zap.NewNop()
	repo := &stubChatStateRepo{}
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	p.governance = NewGovernanceService(repo, logger)

	result, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "/unmute",
	}, "sess-1")

	assert.NoError(t, err)
	assert.True(t, result.Handled)
	assert.Contains(t, result.Response, "已取消静默")
	assert.True(t, repo.setMuteUntilCalled)
	assert.Nil(t, repo.lastMuteUntil)
}

func TestPluginControlInbound_CarriesPersistedModelOverride(t *testing.T) {
	logger := zap.NewNop()
	p := New(config.FeishuConfig{}, channel.NewRouter(nil, logger), logger)
	p.governance = NewGovernanceService(&stubChatStateRepo{
		getRecord: &ChatStateRecord{
			Platform:      "feishu",
			TenantKey:     "tenant-a",
			ChatID:        "oc_chat",
			State:         ChatStateActive,
			RolloutMode:   RolloutModeAllow,
			ModelOverride: "gpt-5.2",
		},
	}, logger)

	result, err := p.ControlInbound(context.Background(), channel.InboundMessage{
		Platform:  channel.PlatformFeishu,
		TenantKey: "tenant-a",
		ChatID:    "oc_chat",
		Content:   "hello",
	}, "sess-1")

	assert.NoError(t, err)
	assert.False(t, result.Handled)
	assert.False(t, result.Drop)
	assert.Equal(t, "gpt-5.2", result.ModelOverride)
}
