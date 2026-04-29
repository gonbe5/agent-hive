package airouter

import (
	"bytes"
	"context"
	"fmt"

	"go.uber.org/zap"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// STTAdapter 语音转文字服务适配器（OpenAI Whisper API 兼容）
type STTAdapter struct {
	router *Router
	logger *zap.Logger
}

// NewSTTAdapter 创建 STT 适配器
func NewSTTAdapter(router *Router, logger *zap.Logger) *STTAdapter {
	return &STTAdapter{router: router, logger: logger}
}

func (a *STTAdapter) ServiceType() ServiceType { return ServiceSTT }

func (a *STTAdapter) Execute(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	audioData, ok := req.Params["audio"].([]byte)
	if !ok || len(audioData) == 0 {
		return nil, fmt.Errorf("STT requires 'audio' parameter ([]byte)")
	}
	language, _ := req.Params["language"].(string)

	providers := a.router.GetProviders(ServiceSTT)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no STT provider configured")
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
		model = openai.AudioModelWhisper1
	}

	sttParams := openai.AudioTranscriptionNewParams{
		File:  bytes.NewReader(audioData),
		Model: model,
	}
	if language != "" {
		sttParams.Language = param.NewOpt(language)
	}

	resp, err := client.Audio.Transcriptions.New(ctx, sttParams)
	if err != nil {
		return nil, fmt.Errorf("STT transcription failed: %w", err)
	}

	a.logger.Info("STT 转写完成",
		zap.String("provider", provider.Name),
		zap.String("model", model),
		zap.Int("audio_bytes", len(audioData)),
	)

	return &ServiceResponse{
		Content: resp.Text,
		Metadata: map[string]any{
			"model":    model,
			"provider": provider.Name,
		},
	}, nil
}
