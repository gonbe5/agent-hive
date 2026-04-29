package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chef-guo/agents-hive/internal/specdriven"
	"github.com/chef-guo/agents-hive/internal/store"
)

// ─── 纯逻辑测试（无 DB）────────────────────────────────────────────

// TestNormalizeFocusMRU_ActiveHead：Active 必须在 list 头，即使原 list 里
// Active 在尾部——Normalize 把它提前。
func TestNormalizeFocusMRU_ActiveHead(t *testing.T) {
	st := specdriven.SessionSpecState{
		ActiveChangeID: "c3",
		FocusMRU:       []string{"c1", "c2", "c3"},
	}
	out := store.NormalizeFocusMRU(st)
	assert.Equal(t, []string{"c3", "c1", "c2"}, out.FocusMRU)
}

// TestNormalizeFocusMRU_Dedupe：重复 id 只保留第一次出现（由 Active 逻辑决定位置）。
func TestNormalizeFocusMRU_Dedupe(t *testing.T) {
	st := specdriven.SessionSpecState{
		FocusMRU: []string{"c1", "c2", "c1", "c3", "c2"},
	}
	out := store.NormalizeFocusMRU(st)
	assert.Equal(t, []string{"c1", "c2", "c3"}, out.FocusMRU)
}

// TestNormalizeFocusMRU_EvictTail：超过 FocusMRUCap 时 evict 尾部（最老的）。
func TestNormalizeFocusMRU_EvictTail(t *testing.T) {
	var focus []string
	// 造 20 个 id（c00..c19），c00 最老
	for i := range 20 {
		focus = append(focus, "c"+string(rune('A'+i)))
	}
	st := specdriven.SessionSpecState{
		ActiveChangeID: "cT", // Active 一定要在头
		FocusMRU:       focus,
	}
	out := store.NormalizeFocusMRU(st)
	assert.Len(t, out.FocusMRU, store.FocusMRUCap)
	assert.Equal(t, "cT", out.FocusMRU[0], "Active 必须在头")
	assert.NotContains(t, out.FocusMRU[1:], "cS", "尾部应被 evict")
}

// TestNormalizeFocusMRU_NoActive：ActiveChangeID 空时不前插，只去重+裁剪。
func TestNormalizeFocusMRU_NoActive(t *testing.T) {
	st := specdriven.SessionSpecState{FocusMRU: []string{"a", "b", "c"}}
	out := store.NormalizeFocusMRU(st)
	assert.Equal(t, []string{"a", "b", "c"}, out.FocusMRU)
	assert.Empty(t, out.ActiveChangeID)
}

// TestTouchChange_InitChangesMap：Changes nil 时不 panic，自动初始化。
// Changes map nil 是 zero-value SessionSpecState 的默认形态——touch 必须安全。
func TestTouchChange_InitChangesMap(t *testing.T) {
	st := specdriven.SessionSpecState{}
	ref := specdriven.ChangeRef{ID: "x1", Status: "draft", LastTouched: time.Now()}
	out := store.TouchChange(st, "x1", ref)
	assert.Equal(t, "x1", out.ActiveChangeID)
	assert.Equal(t, []string{"x1"}, out.FocusMRU)
	assert.Equal(t, ref, out.Changes["x1"])
}

// TestTouchChange_EmptyIDNoop：id="" 是 noop，不污染 state。
func TestTouchChange_EmptyIDNoop(t *testing.T) {
	st := specdriven.SessionSpecState{ActiveChangeID: "c1", FocusMRU: []string{"c1"}}
	out := store.TouchChange(st, "", specdriven.ChangeRef{})
	assert.Equal(t, "c1", out.ActiveChangeID)
	assert.Equal(t, []string{"c1"}, out.FocusMRU)
}

// TestTouchChange_PromoteExisting：touch 一个已存在的 id，应前移到头部
// 而不是新增——防 Focus 列表膨胀。
func TestTouchChange_PromoteExisting(t *testing.T) {
	st := specdriven.SessionSpecState{
		ActiveChangeID: "c1",
		FocusMRU:       []string{"c1", "c2", "c3"},
		Changes: map[string]specdriven.ChangeRef{
			"c1": {ID: "c1"}, "c2": {ID: "c2"}, "c3": {ID: "c3"},
		},
	}
	out := store.TouchChange(st, "c3", specdriven.ChangeRef{ID: "c3", Status: "in_progress"})
	assert.Equal(t, "c3", out.ActiveChangeID)
	assert.Equal(t, []string{"c3", "c1", "c2"}, out.FocusMRU)
	assert.Equal(t, "in_progress", out.Changes["c3"].Status)
	assert.Len(t, out.Changes, 3, "原有的 c1/c2 不应消失")
}

// ─── PG 集成测试（TEST_DATABASE_URL 驱动）───────────────────────────────

func setupSpecSessionStateDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 SpecSessionState PG 集成测试")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS hive_spec_session_state (
			session_id TEXT PRIMARY KEY,
			active_change_id TEXT NOT NULL DEFAULT '',
			focus_mru JSONB NOT NULL DEFAULT '[]'::jsonb,
			changes JSONB NOT NULL DEFAULT '{}'::jsonb,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	require.NoError(t, err)
	_, _ = pool.Exec(ctx, `DELETE FROM hive_spec_session_state`)

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM hive_spec_session_state`)
		pool.Close()
	}
	return pool, cleanup
}

func TestSpecSessionStateStore_SaveLoadRoundTrip(t *testing.T) {
	pool, cleanup := setupSpecSessionStateDB(t)
	defer cleanup()
	s := store.NewSpecSessionStateStore(pool, nil)
	ctx := context.Background()

	state := specdriven.SessionSpecState{
		ActiveChangeID: "c1",
		FocusMRU:       []string{"c1", "c2"},
		Changes: map[string]specdriven.ChangeRef{
			"c1": {ID: "c1", Status: "draft", Title: "First", LastTouched: time.Now().UTC().Truncate(time.Second)},
			"c2": {ID: "c2", Status: "in_progress", LastTouched: time.Now().UTC().Truncate(time.Second)},
		},
	}
	require.NoError(t, s.Save(ctx, "sess-1", state))

	got, found, err := s.Load(ctx, "sess-1")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "c1", got.ActiveChangeID)
	assert.Equal(t, []string{"c1", "c2"}, got.FocusMRU)
	assert.Equal(t, "draft", got.Changes["c1"].Status)
	assert.Equal(t, "in_progress", got.Changes["c2"].Status)
}

func TestSpecSessionStateStore_LoadMissing(t *testing.T) {
	pool, cleanup := setupSpecSessionStateDB(t)
	defer cleanup()
	s := store.NewSpecSessionStateStore(pool, nil)

	got, found, err := s.Load(context.Background(), "absent")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, got.ActiveChangeID)
}

func TestSpecSessionStateStore_SaveNormalizesOnWrite(t *testing.T) {
	pool, cleanup := setupSpecSessionStateDB(t)
	defer cleanup()
	s := store.NewSpecSessionStateStore(pool, nil)
	ctx := context.Background()

	// 故意塞超 cap、乱序、重复——Save 应该内部 Normalize
	var focus []string
	for i := range 20 {
		focus = append(focus, "c"+string(rune('A'+i)))
	}
	state := specdriven.SessionSpecState{
		ActiveChangeID: "cA", // 已在 list 里
		FocusMRU:       focus,
	}
	require.NoError(t, s.Save(ctx, "sess-2", state))

	got, found, err := s.Load(ctx, "sess-2")
	require.NoError(t, err)
	require.True(t, found)
	assert.LessOrEqual(t, len(got.FocusMRU), store.FocusMRUCap)
	assert.Equal(t, "cA", got.FocusMRU[0])
}

func TestSpecSessionStateStore_Delete(t *testing.T) {
	pool, cleanup := setupSpecSessionStateDB(t)
	defer cleanup()
	s := store.NewSpecSessionStateStore(pool, nil)
	ctx := context.Background()

	require.NoError(t, s.Save(ctx, "sess-3", specdriven.SessionSpecState{ActiveChangeID: "x"}))
	require.NoError(t, s.Delete(ctx, "sess-3"))
	_, found, err := s.Load(ctx, "sess-3")
	require.NoError(t, err)
	assert.False(t, found)
}
