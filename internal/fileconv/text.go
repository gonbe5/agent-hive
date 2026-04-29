package fileconv

import (
	"encoding/base64"
	"fmt"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// decodeBase64 尝试多种 base64 编码解码（标准/URL-safe，有/无 padding）
func decodeBase64(data string) ([]byte, error) {
	// 优先尝试标准编码（带 padding）
	if decoded, err := base64.StdEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	// 尝试无 padding 的标准编码
	if decoded, err := base64.RawStdEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	// 尝试 URL-safe 编码
	if decoded, err := base64.URLEncoding.DecodeString(data); err == nil {
		return decoded, nil
	}
	// 尝试无 padding 的 URL-safe 编码
	return base64.RawURLEncoding.DecodeString(data)
}

// convertText 将 base64 编码的文本文件解码并格式化输出
func convertText(filename, base64Data string) (*ConvertResult, error) {
	data, err := decodeBase64(base64Data)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, "文本数据 base64 解码失败", err)
	}

	content := string(data)
	text := fmt.Sprintf("--- %s ---\n%s", filename, content)
	return &ConvertResult{Type: "text", Text: text}, nil
}
