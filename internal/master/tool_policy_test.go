package master

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/config"
)

func TestBuildToolPolicy_DefaultConfig(t *testing.T) {
	// 使用默认配置，验证 buildToolPolicy 转换不丢失字段
	cfg := config.DefaultToolPolicyConfig

	policy := buildToolPolicy(cfg)
	if policy == nil {
		t.Fatal("默认配置应返回非 nil ToolPolicy")
	}

	// 默认 master_profile="master_direct" → MasterFilter 应为非 nil
	filter := policy.MasterFilter()
	if filter == nil {
		t.Error("默认 master_direct profile 应产生非 nil MasterFilter")
	}

	// P0-3: master_direct profile 应包含所有常用工具（直接执行路径）
	allowed := []string{"skill", "memory", "question", "spawn_agent", "task", "tool_search", "bash", "write_file", "edit", "read_file", "glob", "grep"}
	for _, name := range allowed {
		if !filter.IsAllowed(name) {
			t.Errorf("master_direct profile 应允许工具 %q", name)
		}
	}

	// 无 warnings
	if len(policy.Warnings()) != 0 {
		t.Errorf("默认配置不应有 warning: %v", policy.Warnings())
	}
}

func TestBuildToolPolicy_CodingProfile(t *testing.T) {
	cfg := config.DefaultToolPolicyConfig
	cfg.MasterProfile = "coding"

	policy := buildToolPolicy(cfg)
	filter := policy.MasterFilter()
	if filter == nil {
		t.Fatal("coding profile 应产生非 nil filter")
	}

	// coding profile 应允许 fs/runtime/web/lsp 中的工具
	allowed := []string{"read_file", "write_file", "edit", "glob", "grep", "ls", "bash", "websearch", "webfetch", "tool_search", "skill", "memory"}
	for _, name := range allowed {
		if !filter.IsAllowed(name) {
			t.Errorf("工具 %q 应被 coding profile 允许", name)
		}
	}

	// 不在 coding profile 中的工具应被拒绝
	denied := []string{"spawn_agent", "create_tool", "remove_tool", "send_im_message"}
	for _, name := range denied {
		if filter.IsAllowed(name) {
			t.Errorf("工具 %q 不应被 coding profile 允许", name)
		}
	}
}

func TestBuildToolPolicy_SubagentDeny(t *testing.T) {
	cfg := config.DefaultToolPolicyConfig

	policy := buildToolPolicy(cfg)

	// SubAgent filter 应 deny spawn_agent, create_tool, remove_tool
	filter := policy.BuildFilter("", nil, true, false)
	if filter == nil {
		t.Fatal("subagent 有 deny 时 filter 不应为 nil")
	}

	for _, name := range []string{"spawn_agent", "create_tool", "remove_tool"} {
		if filter.IsAllowed(name) {
			t.Errorf("subagent 不应允许 %q", name)
		}
	}

	// 非 deny 工具应被允许
	if !filter.IsAllowed("read_file") {
		t.Error("subagent 应允许 read_file")
	}
}

func TestBuildToolPolicy_LeafDeny(t *testing.T) {
	cfg := config.DefaultToolPolicyConfig

	policy := buildToolPolicy(cfg)

	// Leaf subagent 应额外 deny parallel_dispatch, task
	filter := policy.BuildFilter("", nil, true, true)
	if filter == nil {
		t.Fatal("leaf subagent filter 不应为 nil")
	}

	for _, name := range []string{"parallel_dispatch", "task"} {
		if filter.IsAllowed(name) {
			t.Errorf("leaf subagent 不应允许 %q", name)
		}
	}
}

func TestBuildToolPolicy_MasterProfile_NotFull(t *testing.T) {
	cfg := config.DefaultToolPolicyConfig
	if cfg.MasterProfile != "master_direct" {
		t.Fatalf("默认 MasterProfile 应为 master_direct, got %q", cfg.MasterProfile)
	}

	policy := buildToolPolicy(cfg)
	filter := policy.MasterFilter()
	if filter == nil {
		t.Fatal("master_direct profile 应产生非 nil filter")
	}

	// master_direct profile 应允许所有常用工具（P0-3 Phase 1）
	for _, name := range []string{"bash", "write_file", "edit", "multiedit", "apply_patch", "create_tool", "remove_tool", "read_file", "glob", "grep", "tool_search", "skill", "memory", "question", "spawn_agent", "task", "parallel_dispatch", "send_im_message", "feishu_api", "wechat_send_rich_message", "wechat_contacts"} {
		if !filter.IsAllowed(name) {
			t.Errorf("master_direct profile 应允许 %q", name)
		}
	}

	// 不在 master_direct 中的工具应被拒绝
	for _, name := range []string{"wechat_ops"} {
		if filter.IsAllowed(name) {
			t.Errorf("master_direct profile 不应允许 %q", name)
		}
	}
}
