package fileconv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// convertAudio 使用 Whisper 回调转录音频文件
func convertAudio(ctx context.Context, data []byte, filename string, whisperFn WhisperFunc) (*ConvertResult, error) {
	if whisperFn == nil {
		return nil, errs.New(errs.CodeInvalidInput, "音频转录需要提供 WhisperFunc 回调")
	}

	text, err := whisperFn(ctx, data, filename)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "音频转录失败", err)
	}

	result := fmt.Sprintf("--- %s [转录] ---\n%s", filename, text)
	return &ConvertResult{Type: "text", Text: result}, nil
}

// convertVideo 从视频中提取音轨并使用 Whisper 转录
func convertVideo(ctx context.Context, data []byte, filename string, whisperFn WhisperFunc) (*ConvertResult, error) {
	if whisperFn == nil {
		return nil, errs.New(errs.CodeInvalidInput, "视频转录需要提供 WhisperFunc 回调")
	}

	// 检查 ffmpeg 是否可用
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, errs.New(errs.CodeUnavailable, "视频转录需要安装 ffmpeg，请先安装: https://ffmpeg.org")
	}

	// 创建临时视频文件
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp4"
	}
	tmpFile, err := os.CreateTemp("", "fileconv-video-*"+ext)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "创建临时视频文件失败", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// 写入视频数据
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return nil, errs.Wrap(errs.CodeInternal, "写入临时视频文件失败", err)
	}
	tmpFile.Close()

	// 使用 ffmpeg 提取音频为 WAV 格式（输出到 stdout）
	cmd := exec.CommandContext(ctx, "ffmpeg", "-i", tmpPath, "-vn", "-f", "wav", "-")
	audioData, err := cmd.Output()
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "ffmpeg 提取音轨失败", err)
	}

	if len(audioData) == 0 {
		return nil, errs.New(errs.CodeInternal, "ffmpeg 提取的音频数据为空")
	}

	// 使用 Whisper 转录
	text, err := whisperFn(ctx, audioData, filename)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "视频音频转录失败", err)
	}

	result := fmt.Sprintf("--- %s [转录] ---\n%s", filename, text)
	return &ConvertResult{Type: "text", Text: result}, nil
}
