package command

import (
	"testing"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/skills"
)

func newTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func boolPtr(b bool) *bool { return &b }

func TestRegistry_Register_Priority(t *testing.T) {
	tests := []struct {
		name         string
		first        *Info
		second       *Info
		wantSource   Source
		wantTemplate string
	}{
		{
			name:         "高优先级不被低优先级覆盖",
			first:        &Info{Name: "foo", Description: "内置", Source: SourceBuiltin, Template: "builtin"},
			second:       &Info{Name: "foo", Description: "技能", Source: SourceSkill, Template: "skill"},
			wantSource:   SourceBuiltin,
			wantTemplate: "builtin",
		},
		{
			name:         "低优先级被高优先级覆盖",
			first:        &Info{Name: "bar", Description: "技能", Source: SourceSkill, Template: "skill"},
			second:       &Info{Name: "bar", Description: "配置", Source: SourceConfig, Template: "config"},
			wantSource:   SourceConfig,
			wantTemplate: "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(newTestLogger())
			reg.Register(tt.first)
			reg.Register(tt.second)

			cmd, err := reg.Get(tt.first.Name)
			if err != nil {
				t.Fatalf("期望找到命令，得到错误: %v", err)
			}
			if cmd.Source != tt.wantSource {
				t.Errorf("期望来源为 %s，得到 %s", tt.wantSource, cmd.Source)
			}
			if cmd.Template != tt.wantTemplate {
				t.Errorf("期望模板为 %s，得到 %s", tt.wantTemplate, cmd.Template)
			}
		})
	}
}

func TestRegistry_LoadFromSkills(t *testing.T) {
	skillReg := skills.NewRegistry(newTestLogger())
	trueVal := boolPtr(true)
	skillReg.Register(&skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:          "code-review",
			Description:   "代码审查",
			UserInvocable: trueVal,
		},
	})
	// UserInvocable=nil 也应加载
	skillReg.Register(&skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:        "research",
			Description: "研究",
		},
	})
	// UserInvocable=false 不应加载
	falseVal := boolPtr(false)
	skillReg.Register(&skills.Skill{
		Metadata: skills.SkillMetadata{
			Name:          "internal-tool",
			Description:   "内部工具",
			UserInvocable: falseVal,
		},
	})

	reg := NewRegistry(newTestLogger())
	reg.LoadFromSkills(skillReg)

	if _, err := reg.Get("code-review"); err != nil {
		t.Errorf("期望找到 code-review 命令: %v", err)
	}
	if _, err := reg.Get("research"); err != nil {
		t.Errorf("期望找到 research 命令: %v", err)
	}
	if _, err := reg.Get("internal-tool"); err == nil {
		t.Error("不期望找到 internal-tool 命令（UserInvocable=false）")
	}
}

func TestRegistry_LoadFromConfig(t *testing.T) {
	reg := NewRegistry(newTestLogger())
	reg.LoadFromConfig(map[string]config.CommandConfig{
		"review": {
			Description: "审查分支",
			Template:    "审查分支 $1 的代码变更",
		},
		"deploy": {
			Description: "部署",
			Template:    "部署到 $ARGUMENTS",
			Subtask:     true,
		},
	})

	cmd, err := reg.Get("review")
	if err != nil {
		t.Fatalf("期望找到 review 命令: %v", err)
	}
	if cmd.Source != SourceConfig {
		t.Errorf("期望来源为 config，得到 %s", cmd.Source)
	}
	if cmd.Template != "审查分支 $1 的代码变更" {
		t.Errorf("模板不匹配: %s", cmd.Template)
	}

	deploy, err := reg.Get("deploy")
	if err != nil {
		t.Fatalf("期望找到 deploy 命令: %v", err)
	}
	if !deploy.Subtask {
		t.Error("期望 deploy.Subtask 为 true")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry(newTestLogger())
	reg.Register(&Info{Name: "zzz", Source: SourceSkill, Template: "z"})
	reg.Register(&Info{Name: "aaa", Source: SourceConfig, Template: "a"})
	reg.Register(&Info{Name: "mmm", Source: SourceBuiltin, Template: "m"})

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("期望 3 个命令，得到 %d", len(list))
	}
	// 验证按名称排序
	if list[0].Name != "aaa" || list[1].Name != "mmm" || list[2].Name != "zzz" {
		t.Errorf("命令未按名称排序: %v", []string{list[0].Name, list[1].Name, list[2].Name})
	}
}

func TestRegistry_ListBySource(t *testing.T) {
	reg := NewRegistry(newTestLogger())
	reg.Register(&Info{Name: "builtin-a", Source: SourceBuiltin, Template: "a"})
	reg.Register(&Info{Name: "config-b", Source: SourceConfig, Template: "b"})
	reg.Register(&Info{Name: "config-c", Source: SourceConfig, Template: "c"})
	reg.Register(&Info{Name: "skill-d", Source: SourceSkill, Template: "d"})

	tests := []struct {
		name      string
		source    Source
		wantCount int
		wantNames []string
	}{
		{
			name:      "过滤 builtin",
			source:    SourceBuiltin,
			wantCount: 1,
			wantNames: []string{"builtin-a"},
		},
		{
			name:      "过滤 config",
			source:    SourceConfig,
			wantCount: 2,
			wantNames: []string{"config-b", "config-c"},
		},
		{
			name:      "过滤 skill",
			source:    SourceSkill,
			wantCount: 1,
			wantNames: []string{"skill-d"},
		},
		{
			name:      "过滤 mcp（无结果）",
			source:    SourceMCP,
			wantCount: 0,
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reg.ListBySource(tt.source)
			if len(result) != tt.wantCount {
				t.Fatalf("期望 %d 个命令，得到 %d", tt.wantCount, len(result))
			}
			for i, cmd := range result {
				if i < len(tt.wantNames) && cmd.Name != tt.wantNames[i] {
					t.Errorf("命令[%d]: 期望 %s，得到 %s", i, tt.wantNames[i], cmd.Name)
				}
			}
		})
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry(newTestLogger())
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("期望返回错误，但得到 nil")
	}
}
