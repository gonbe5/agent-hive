package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// FocusMRUCap 是 FocusMRU 列表上限（design.md D4）。
// 溢出时 evict 最老条目（list 尾）。设成 16 是经验值——
// 单个 session 同时 in-flight 超过 16 个 change 的场景极少，再高只是占内存。
const FocusMRUCap = 16

// SpecSessionStateStore 负责把 SessionSpecState 写入 hive_spec_session_state。
// 读写都走"整行 upsert"——单 session 的 ingress 是串行（session_loop.go 保证），
// 不需要 CAS。updated_at 仅用于 debug / housekeeping 清理。
type SpecSessionStateStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewSpecSessionStateStore 构造 store。
func NewSpecSessionStateStore(pool *pgxpool.Pool, logger *zap.Logger) *SpecSessionStateStore {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SpecSessionStateStore{pool: pool, logger: logger}
}

// Load 读一个 session 的 SpecState。未找到返回 zero-value + found=false。
// 注意：Changes map nil-safe——zero value 下 caller 不能直接 assign，
// 应通过 MergeChangeRef 等 helper 走。
func (s *SpecSessionStateStore) Load(ctx context.Context, sessionID string) (specdriven.SessionSpecState, bool, error) {
	var (
		active     string
		focusBytes []byte
		chgBytes   []byte
		updatedAt  time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT active_change_id, focus_mru, changes, updated_at
		FROM hive_spec_session_state WHERE session_id = $1
	`, sessionID).Scan(&active, &focusBytes, &chgBytes, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return specdriven.SessionSpecState{}, false, nil
	}
	if err != nil {
		return specdriven.SessionSpecState{}, false, err
	}

	state := specdriven.SessionSpecState{ActiveChangeID: active}
	if len(focusBytes) > 0 {
		if err := json.Unmarshal(focusBytes, &state.FocusMRU); err != nil {
			return specdriven.SessionSpecState{}, false, fmt.Errorf("decode focus_mru: %w", err)
		}
	}
	if len(chgBytes) > 0 {
		if err := json.Unmarshal(chgBytes, &state.Changes); err != nil {
			return specdriven.SessionSpecState{}, false, fmt.Errorf("decode changes: %w", err)
		}
	}
	return state, true, nil
}

// Save 整行 upsert。调用方应在锁外调用（见 session_loop.go 的 ingress hook）。
// 每次写入前应用 NormalizeFocusMRU 保证 cap。
func (s *SpecSessionStateStore) Save(ctx context.Context, sessionID string, state specdriven.SessionSpecState) error {
	state = NormalizeFocusMRU(state)
	focusBytes, err := json.Marshal(state.FocusMRU)
	if err != nil {
		return fmt.Errorf("encode focus_mru: %w", err)
	}
	if len(state.FocusMRU) == 0 {
		focusBytes = []byte("[]")
	}
	chgBytes, err := json.Marshal(state.Changes)
	if err != nil {
		return fmt.Errorf("encode changes: %w", err)
	}
	if len(state.Changes) == 0 {
		chgBytes = []byte("{}")
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO hive_spec_session_state (session_id, active_change_id, focus_mru, changes, updated_at)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, NOW())
		ON CONFLICT (session_id) DO UPDATE
		SET active_change_id = EXCLUDED.active_change_id,
		    focus_mru        = EXCLUDED.focus_mru,
		    changes          = EXCLUDED.changes,
		    updated_at       = NOW()
	`, sessionID, state.ActiveChangeID, focusBytes, chgBytes)
	return err
}

// Delete 清除某个 session 的 spec state（session 销毁时调用）。
func (s *SpecSessionStateStore) Delete(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM hive_spec_session_state WHERE session_id = $1`, sessionID)
	return err
}

// NormalizeFocusMRU 做两件事：
//  1. 保证 ActiveChangeID 在 FocusMRU 头部（最近 touch）
//  2. 去重 + 按长度裁剪到 FocusMRUCap（尾部最老，evict 掉）
//
// Changes map 不动（即使被 evict 出 focus，change 记录仍保留——UI 还可能查）。
func NormalizeFocusMRU(state specdriven.SessionSpecState) specdriven.SessionSpecState {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(state.FocusMRU)+1)

	// 头部：ActiveChangeID（若非空）
	if state.ActiveChangeID != "" {
		out = append(out, state.ActiveChangeID)
		seen[state.ActiveChangeID] = struct{}{}
	}
	// 其余：按原顺序追加，跳过重复
	for _, id := range state.FocusMRU {
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		out = append(out, id)
		seen[id] = struct{}{}
		if len(out) >= FocusMRUCap {
			break
		}
	}
	state.FocusMRU = out
	return state
}

// TouchChange 是 ingress 路径专用 helper：
//  1. ActiveChangeID 置为 id
//  2. FocusMRU 把 id 推到头（并去重 / 裁剪）
//  3. Changes[id] = ref（覆盖，调用方组装好最新 ChangeRef）
//
// 任何 SpecState 修改都应走这个函数，避免手搓 map 遗漏 Normalize。
func TouchChange(state specdriven.SessionSpecState, id string, ref specdriven.ChangeRef) specdriven.SessionSpecState {
	if id == "" {
		return state
	}
	if state.Changes == nil {
		state.Changes = map[string]specdriven.ChangeRef{}
	}
	state.Changes[id] = ref
	state.ActiveChangeID = id
	return NormalizeFocusMRU(state)
}
