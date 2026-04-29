package feishu

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresChatStateRepo_GetRequiresTenantKey 验证租户键为空时仓储必须 fail-closed。
func TestPostgresChatStateRepo_GetRequiresTenantKey(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)

	record, err := repo.Get(context.Background(), "feishu", "", "chat-1")

	assert.Nil(t, record)
	assert.ErrorIs(t, err, ErrTenantKeyRequired)
}

// TestPostgresChatStateRepo_RequiresTenantKeyAcrossEntryPoints 验证所有公开入口在 tenantKey 为空时都必须 fail-closed。
func TestPostgresChatStateRepo_RequiresTenantKeyAcrossEntryPoints(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "Get",
			run: func() error {
				_, err := repo.Get(ctx, "feishu", "", "chat-1")
				return err
			},
		},
		{
			name: "ListActive",
			run: func() error {
				_, err := repo.ListActive(ctx, "feishu", "")
				return err
			},
		},
		{
			name: "Upsert",
			run: func() error {
				return repo.Upsert(ctx, ChatStateRecord{
					Platform:  "feishu",
					TenantKey: "",
					ChatID:    "chat-1",
				})
			},
		},
		{
			name: "MarkEvicted",
			run: func() error {
				_, _, err := repo.MarkEvicted(ctx, "feishu", "", "chat-1", "evt-1", 123, "tester")
				return err
			},
		},
		{
			name: "MarkActive",
			run: func() error {
				_, _, err := repo.MarkActive(ctx, "feishu", "", "chat-1", "evt-1", 123, "tester")
				return err
			},
		},
		{
			name: "SetSessionID",
			run: func() error {
				return repo.SetSessionID(ctx, "feishu", "", "chat-1", "sess-1", "tester")
			},
		},
		{
			name: "SetMuteUntil",
			run: func() error {
				return repo.SetMuteUntil(ctx, "feishu", "", "chat-1", &now, "tester")
			},
		},
		{
			name: "SetRolloutMode",
			run: func() error {
				return repo.SetRolloutMode(ctx, "feishu", "", "chat-1", RolloutModeAllow, "tester")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			assert.ErrorIs(t, err, ErrTenantKeyRequired)
		})
	}
}

func TestPostgresChatStateRepo_MutateGovernanceFields(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	repo := NewPostgresChatStateRepo(pool, nil)
	ctx := context.Background()

	err := repo.Upsert(ctx, ChatStateRecord{
		Platform:    "feishu",
		TenantKey:   "tenant-1",
		ChatID:      "chat-1",
		SessionID:   "sess-1",
		State:       ChatStateActive,
		RolloutMode: RolloutModeAllow,
		UpdatedBy:   "seed",
	})
	require.NoError(t, err)

	muteUntil := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	require.NoError(t, repo.SetMuteUntil(ctx, "feishu", "tenant-1", "chat-1", &muteUntil, "tester"))
	require.NoError(t, repo.SetRolloutMode(ctx, "feishu", "tenant-1", "chat-1", RolloutModeDeny, "tester"))
	require.NoError(t, repo.SetSessionID(ctx, "feishu", "tenant-1", "chat-1", "sess-2", "tester"))

	record, err := repo.Get(ctx, "feishu", "tenant-1", "chat-1")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "sess-2", record.SessionID)
	assert.Equal(t, RolloutModeDeny, record.RolloutMode)
	if assert.NotNil(t, record.MuteUntil) {
		assert.WithinDuration(t, muteUntil, *record.MuteUntil, time.Second)
	}
}

// TestPlanLifecycleTransition 验证 lifecycle 规则的纯逻辑判定：新建、重复、同时间和更旧事件都要有确定语义。
func TestPlanLifecycleTransition(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name          string
		current       *ChatStateRecord
		targetState   ChatLifecycleState
		eventID       string
		eventTime     int64
		updatedBy     string
		wantChanged   bool
		wantState     ChatLifecycleState
		wantEventID   string
		wantEventTime int64
		wantUpdatedBy string
		wantSessionID string
	}{
		{
			name:          "create evicted on missing row",
			current:       nil,
			targetState:   ChatStateEvicted,
			eventID:       "evt-1",
			eventTime:     100,
			updatedBy:     "tester",
			wantChanged:   true,
			wantState:     ChatStateEvicted,
			wantEventID:   "evt-1",
			wantEventTime: 100,
			wantUpdatedBy: "tester",
			wantSessionID: "",
		},
		{
			name: "same event is noop",
			current: &ChatStateRecord{
				SessionID:              "sess-1",
				State:                  ChatStateEvicted,
				LastLifecycleEventID:   "evt-1",
				LastLifecycleEventTime: 100,
				UpdatedAt:              now,
				UpdatedBy:              "seed",
			},
			targetState:   ChatStateEvicted,
			eventID:       "evt-1",
			eventTime:     100,
			updatedBy:     "tester",
			wantChanged:   false,
			wantState:     ChatStateEvicted,
			wantEventID:   "evt-1",
			wantEventTime: 100,
			wantUpdatedBy: "seed",
			wantSessionID: "sess-1",
		},
		{
			name: "same time different event is noop",
			current: &ChatStateRecord{
				SessionID:              "sess-1",
				State:                  ChatStateEvicted,
				LastLifecycleEventID:   "evt-1",
				LastLifecycleEventTime: 100,
				UpdatedAt:              now,
				UpdatedBy:              "seed",
			},
			targetState:   ChatStateActive,
			eventID:       "evt-2",
			eventTime:     100,
			updatedBy:     "tester",
			wantChanged:   false,
			wantState:     ChatStateEvicted,
			wantEventID:   "evt-1",
			wantEventTime: 100,
			wantUpdatedBy: "seed",
			wantSessionID: "sess-1",
		},
		{
			name: "older event is noop",
			current: &ChatStateRecord{
				SessionID:              "sess-1",
				State:                  ChatStateEvicted,
				LastLifecycleEventID:   "evt-1",
				LastLifecycleEventTime: 100,
				UpdatedAt:              now,
				UpdatedBy:              "seed",
			},
			targetState:   ChatStateActive,
			eventID:       "evt-0",
			eventTime:     99,
			updatedBy:     "tester",
			wantChanged:   false,
			wantState:     ChatStateEvicted,
			wantEventID:   "evt-1",
			wantEventTime: 100,
			wantUpdatedBy: "seed",
			wantSessionID: "sess-1",
		},
		{
			name: "newer event flips state and preserves session",
			current: &ChatStateRecord{
				SessionID:              "sess-1",
				State:                  ChatStateEvicted,
				LastLifecycleEventID:   "evt-1",
				LastLifecycleEventTime: 100,
				UpdatedAt:              now,
				UpdatedBy:              "seed",
			},
			targetState:   ChatStateActive,
			eventID:       "evt-2",
			eventTime:     101,
			updatedBy:     "tester",
			wantChanged:   true,
			wantState:     ChatStateActive,
			wantEventID:   "evt-2",
			wantEventTime: 101,
			wantUpdatedBy: "tester",
			wantSessionID: "sess-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := planLifecycleTransition(tt.current, tt.targetState, tt.eventID, tt.eventTime, tt.updatedBy)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantChanged, changed)
			assert.Equal(t, tt.wantState, got.State)
			assert.Equal(t, tt.wantEventID, got.LastLifecycleEventID)
			assert.EqualValues(t, tt.wantEventTime, got.LastLifecycleEventTime)
			assert.Equal(t, tt.wantUpdatedBy, got.UpdatedBy)
			assert.Equal(t, tt.wantSessionID, got.SessionID)
		})
	}
}

// TestPostgresChatStateRepo_UnimplementedEntryPoints 验证 tenantKey 合法时，未实现能力必须显式返回未实现错误。
func TestPostgresChatStateRepo_UnimplementedEntryPoints(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "ListActive",
			run: func() error {
				_, err := repo.ListActive(ctx, "feishu", "tenant-1")
				return err
			},
		},
		{
			name: "Upsert",
			run: func() error {
				return repo.Upsert(ctx, ChatStateRecord{
					Platform:  "feishu",
					TenantKey: "tenant-1",
					ChatID:    "chat-1",
				})
			},
		},
		{
			name: "SetSessionID",
			run: func() error {
				return repo.SetSessionID(ctx, "feishu", "tenant-1", "chat-1", "sess-1", "tester")
			},
		},
		{
			name: "SetMuteUntil",
			run: func() error {
				return repo.SetMuteUntil(ctx, "feishu", "tenant-1", "chat-1", &now, "tester")
			},
		},
		{
			name: "SetRolloutMode",
			run: func() error {
				return repo.SetRolloutMode(ctx, "feishu", "tenant-1", "chat-1", RolloutModeAllow, "tester")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			assert.ErrorIs(t, err, ErrChatStateRepoNotImplemented)
		})
	}
}

func TestNewReliabilityLeaderGateFromChatStateRepo(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)

	gate := NewReliabilityLeaderGateFromChatStateRepo(repo, nil)

	require.NotNil(t, gate)
}

func TestPostgresReliabilityLeaderGate_WithoutPoolFailsClosed(t *testing.T) {
	gate := NewPostgresReliabilityLeaderGate(nil, nil)

	acquired, err := gate.TryAcquire(context.Background())

	assert.False(t, acquired)
	assert.ErrorIs(t, err, ErrChatStateRepoNotImplemented)
}

func TestPostgresChatStateRepo_GetWithoutPoolReturnsNotImplemented(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)

	record, err := repo.Get(context.Background(), "feishu", "tenant-1", "chat-1")

	assert.Nil(t, record)
	assert.ErrorIs(t, err, ErrChatStateRepoNotImplemented)
}

func TestPostgresChatStateRepo_ListActiveWithoutPoolReturnsNotImplemented(t *testing.T) {
	repo := NewPostgresChatStateRepo(nil, nil)

	records, err := repo.ListActive(context.Background(), "feishu", "tenant-1")

	assert.Nil(t, records)
	assert.ErrorIs(t, err, ErrChatStateRepoNotImplemented)
}

func setupChatStateRepoDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 ChatStateRepo PG 集成测试")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS feishu_chat_state (
			platform VARCHAR(50) NOT NULL,
			tenant_key VARCHAR(255) NOT NULL,
			chat_id VARCHAR(255) NOT NULL,
			session_id VARCHAR(255) NOT NULL DEFAULT '',
			model_override VARCHAR(255) NOT NULL DEFAULT '',
			agent_profile VARCHAR(255) NOT NULL DEFAULT '',
			state VARCHAR(32) NOT NULL DEFAULT 'active',
			mute_until TIMESTAMP WITH TIME ZONE,
			rollout_mode VARCHAR(32) NOT NULL DEFAULT 'allow',
			suppress_outbound BOOLEAN NOT NULL DEFAULT FALSE,
			last_lifecycle_event_id VARCHAR(255) NOT NULL DEFAULT '',
			last_lifecycle_event_time BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_by VARCHAR(255) NOT NULL DEFAULT '',
			PRIMARY KEY (platform, tenant_key, chat_id)
		)
	`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM feishu_chat_state`)
	require.NoError(t, err)

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM feishu_chat_state`)
		pool.Close()
	}
	return pool, cleanup
}

// TestPostgresChatStateRepo_GetMissing 验证不存在记录时返回 nil,nil。
func TestPostgresChatStateRepo_GetMissing(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	repo := NewPostgresChatStateRepo(pool, nil)

	record, err := repo.Get(context.Background(), "feishu", "tenant-1", "chat-404")
	require.NoError(t, err)
	assert.Nil(t, record)
}

func TestPostgresChatStateRepo_ListActive_ReturnsOnlyActiveChats(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode, suppress_outbound,
			last_lifecycle_event_id, last_lifecycle_event_time, updated_by
		) VALUES
			($1, $2, $3, $4, '', '', $5, $6, $7, $8, $9, $10),
			($1, $2, $11, $12, '', '', $13, $6, $14, $15, $16, $10),
			($1, $17, $18, $19, '', '', $5, $6, $7, $8, $9, $10)
	`,
		"feishu", "tenant-1", "chat-active", "sess-a", ChatStateActive, RolloutModeAllow, false, "evt-1", int64(100), "seed",
		"chat-evicted", "sess-e", ChatStateEvicted, true, "evt-2", int64(200),
		"tenant-2", "chat-other", "sess-o",
	)
	require.NoError(t, err)

	repo := NewPostgresChatStateRepo(pool, nil)
	records, err := repo.ListActive(ctx, "feishu", "tenant-1")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "chat-active", records[0].ChatID)
	assert.Equal(t, ChatStateActive, records[0].State)
}

// TestPostgresChatStateRepo_MarkLifecycle_CreateAndGet 验证首次 lifecycle 会建行并可被 Get 读回。
func TestPostgresChatStateRepo_MarkLifecycle_CreateAndGet(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	repo := NewPostgresChatStateRepo(pool, nil)
	ctx := context.Background()

	record, changed, err := repo.MarkEvicted(ctx, "feishu", "tenant-1", "chat-1", "evt-1", 100, "tester")
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, record)
	assert.Equal(t, ChatStateEvicted, record.State)
	assert.Equal(t, "evt-1", record.LastLifecycleEventID)
	assert.EqualValues(t, 100, record.LastLifecycleEventTime)
	assert.Equal(t, "", record.SessionID)
	assert.Equal(t, "tester", record.UpdatedBy)

	got, err := repo.Get(ctx, "feishu", "tenant-1", "chat-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, *record, *got)
}

// TestPostgresChatStateRepo_MarkLifecycle_PreservesSessionID 验证 lifecycle 状态切换不会抹掉现有 session_id。
func TestPostgresChatStateRepo_MarkLifecycle_PreservesSessionID(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode, suppress_outbound,
			last_lifecycle_event_id, last_lifecycle_event_time, updated_by
		) VALUES ($1, $2, $3, $4, '', '', $5, $6, $7, $8, $9, $10)
	`,
		"feishu", "tenant-1", "chat-1", "sess-1", ChatStateActive, RolloutModeAllow, false, "evt-1", int64(100), "seed",
	)
	require.NoError(t, err)

	repo := NewPostgresChatStateRepo(pool, nil)

	evicted, changed, err := repo.MarkEvicted(ctx, "feishu", "tenant-1", "chat-1", "evt-2", 200, "tester")
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, evicted)
	assert.Equal(t, "sess-1", evicted.SessionID)
	assert.Equal(t, ChatStateEvicted, evicted.State)
	assert.True(t, evicted.SuppressOutbound)

	active, changed, err := repo.MarkActive(ctx, "feishu", "tenant-1", "chat-1", "evt-3", 300, "tester-2")
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, active)
	assert.Equal(t, "sess-1", active.SessionID)
	assert.Equal(t, ChatStateActive, active.State)
	assert.True(t, active.SuppressOutbound)
	assert.Equal(t, "evt-3", active.LastLifecycleEventID)
	assert.EqualValues(t, 300, active.LastLifecycleEventTime)
	assert.Equal(t, "tester-2", active.UpdatedBy)
}

func TestPostgresChatStateRepo_MutateGovernanceOverrides(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresChatStateRepo(pool, nil)

	require.NoError(t, repo.SetModelOverride(ctx, "feishu", "tenant-1", "chat-1", "gpt-5.2", "tester"))
	require.NoError(t, repo.SetAgentProfile(ctx, "feishu", "tenant-1", "chat-1", "code-assistant", "tester"))

	got, err := repo.Get(ctx, "feishu", "tenant-1", "chat-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "gpt-5.2", got.ModelOverride)
	assert.Equal(t, "code-assistant", got.AgentProfile)
	assert.Equal(t, "tester", got.UpdatedBy)
}

// TestPostgresChatStateRepo_MarkLifecycle_RejectsStaleOrDuplicate 验证旧事件、同时间事件和重复事件都不会重复生效。
func TestPostgresChatStateRepo_MarkLifecycle_RejectsStaleOrDuplicate(t *testing.T) {
	pool, cleanup := setupChatStateRepoDB(t)
	defer cleanup()

	repo := NewPostgresChatStateRepo(pool, nil)
	ctx := context.Background()

	first, changed, err := repo.MarkEvicted(ctx, "feishu", "tenant-1", "chat-1", "evt-1", 100, "tester")
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, first)

	sameEvent, changed, err := repo.MarkEvicted(ctx, "feishu", "tenant-1", "chat-1", "evt-1", 100, "tester-dup")
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, sameEvent)
	assert.Equal(t, *first, *sameEvent)

	sameTime, changed, err := repo.MarkActive(ctx, "feishu", "tenant-1", "chat-1", "evt-2", 100, "tester-same-time")
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, sameTime)
	assert.Equal(t, ChatStateEvicted, sameTime.State)
	assert.Equal(t, "evt-1", sameTime.LastLifecycleEventID)

	older, changed, err := repo.MarkActive(ctx, "feishu", "tenant-1", "chat-1", "evt-0", 99, "tester-old")
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, older)
	assert.Equal(t, ChatStateEvicted, older.State)
	assert.Equal(t, "evt-1", older.LastLifecycleEventID)
	assert.EqualValues(t, 100, older.LastLifecycleEventTime)
}
