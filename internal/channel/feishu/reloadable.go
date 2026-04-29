package feishu

import "github.com/chef-guo/agents-hive/internal/config"

// Reloadable 定义飞书相关组件的热更新最小契约。
// 调用方在配置热重载后可将最新 FeishuConfig 下发给长生命周期组件。
type Reloadable interface {
	ReloadFromConfig(cfg config.FeishuConfig) error
}
