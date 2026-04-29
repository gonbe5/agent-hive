package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// FlexStringSlice 兼容 YAML 字符串和数组两种写法。
// 字符串写法按空格拆分为 []string，数组写法直接解析。
//
//	allowed-tools: bash read_file glob grep   → ["bash","read_file","glob","grep"]
//	allowed-tools:
//	  - bash
//	  - read_file                              → ["bash","read_file"]
type FlexStringSlice []string

func (f *FlexStringSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// 单个字符串，按空格拆分
		s := strings.TrimSpace(value.Value)
		if s == "" {
			*f = nil
			return nil
		}
		*f = strings.Fields(s)
		return nil
	case yaml.SequenceNode:
		// 标准数组
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*f = items
		return nil
	default:
		return fmt.Errorf("allowed-tools: expected string or list, got %v", value.Kind)
	}
}

// DisclosureLevel 表示 skill 的渐进式披露级别
type DisclosureLevel int

const (
	// LevelMetadataOnly 表示仅加载了 frontmatter 元数据
	LevelMetadataOnly DisclosureLevel = 1
	// LevelFullContent 表示已加载完整的 SKILL.md 正文
	LevelFullContent DisclosureLevel = 2
	// LevelBundledFiles 表示已编目捆绑文件
	LevelBundledFiles DisclosureLevel = 3
)

// BundledFiles 编目 skill 目录中的附带文件
type BundledFiles struct {
	Scripts    []string `json:"scripts,omitempty"`
	References []string `json:"references,omitempty"`
	Assets     []string `json:"assets,omitempty"`
}

// Skill 表示从文件系统发现的 skill（Markdown 指令包）
type Skill struct {
	Metadata SkillMetadata
	Content  string // SKILL.md 中 YAML frontmatter 之后的 Markdown 正文
	Path     string // skill 的目录路径（用于加载支持文件）

	Bundled BundledFiles    `json:"bundled,omitempty"`
	Loaded  DisclosureLevel `json:"loaded"`

	loadOnce   sync.Once
	loadErr    error
	bundleOnce sync.Once
	bundleErr  error
}

// SkillMetadata 来自 SKILL.md 的 YAML frontmatter
type SkillMetadata struct {
	Name                   string   `yaml:"name"         json:"name"`
	Description            string   `yaml:"description"  json:"description"`
	DisableModelInvocation bool     `yaml:"disable-model-invocation" json:"disable_model_invocation"`
	UserInvocable          *bool    `yaml:"user-invocable" json:"user_invocable,omitempty"`
	AllowedTools           FlexStringSlice `yaml:"allowed-tools" json:"allowed_tools,omitempty"`
	ArgumentHint           string   `yaml:"argument-hint" json:"argument_hint,omitempty"`

	// Open standard fields
	License       string            `yaml:"license"       json:"license,omitempty"`
	Compatibility string            `yaml:"compatibility" json:"compatibility,omitempty"`
	ExtraMetadata map[string]string `yaml:"metadata"      json:"metadata,omitempty"`

	// Claude Code 扩展字段
	Model     string      `yaml:"model"      json:"model,omitempty"`
	Context   string      `yaml:"context"    json:"context,omitempty"` // "fork" = 隔离的 sub-agent
	Agent     string      `yaml:"agent"      json:"agent,omitempty"`   // context=fork 时的 sub-agent 类型
	Hooks     *HookConfig `yaml:"hooks"      json:"hooks,omitempty"`   // 生命周期 hook
	DependsOn []string    `yaml:"depends-on" json:"depends_on,omitempty"` // 此 skill 依赖的其他 skill
	Version   string      `yaml:"version"    json:"version,omitempty"`    // semver 版本号

	// 域 F：Agent 路由扩展字段
	Domain          string   `yaml:"domain"           json:"domain,omitempty"`           // 业务域：content/analytics/meeting/brand 等
	TriggerKeywords []string `yaml:"trigger_keywords" json:"trigger_keywords,omitempty"` // 触发关键词（场景识别用）
	Priority        int      `yaml:"priority"         json:"priority,omitempty"`         // 优先级 1-10（高=更可能被选中）
	Complexity      string   `yaml:"complexity"       json:"complexity,omitempty"`       // 流程复杂度：low/medium/high

	// hive-skill-on-demand 扩展字段
	Scope                SkillScope `yaml:"scope"                 json:"scope,omitempty"`                 // public | personal，frontmatter 优先于 path inference
	ProvidesRequirements []string   `yaml:"provides_requirements" json:"provides_requirements,omitempty"` // 声明此 skill 满足的 spec requirement 名字（与 add-spec-driven-cognition 对齐）
	UserID               string     `yaml:"-"                     json:"user_id,omitempty"`               // personal scope 时注入；public 为空；不从 frontmatter 读
}

// IsUserInvocable 返回 skill 是否可由用户手动调用（默认为 true）
func (m SkillMetadata) IsUserInvocable() bool {
	if m.UserInvocable == nil {
		return true
	}
	return *m.UserInvocable
}

// SkillResolver 用于在 Render 时解析 ${SKILL:name} 占位符
type SkillResolver interface {
	Invoke(name string, rctx RenderContext) (string, error)
}

// RenderContext 保存渲染 skill 内容的上下文
type RenderContext struct {
	Arguments string
	SessionID string
	SkillDir  string        // skill 的目录路径（用于 ${CLAUDE_SKILL_DIR}）
	Resolver  SkillResolver // 用于解析 ${SKILL:name} 占位符（可选）
	depth     int           // 递归深度（防止循环引用）
}

// reArgIndex 匹配 $ARGUMENTS[N]
var reArgIndex = regexp.MustCompile(`\$ARGUMENTS\[(\d+)\]`)

// reShorthand 匹配 $N（位置简写）
var reShorthand = regexp.MustCompile(`\$(\d+)\b`)

// reSkillRef 匹配 ${SKILL:name} 和 ${SKILL:name:args}
var reSkillRef = regexp.MustCompile(`\$\{SKILL:([a-z0-9][a-z0-9-]*)(?::([^}]*))?\}`)

// maxSkillRefDepth 防止循环引用的最大递归深度
const maxSkillRefDepth = 3

// Render 返回替换变量占位符后的 skill 内容。
// 替换顺序（以避免部分匹配）：
//  1. $ARGUMENTS[N] — 索引参数
//  2. $N — 位置简写
//  3. $ARGUMENTS — 完整参数字符串（仅当后面不是 [ 时）
//  4. ${CLAUDE_SKILL_DIR} — skill 目录路径
//  5. ${CLAUDE_SESSION_ID} — session 标识符
//  6. ${SKILL:name} / ${SKILL:name:args} — skill 引用（需要 Resolver）
func (s *Skill) Render(rctx RenderContext) string {
	parts := splitArguments(rctx.Arguments)
	content := s.Content

	// 1. 替换 $ARGUMENTS[N]
	content = reArgIndex.ReplaceAllStringFunc(content, func(match string) string {
		sub := reArgIndex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		idx, err := strconv.Atoi(sub[1])
		if err != nil || idx < 0 || idx >= len(parts) {
			return match
		}
		return parts[idx]
	})

	// 2. 替换 $N 简写
	content = reShorthand.ReplaceAllStringFunc(content, func(match string) string {
		sub := reShorthand.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		idx, err := strconv.Atoi(sub[1])
		if err != nil || idx < 0 || idx >= len(parts) {
			return match
		}
		return parts[idx]
	})

	// 3. 替换 $ARGUMENTS（后面不是 [）
	content = replaceArgumentsPlain(content, rctx.Arguments)

	// 4. 替换 ${CLAUDE_SKILL_DIR}
	content = strings.ReplaceAll(content, "${CLAUDE_SKILL_DIR}", rctx.SkillDir)

	// 5. 替换 ${CLAUDE_SESSION_ID}
	content = strings.ReplaceAll(content, "${CLAUDE_SESSION_ID}", rctx.SessionID)

	// 6. 替换 ${SKILL:name} / ${SKILL:name:args}（需要 Resolver，且不超过最大深度）
	if rctx.Resolver != nil && rctx.depth < maxSkillRefDepth {
		content = reSkillRef.ReplaceAllStringFunc(content, func(match string) string {
			sub := reSkillRef.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			refName := sub[1]
			refArgs := ""
			if len(sub) >= 3 {
				refArgs = sub[2]
			}
			childCtx := RenderContext{
				Arguments: refArgs,
				SessionID: rctx.SessionID,
				Resolver:  rctx.Resolver,
				depth:     rctx.depth + 1,
			}
			result, err := rctx.Resolver.Invoke(refName, childCtx)
			if err != nil {
				// 解析失败时保留原始占位符，不中断渲染
				return match
			}
			return result
		})
	}

	return content
}

// replaceArgumentsPlain 在 $ARGUMENTS 后面不是 [ 时替换它，以避免
// 破坏超出范围索引的未替换 $ARGUMENTS[N] 占位符
func replaceArgumentsPlain(content, replacement string) string {
	const placeholder = "$ARGUMENTS"
	var b strings.Builder
	for {
		idx := strings.Index(content, placeholder)
		if idx == -1 {
			b.WriteString(content)
			break
		}
		after := idx + len(placeholder)
		if after < len(content) && content[after] == '[' {
			// Don't replace — 这是 $ARGUMENTS[...]，跳过 $ARGUMENTS
			b.WriteString(content[:after])
			content = content[after:]
			continue
		}
		b.WriteString(content[:idx])
		b.WriteString(replacement)
		content = content[after:]
	}
	return b.String()
}

// splitArguments 按空格分割参数字符串，同时尊重双引号段
func splitArguments(args string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(args); i++ {
		ch := args[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case ch == ' ' && !inQuotes:
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

// namePattern 根据开放标准验证 skill 名称
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateName 检查 skill 名称是否符合 Agent Skills 开放标准：
//   - ≤64 字符
//   - 仅小写字母数字 + 连字符
//   - 不能以连字符开头或结尾
//   - 不能有连续连字符（--）
func ValidateName(name string) error {
	if len(name) > 64 {
		return errs.New(errs.CodeSkillInvalidName, fmt.Sprintf("skill name %q exceeds 64 characters", name))
	}
	if !namePattern.MatchString(name) {
		return errs.New(errs.CodeSkillInvalidName, fmt.Sprintf("skill name %q must be lowercase alphanumeric with hyphens, cannot start/end with hyphen", name))
	}
	if strings.Contains(name, "--") {
		return errs.New(errs.CodeSkillInvalidName, fmt.Sprintf("skill name %q contains consecutive hyphens", name))
	}
	return nil
}

// LoadContent 从磁盘懒加载完整的 SKILL.md 正文，推进到 LevelFullContent。
// 这通过 sync.Once 保证线程安全
func (s *Skill) LoadContent() error {
	s.loadOnce.Do(func() {
		if s.Loaded >= LevelFullContent {
			return
		}
		skillFile := filepath.Join(s.Path, "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			s.loadErr = errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("read %s", skillFile), err)
			return
		}
		_, body, err := parseFrontmatter(string(data))
		if err != nil {
			s.loadErr = errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("parse frontmatter in %s", skillFile), err)
			return
		}
		s.Content = body
		s.Loaded = LevelFullContent
	})
	return s.loadErr
}

// LoadBundledFiles 编目 scripts/、references/ 和 assets/ 子目录，
// 推进到 LevelBundledFiles。通过 sync.Once 保证线程安全
func (s *Skill) LoadBundledFiles() error {
	s.bundleOnce.Do(func() {
		if s.Loaded < LevelFullContent {
			if err := s.LoadContent(); err != nil {
				s.bundleErr = err
				return
			}
		}
		s.Bundled = BundledFiles{
			Scripts:    listDir(filepath.Join(s.Path, "scripts")),
			References: listDir(filepath.Join(s.Path, "references")),
			Assets:     listDir(filepath.Join(s.Path, "assets")),
		}
		s.Loaded = LevelBundledFiles
	})
	return s.bundleErr
}

// listDir 返回目录中的文件名称，如果目录不存在则返回 nil
func listDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// SkillSummary 是用于 Level 1 列表的 skill 轻量级视图
type SkillSummary struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	OverriddenPublic bool   `json:"overridden_public,omitempty"` // personal 层覆盖同名 public 时为 true
}
