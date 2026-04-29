package fileconv

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// maxConvertSize 文件转换最大大小限制（100MB）
const maxConvertSize = 100 * 1024 * 1024

// ConvertResult 文件转换结果
type ConvertResult struct {
	Type string // "text" | "image" | "file"
	Text string // Type="text" 时的提取文本
}

// WhisperFunc 音频转录回调函数类型
type WhisperFunc func(ctx context.Context, audioData []byte, filename string) (string, error)

// 代码文件扩展名集合（MIME 为 application/octet-stream 时按文本处理）
var codeExtensions = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".java": true,
	".c": true, ".cpp": true, ".cc": true, ".cxx": true, ".h": true, ".hpp": true,
	".rs": true, ".sh": true, ".bash": true, ".zsh": true,
	".rb": true, ".lua": true, ".php": true, ".swift": true,
	".kt": true, ".kts": true, ".scala": true,
	".css": true, ".html": true, ".htm": true, ".xml": true, ".json": true,
	".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".cfg": true,
	".sql": true, ".r": true, ".pl": true, ".pm": true,
	".ex": true, ".exs": true, ".erl": true, ".hs": true,
	".ml": true, ".clj": true, ".dart": true, ".v": true, ".zig": true,
	".md": true, ".txt": true, ".csv": true, ".tsv": true, ".log": true,
	".dockerfile": true, ".makefile": true, ".tf": true,
}

// Office 文档 MIME 类型
var officeMIMETypes = map[string]string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "docx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
}

// Convert 根据 MIME type 智能转换文件内容
// - image/* → Type="image" (直接传给 LLM Vision)
// - application/pdf → Type="file" (直接传给 LLM)
// - Office docs → Type="text" (提取文本)
// - text/* → Type="text" (base64解码)
// - audio/* → Type="text" (Whisper转录)
// - video/* → Type="text" (ffmpeg提取音轨→Whisper转录)
func Convert(ctx context.Context, filename, mimeType, base64Data string, whisperFn WhisperFunc) (*ConvertResult, error) {
	// 校验输入参数
	if filename == "" {
		return nil, errs.New(errs.CodeInvalidInput, "文件名不能为空")
	}
	if mimeType == "" {
		return nil, errs.New(errs.CodeInvalidInput, "MIME 类型不能为空")
	}
	if base64Data == "" {
		return nil, errs.New(errs.CodeInvalidInput, "文件数据不能为空")
	}

	// 检查文件大小限制，防止 OOM
	if len(base64Data) > maxConvertSize {
		return nil, errs.New(errs.CodeInvalidInput,
			fmt.Sprintf("文件过大: %d 字节，超过 %d 字节限制", len(base64Data), maxConvertSize))
	}

	// 图片：直接透传给 LLM Vision
	if strings.HasPrefix(mimeType, "image/") {
		return &ConvertResult{Type: "image"}, nil
	}

	// PDF：直接透传给 LLM
	if mimeType == "application/pdf" {
		return &ConvertResult{Type: "file"}, nil
	}

	// Office 文档：提取文本
	if docType, ok := officeMIMETypes[mimeType]; ok {
		return convertOffice(filename, docType, base64Data)
	}

	// 代码文件：按文本处理（MIME 为 application/octet-stream 但扩展名是代码文件）
	if mimeType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(filename))
		if codeExtensions[ext] {
			return convertText(filename, base64Data)
		}
	}

	// 文本文件
	if strings.HasPrefix(mimeType, "text/") {
		return convertText(filename, base64Data)
	}

	// 音频文件
	if strings.HasPrefix(mimeType, "audio/") {
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInvalidInput, "音频数据 base64 解码失败", err)
		}
		return convertAudio(ctx, data, filename, whisperFn)
	}

	// 视频文件
	if strings.HasPrefix(mimeType, "video/") {
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInvalidInput, "视频数据 base64 解码失败", err)
		}
		return convertVideo(ctx, data, filename, whisperFn)
	}

	return nil, errs.New(errs.CodeInvalidInput, "不支持的文件类型: "+mimeType)
}

// convertOffice 转换 Office 文档
func convertOffice(filename, docType, base64Data string) (*ConvertResult, error) {
	var text string
	var err error

	switch docType {
	case "docx":
		text, err = extractDOCX(filename, base64Data)
	case "pptx":
		text, err = extractPPTX(filename, base64Data)
	case "xlsx":
		text, err = extractXLSX(filename, base64Data)
	default:
		return nil, errs.New(errs.CodeInvalidInput, "不支持的 Office 文档类型: "+docType)
	}

	if err != nil {
		return nil, err
	}

	return &ConvertResult{Type: "text", Text: text}, nil
}
