package compaction

import (
	"context"

	"github.com/chef-guo/agents-hive/internal/llm"
)

// Compactor 压缩器接口，每个实现负责一种压缩策略
type Compactor interface {
	// Compact 对消息列表执行压缩，budget 为目标 token 上限
	Compact(ctx context.Context, messages []llm.MessageWithTools, budget int) ([]llm.MessageWithTools, error)
	// Name 返回压缩器名称（用于日志和配置）
	Name() string
}

// Pipeline 可插拔压缩管线，按顺序串联多个 Compactor
type Pipeline struct {
	stages []Compactor
}

// NewPipeline 根据阶段名称列表构建管线
// registry 为名称→Compactor 的映射，names 中找不到的名称会被记录到 skipped 返回值
func NewPipeline(registry map[string]Compactor, names []string) (pipeline *Pipeline, skipped []string) {
	var stages []Compactor
	for _, name := range names {
		if c, ok := registry[name]; ok {
			stages = append(stages, c)
		} else {
			skipped = append(skipped, name)
		}
	}
	return &Pipeline{stages: stages}, skipped
}

// Compact 依次执行所有阶段
func (p *Pipeline) Compact(ctx context.Context, messages []llm.MessageWithTools, budget int) ([]llm.MessageWithTools, error) {
	if p == nil || len(p.stages) == 0 {
		return messages, nil
	}
	for _, stage := range p.stages {
		var err error
		messages, err = stage.Compact(ctx, messages, budget)
		if err != nil {
			return messages, err
		}
	}
	return messages, nil
}

// StageNames 返回管线中所有阶段的名称
func (p *Pipeline) StageNames() []string {
	if p == nil {
		return nil
	}
	names := make([]string, len(p.stages))
	for i, s := range p.stages {
		names[i] = s.Name()
	}
	return names
}

// EstimateTokens 使用 TokenCounter（可选）或启发式估算消息 token 数
func EstimateTokens(messages []llm.MessageWithTools, tc *llm.TokenCounter, useTiktoken bool) int {
	if useTiktoken && tc != nil {
		return tc.CountMessages(messages)
	}
	return llm.EstimateMessagesTokens(messages)
}

// EstimateSingleTokens 估算单条消息 token 数
func EstimateSingleTokens(msg llm.MessageWithTools, tc *llm.TokenCounter, useTiktoken bool) int {
	if useTiktoken && tc != nil {
		return tc.CountMessage(msg)
	}
	return llm.EstimateMessageTokens(msg)
}
