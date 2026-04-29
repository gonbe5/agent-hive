package airouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// VideoAdapter 视频生成服务适配器
type VideoAdapter struct {
	router *Router
	logger *zap.Logger
}

func NewVideoAdapter(router *Router, logger *zap.Logger) *VideoAdapter {
	return &VideoAdapter{router: router, logger: logger}
}

func (a *VideoAdapter) ServiceType() ServiceType { return ServiceVideoGen }

func (a *VideoAdapter) Execute(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	prompt, _ := req.Params["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("video generation requires a 'prompt' parameter")
	}
	duration, _ := req.Params["duration"].(string)
	if duration == "" {
		duration = "5s"
	}
	resolution, _ := req.Params["resolution"].(string)
	if resolution == "" {
		resolution = "1080p"
	}

	providers := a.router.GetProviders(ServiceVideoGen)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no video generation provider configured")
	}

	provider := providers[0]

	switch {
	case strings.Contains(provider.ProviderType, "jimeng"),
		strings.Contains(provider.ProviderType, "volcengine"):
		return a.executeJimeng(ctx, provider, prompt, duration, resolution)
	case strings.Contains(provider.ProviderType, "sora"),
		strings.Contains(provider.ProviderType, "openai"):
		return a.executeOpenAIVideo(ctx, provider, prompt, duration, resolution)
	default:
		return a.executeGenericVideo(ctx, provider, prompt, duration, resolution)
	}
}

func (a *VideoAdapter) executeOpenAIVideo(ctx context.Context, provider ProviderConfig, prompt, duration, resolution string) (*ServiceResponse, error) {
	endpoint := provider.BaseURL
	if endpoint == "" {
		endpoint = "https://www.gmini.xyz/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/videos/generations"

	model, _ := provider.ConfigJSON["model"].(string)
	if model == "" {
		model = "sora"
	}

	reqBody := map[string]any{
		"model":    model,
		"prompt":   prompt,
		"duration": duration,
		"size":     resolution,
		"n":        1,
	}

	return a.doHTTPVideoRequest(ctx, endpoint, provider.APIKey, reqBody, provider.Name)
}

func (a *VideoAdapter) executeJimeng(ctx context.Context, provider ProviderConfig, prompt, duration, resolution string) (*ServiceResponse, error) {
	endpoint := provider.BaseURL
	if endpoint == "" {
		endpoint = "https://jimeng.jianying.com/api/video/generate"
	}

	reqBody := map[string]any{
		"prompt":     prompt,
		"duration":   duration,
		"resolution": resolution,
		"n":          1,
	}

	return a.doHTTPVideoRequest(ctx, endpoint, provider.APIKey, reqBody, provider.Name)
}

func (a *VideoAdapter) executeGenericVideo(ctx context.Context, provider ProviderConfig, prompt, duration, resolution string) (*ServiceResponse, error) {
	if provider.BaseURL == "" {
		return nil, fmt.Errorf("generic video provider requires base_url")
	}

	reqBody := map[string]any{
		"prompt":     prompt,
		"duration":   duration,
		"resolution": resolution,
	}

	return a.doHTTPVideoRequest(ctx, provider.BaseURL, provider.APIKey, reqBody, provider.Name)
}

func (a *VideoAdapter) doHTTPVideoRequest(ctx context.Context, endpoint, apiKey string, body map[string]any, providerName string) (*ServiceResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal video request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create video request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("video API call failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read video response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("video API error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var videoResp struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBody, &videoResp); err != nil {
		return nil, fmt.Errorf("parse video response: %w", err)
	}

	url := videoResp.URL
	if url == "" && len(videoResp.Data) > 0 {
		url = videoResp.Data[0].URL
	}
	if url == "" {
		return nil, fmt.Errorf("video API returned no URL")
	}

	a.logger.Info("视频生成成功",
		zap.String("provider", providerName),
		zap.String("url", url),
	)

	return &ServiceResponse{
		URL:      url,
		MimeType: "video/mp4",
		Metadata: map[string]any{
			"provider": providerName,
		},
	}, nil
}
