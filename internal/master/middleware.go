package master

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/llm"
	"github.com/chef-guo/agents-hive/internal/mcphost"
)

type AgentState struct {
	SessionID    string
	UserID       string
	SystemPrompt string
	Messages     []llm.MessageWithTools
	Request      *llm.ChatWithToolsRequest
	Response     *llm.ChatWithToolsResponse
	Evidence     ToolEvidence
}

type ToolExecutor func(ctx context.Context, call *ToolCall) (*ToolResult, error)

type ToolCall struct {
	Name      string
	Arguments json.RawMessage
}

type ToolResult struct {
	Result *mcphost.ToolResult
}

type BeforeModelMiddleware interface {
	BeforeModel(context.Context, *AgentState) error
}

type AfterModelMiddleware interface {
	AfterModel(context.Context, *AgentState) error
}

type ToolCallMiddleware interface {
	WrapToolCall(context.Context, *ToolCall, ToolExecutor) (*ToolResult, error)
}

type Middleware interface{}

type MiddlewarePipeline struct {
	middlewares []Middleware
}

func NewMiddlewarePipeline(middlewares ...Middleware) MiddlewarePipeline {
	return MiddlewarePipeline{middlewares: middlewares}
}

func (p MiddlewarePipeline) BeforeModel(ctx context.Context, state *AgentState) error {
	for _, mw := range p.middlewares {
		if mw == nil {
			continue
		}
		if hook, ok := mw.(BeforeModelMiddleware); ok {
			if err := hook.BeforeModel(ctx, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p MiddlewarePipeline) AfterModel(ctx context.Context, state *AgentState) error {
	for _, mw := range p.middlewares {
		if mw == nil {
			continue
		}
		if hook, ok := mw.(AfterModelMiddleware); ok {
			if err := hook.AfterModel(ctx, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p MiddlewarePipeline) WrapToolCall(ctx context.Context, call *ToolCall, next ToolExecutor) (*ToolResult, error) {
	wrapped := next
	for i := len(p.middlewares) - 1; i >= 0; i-- {
		mw := p.middlewares[i]
		if mw == nil {
			continue
		}
		hook, ok := mw.(ToolCallMiddleware)
		if !ok {
			continue
		}
		current := wrapped
		wrapped = func(runCtx context.Context, runCall *ToolCall) (*ToolResult, error) {
			return hook.WrapToolCall(runCtx, runCall, current)
		}
	}
	return wrapped(ctx, call)
}

type NoopMiddleware struct{}

func (NoopMiddleware) BeforeModel(context.Context, *AgentState) error { return nil }

func (NoopMiddleware) AfterModel(context.Context, *AgentState) error { return nil }

func (NoopMiddleware) WrapToolCall(ctx context.Context, call *ToolCall, next ToolExecutor) (*ToolResult, error) {
	return next(ctx, call)
}

func buildMiddlewarePipeline(guards config.QualityGuardsConfig) MiddlewarePipeline {
	if !guards.PostValidation {
		return NewMiddlewarePipeline()
	}
	return NewMiddlewarePipeline(GroundingValidator{})
}
