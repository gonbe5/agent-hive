package config

// ToolGroupConfig 定义一组工具的命名集合，用于批量引用
type ToolGroupConfig struct {
	Name  string   `json:"name" yaml:"name"`
	Tools []string `json:"tools" yaml:"tools"`
}

// ToolProfileConfig 定义命名的工具配置文件，工具列表支持 "group:xxx" 引用
type ToolProfileConfig struct {
	Name  string   `json:"name" yaml:"name"`
	Tools []string `json:"tools" yaml:"tools"`
}

// ToolPolicyConfig 配置工具过滤策略
type ToolPolicyConfig struct {
	// Groups 工具分组定义（如 fs、runtime、web）
	Groups []ToolGroupConfig `json:"groups,omitempty" yaml:"groups,omitempty"`

	// Profiles 命名的工具 Profile（如 coding、readonly、messaging）
	Profiles []ToolProfileConfig `json:"profiles,omitempty" yaml:"profiles,omitempty"`

	// GlobalDeny 全局拒绝列表，所有 agent 都不可使用的工具
	GlobalDeny []string `json:"global_deny,omitempty" yaml:"global_deny,omitempty"`

	// SubagentDeny 子 agent 拒绝列表，所有子 agent 不可使用的工具
	SubagentDeny []string `json:"subagent_deny,omitempty" yaml:"subagent_deny,omitempty"`

	// SubagentLeafDeny 叶子子 agent 额外拒绝列表
	SubagentLeafDeny []string `json:"subagent_leaf_deny,omitempty" yaml:"subagent_leaf_deny,omitempty"`

	// MasterProfile Master agent 使用的 Profile 名称，空或 "full" 表示不限制
	MasterProfile string `json:"master_profile,omitempty" yaml:"master_profile,omitempty"`
}
