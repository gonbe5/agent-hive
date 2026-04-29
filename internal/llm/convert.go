package llm

import (
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

// contentToSDKParts 将 ContentPart 切片转换为 openai SDK 的 content part 参数
func contentToSDKParts(parts []ContentPart) []openai.ChatCompletionContentPartUnionParam {
	result := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case ContentText:
			result = append(result, openai.TextContentPart(p.Text))

		case ContentImage:
			imgParam := openai.ChatCompletionContentPartImageImageURLParam{
				URL: p.ImageURL,
			}
			if p.Detail != "" {
				imgParam.Detail = p.Detail
			}
			result = append(result, openai.ImageContentPart(imgParam))

		case ContentAudio:
			audioParam := openai.ChatCompletionContentPartInputAudioInputAudioParam{
				Data:   p.AudioData,
				Format: p.AudioFormat,
			}
			result = append(result, openai.InputAudioContentPart(audioParam))

		case ContentFile:
			fileParam := openai.ChatCompletionContentPartFileFileParam{}
			if p.FileID != "" {
				fileParam.FileID = param.NewOpt(p.FileID)
			}
			if p.FileData != "" {
				fileParam.FileData = param.NewOpt(p.FileData)
			}
			if p.Filename != "" {
				fileParam.Filename = param.NewOpt(p.Filename)
			}
			result = append(result, openai.FileContentPart(fileParam))
		}
	}
	return result
}

// toSDKUserMessage 将 Content 转换为 openai SDK user message
// 纯文本走 openai.UserMessage(string)，多模态走 openai.UserMessage([]parts)
func toSDKUserMessage(content Content) openai.ChatCompletionMessageParamUnion {
	if !content.IsMultimodal() {
		return openai.UserMessage(content.Text())
	}
	parts := content.Parts()
	sdkParts := contentToSDKParts(parts)
	return openai.UserMessage(sdkParts)
}

// contentToSDKString 返回 Content 的纯文本表示（用于 assistant/tool 消息）
func contentToSDKString(content Content) string {
	return content.Text()
}
