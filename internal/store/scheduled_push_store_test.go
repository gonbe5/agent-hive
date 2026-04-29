package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/store"
)

func setupScheduledPushDB(t *testing.T) (*store.PostgresStore, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 scheduled push PG 集成测试")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	pg, err := store.NewPostgresStore(ctx, store.PostgresConfig{DSN: dsn, MaxConns: 2}, zap.NewNop())
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM scheduled_pushes`)
	require.NoError(t, err)

	cleanup := func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM scheduled_pushes`)
		_ = pg.Close()
	}
	return pg, cleanup
}

func TestPostgresStore_ScheduledPushCRUD(t *testing.T) {
	pg, cleanup := setupScheduledPushDB(t)
	defer cleanup()
	ctx := context.Background()

	rec := &store.ScheduledPushRecord{
		ID:          "sched-it-1",
		Name:        "daily-report",
		Platform:    "feishu",
		Prompt:      "scheduled_push:task_done:chat_id=oc_sched_1:title=日报生成完成:summary=请查收",
		IntervalSec: 60,
		Enabled:     true,
		CreatedBy:   "u1",
	}
	require.NoError(t, pg.SaveScheduledPush(ctx, rec))

	got, err := pg.GetScheduledPush(ctx, rec.ID)
	require.NoError(t, err)
	require.Equal(t, rec.Name, got.Name)
	require.Equal(t, rec.Platform, got.Platform)
	require.Equal(t, rec.IntervalSec, got.IntervalSec)

	listed, err := pg.ListScheduledPushes(ctx, "feishu")
	require.NoError(t, err)
	require.Len(t, listed, 1)

	lastRun := time.Now().UTC().Truncate(time.Second)
	nextRun := lastRun.Add(time.Minute)
	require.NoError(t, pg.UpdateScheduledPushRun(ctx, rec.ID, lastRun, nextRun, ""))

	got, err = pg.GetScheduledPush(ctx, rec.ID)
	require.NoError(t, err)
	require.WithinDuration(t, lastRun, got.LastRunAt, time.Second)
	require.WithinDuration(t, nextRun, got.NextRunAt, time.Second)

	require.NoError(t, pg.DeleteScheduledPush(ctx, rec.ID))
	_, err = pg.GetScheduledPush(ctx, rec.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}
