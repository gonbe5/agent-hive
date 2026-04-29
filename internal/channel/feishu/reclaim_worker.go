package feishu

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type ReclaimWorker struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	stopCh chan struct{}
}

func NewReclaimWorker(pool *pgxpool.Pool, logger *zap.Logger) *ReclaimWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReclaimWorker{
		pool:   pool,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

func (w *ReclaimWorker) Start() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.reclaimStale()
			}
		}
	}()
}

func (w *ReclaimWorker) Stop() {
	close(w.stopCh)
}

func (w *ReclaimWorker) reclaimStale() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Clear claimed_at for claims older than 90s
	res, err := w.pool.Exec(ctx, `
		UPDATE feishu_event_dedup 
		SET claimed_at = NULL 
		WHERE processed = FALSE AND claimed_at < $1
	`, time.Now().Add(-90*time.Second))

	if err != nil {
		w.logger.Error("Failed to reclaim stale events", zap.Error(err))
		return
	}

	if rows := res.RowsAffected(); rows > 0 {
		w.logger.Info("Reclaimed stale events", zap.Int64("count", rows))
	}
}
