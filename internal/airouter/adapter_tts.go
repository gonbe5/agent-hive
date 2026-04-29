package airouter

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// TTSAdapter 文字转语音服务适配器（OpenAI Speech API 兼容）
type TTSAdapter struct {
	router *Router
	logger *zap.Logger
}

// NewTTSAdapter 创建 TTS 适配器
func NewTTSAdapter(router *Router, logger *zap.Logger) *TTSAdapter {
	return &TTSAdapter{router: router, logger: logger}
}

func (a *TTSAdapter) ServiceType() ServiceType { return ServiceTTS }

func (a *TTSAdapter) Execute(ctx context.Context, req ServiceRequest) (*ServiceResponse, error) {
	text, _ := req.Params["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("TTS requires 'text' parameter")
	}
	voice, _ := req.Params["voice"].(string)
	if voice == "" {
		voice = "alloy"
	}
	format, _ := req.Params["format"].(string)
	if format == "" {
		format = "mp3"
	}

	providers := a.router.GetProviders(ServiceTTS)
	if len(providers) == 0 {
		return nil, fmt.Errorf("no TTS provider configured")
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
		model = openai.SpeechModelTTS1
	}

	resp, err := client.Audio.Speech.New(ctx, openai.AudioSpeechNewParams{
		Input: text,
		Model: model,
		Voice: openai.AudioSpeechNewParamsVoice(voice),
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormat(format),
	})
	if err != nil {
		return nil, fmt.Errorf("TTS generation failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TTS response: %w", err)
	}

	mimeType := "audio/mpeg"
	switch format {
	case "opus":
		mimeType = "audio/opus"
	case "aac":
		mimeType = "audio/aac"
	case "flac":
		mimeType = "audio/flac"
	case "wav":
		mimeType = "audio/wav"
	case "pcm":
		mimeType = "audio/pcm"
	}

	a.logger.Info("TTS 合成完成",
		zap.String("provider", provider.Name),
		zap.String("model", model),
		zap.String("voice", voice),
		zap.Int("bytes", len(data)),
	)

	return &ServiceResponse{
		Data:     data,
		MimeType: mimeType,
		Metadata: map[string]any{
			"model":    model,
			"voice":    voice,
			"provider": provider.Name,
			"format":   format,
		},
	}, nil
}
