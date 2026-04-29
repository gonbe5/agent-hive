package channel

import (
	"encoding/json"
	"strings"
)

// IsRawJSON 检测字符串是否为合法的完整 JSON 对象或数组。
func IsRawJSON(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
		return false
	}
	var v interface{}
	return json.Unmarshal([]byte(trimmed), &v) == nil
}
