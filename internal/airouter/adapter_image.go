package airouter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// reDataURI 匹配 text 中内嵌的 data URI 图片（Gemini 2.5 代理可能把图片放在 markdown 里）
var reDataURI = regexp.MustCompile(`data:image/[^;]+;base64,[A-Za-z0-9+/\-_]+=*`)

// ImageAdapter 图片生成服务适配器
type ImageAdapter struct {
	router *Router
	logger *zap.Logger
}

// NewImageAdapter creates a new image generation adapter
func NewImageAdapter(router *Router, logger *zap.Logger) *ImageAdapter {
	return &ImageAdapter{router: router, logger: logger}
}

func (a *ImageAdapter) ServiceType() ServiceType { return ServiceImageGen }

func (a *ImageAdapter) Execute(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	prompt, _ := req.Params["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("image generation requires a 'prompt' parameter")
	}
	size, _ := req.Params["size"].(string)
	if size == "" {
		size = "1024x1024"
	}
	quality, _ := req.Params["quality"].(string)
	if quality == "" {
		quality = "auto"
	}
	style, _ := req.Params["style"].(string)

	// Get image_gen providers from router
	providers := a.router.GetProviders(ServiceImageGen)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no image generation provider configured")
	}

	provider := providers[0] // Use first available

	// Determine which API to use based on provider type
	switch {
	case strings.Contains(provider.ProviderType, "jimeng"),
		strings.Contains(provider.ProviderType, "volcengine"):
		return a.executeJimeng(ctx, provider, prompt, size, quality)
	case strings.Contains(provider.ProviderType, "google"),
		strings.Contains(provider.ProviderType, "gemini"):
		return a.executeGemini(ctx, provider, prompt, size, quality)
	default:
		// Default to OpenAI-compatible API (DALL-E)
		return a.executeOpenAI(ctx, provider, prompt, size, quality, style)
	}
}

// executeOpenAI calls OpenAI DALL-E API
func (a *ImageAdapter) executeOpenAI(ctx context.Context, provider ProviderConfig, prompt, size, quality, style string) (*ServiceResponse, error) {
	opts := []option.RequestOption{
		option.WithAPIKey(provider.APIKey),
	}
	if provider.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(provider.BaseURL))
	}

	client := openai.NewClient(opts...)

	// Build model param
	model, _ := provider.ConfigJSON["model"].(string)
	if model == "" {
		model = "dall-e-3"
	}

	params := openai.ImageGenerateParams{
		Prompt: prompt,
		Model:  openai.ImageModel(model),
		N:      openai.Int(1),
		Size:   openai.ImageGenerateParamsSize(size),
	}

	// DALL-E 3 specific params
	if strings.Contains(model, "dall-e-3") {
		params.Quality = openai.ImageGenerateParamsQuality(quality)
		if style != "" {
			params.Style = openai.ImageGenerateParamsStyle(style)
		}
	}

	resp, err := client.Images.Generate(ctx, params)
	if err != nil {
		// 三方代理返回 HTML 而非 JSON 时，SDK 会产生 "expected destination type" 错误
		// 将其转换为用户可理解的提示
		errMsg := err.Error()
		if strings.Contains(errMsg, "text/html") || strings.Contains(errMsg, "expected destination type") {
			return nil, fmt.Errorf("API 返回了 HTML 响应而非 JSON（base_url 配置可能有误，或服务暂不可用）: %w", err)
		}
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no image generated")
	}

	img := resp.Data[0]

	result := &ServiceResponse{
		URL:      img.URL,
		MimeType: "image/png",
		Metadata: map[string]any{
			"model":          model,
			"provider":       provider.Name,
			"revised_prompt": img.RevisedPrompt,
		},
	}

	// If response contains base64 data
	if img.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(img.B64JSON)
		if err == nil {
			result.Data = data
		}
	}

	a.logger.Info("图片生成成功",
		zap.String("model", model),
		zap.String("provider", provider.Name),
		zap.String("size", size),
	)

	return result, nil
}

// executeGemini calls Google Generative Language API for image generation.
// Supports both Imagen models (via :predict) and Gemini models with image output (via :generateContent).
// base_url defaults to https://generativelanguage.googleapis.com; override for third-party proxies.
func (a *ImageAdapter) executeGemini(ctx context.Context, provider ProviderConfig, prompt, size, quality string) (*ServiceResponse, error) {
	baseURL := provider.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")


	// 优先使用 llm_models.model（通过 ProviderConfig.Model 注入），fallback 到 config_json["model"]，最后才硬编码默认值
	model := provider.Model
	if model == "" {
		model, _ = provider.ConfigJSON["model"].(string)
	}
	if model == "" {
		model = "imagen-3.0-generate-002"
	}

	// Imagen models use :predict endpoint; Gemini models use :generateContent
	if strings.HasPrefix(model, "imagen") {
		return a.executeGeminiImagen(ctx, provider, baseURL, model, prompt, size, quality)
	}
	return a.executeGeminiGenerateContent(ctx, provider, baseURL, model, prompt)
}

// executeGeminiImagen calls the Imagen predict endpoint.
func (a *ImageAdapter) executeGeminiImagen(ctx context.Context, provider ProviderConfig, baseURL, model, prompt, size, quality string) (*ServiceResponse, error) {
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:predict", baseURL, model)

	// Map WxH size string to aspectRatio
	aspectRatio := "1:1"
	switch size {
	case "1792x1024":
		aspectRatio = "16:9"
	case "1024x1792":
		aspectRatio = "9:16"
	case "1024x1024":
		aspectRatio = "1:1"
	}

	reqBody := map[string]any{
		"instances": []map[string]any{
			{"prompt": prompt},
		},
		"parameters": map[string]any{
			"sampleCount":      1,
			"aspectRatio":      aspectRatio,
			"imageOutputStyle": quality, // maps "auto"/"standard"/"hd" to Imagen output quality hint
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini imagen request: %w", err)
	}

	// Google 原生 API 密钥（AIzaSy...）用 ?key= query param；代理 sk- 格式用 Bearer header
	reqURL := endpoint
	if provider.APIKey != "" {
		if strings.HasPrefix(provider.APIKey, "AIza") {
			reqURL = fmt.Sprintf("%s?key=%s", endpoint, provider.APIKey)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create gemini imagen request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" && !strings.HasPrefix(provider.APIKey, "AIza") {
		auth := provider.APIKey
		if !strings.HasPrefix(auth, "Bearer ") {
			auth = "Bearer " + auth
		}
		httpReq.Header.Set("Authorization", auth)
	}

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini imagen API call failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read gemini imagen response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini imagen API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var result struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		} `json:"predictions"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse gemini imagen response: %w", err)
	}
	if len(result.Predictions) == 0 {
		return nil, fmt.Errorf("gemini imagen returned no images")
	}

	pred := result.Predictions[0]
	mimeType := pred.MimeType
	if mimeType == "" {
		mimeType = "image/png"
	}
	imgData, err := base64.StdEncoding.DecodeString(pred.BytesBase64Encoded)
	if err != nil {
		return nil, fmt.Errorf("decode gemini imagen base64: %w", err)
	}

	a.logger.Info("Gemini Imagen 图片生成成功", zap.String("model", model), zap.String("provider", provider.Name))
	return &ServiceResponse{
		Data:     imgData,
		MimeType: mimeType,
		Metadata: map[string]any{"model": model, "provider": provider.Name},
	}, nil
}

// executeGeminiGenerateContent calls generateContent for Gemini models that support image output.
func (a *ImageAdapter) executeGeminiGenerateContent(ctx context.Context, provider ProviderConfig, baseURL, model, prompt string) (*ServiceResponse, error) {
	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, model)

	reqBody := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": prompt}}},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini generateContent request: %w", err)
	}

	reqURL := endpoint
	if provider.APIKey != "" && strings.HasPrefix(provider.APIKey, "AIza") {
		reqURL = fmt.Sprintf("%s?key=%s", endpoint, provider.APIKey)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create gemini generateContent request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" && !strings.HasPrefix(provider.APIKey, "AIza") {
		auth := provider.APIKey
		if !strings.HasPrefix(auth, "Bearer ") {
			auth = "Bearer " + auth
		}
		httpReq.Header.Set("Authorization", auth)
	}

	var respBody []byte
	var httpResp *http.Response
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*2) * time.Second):
			}
			// 重建请求（body 已被读取）
			httpReq, err = http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(bodyBytes)))
			if err != nil {
				return nil, fmt.Errorf("create gemini generateContent request: %w", err)
			}
			httpReq.Header.Set("Content-Type", "application/json")
			if provider.APIKey != "" && !strings.HasPrefix(provider.APIKey, "AIza") {
				auth := provider.APIKey
				if !strings.HasPrefix(auth, "Bearer ") {
					auth = "Bearer " + auth
				}
				httpReq.Header.Set("Authorization", auth)
			}
		}
		var doErr error
		httpResp, doErr = http.DefaultClient.Do(httpReq)
		if doErr != nil {
			if attempt < maxRetries-1 {
				a.logger.Warn("gemini generateContent 请求失败，重试", zap.Error(doErr), zap.Int("attempt", attempt+1))
				continue
			}
			return nil, fmt.Errorf("gemini generateContent API call failed: %w", doErr)
		}
		respBody, err = io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read gemini generateContent response: %w", err)
		}
		if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= 500 {
			if attempt < maxRetries-1 {
				a.logger.Warn("gemini generateContent 上游错误，重试", zap.Int("status", httpResp.StatusCode), zap.Int("attempt", attempt+1))
				continue
			}
			return nil, fmt.Errorf("gemini generateContent API error (status %d): %s", httpResp.StatusCode, string(respBody))
		}
		if httpResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("gemini generateContent API error (status %d): %s", httpResp.StatusCode, string(respBody))
		}
		// 检查是否为负载过高的 200 响应（部分代理用 200 返回错误）
		if strings.Contains(string(respBody), "负载") || strings.Contains(string(respBody), "upstream_error") {
			if attempt < maxRetries-1 {
				a.logger.Warn("gemini generateContent 上游负载，重试", zap.String("resp", string(respBody)[:min(len(respBody), 100)]), zap.Int("attempt", attempt+1))
				continue
			}
			return nil, fmt.Errorf("gemini generateContent API error: %s", string(respBody))
		}
		break
	}
	if httpResp == nil {
		return nil, fmt.Errorf("gemini generateContent: no response after retries")
	}

	// gcPart 同时兼容 camelCase（REST API）和 snake_case（部分 SDK/代理格式）
	type gcInlineData struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	type gcPart struct {
		Text           string        `json:"text"`
		InlineData     *gcInlineData `json:"inlineData"`     // REST API camelCase
		InlineDataSnake *gcInlineData `json:"inline_data"`   // Proto JSON snake_case
	}
	var gcResp struct {
		Candidates []struct {
			Content struct {
				Parts []gcPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &gcResp); err != nil {
		return nil, fmt.Errorf("parse gemini generateContent response: %w", err)
	}

	for _, cand := range gcResp.Candidates {
		for _, part := range cand.Content.Parts {
			// 优先：inlineData 字段（兼容 camelCase 和 snake_case）
			inlineData := part.InlineData
			if inlineData == nil {
				inlineData = part.InlineDataSnake
			}
			if inlineData != nil && strings.HasPrefix(inlineData.MimeType, "image/") {
				imgData, err := base64.StdEncoding.DecodeString(inlineData.Data)
				if err != nil {
					return nil, fmt.Errorf("decode gemini generateContent base64: %w", err)
				}
				a.logger.Info("Gemini generateContent 图片生成成功(inlineData)", zap.String("model", model))
				return &ServiceResponse{
					Data:     imgData,
					MimeType: inlineData.MimeType,
					Metadata: map[string]any{"model": model, "provider": provider.Name},
				}, nil
			}
			// 兜底：text 字段中内嵌的 data URI（Gemini 2.5 代理返回 markdown 图片格式）
			if part.Text != "" {
				if uri := reDataURI.FindString(part.Text); uri != "" {
					mimeType := "image/png"
					if idx := strings.Index(uri, ";"); idx > 5 {
						mimeType = uri[5:idx] // "data:" 之后到 ";" 之前
					}
					// 从 data URI 中提取纯 base64 部分（逗号后）
					b64Part := uri
					if commaIdx := strings.Index(uri, ","); commaIdx >= 0 {
						b64Part = uri[commaIdx+1:]
					}
					imgData, err := base64.StdEncoding.DecodeString(b64Part)
					if err != nil {
						return nil, fmt.Errorf("decode gemini generateContent text data URI base64: %w", err)
					}
					a.logger.Info("Gemini generateContent 图片生成成功(text data URI)", zap.String("model", model))
					return &ServiceResponse{
						Data:     imgData,
						MimeType: mimeType,
						Metadata: map[string]any{"model": model, "provider": provider.Name},
					}, nil
				}
			}
		}
	}

	// 没找到图片：打印原始响应辅助排查
	a.logger.Warn("gemini generateContent 未找到图片，原始响应",
		zap.String("model", model),
		zap.String("response", string(respBody)),
	)
	return nil, fmt.Errorf("gemini generateContent returned no image in response")
}

// executeJimeng calls ByteDance Jimeng API for image generation
func (a *ImageAdapter) executeJimeng(ctx context.Context, provider ProviderConfig, prompt, size, quality string) (*ServiceResponse, error) {
	// Jimeng API endpoint
	endpoint := provider.BaseURL
	if endpoint == "" {
		endpoint = "https://jimeng.jianying.com/api/image/generate"
	}

	reqBody := map[string]any{
		"prompt": prompt,
		"size":   size,
		"n":      1,
	}
	if quality != "" {
		reqBody["quality"] = quality
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal jimeng request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create jimeng request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jimeng API call failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read jimeng response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jimeng API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	// Parse response
	var jimengResp struct {
		Data struct {
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &jimengResp); err != nil {
		return nil, fmt.Errorf("parse jimeng response: %w", err)
	}

	if len(jimengResp.Data.Images) == 0 {
		return nil, fmt.Errorf("jimeng returned no images")
	}

	return &ServiceResponse{
		URL:      jimengResp.Data.Images[0].URL,
		MimeType: "image/png",
		Metadata: map[string]any{
			"provider": provider.Name,
			"size":     size,
		},
	}, nil
}

