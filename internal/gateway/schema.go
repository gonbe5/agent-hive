package gateway

import (
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// ValidateParams 轻量级 JSON 参数验证
// 检查必需字段是否存在
func ValidateParams(params json.RawMessage, requiredFields []string) error {
	if len(requiredFields) == 0 {
		return nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return errs.Wrap(errs.CodeInvalidArgument, "参数必须是 JSON 对象", err)
	}

	for _, field := range requiredFields {
		if _, ok := m[field]; !ok {
			return errs.New(errs.CodeInvalidArgument, "缺少必需参数: "+field)
		}
	}
	return nil
}
