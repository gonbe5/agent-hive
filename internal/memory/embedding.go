package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// EmbeddingProvider 向量嵌入提供者接口
type EmbeddingProvider interface {
	// Embed 将文本列表转换为向量，返回与输入等长的向量切片
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimensions 返回向量维度
	Dimensions() int
}

// embeddingRequest OpenAI 兼容的 Embedding API 请求
type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// embeddingResponse OpenAI 兼容的 Embedding API 响应
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// OpenAIEmbedder 使用 OpenAI 兼容 API 的嵌入提供者
// 支持 OpenAI、通义千问、豆包、Moonshot 等提供 /v1/embeddings 端点的服务
type OpenAIEmbedder struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *http.Client
	logger     *zap.Logger
}

// 各 Provider 的默认 Embedding 模型和维度
var defaultEmbeddingModels = map[string]struct {
	model      string
	dimensions int
}{
	"openai":    {model: "text-embedding-3-small", dimensions: 1536},
	"qwen":      {model: "text-embedding-v3", dimensions: 1024},
	"doubao":    {model: "doubao-embedding", dimensions: 2560},
	"moonshot":  {model: "moonshot-v1-embedding", dimensions: 1024},
	"minimax":   {model: "embo-01", dimensions: 1024},
	"deepseek":  {model: "text-embedding-3-small", dimensions: 1536}, // DeepSeek 无自有 embedding，回退 OpenAI
	"anthropic": {model: "text-embedding-3-small", dimensions: 1536}, // Anthropic 无 embedding API，回退 OpenAI
	"xai":       {model: "text-embedding-3-small", dimensions: 1536}, // xAI 暂无自有 embedding，回退 OpenAI 兼容
}

// NewOpenAIEmbedder 创建 OpenAI 兼容的嵌入提供者
// provider 用于自动选择默认模型和维度
// 如果 model 为空，根据 provider 自动选择
func NewOpenAIEmbedder(baseURL, apiKey, model, provider string, logger *zap.Logger) *OpenAIEmbedder {
	dim := 1536
	if model == "" {
		if def, ok := defaultEmbeddingModels[strings.ToLower(provider)]; ok {
			model = def.model
			dim = def.dimensions
		} else {
			model = "text-embedding-3-small"
		}
	}

	// 确保 baseURL 以 /v1 结尾
	if baseURL != "" && !strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimRight(baseURL, "/") + "/v1"
	}
	if baseURL == "" {
		baseURL = "https://www.gmini.xyz/v1"
	}

	return &OpenAIEmbedder{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		dimensions: dim,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// Embed 调用 Embedding API 将文本转换为向量
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := embeddingRequest{
		Model: e.model,
		Input: texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, "序列化 embedding 请求失败", err)
	}

	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "创建 embedding 请求失败", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.CodeUnavailable, "embedding 请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, errs.New(errs.CodeUnavailable,
			fmt.Sprintf("embedding API 返回 %d: %s", resp.StatusCode, string(body)))
	}

	var result embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, "解析 embedding 响应失败", err)
	}

	// 按 index 排序组装结果
	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	// 更新实际维度
	if len(result.Data) > 0 && len(result.Data[0].Embedding) > 0 {
		e.dimensions = len(result.Data[0].Embedding)
	}

	e.logger.Debug("embedding 完成",
		zap.Int("texts", len(texts)),
		zap.Int("tokens", result.Usage.TotalTokens),
		zap.Int("dimensions", e.dimensions),
	)

	return vectors, nil
}

// Dimensions 返回向量维度
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dimensions
}
