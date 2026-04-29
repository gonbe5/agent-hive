// internal/channel/feishu/dedup.go
package feishu

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PostgresEventClaimer struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewPostgresEventClaimer(pool *pgxpool.Pool, logger *zap.Logger) *PostgresEventClaimer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PostgresEventClaimer{pool: pool, logger: logger}
}

func (c *PostgresEventClaimer) ClaimEvent(eventID string, lease time.Duration) (master.ClaimToken, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond) // fail-closed short timeout
	defer cancel()

	var processed bool
	var claimedAt *time.Time
	err := c.pool.QueryRow(ctx, "SELECT processed, claimed_at FROM feishu_event_dedup WHERE event_id = $1", eventID).Scan(&processed, &claimedAt)
	if err == nil {
		if processed {
			return master.ClaimToken{}, false
		}
		if claimedAt != nil && time.Since(*claimedAt) < lease {
			return master.ClaimToken{}, false // Someone else is processing it
		}
	}

	tokenBytes := make([]byte, 16)
	var nonce uint64
	rand.Read(tokenBytes)
	for i := 0; i < 8; i++ {
		nonce = (nonce << 8) | uint64(tokenBytes[i])
	}

	now := time.Now()
	_, err = c.pool.Exec(ctx, `
		INSERT INTO feishu_event_dedup (event_id, claimed_at, processed)
		VALUES ($1, $2, FALSE)
		ON CONFLICT (event_id) DO UPDATE
		SET claimed_at = EXCLUDED.claimed_at
		WHERE feishu_event_dedup.processed = FALSE AND (feishu_event_dedup.claimed_at IS NULL OR feishu_event_dedup.claimed_at < $3)
	`, eventID, now, now.Add(-lease))

	if err != nil {
		c.logger.Error("Failed to claim event (fail-closed)", zap.String("event_id", eventID), zap.Error(err))
		return master.ClaimToken{}, false // Fail-closed: if DB fails, we return false so Feishu retries later
	}

	return master.ClaimToken{
		EventID:  eventID,
		IssuedAt: now,
		Nonce:    nonce,
	}, true
}

func (c *PostgresEventClaimer) CompleteEvent(token master.ClaimToken) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	res, err := c.pool.Exec(ctx, `
		UPDATE feishu_event_dedup
		SET processed = TRUE, processed_at = $1
		WHERE event_id = $2 AND processed = FALSE
	`, time.Now(), token.EventID)

	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return master.ErrClaimTokenMismatch
	}
	return nil
}

func (c *PostgresEventClaimer) State(eventID string) master.ClaimState {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var processed bool
	var claimedAt *time.Time
	err := c.pool.QueryRow(ctx, "SELECT processed, claimed_at FROM feishu_event_dedup WHERE event_id = $1", eventID).Scan(&processed, &claimedAt)
	if err != nil {
		return master.ClaimStateUnknown
	}
	if processed {
		return master.ClaimStateCompleted
	}
	if claimedAt != nil {
		return master.ClaimStateClaimed
	}
	return master.ClaimStateUnknown
}

func (c *PostgresEventClaimer) Reclaim(now time.Time) []master.ClaimToken {
	// Reclaim is handled by the dedicated ReclaimWorker via SQL,
	// so this method satisfies the interface but delegates actual work.
	return nil
}
