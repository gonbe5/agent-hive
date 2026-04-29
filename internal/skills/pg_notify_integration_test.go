package skills_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
)

// pgSkillsSchemaDDL 复刻 postgres_migrate.go 中 hive_skills 建表 + pgAddSkillsUserID 迁移产物。
// 测试里直接复用 DDL 字符串；若 prod 迁移演进，本文件必须同步。
const pgSkillsSchemaDDL = `
DROP TABLE IF EXISTS hive_skills CASCADE;
DROP FUNCTION IF EXISTS hive_skills_notify() CASCADE;

CREATE TABLE hive_skills (
    name        TEXT NOT NULL,
    user_id     TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL,
    level       TEXT NOT NULL DEFAULT 'user',
    path        TEXT,
    revision    INTEGER NOT NULL DEFAULT 1,
    updated_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (name, user_id)
);

CREATE OR REPLACE FUNCTION hive_skills_notify() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify(
        'hive_skill_changed',
        json_build_object(
            'name',    COALESCE(NEW.name, OLD.name),
            'user_id', COALESCE(NEW.user_id, OLD.user_id, ''),
            'op',      TG_OP
        )::text
    );
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS hive_skills_notify_trigger ON hive_skills;
CREATE TRIGGER hive_skills_notify_trigger
    AFTER INSERT OR UPDATE OR DELETE ON hive_skills
    FOR EACH ROW EXECUTE FUNCTION hive_skills_notify();
`

// setupPGForSkillNotify 连接 TEST_DATABASE_URL，重建 hive_skills 表 + 触发器。
func setupPGForSkillNotify(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过 pg_notify 复合 key 集成测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, pgSkillsSchemaDDL)
	require.NoError(t, err, "重建 hive_skills schema 失败")

	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS hive_skills CASCADE`)
		pool.Close()
	}
	return pool, cleanup
}

// TestPGNotify_CompositeKeyPreventsCrossTenantOverwrite — MAJOR 2 跨租户隔离
// 模拟两个 hive 实例：
//   - pool A（"writer"）：模拟另一台机器推 personal skill
//   - pool B（"listener"）：SkillService.Start() 监听 hive_skill_changed
//
// 两用户（alice, bob）并发推同名 personal skill "hello"：
//   - 老版本（PK=name, payload=raw name）：最后一条 NOTIFY 覆盖 dbCache[hello]
//   - 新版本（PK=(name,user_id), payload=JSON {name,user_id}）：两个条目独立共存
//
// 断言：dbCache 里 dbCacheKey{hello,alice} 和 {hello,bob} 同时存在且内容不互相覆盖。
func TestPGNotify_CompositeKeyPreventsCrossTenantOverwrite(t *testing.T) {
	writerPool, cleanup := setupPGForSkillNotify(t)
	defer cleanup()

	// Listener 端：独立 pool + SkillStore + OverlayRegistry + SkillService
	dsn := os.Getenv("TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listenerPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer listenerPool.Close()

	logger := zap.NewNop()
	listenerStore := store.NewSkillStore(listenerPool, logger)
	overlay := skills.NewOverlayRegistry(logger)
	svc := skills.NewSkillService(listenerStore, overlay, logger)

	// LoadAll 初始为空表
	require.NoError(t, svc.LoadAll(ctx))

	// 启动 LISTEN goroutine
	listenerCtx, stopListener := context.WithCancel(ctx)
	// MAJOR 4 契约：先 stopListener 让 SkillService LISTEN goroutine 收到 ctx.Done 退出，
	// 再 goleak.VerifyNone 断言无残留；Close pool 放在 defer 外层保证顺序。
	defer func() {
		stopListener()
		// 给 LISTEN goroutine 一个机会 gracefully 退出再校验
		time.Sleep(200 * time.Millisecond)
		goleak.VerifyNone(t,
			// pgx 连接池内部可能有 keepalive goroutine，不属于本 change 管辖
			goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
			goleak.IgnoreAnyFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).triggerHealthCheck.func1"),
		)
	}()
	svc.Start(listenerCtx)

	// 给 LISTEN 一点时间进入 WaitForNotification 状态
	time.Sleep(300 * time.Millisecond)

	// Writer 端：直接 SkillStore.Upsert 两条同名 personal skill，
	// 不设 invalidate callback（模拟跨实例：本实例不直接调 reload，纯靠 pg_notify）
	writerStore := store.NewSkillStore(writerPool, logger)

	aliceBody := "---\nname: hello\ndescription: alice version\n---\nalice body\n"
	bobBody := "---\nname: hello\ndescription: bob version\n---\nbob body\n"

	require.NoError(t, writerStore.Upsert(ctx, "hello", "alice", aliceBody, "user", "", "alice", 0))
	require.NoError(t, writerStore.Upsert(ctx, "hello", "bob", bobBody, "user", "", "bob", 0))

	// 等待 listener 端 OverlayRegistry 看到两条记录（pg_notify 异步）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		aliceSkill, errA := overlay.Get("hello", "alice")
		bobSkill, errB := overlay.Get("hello", "bob")
		if errA == nil && errB == nil && aliceSkill != nil && bobSkill != nil {
			// 都命中：验证内容独立
			if strings.Contains(aliceSkill.Content, "alice body") &&
				strings.Contains(bobSkill.Content, "bob body") {
				// 再次断言：跨 userID 查询不互相覆盖
				require.Equal(t, "alice", aliceSkill.Metadata.UserID, "alice skill UserID mismatch")
				require.Equal(t, "bob", bobSkill.Metadata.UserID, "bob skill UserID mismatch")
				require.Equal(t, skills.ScopePersonal, aliceSkill.Metadata.Scope)
				require.Equal(t, skills.ScopePersonal, bobSkill.Metadata.Scope)
				// public 层无此 skill，Get(name, "") 层 3 回落 4 会触发 Registry.Get(name)→not found
				if _, err := overlay.Get("hello", ""); err == nil {
					t.Error("public 层不应被 personal skill 污染")
				}
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// 超时诊断
	aliceSkill, errA := overlay.Get("hello", "alice")
	bobSkill, errB := overlay.Get("hello", "bob")
	t.Fatalf("pg_notify 复合 key 未生效：\n  alice: skill=%v err=%v\n  bob:   skill=%v err=%v",
		fmt.Sprintf("%+v", aliceSkill), errA,
		fmt.Sprintf("%+v", bobSkill), errB)
}

// TestPGNotify_DeleteOneTenantDoesNotAffectOther — 删除 alice 的 hello 后，bob 的 hello 仍然存在
func TestPGNotify_DeleteOneTenantDoesNotAffectOther(t *testing.T) {
	writerPool, cleanup := setupPGForSkillNotify(t)
	defer cleanup()

	dsn := os.Getenv("TEST_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	listenerPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer listenerPool.Close()

	logger := zap.NewNop()
	listenerStore := store.NewSkillStore(listenerPool, logger)
	overlay := skills.NewOverlayRegistry(logger)
	svc := skills.NewSkillService(listenerStore, overlay, logger)
	require.NoError(t, svc.LoadAll(ctx))

	listenerCtx, stopListener := context.WithCancel(ctx)
	defer func() {
		stopListener()
		time.Sleep(200 * time.Millisecond)
		goleak.VerifyNone(t,
			goleak.IgnoreTopFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).backgroundHealthCheck"),
			goleak.IgnoreAnyFunction("github.com/jackc/pgx/v5/pgxpool.(*Pool).triggerHealthCheck.func1"),
		)
	}()
	svc.Start(listenerCtx)
	time.Sleep(300 * time.Millisecond)

	writerStore := store.NewSkillStore(writerPool, logger)
	body := func(u string) string {
		return fmt.Sprintf("---\nname: hello\ndescription: %s version\n---\n%s body\n", u, u)
	}
	require.NoError(t, writerStore.Upsert(ctx, "hello", "alice", body("alice"), "user", "", "alice", 0))
	require.NoError(t, writerStore.Upsert(ctx, "hello", "bob", body("bob"), "user", "", "bob", 0))

	// 等两条都就位
	waitForOverlay := func(userID string, wantSubstr string) {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s, err := overlay.Get("hello", userID)
			if err == nil && strings.Contains(s.Content, wantSubstr) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("timeout waiting for overlay.Get(hello, %s) to contain %q", userID, wantSubstr)
	}
	waitForOverlay("alice", "alice body")
	waitForOverlay("bob", "bob body")

	// 删除 alice
	require.NoError(t, writerStore.Delete(ctx, "hello", "alice"))

	// 等 alice 从 overlay 消失
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := overlay.Get("hello", "alice"); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := overlay.Get("hello", "alice"); err == nil {
		t.Error("alice hello 应该已从 overlay 删除")
	}
	// bob 不应受影响
	bobSkill, err := overlay.Get("hello", "bob")
	require.NoError(t, err, "bob 的 hello 必须保留")
	require.Contains(t, bobSkill.Content, "bob body")
}
