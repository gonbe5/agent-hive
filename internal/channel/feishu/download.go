package feishu

import (
	"context"
	"fmt"
)

// ResourceType 标识可下载的资源类型。
type ResourceType string

const (
	ResourceTypeImage ResourceType = "image"
	ResourceTypeFile  ResourceType = "file"
	ResourceTypeAudio ResourceType = "audio"
	ResourceTypeVideo ResourceType = "video"
	ResourceTypeMedia ResourceType = "media"
)

// DownloadRequest 是下载消息资源的请求参数。
type DownloadRequest struct {
	MessageID string       // 消息 ID（飞书 om_xxx）
	FileKey   string       // 资源 key（image_key / file_key / media_key）
	Type      ResourceType // 资源类型
}

// DownloadResult 是下载结果。
type DownloadResult struct {
	Data     []byte // 文件内容
	FileName string // 文件名（仅 file 类型有，其他类型可能为空）
}

// DownloadMessageResource 下载消息中的资源（图片/文件/音视频）。
// 这是对 Client.DownloadMessageResource 的薄封装，便于 MCP tool 调用。
func DownloadMessageResource(ctx context.Context, client *Client, req DownloadRequest) (*DownloadResult, error) {
	if req.MessageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	if req.FileKey == "" {
		return nil, fmt.Errorf("file_key is required")
	}
	if req.Type == "" {
		req.Type = ResourceTypeFile // 默认 file
	}

	data, fileName, err := client.DownloadMessageResource(ctx, req.MessageID, req.FileKey, string(req.Type))
	if err != nil {
		return nil, err
	}

	return &DownloadResult{
		Data:     data,
		FileName: fileName,
	}, nil
}
