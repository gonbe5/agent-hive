package feishu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type ChatLifecycleState string

const (
	ChatStateActive  ChatLifecycleState = "active"
	ChatStateEvicted ChatLifecycleState = "evicted"
)

type GovernanceRolloutMode string

const (
	RolloutModeAllow GovernanceRolloutMode = "allow"
	RolloutModeDeny  GovernanceRolloutMode = "deny"
)

var ErrTenantKeyRequired = errors.New("tenant key required")
var ErrChatStateRepoNotImplemented = errors.New("chat state repo not implemented")

type ChatStateRecord struct {
	Platform               string
	TenantKey              string
	ChatID                 string
	SessionID              string
	ModelOverride          string
	AgentProfile           string
	State                  ChatLifecycleState
	MuteUntil              *time.Time
	RolloutMode            GovernanceRolloutMode
	SuppressOutbound       bool
	LastLifecycleEventID   string
	LastLifecycleEventTime int64
	UpdatedAt              time.Time
	UpdatedBy              string
}

type ChatStateRepo interface {
	Get(ctx context.Context, platform, tenantKey, chatID string) (*ChatStateRecord, error)
	ListActive(ctx context.Context, platform, tenantKey string) ([]ChatStateRecord, error)
	Upsert(ctx context.Context, record ChatStateRecord) error
	MarkEvicted(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error)
	MarkActive(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error)
	SetSessionID(ctx context.Context, platform, tenantKey, chatID, sessionID, updatedBy string) error
	SetMuteUntil(ctx context.Context, platform, tenantKey, chatID string, muteUntil *time.Time, updatedBy string) error
	SetRolloutMode(ctx context.Context, platform, tenantKey, chatID string, mode GovernanceRolloutMode, updatedBy string) error
	SetModelOverride(ctx context.Context, platform, tenantKey, chatID, modelOverride, updatedBy string) error
	SetAgentProfile(ctx context.Context, platform, tenantKey, chatID, agentProfile, updatedBy string) error
}

type PostgresChatStateRepo struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewPostgresChatStateRepo(pool *pgxpool.Pool, logger *zap.Logger) *PostgresChatStateRepo {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PostgresChatStateRepo{
		pool:   pool,
		logger: logger,
	}
}

func (r *PostgresChatStateRepo) Get(ctx context.Context, platform, tenantKey, chatID string) (*ChatStateRecord, error) {
	if err := validateTenantKey(tenantKey); err != nil {
		return nil, err
	}
	if r.pool == nil {
		return nil, ErrChatStateRepoNotImplemented
	}

	row := r.pool.QueryRow(ctx, `
		SELECT platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
		       suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
		       updated_at, updated_by
		  FROM feishu_chat_state
		 WHERE platform = $1 AND tenant_key = $2 AND chat_id = $3
	`, platform, tenantKey, chatID)

	record, err := scanChatStateRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get chat state: %w", err)
	}
	return record, nil
}

func (r *PostgresChatStateRepo) ListActive(ctx context.Context, platform, tenantKey string) ([]ChatStateRecord, error) {
	if err := validateTenantKey(tenantKey); err != nil {
		return nil, err
	}
	if r.pool == nil {
		return nil, ErrChatStateRepoNotImplemented
	}

	rows, err := r.pool.Query(ctx, `
		SELECT platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
		       suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
		       updated_at, updated_by
		  FROM feishu_chat_state
		 WHERE platform = $1 AND tenant_key = $2 AND state = $3
		 ORDER BY updated_at DESC, chat_id ASC
	`, platform, tenantKey, ChatStateActive)
	if err != nil {
		return nil, fmt.Errorf("list active chats: %w", err)
	}
	defer rows.Close()

	var records []ChatStateRecord
	for rows.Next() {
		record, err := scanChatStateRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active chat: %w", err)
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active chats: %w", err)
	}
	return records, nil
}

func (r *PostgresChatStateRepo) Upsert(ctx context.Context, record ChatStateRecord) error {
	if err := validateTenantKey(record.TenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	if record.RolloutMode == "" {
		record.RolloutMode = RolloutModeAllow
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
			suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
			updated_at, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), $13
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			session_id = EXCLUDED.session_id,
			model_override = EXCLUDED.model_override,
			agent_profile = EXCLUDED.agent_profile,
			state = EXCLUDED.state,
			mute_until = EXCLUDED.mute_until,
			rollout_mode = EXCLUDED.rollout_mode,
			suppress_outbound = EXCLUDED.suppress_outbound,
			last_lifecycle_event_id = EXCLUDED.last_lifecycle_event_id,
			last_lifecycle_event_time = EXCLUDED.last_lifecycle_event_time,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`,
		record.Platform, record.TenantKey, record.ChatID, record.SessionID, record.ModelOverride, record.AgentProfile,
		record.State, record.MuteUntil, record.RolloutMode, record.SuppressOutbound, record.LastLifecycleEventID, record.LastLifecycleEventTime, record.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("upsert chat state: %w", err)
	}
	return nil
}

func (r *PostgresChatStateRepo) MarkEvicted(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error) {
	if err := validateTenantKey(tenantKey); err != nil {
		return nil, false, err
	}
	return r.applyLifecycleEvent(ctx, platform, tenantKey, chatID, ChatStateEvicted, eventID, eventTime, updatedBy)
}

func (r *PostgresChatStateRepo) MarkActive(ctx context.Context, platform, tenantKey, chatID, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool, error) {
	if err := validateTenantKey(tenantKey); err != nil {
		return nil, false, err
	}
	return r.applyLifecycleEvent(ctx, platform, tenantKey, chatID, ChatStateActive, eventID, eventTime, updatedBy)
}

func (r *PostgresChatStateRepo) SetSessionID(ctx context.Context, platform, tenantKey, chatID, sessionID, updatedBy string) error {
	if err := validateTenantKey(tenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode,
			suppress_outbound, updated_at, updated_by
		) VALUES (
			$1, $2, $3, $4, '', '', $5, $6, FALSE, NOW(), $7
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			session_id = EXCLUDED.session_id,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`, platform, tenantKey, chatID, sessionID, ChatStateActive, RolloutModeAllow, updatedBy)
	if err != nil {
		return fmt.Errorf("set chat session id: %w", err)
	}
	return nil
}

func (r *PostgresChatStateRepo) SetMuteUntil(ctx context.Context, platform, tenantKey, chatID string, muteUntil *time.Time, updatedBy string) error {
	if err := validateTenantKey(tenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
			suppress_outbound, updated_at, updated_by
		) VALUES (
			$1, $2, $3, '', '', '', $4, $5, $6, FALSE, NOW(), $7
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			mute_until = EXCLUDED.mute_until,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`, platform, tenantKey, chatID, ChatStateActive, muteUntil, RolloutModeAllow, updatedBy)
	if err != nil {
		return fmt.Errorf("set chat mute_until: %w", err)
	}
	return nil
}

func (r *PostgresChatStateRepo) SetRolloutMode(ctx context.Context, platform, tenantKey, chatID string, mode GovernanceRolloutMode, updatedBy string) error {
	if err := validateTenantKey(tenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode,
			suppress_outbound, updated_at, updated_by
		) VALUES (
			$1, $2, $3, '', '', '', $4, $5, FALSE, NOW(), $6
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			rollout_mode = EXCLUDED.rollout_mode,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`, platform, tenantKey, chatID, ChatStateActive, mode, updatedBy)
	if err != nil {
		return fmt.Errorf("set chat rollout_mode: %w", err)
	}
	return nil
}

func (r *PostgresChatStateRepo) SetModelOverride(ctx context.Context, platform, tenantKey, chatID, modelOverride, updatedBy string) error {
	if err := validateTenantKey(tenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode,
			suppress_outbound, updated_at, updated_by
		) VALUES (
			$1, $2, $3, '', $4, '', $5, $6, FALSE, NOW(), $7
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			model_override = EXCLUDED.model_override,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`, platform, tenantKey, chatID, modelOverride, ChatStateActive, RolloutModeAllow, updatedBy)
	if err != nil {
		return fmt.Errorf("set chat model_override: %w", err)
	}
	return nil
}

func (r *PostgresChatStateRepo) SetAgentProfile(ctx context.Context, platform, tenantKey, chatID, agentProfile, updatedBy string) error {
	if err := validateTenantKey(tenantKey); err != nil {
		return err
	}
	if r.pool == nil {
		return ErrChatStateRepoNotImplemented
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feishu_chat_state (
			platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, rollout_mode,
			suppress_outbound, updated_at, updated_by
		) VALUES (
			$1, $2, $3, '', '', $4, $5, $6, FALSE, NOW(), $7
		)
		ON CONFLICT (platform, tenant_key, chat_id) DO UPDATE SET
			agent_profile = EXCLUDED.agent_profile,
			updated_at = NOW(),
			updated_by = EXCLUDED.updated_by
	`, platform, tenantKey, chatID, agentProfile, ChatStateActive, RolloutModeAllow, updatedBy)
	if err != nil {
		return fmt.Errorf("set chat agent_profile: %w", err)
	}
	return nil
}

func validateTenantKey(tenantKey string) error {
	if tenantKey == "" {
		return ErrTenantKeyRequired
	}
	return nil
}

func (r *PostgresChatStateRepo) applyLifecycleEvent(
	ctx context.Context,
	platform string,
	tenantKey string,
	chatID string,
	targetState ChatLifecycleState,
	eventID string,
	eventTime int64,
	updatedBy string,
) (*ChatStateRecord, bool, error) {
	if r.pool == nil {
		return nil, false, ErrChatStateRepoNotImplemented
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("begin lifecycle tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var current *ChatStateRecord
	row := tx.QueryRow(ctx, `
		SELECT platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
		       suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
		       updated_at, updated_by
		  FROM feishu_chat_state
		 WHERE platform = $1 AND tenant_key = $2 AND chat_id = $3
		 FOR UPDATE
	`, platform, tenantKey, chatID)

	current, err = scanChatStateRecord(row)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("lock chat state: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		current = nil
	}

	next, changed := planLifecycleTransition(current, targetState, eventID, eventTime, updatedBy)
	next.Platform = platform
	next.TenantKey = tenantKey
	next.ChatID = chatID

	if current == nil {
		if next.State == ChatStateEvicted {
			next.SuppressOutbound = true
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO feishu_chat_state (
				platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
				suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
				updated_at, updated_by
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), $13
			)
		`,
			next.Platform, next.TenantKey, next.ChatID, next.SessionID, next.ModelOverride, next.AgentProfile, next.State, next.MuteUntil,
			next.RolloutMode, next.SuppressOutbound, next.LastLifecycleEventID, next.LastLifecycleEventTime, next.UpdatedBy,
		)
	} else {
		if !changed {
			if err := tx.Commit(ctx); err != nil {
				return nil, false, fmt.Errorf("commit noop lifecycle tx: %w", err)
			}
			return next, false, nil
		}
		next.SuppressOutbound = current.SuppressOutbound
		if next.State == ChatStateEvicted {
			next.SuppressOutbound = true
		}

		_, err = tx.Exec(ctx, `
			UPDATE feishu_chat_state
			   SET state = $4,
			       suppress_outbound = $5,
			       last_lifecycle_event_id = $6,
			       last_lifecycle_event_time = $7,
			       updated_at = NOW(),
			       updated_by = $8
			 WHERE platform = $1 AND tenant_key = $2 AND chat_id = $3
		`, platform, tenantKey, chatID, next.State, next.SuppressOutbound, next.LastLifecycleEventID, next.LastLifecycleEventTime, next.UpdatedBy)
	}
	if err != nil {
		return nil, false, fmt.Errorf("persist lifecycle state: %w", err)
	}

	row = tx.QueryRow(ctx, `
		SELECT platform, tenant_key, chat_id, session_id, model_override, agent_profile, state, mute_until, rollout_mode,
		       suppress_outbound, last_lifecycle_event_id, last_lifecycle_event_time,
		       updated_at, updated_by
		  FROM feishu_chat_state
		 WHERE platform = $1 AND tenant_key = $2 AND chat_id = $3
	`, platform, tenantKey, chatID)
	persisted, err := scanChatStateRecord(row)
	if err != nil {
		return nil, false, fmt.Errorf("reload lifecycle state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit lifecycle tx: %w", err)
	}
	return persisted, changed, nil
}

func planLifecycleTransition(current *ChatStateRecord, targetState ChatLifecycleState, eventID string, eventTime int64, updatedBy string) (*ChatStateRecord, bool) {
	if current == nil {
		return &ChatStateRecord{
			SessionID:              "",
			State:                  targetState,
			RolloutMode:            RolloutModeAllow,
			SuppressOutbound:       false,
			LastLifecycleEventID:   eventID,
			LastLifecycleEventTime: eventTime,
			UpdatedBy:              updatedBy,
		}, true
	}

	next := *current
	if eventTime < current.LastLifecycleEventTime {
		return &next, false
	}
	if eventTime == current.LastLifecycleEventTime {
		return &next, false
	}
	if current.LastLifecycleEventID == eventID && current.State == targetState {
		return &next, false
	}

	next.State = targetState
	next.LastLifecycleEventID = eventID
	next.LastLifecycleEventTime = eventTime
	next.UpdatedBy = updatedBy
	return &next, true
}

type chatStateScanner interface {
	Scan(dest ...any) error
}

func scanChatStateRecord(row chatStateScanner) (*ChatStateRecord, error) {
	var record ChatStateRecord
	err := row.Scan(
		&record.Platform,
		&record.TenantKey,
		&record.ChatID,
		&record.SessionID,
		&record.ModelOverride,
		&record.AgentProfile,
		&record.State,
		&record.MuteUntil,
		&record.RolloutMode,
		&record.SuppressOutbound,
		&record.LastLifecycleEventID,
		&record.LastLifecycleEventTime,
		&record.UpdatedAt,
		&record.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &record, nil
}
