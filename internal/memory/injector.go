package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Injector 将相关记忆注入到 LLM 上下文
type Injector struct {
	store         MemoryStore
	hybrid        *HybridSearcher // 混合搜索引擎（可选）
	maxTokens     int             // 注入的最大 token 数
	topK          int             // 最大记忆条数
	minConfidence float64         // 最低注入置信度
	logger        *zap.Logger
}

// NewInjector 创建记忆注入器
func NewInjector(store MemoryStore, maxTokens, topK int, logger *zap.Logger) *Injector {
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	if topK <= 0 {
		topK = 10
	}
	return &Injector{
		store:         store,
		maxTokens:     maxTokens,
		topK:          topK,
		minConfidence: 0.5,
		logger:        logger,
	}
}

// SetHybridSearcher 设置混合搜索引擎（启用 embedding 后调用）
func (inj *Injector) SetHybridSearcher(h *HybridSearcher) {
	inj.hybrid = h
}

// SetMinConfidence 设置 memory 注入最低置信度。
func (inj *Injector) SetMinConfidence(v float64) {
	if v <= 0 || v > 1 {
		return
	}
	inj.minConfidence = v
}

// InjectContext 基于用户消息查询相关记忆，返回注入文本
// 返回空字符串表示无相关记忆
func (inj *Injector) InjectContext(ctx context.Context, userMessage string, sessionID string, userID string) (string, error) {
	result, err := inj.InjectContextDetailed(ctx, userMessage, sessionID, userID)
	return result.Text, err
}

// InjectContextDetailed 基于用户消息查询相关记忆，返回结构化注入结果。
func (inj *Injector) InjectContextDetailed(ctx context.Context, userMessage string, sessionID string, userID string) (InjectionResult, error) {
	var out InjectionResult
	if userMessage == "" {
		return out, nil
	}

	// 搜索相关记忆（优先使用混合搜索）
	var result *SearchResult

	if inj.hybrid != nil {
		// 混合搜索：FTS5 + 向量语义
		scoredIDs, err := inj.hybrid.Search(ctx, userMessage, inj.topK, userID)
		if err != nil {
			inj.logger.Warn("混合搜索失败，回退到 FTS5", zap.Error(err))
		} else if len(scoredIDs) > 0 {
			// 根据 scoredIDs 从 store 获取完整记忆
			memories := make([]MemoryRecord, 0, len(scoredIDs))
			for _, sid := range scoredIDs {
				mem, err := inj.store.Get(ctx, sid.ID)
				if err != nil {
					continue
				}
				mem.Score = sid.Score
				memories = append(memories, *mem)
			}
			result = &SearchResult{Memories: memories, Total: len(memories)}
		}
	}

	// 回退到纯 FTS5 搜索
	if result == nil || len(result.Memories) == 0 {
		var err error
		result, err = inj.store.Search(ctx, SearchOptions{
			Query:  userMessage,
			Limit:  inj.topK,
			UserID: userID,
		})
		if err != nil {
			inj.logger.Warn("搜索相关记忆失败", zap.Error(err))
			return out, err
		}
	}

	if result == nil || len(result.Memories) == 0 {
		inj.logger.Debug("无相关记忆", zap.String("query", userMessage))
		return out, nil
	}

	// 格式化为 Markdown 注入文本
	var sb strings.Builder
	sb.WriteString("## 相关记忆\n\n")
	headerTokens := estimateTokens(sb.String())
	totalTokens := headerTokens
	now := time.Now()

	for _, mem := range result.Memories {
		g := DecodeGovernance(mem.Metadata)
		if userID != "" && mem.UserID != "" && mem.UserID != userID {
			out.SkippedCrossUser++
			out.SkippedMemoryIDs = append(out.SkippedMemoryIDs, mem.ID)
			continue
		}
		if !g.ExpiresAt.IsZero() && now.After(g.ExpiresAt) {
			out.SkippedExpired++
			out.SkippedMemoryIDs = append(out.SkippedMemoryIDs, mem.ID)
			continue
		}
		if g.Confidence > 0 && g.Confidence < inj.minConfidence {
			out.SkippedLowTrust++
			out.SkippedMemoryIDs = append(out.SkippedMemoryIDs, mem.ID)
			continue
		}

		line := fmt.Sprintf("- [%s] %s\n", mem.Type, mem.Content)
		lineTokens := estimateTokens(line)

		// 检查 token 是否超限
		if totalTokens+lineTokens > inj.maxTokens {
			inj.logger.Debug("记忆注入达到 token 上限",
				zap.Int("current_tokens", totalTokens),
				zap.Int("max_tokens", inj.maxTokens),
			)
			out.SkippedTokenBudget++
			out.SkippedMemoryIDs = append(out.SkippedMemoryIDs, mem.ID)
			continue
		}

		sb.WriteString(line)
		totalTokens += lineTokens
		out.Memories = append(out.Memories, InjectedMemory{
			ID:         mem.ID,
			Type:       mem.Type,
			Score:      mem.Score,
			Confidence: g.Confidence,
			Source:     g.Source,
		})
	}

	// 只有标题没有实际内容时返回空
	if totalTokens <= headerTokens {
		return out, nil
	}

	out.Text = sb.String()
	out.EstimatedTokens = totalTokens
	inj.logger.Debug("注入相关记忆",
		zap.Int("count", len(out.Memories)),
		zap.Int("estimated_tokens", totalTokens),
	)
	return out, nil
}

// estimateTokens 粗略估算文本的 token 数（约 4 个字符 = 1 token）
func estimateTokens(text string) int {
	n := len(text) / 4
	if n == 0 && len(text) > 0 {
		n = 1
	}
	return n
}
