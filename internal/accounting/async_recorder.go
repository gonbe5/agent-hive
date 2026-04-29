package accounting

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/llm"
)

const asyncRecorderBufSize = 256

// AsyncRecorder 用 channel+worker 模式包裹 CostTracker，提供背压和优雅关闭。
//
//	LLM 调用完成
//	    │
//	    └── Submit(entry)  ──►  ch (256 缓冲)  ──►  worker goroutine  ──►  CostTracker.Record
//	                                                      │
//	                                              Stop() 时 drain 后退出
//
// 调用方只需调用 Submit，不阻塞。ch 满时丢弃并记录 Warn（背压保护）。
// Stop 会等待 worker 处理完 ch 中所有已入队条目后再返回，确保 shutdown 不丢数据。
type AsyncRecorder struct {
	inner     CostTracker
	logger    *zap.Logger
	ch        chan UsageEntry
	done      chan struct{}
	closed    atomic.Bool // Stop() 后置 true，防止 Submit panic
	authEngine QuotaIncrementer // Phase 5B: 配额累加器（可选，nil 时跳过）
}

// NewAsyncRecorder 创建并启动 AsyncRecorder。
// 调用方在 shutdown 时必须调用 Stop()，确保飞行中的条目写完后再关闭 DB 连接池。
func NewAsyncRecorder(inner CostTracker, logger *zap.Logger) *AsyncRecorder {
	r := &AsyncRecorder{
		inner:  inner,
		logger: logger,
		ch:     make(chan UsageEntry, asyncRecorderBufSize),
		done:   make(chan struct{}),
	}
	go r.worker()
	return r
}

// QuotaIncrementer 配额累加接口
type QuotaIncrementer interface {
	IncrementTokenUsage(ctx context.Context, userID string, tokens int64) error
}

// SetAuthEngine 注入配额累加能力（auth 未启用时不调用）
func (r *AsyncRecorder) SetAuthEngine(engine QuotaIncrementer) {
	r.authEngine = engine
}

// Submit 将 entry 投入队列（非阻塞）。ch 满时丢弃并记录 Warn。
// Stop() 后调用安全（静默丢弃，不 panic）。
func (r *AsyncRecorder) Submit(entry UsageEntry) {
	if r.closed.Load() {
		return
	}
	select {
	case r.ch <- entry:
	default:
		r.logger.Warn("成本追踪队列已满，丢弃本次记录",
			zap.String("session_id", entry.SessionID),
			zap.String("model", entry.Model),
		)
	}
}

// RecordUsage 根据模型查询定价、计算成本并异步提交用量记录。
// 封装了 GetModelMeta → CalcCost → Submit 的重复逻辑。
// Phase 5: userID 参数用于配额累加。
func (r *AsyncRecorder) RecordUsage(sessionID, userID, model string, usage llm.Usage) {
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		return
	}
	costUSD := float64(0)
	if meta := llm.GetModelMeta(model); meta != nil {
		costUSD = CalcCost(usage.PromptTokens, usage.CompletionTokens,
			meta.CostPerInputToken, meta.CostPerOutputToken)
	}
	r.Submit(UsageEntry{
		SessionID:        sessionID,
		UserID:           userID,
		Model:            model,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CostUSD:          costUSD,
	})
}

// Stop 关闭入队通道，等待 worker 处理完所有已入队条目后返回。
// 必须在关闭 DB 连接池之前调用。
func (r *AsyncRecorder) Stop() {
	r.closed.Store(true)
	close(r.ch)
	<-r.done
}

// worker 消费 ch，逐条写入 inner CostTracker。
func (r *AsyncRecorder) worker() {
	defer close(r.done)
	for entry := range r.ch {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := r.inner.Record(ctx, entry); err != nil {
			r.logger.Warn("成本记录失败", zap.Error(err),
				zap.String("session_id", entry.SessionID),
				zap.String("model", entry.Model),
			)
		} else {
			// C1 统一方案：Record 成功后累加配额
			if entry.UserID != "" && r.authEngine != nil {
				totalTokens := entry.PromptTokens + entry.CompletionTokens
				if err := r.authEngine.IncrementTokenUsage(ctx, entry.UserID, totalTokens); err != nil {
					r.logger.Warn("配额累加失败", zap.String("user_id", entry.UserID), zap.Error(err))
				}
			}
		}
		cancel()
	}
}

// — CostTracker 接口代理（AsyncRecorder 本身也实现 CostTracker，方便替换）—

// Record 同步写入（供接口兼容，内部直接调用 inner）。
// 通常不直接调用，使用 Submit 进行异步写入。
func (r *AsyncRecorder) Record(ctx context.Context, entry UsageEntry) error {
	return r.inner.Record(ctx, entry)
}

// GetSessionCost 代理到 inner。
func (r *AsyncRecorder) GetSessionCost(ctx context.Context, sessionID string) (*CostSummary, error) {
	return r.inner.GetSessionCost(ctx, sessionID)
}

// GetTotalCost 代理到 inner。
func (r *AsyncRecorder) GetTotalCost(ctx context.Context, filter CostFilter) (*CostSummary, error) {
	return r.inner.GetTotalCost(ctx, filter)
}

// Cleanup 代理到 inner。
func (r *AsyncRecorder) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	return r.inner.Cleanup(ctx, retentionDays)
}

// GetCostByUser 代理到 inner。
func (r *AsyncRecorder) GetCostByUser(ctx context.Context) ([]UserCost, error) {
	return r.inner.GetCostByUser(ctx)
}

// 编译期接口合规检查
var _ CostTracker = (*AsyncRecorder)(nil)
