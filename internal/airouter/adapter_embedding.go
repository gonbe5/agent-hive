package airouter

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// EmbeddingAdapter 向量化服务适配器（OpenAI Embeddings API 兼容）
type EmbeddingAdapter struct {
	router *Router
	logger *zap.Logger
}

// NewEmbeddingAdapter 创建 Embedding 适配器
func NewEmbeddingAdapter(router *Router, logger *zap.Logger) *EmbeddingAdapter {
	return &EmbeddingAdapter{router: router, logger: logger}
}

func (a *EmbeddingAdapter) ServiceType() ServiceType { return ServiceEmbedding }

func (a *EmbeddingAdapter) Execute(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	text, _ := req.Params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("embedding requires 'text' parameter")
	}

	providers := a.router.GetProviders(ServiceEmbedding)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no embedding provider configured")
	}

	provider := providers[0]
	opts := []option.RequestOption{
		option.WithAPIKey(provider.APIKey),
	}
	if provider.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(provider.BaseURL))
	}

	client := openai.NewClient(opts...)

	model, _ := provider.ConfigJSON["model"].(string)
	if model == "" {
		model = openai.EmbeddingModelTextEmbedding3Small
	}

	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Model: model,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding generation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	embedding := resp.Data[0].Embedding

	a.logger.Info("向量化完成",
		zap.String("provider", provider.Name),
		zap.String("model", model),
		zap.Int("dimensions", len(embedding)),
	)

	return &ServiceResponse{
		Metadata: map[string]any{
			"model":      model,
			"provider":   provider.Name,
			"dimensions": len(embedding),
			"embedding":  embedding,
		},
		Usage: Usage{
			InputTokens: resp.Usage.PromptTokens,
			TotalTokens: resp.Usage.TotalTokens,
		},
	}, nil
}
