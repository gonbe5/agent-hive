package master

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/compaction"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/specdriven"
)

// specStatePinPrefix 标记一条消息为 spec-driven 状态锚（pin）——compaction 后不得被
// 再次 truncate/summarize 吞掉，保证 LLM 在截断上下文后仍知道 active change / task / revision。
//
// 放 session_compact.go 而不是 react_processor.go：锚点本身就是 compaction 管线的附产品，
// PreserveSpecStateOnCompact 是 compaction 管线的最后一道"preservation whitelist"
// （task 3.8 原词），跟 TruncateCompactor / LLMSummaryCompactor 同层语义——都是 Compact 的产物。
const specStatePinPrefix = "[SPEC-STATE]"

// PreserveSpecStateOnCompact 在 compaction 管线末尾注入 spec-driven 状态锚。
//
// 为什么需要：prompt_builder.prepareMessagesWithCompression 压缩后，older context 被 truncate
// 或 LLM summary 吞掉，LLM 可能丢失"当前 change_id=X, current_task_key=Y, revision=Z"的运行时
// 感知——但 SessionState.specCtx 仍然持有这些值（atomic.Pointer，不被 compact 触碰）。
// 锚点把运行时 specCtx 物化成 messages[0] 的 system marker，compaction 无法吞掉（pin 每次
// 压缩后重新注入），LLM 永远能读到最新 spec 状态。
//
// 设计契约：
//  1. session == nil 或 session.LoadSpecCtx() == nil 或 ChangeID 空 → no-op（非 spec 会话不污染）
//  2. 若 messages[0..1] 已有 [SPEC-STATE] pin（上一轮压缩残留）→ 原位替换，不重复前插
//     （否则每次压缩都 +1 条 system，100 轮后 messages 前面 100 条全是 pin）
//  3. 否则在 [0] 前插新 pin
//
// 用 LoadSpecCtx 而非 SpecState.ActiveChangeID 的原因：
//   - specCtx 是 atomic.Pointer 无锁读（P0-6 红线），调用方不需要再持 session.mu
//   - specCtx 携带 CurrentTaskKey/Revision（task-key 精度），比 SessionSpecState.ActiveChangeID
//     更全——后者仅标记 change 层，不标 task
//
// 蓝军视角（任意一条 mutation 必须杀穿本函数）：
//   - R1 去掉 ctx.ChangeID == "" 短路 → "空 ChangeID 时仍注 pin"分支产生错误 pin 内容 → 红
//   - R2 去掉 idempotent 替换分支（always prepend）→ 重复 2 次调用后有 2 条 pin → 红
//   - R3 formatSpecStatePin 去掉 current_task_key 字段 → 断言 Content 包含 "task_key" → 红
func PreserveSpecStateOnCompact(session *SessionState, messages []llm.MessageWithTools) []llm.MessageWithTools {
	if session == nil {
		return messages
	}
	ctx := session.LoadSpecCtx()
	if ctx == nil || ctx.ChangeID == "" {
		return messages
	}
	pin := llm.MessageWithTools{
		Role:    "system",
		Content: llm.NewTextContent(formatSpecStatePin(ctx)),
	}
	// 幂等：若前两条里已有 [SPEC-STATE] pin，原位替换
	// 为什么只看前 2 条：session_memory 可能在 [0] 插"[会话记忆]"，SPEC-STATE 会落到 [1]；
	// 更靠后的 pin 不是合法位置（说明被后续消息挤下去了，该新一条放顶上重新置顶）。
	for i := 0; i < len(messages) && i < 2; i++ {
		if messages[i].Role == "system" && strings.HasPrefix(messages[i].Content.Text(), specStatePinPrefix) {
			out := make([]llm.MessageWithTools, len(messages))
			copy(out, messages)
			out[i] = pin
			return out
		}
	}
	out := make([]llm.MessageWithTools, 0, len(messages)+1)
	out = append(out, pin)
	out = append(out, messages...)
	return out
}

// formatSpecStatePin 序列化 specdriven.Context → 单行 system marker。
// 格式：`[SPEC-STATE] change_id=X current_task_key=Y revision=N`
// LLM 侧解析能力不重要——重点是 change_id/current_task_key/revision 三项可 grep，
// 后续 debug/观测能直接 filter messages 拿到 spec 状态历史。
func formatSpecStatePin(ctx *specdriven.Context) string {
	return fmt.Sprintf("%s change_id=%s current_task_key=%s revision=%d",
		specStatePinPrefix, ctx.ChangeID, ctx.CurrentTaskKey, ctx.Revision)
}

const (
	// MaxTokensInContext 上下文中最多保留的 token 数（已弃用，使用配置）
	MaxTokensInContext = 8000
	// ApproxCharsPerToken 估算每个 token 的平均字符数（中英文混合）
	ApproxCharsPerToken = 4
)

// --- 工具输出裁剪常量 ---

const (
	// PruneProtectedTurns 裁剪时保护的最近对话轮数
	PruneProtectedTurns = 2
	// PruneToolOutputThreshold 工具输出超过此阈值（字节）时进行裁剪
	PruneToolOutputThreshold = 20 * 1024 // 20KB
	// PruneContextBudget 累积保护的上下文预算（字节）
	PruneContextBudget = 40 * 1024 // 40KB
)

// CompactionContext 压缩上下文，包含配置和依赖
// Deprecated: 新代码请使用 compaction.Pipeline
type CompactionContext struct {
	Config       config.CompactionConfig
	TokenCounter *llm.TokenCounter
	LLMClient    *llm.Client
	Logger       *zap.Logger
}

// CompactionStats 压缩统计信息
type CompactionStats struct {
	Original       int    // 原始消息数
	Remaining      int    // 保留消息数（包括摘要）
	Compressed     int    // 被压缩的消息数
	Strategy       string // 使用的策略
	OriginalToken  int    // 原始 token 数
	RemainingToken int    // 压缩后 token 数
	LazySkipped    bool   // 懒惰模式下是否跳过压缩
	TriggerCount   int    // 累计触发压缩次数（仅统计用）
}

// CompactionRecommendation 压缩建议（懒惰模式）
type CompactionRecommendation struct {
	ShouldCompact bool   // 是否建议压缩
	CurrentTokens int    // 当前 token 数
	Threshold     int    // 阈值
	Reason        string // 建议原因
}

// CompactMessages 压缩消息历史（向后兼容入口）。
// Deprecated: 新代码请使用 compaction.Pipeline。
func CompactMessages(messages []llm.MessageWithTools, maxTokens int) []llm.MessageWithTools {
	tc := &compaction.TruncateCompactor{UseTiktoken: false}
	result, _ := tc.Compact(context.Background(), messages, maxTokens)
	return result
}

// CompactMessagesWithContext 使用完整上下文进行压缩（向后兼容入口）。
// Deprecated: 新代码请使用 compaction.Pipeline。
func CompactMessagesWithContext(ctx context.Context, messages []llm.MessageWithTools, compactCtx CompactionContext) ([]llm.MessageWithTools, *CompactionStats) {
	if len(messages) == 0 {
		return messages, &CompactionStats{}
	}

	cfg := compactCtx.Config
	if !cfg.Enabled {
		return messages, &CompactionStats{
			Original:  len(messages),
			Remaining: len(messages),
		}
	}

	totalTokens := compaction.EstimateTokens(messages, compactCtx.TokenCounter, cfg.UseTiktoken)

	// 懒惰模式
	if cfg.LazyMode && totalTokens <= cfg.LazyThreshold {
		return messages, &CompactionStats{
			Original:       len(messages),
			Remaining:      len(messages),
			OriginalToken:  totalTokens,
			RemainingToken: totalTokens,
			LazySkipped:    true,
		}
	}

	if totalTokens <= cfg.MaxTokens {
		return messages, &CompactionStats{
			Original:       len(messages),
			Remaining:      len(messages),
			OriginalToken:  totalTokens,
			RemainingToken: totalTokens,
		}
	}

	// 构建 pipeline 并执行
	registry := map[string]compaction.Compactor{
		"truncate": &compaction.TruncateCompactor{
			TokenCounter: compactCtx.TokenCounter,
			UseTiktoken:  cfg.UseTiktoken,
		},
	}
	if cfg.Strategy == config.StrategyLLMSummary && compactCtx.LLMClient != nil {
		registry["llm_summary"] = &compaction.LLMSummaryCompactor{
			LLMClient:    compactCtx.LLMClient,
			TokenCounter: compactCtx.TokenCounter,
			UseTiktoken:  cfg.UseTiktoken,
			Timeout:      cfg.CompactTimeout,
			Logger:       compactCtx.Logger,
		}
	}

	stages := []string{"truncate"}
	actualStrategy := config.StrategyTruncate
	if cfg.Strategy == config.StrategyLLMSummary && compactCtx.LLMClient != nil {
		stages = []string{"llm_summary"}
		actualStrategy = config.StrategyLLMSummary
	}

	pipeline, _ := compaction.NewPipeline(registry, stages)
	result, err := pipeline.Compact(ctx, messages, cfg.MaxTokens)
	if err != nil {
		if compactCtx.Logger != nil {
			compactCtx.Logger.Warn("压缩管线执行失败，降级到截断", zap.Error(err))
		}
		tc := &compaction.TruncateCompactor{
			TokenCounter: compactCtx.TokenCounter,
			UseTiktoken:  cfg.UseTiktoken,
		}
		result, _ = tc.Compact(ctx, messages, cfg.MaxTokens)
		actualStrategy = config.StrategyTruncate
	}

	return result, &CompactionStats{
		Original:      len(messages),
		Remaining:     len(result),
		Compressed:    len(messages) - len(result),
		Strategy:      string(actualStrategy),
		OriginalToken: totalTokens,
	}
}

// PruneToolOutputs 在 LLM 驱动的 compaction 之前执行低成本的工具输出裁剪。
// Deprecated: 新代码请使用 compaction.ToolResultBudgetCompactor。
func PruneToolOutputs(messages []llm.MessageWithTools, protectedTurns, threshold, contextBudget int) []llm.MessageWithTools {
	c := &compaction.ToolResultBudgetCompactor{
		ProtectedTurns:  protectedTurns,
		OutputThreshold: threshold,
		ContextBudget:   contextBudget,
	}
	result, _ := c.Compact(context.Background(), messages, 0)
	return result
}

// EvaluateCompactionNeed 评估是否需要压缩（懒惰模式辅助函数）
func EvaluateCompactionNeed(messages []llm.MessageWithTools, compactCtx CompactionContext) CompactionRecommendation {
	cfg := compactCtx.Config

	if !cfg.Enabled {
		return CompactionRecommendation{
			ShouldCompact: false,
			Reason:        "压缩已禁用",
		}
	}

	totalTokens := compaction.EstimateTokens(messages, compactCtx.TokenCounter, cfg.UseTiktoken)

	threshold := cfg.LazyThreshold
	if threshold == 0 {
		threshold = cfg.MaxTokens
	}

	if totalTokens <= threshold {
		return CompactionRecommendation{
			ShouldCompact: false,
			CurrentTokens: totalTokens,
			Threshold:     threshold,
			Reason:        "未达到压缩阈值",
		}
	}

	return CompactionRecommendation{
		ShouldCompact: true,
		CurrentTokens: totalTokens,
		Threshold:     threshold,
		Reason:        "超过压缩阈值",
	}
}

// PrepareMessagesForLLM 在发送给 LLM 前准备消息（向后兼容入口）。
// Deprecated: 新代码请使用 Master.prepareMessagesWithCompression。
func PrepareMessagesForLLM(messages []llm.MessageWithTools) []llm.MessageWithTools {
	pruned := PruneToolOutputs(messages, PruneProtectedTurns, PruneToolOutputThreshold, PruneContextBudget)
	return CompactMessages(pruned, MaxTokensInContext)
}

// PrepareMessagesForLLMWithMaxTokens 带自定义 maxTokens 的版本。
// Deprecated: 新代码请使用 Master.prepareMessagesWithCompression。
func PrepareMessagesForLLMWithMaxTokens(messages []llm.MessageWithTools, maxTokens int) []llm.MessageWithTools {
	pruned := PruneToolOutputs(messages, PruneProtectedTurns, PruneToolOutputThreshold, PruneContextBudget)
	return CompactMessages(pruned, maxTokens)
}

// 以下为内部辅助函数，保留供 strconv 包使用

// formatInt 格式化整数为字符串
func formatInt(n int) string {
	return strconv.Itoa(n)
}
