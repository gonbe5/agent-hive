package feishu

import (
	"context"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/observability"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

const (
	defaultRetryQueueBatchSize       = 100
	defaultRetryQueueMaxAttempts     = 5
	defaultRetryQueueTickInterval    = 30 * time.Second
	defaultRetryQueueProcessingLease = 2 * time.Minute
	defaultRetryTenantKey            = "default"
)

const claimRetryQueueItemsSQL = `
WITH picked AS (
	SELECT id
	FROM feishu_outbound_retry_queue
	WHERE next_retry_at <= $1
	  AND retry_count < $2
	  AND reason = ANY($4)
	ORDER BY next_retry_at, id
	LIMIT $3
	FOR UPDATE SKIP LOCKED
)
UPDATE feishu_outbound_retry_queue q
SET next_retry_at = $5
FROM picked
WHERE q.id = picked.id
RETURNING q.id, q.message_id, q.platform, q.tenant_key, q.chat_id, q.sender_id, q.reason, q.error_msg, q.payload, q.retry_count, q.created_at
`

type PostgresRetryQueue struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	closed bool
}

func NewPostgresRetryQueue(pool *pgxpool.Pool, logger *zap.Logger) *PostgresRetryQueue {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PostgresRetryQueue{pool: pool, logger: logger}
}

func (q *PostgresRetryQueue) Enqueue(item channel.RetryItem) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := q.pool.Exec(ctx, `
		INSERT INTO feishu_outbound_retry_queue 
		(message_id, platform, tenant_key, chat_id, sender_id, reason, error_msg, payload, next_retry_at) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, item.MessageID, item.Platform, normalizeRetryTenantKey(item.TenantKey), item.ChatID, item.SenderID, string(item.Reason), item.ErrorMsg, item.Payload, time.Now().Add(5*time.Minute))

	if err != nil {
		q.logger.Error("Failed to enqueue retry item", zap.String("msg_id", item.MessageID), zap.Error(err))
	}
	return err
}

func (q *PostgresRetryQueue) Snapshot() []channel.RetryItem {
	return nil // Not implemented for Postgres
}

func (q *PostgresRetryQueue) Len() int {
	return 0 // Not implemented for Postgres
}

func (q *PostgresRetryQueue) Stop() error {
	q.closed = true
	return nil
}

type RetryQueueWorker struct {
	pool            *pgxpool.Pool
	logger          *zap.Logger
	stopCh          chan struct{}
	tickInterval    time.Duration
	batchSize       int
	maxAttempts     int
	processingLease time.Duration
	now             func() time.Time
	handler         func(context.Context, channel.RetryItem) error
	handledReasons  []string
	noHandlerWarned bool
	metricsWriter   observability.MetricsWriter
}

func NewRetryQueueWorker(pool *pgxpool.Pool, logger *zap.Logger) *RetryQueueWorker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryQueueWorker{
		pool:            pool,
		logger:          logger,
		stopCh:          make(chan struct{}),
		tickInterval:    defaultRetryQueueTickInterval,
		batchSize:       defaultRetryQueueBatchSize,
		maxAttempts:     defaultRetryQueueMaxAttempts,
		processingLease: defaultRetryQueueProcessingLease,
		now:             time.Now,
	}
}

func (w *RetryQueueWorker) WithHandler(handler func(context.Context, channel.RetryItem) error) *RetryQueueWorker {
	if w == nil {
		return nil
	}
	w.handler = handler
	return w
}

func (w *RetryQueueWorker) WithHandledReasons(reasons ...channel.RetryReason) *RetryQueueWorker {
	if w == nil {
		return nil
	}
	w.handledReasons = w.handledReasons[:0]
	for _, reason := range reasons {
		if reason == "" {
			continue
		}
		w.handledReasons = append(w.handledReasons, string(reason))
	}
	return w
}

func (w *RetryQueueWorker) WithMetricsWriter(writer observability.MetricsWriter) *RetryQueueWorker {
	if w == nil {
		return nil
	}
	w.metricsWriter = writer
	return w
}

func (w *RetryQueueWorker) Start() {
	go func() {
		ticker := time.NewTicker(w.tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-w.stopCh:
				return
			case <-ticker.C:
				w.processQueue()
			}
		}
	}()
}

func (w *RetryQueueWorker) Stop() {
	close(w.stopCh)
}

func (w *RetryQueueWorker) processQueue() {
	if w == nil || w.pool == nil {
		return
	}
	if w.handler == nil {
		if !w.noHandlerWarned {
			w.logger.Warn("retry queue worker 未配置 handler，跳过处理")
			w.noHandlerWarned = true
		}
		return
	}
	now := w.now()
	items, err := w.claimDueItems(context.Background(), now)
	if err != nil {
		w.logger.Warn("retry queue claim 失败", zap.Error(err))
		return
	}
	for _, item := range items {
		err := w.handler(context.Background(), item.retryItem())
		if err == nil {
			if delErr := w.deleteItem(context.Background(), item.ID); delErr != nil {
				w.logger.Warn("retry queue 删除已成功重放条目失败",
					zap.Int64("id", item.ID),
					zap.String("message_id", item.MessageID),
					zap.Error(delErr))
			}
			continue
		}
		if resErr := w.rescheduleItem(context.Background(), item, err, now); resErr != nil {
			w.logger.Warn("retry queue 重排失败",
				zap.Int64("id", item.ID),
				zap.String("message_id", item.MessageID),
				zap.Error(resErr))
		}
	}
}

type retryQueueDBItem struct {
	ID         int64
	MessageID  string
	Platform   string
	TenantKey  string
	ChatID     string
	SenderID   string
	Reason     string
	ErrorMsg   string
	Payload    []byte
	RetryCount int
	CreatedAt  time.Time
}

func (i retryQueueDBItem) retryItem() channel.RetryItem {
	return channel.RetryItem{
		MessageID:  i.MessageID,
		Platform:   i.Platform,
		TenantKey:  i.TenantKey,
		ChatID:     i.ChatID,
		SenderID:   i.SenderID,
		Reason:     channel.RetryReason(i.Reason),
		ErrorMsg:   i.ErrorMsg,
		Payload:    i.Payload,
		EnqueuedAt: i.CreatedAt,
		Attempts:   i.RetryCount,
	}
}

func (w *RetryQueueWorker) claimDueItems(ctx context.Context, now time.Time) ([]retryQueueDBItem, error) {
	if len(w.handledReasons) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, claimRetryQueueItemsSQL, now, w.maxAttempts, w.batchSize, w.handledReasons, now.Add(w.processingLease))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []retryQueueDBItem
	for rows.Next() {
		var item retryQueueDBItem
		if err := rows.Scan(
			&item.ID,
			&item.MessageID,
			&item.Platform,
			&item.TenantKey,
			&item.ChatID,
			&item.SenderID,
			&item.Reason,
			&item.ErrorMsg,
			&item.Payload,
			&item.RetryCount,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (w *RetryQueueWorker) deleteItem(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := w.pool.Exec(ctx, `DELETE FROM feishu_outbound_retry_queue WHERE id = $1`, id)
	return err
}

func (w *RetryQueueWorker) rescheduleItem(ctx context.Context, item retryQueueDBItem, cause error, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	nextCount, nextRetryAt, exhausted := planRetryFailure(item.RetryCount, w.maxAttempts, now)
	_, err := w.pool.Exec(ctx, `
		UPDATE feishu_outbound_retry_queue
		SET retry_count = $2,
			error_msg = $3,
			next_retry_at = $4
		WHERE id = $1
	`, item.ID, nextCount, truncateRetryError(cause), nextRetryAt)
	if err != nil {
		return err
	}
	if exhausted {
		w.logger.Error("retry queue 条目已耗尽，进入 dead-letter 停止重试",
			zap.Int64("id", item.ID),
			zap.String("message_id", item.MessageID),
			zap.String("tenant_key", item.TenantKey),
			zap.Int("retry_count", nextCount),
			zap.Error(cause))
		w.emitDeadLetterMetric(item)
	}
	return nil
}

func (w *RetryQueueWorker) emitDeadLetterMetric(item retryQueueDBItem) {
	if w == nil || w.metricsWriter == nil {
		return
	}
	_ = w.metricsWriter.Record(context.Background(), observability.Metric{
		Name:  MetricOutboundDeadLetter,
		Value: 1,
		Labels: map[string]any{
			"reason":          item.Reason,
			"tenant_key_hash": channel.TenantKeyHashLabel(item.TenantKey),
		},
		Ts: time.Now(),
	})
}

func normalizeRetryTenantKey(tenantKey string) string {
	if strings.TrimSpace(tenantKey) == "" {
		return defaultRetryTenantKey
	}
	return tenantKey
}

func planRetryFailure(currentRetryCount, maxAttempts int, now time.Time) (nextRetryCount int, nextRetryAt time.Time, exhausted bool) {
	if maxAttempts <= 0 {
		maxAttempts = defaultRetryQueueMaxAttempts
	}
	nextRetryCount = currentRetryCount + 1
	if nextRetryCount >= maxAttempts {
		return nextRetryCount, now, true
	}
	backoffMinutes := 1 << max(0, nextRetryCount-1)
	return nextRetryCount, now.Add(time.Duration(backoffMinutes) * time.Minute), false
}

func truncateRetryError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) <= 1024 {
		return msg
	}
	return msg[:1024]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
