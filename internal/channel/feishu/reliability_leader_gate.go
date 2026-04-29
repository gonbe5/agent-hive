package feishu

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const feishuReliabilityLeaderLockKey = "feishu_longconn_reliability_leader"

// ReliabilityLeaderGate 用于在多副本部署下为恢复编排选择单 leader。
type ReliabilityLeaderGate interface {
	TryAcquire(ctx context.Context) (bool, error)
	Close() error
}

type PostgresReliabilityLeaderGate struct {
	pool   *pgxpool.Pool
	logger *zap.Logger

	mu   sync.Mutex
	conn *pgxpool.Conn
}

func NewPostgresReliabilityLeaderGate(pool *pgxpool.Pool, logger *zap.Logger) *PostgresReliabilityLeaderGate {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PostgresReliabilityLeaderGate{
		pool:   pool,
		logger: logger,
	}
}

func NewReliabilityLeaderGateFromChatStateRepo(repo ChatStateRepo, logger *zap.Logger) ReliabilityLeaderGate {
	pgRepo, ok := repo.(*PostgresChatStateRepo)
	if !ok || pgRepo == nil {
		return nil
	}
	return NewPostgresReliabilityLeaderGate(pgRepo.pool, logger)
}

func (g *PostgresReliabilityLeaderGate) TryAcquire(ctx context.Context) (bool, error) {
	if g == nil {
		return false, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.conn != nil {
		if conn := g.conn.Conn(); conn != nil && !conn.IsClosed() {
			return true, nil
		}
		g.conn.Release()
		g.conn = nil
	}
	if g.pool == nil {
		return false, ErrChatStateRepoNotImplemented
	}

	conn, err := g.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, feishuReliabilityLeaderLockKey).Scan(&acquired); err != nil {
		conn.Release()
		return false, err
	}
	if !acquired {
		conn.Release()
		return false, nil
	}

	g.conn = conn
	return true, nil
}

func (g *PostgresReliabilityLeaderGate) Close() error {
	if g == nil {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.conn == nil {
		return nil
	}
	conn := g.conn
	g.conn = nil
	conn.Release()
	return nil
}

var _ ReliabilityLeaderGate = (*PostgresReliabilityLeaderGate)(nil)

var errReliabilityLeaderGateUnavailable = errors.New("reliability leader gate unavailable")
