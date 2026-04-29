package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// FinderOption 配置 Finder 的可选行为
type FinderOption func(*Finder)

// WithNestedDiscovery 启用对给定根目录下 .claude/skills/ 子目录的递归发现
func WithNestedDiscovery(root string) FinderOption {
	return func(f *Finder) {
		f.nestedRoot = root
	}
}

// WithRemoteURLs 启用从远程 URL 拉取 skill
func WithRemoteURLs(urls []string, discovery *Discovery) FinderOption {
	return func(f *Finder) {
		f.remoteURLs = urls
		f.discovery = discovery
	}
}

// WithPublicSkillsDir 注册一个明确归属 public scope 的搜索路径（$HIVE_DATA/skills/public）
func WithPublicSkillsDir(dir string) FinderOption {
	return func(f *Finder) {
		if dir != "" {
			f.scopedPaths = append(f.scopedPaths, scopedPath{path: dir, scope: ScopePublic})
		}
	}
}

// WithPersonalSkillsRoot 注册 personal skill 根目录（$HIVE_DATA/skills/users）。
// Finder 在 Discover 时枚举子目录作为 userID，每个 userID 目录下的 skill 归属 personal scope。
func WithPersonalSkillsRoot(root string) FinderOption {
	return func(f *Finder) {
		f.personalRoot = root
	}
}

type scopedPath struct {
	path  string
	scope SkillScope
}

// Finder 从文件系统发现和加载 skill
type Finder struct {
	searchPaths  []string
	scopedPaths  []scopedPath // 显式声明 scope 的路径（public skills dir 等）
	personalRoot string       // personal skills 根，子目录名即 userID
	registry     *Registry
	logger       *zap.Logger
	nestedRoot   string     // 如果设置，在此根目录下递归发现 .claude/skills/
	discovery    *Discovery // 远程 skill 发现器
	remoteURLs   []string   // 远程 skill 仓库 URL 列表
}

// NewFinder 创建新的 skill finder
func NewFinder(registry *Registry, logger *zap.Logger, searchPaths []string, opts ...FinderOption) *Finder {
	f := &Finder{
		searchPaths: searchPaths,
		registry:    registry,
		logger:      logger,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Discover 扫描搜索路径中的 SKILL.md 文件并返回发现的 skill
// 处于 Level 1（仅元数据 — Content 为空）。
//
// 扫描顺序 + scope 归属：
//  1. 旧 searchPaths（.claude/skills、~/.claude/skills、skills/）→ ScopePublic（向后兼容）
//  2. scopedPaths（WithPublicSkillsDir 注册的 $HIVE_DATA/skills/public）→ ScopePublic
//  3. personalRoot/<userID>/*（WithPersonalSkillsRoot 注册）→ ScopePersonal + userID
//
// frontmatter 中显式声明的 scope 始终优先于 path-inferred scope。
func (f *Finder) Discover() ([]*Skill, error) {
	var discovered []*Skill

	for _, searchPath := range f.searchPaths {
		skills, err := f.discoverInPathScoped(searchPath, ScopePublic, "")
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, skills...)
	}

	for _, sp := range f.scopedPaths {
		skills, err := f.discoverInPathScoped(sp.path, sp.scope, "")
		if err != nil {
			return nil, err
		}
		discovered = append(discovered, skills...)
	}

	if f.personalRoot != "" {
		personalSkills, err := f.discoverPersonalRoot(f.personalRoot)
		if err != nil {
			f.logger.Warn("personal skills 根目录扫描失败", zap.String("root", f.personalRoot), zap.Error(err))
		} else {
			discovered = append(discovered, personalSkills...)
		}
	}

	return discovered, nil
}

// discoverPersonalRoot 枚举 personalRoot 的一级子目录作为 userID，
// 每个 userID 目录下的 skill 归属 ScopePersonal + 对应 userID。
func (f *Finder) discoverPersonalRoot(root string) ([]*Skill, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("stat personal root %q", root), err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("read personal root %q", root), err)
	}
	var all []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		userID := entry.Name()
		if userID == "" {
			continue
		}
		userDir := filepath.Join(root, userID)
		skills, err := f.discoverInPathScoped(userDir, ScopePersonal, userID)
		if err != nil {
			f.logger.Warn("personal user 目录扫描失败", zap.String("user_id", userID), zap.Error(err))
			continue
		}
		all = append(all, skills...)
	}
	return all, nil
}

// discoverInPath 扫描单个搜索路径中的 skill（兼容旧 caller，默认 public scope + 空 userID）
func (f *Finder) discoverInPath(searchPath string) ([]*Skill, error) {
	return f.discoverInPathScoped(searchPath, ScopePublic, "")
}

// discoverInPathScoped 扫描单个路径，给发现的 skill 统一注入 scope + userID。
// frontmatter 中显式声明的 scope 优先；若 path-inferred scope=personal 但 userID 空，skill 被拒绝。
func (f *Finder) discoverInPathScoped(searchPath string, pathScope SkillScope, userID string) ([]*Skill, error) {
	info, err := os.Stat(searchPath)
	if err != nil {
		if os.IsNotExist(err) {
			f.logger.Debug("skill 搜索路径不存在", zap.String("path", searchPath))
			return nil, nil
		}
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("stat %q", searchPath), err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(searchPath)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("read dir %q", searchPath), err)
	}

	var discovered []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(searchPath, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")

		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			f.logger.Debug("目录没有 SKILL.md，跳过", zap.String("path", skillDir))
			continue
		}

		skill, err := discoverMetadataOnly(skillFile, skillDir)
		if err != nil {
			f.logger.Warn("解析 SKILL.md 失败", zap.String("path", skillFile), zap.Error(err))
			continue
		}

		// Validate name
		if err := ValidateName(skill.Metadata.Name); err != nil {
			f.logger.Warn("无效的 skill 名称，跳过",
				zap.String("name", skill.Metadata.Name),
				zap.String("path", skillDir),
				zap.Error(err))
			continue
		}

		if skill.Metadata.Name != filepath.Base(skillDir) {
			f.logger.Debug("skill 名称与目录名不同，以 frontmatter name 为准",
				zap.String("name", skill.Metadata.Name),
				zap.String("dir", filepath.Base(skillDir)),
				zap.String("path", skillDir))
		}

		// Validate compatibility length
		if len(skill.Metadata.Compatibility) > 500 {
			f.logger.Warn("skill 兼容性字段超过 500 字符，跳过",
				zap.String("name", skill.Metadata.Name),
				zap.String("path", skillDir))
			continue
		}

		// Validate description length
		if len(skill.Metadata.Description) > 1024 {
			f.logger.Warn("skill 描述超过 1024 字符，跳过",
				zap.String("name", skill.Metadata.Name),
				zap.String("path", skillDir),
				zap.Int("length", len(skill.Metadata.Description)))
			continue
		}

		// 应用 scope 归属：frontmatter 显式声明优先，否则用 path-inferred
		effectiveScope := skill.Metadata.Scope
		if effectiveScope == "" {
			effectiveScope = pathScope
		}
		if !effectiveScope.Valid() {
			f.logger.Warn("skill scope 无效，跳过",
				zap.String("name", skill.Metadata.Name),
				zap.String("scope", string(effectiveScope)),
				zap.String("path", skillDir))
			continue
		}

		// personal scope 必须携带 userID（path-inferred 时由目录名注入，frontmatter 声明 personal 但 path 无 userID → 拒绝）
		if effectiveScope == ScopePersonal && userID == "" {
			f.logger.Warn("personal scope skill 缺少 userID，跳过",
				zap.String("name", skill.Metadata.Name),
				zap.String("path", skillDir))
			continue
		}

		skill.Metadata.Scope = effectiveScope
		if effectiveScope == ScopePersonal {
			skill.Metadata.UserID = userID
		} else {
			skill.Metadata.UserID = ""
		}

		discovered = append(discovered, skill)
		f.logger.Debug("发现 skill",
			zap.String("name", skill.Metadata.Name),
			zap.String("path", skillDir),
			zap.String("scope", string(effectiveScope)),
			zap.String("user_id", skill.Metadata.UserID))
	}

	return discovered, nil
}

// DiscoverAndRegister 发现 skill 并在注册表中注册它们
func (f *Finder) DiscoverAndRegister() error {
	skills, err := f.Discover()
	if err != nil {
		return err
	}
	for _, s := range skills {
		if err := f.registry.Register(s); err != nil {
			f.logger.Warn("注册 skill 失败", zap.String("name", s.Metadata.Name), zap.Error(err))
		}
	}

	// Nested discovery
	if f.nestedRoot != "" {
		nested, err := f.DiscoverNested(f.nestedRoot)
		if err != nil {
			f.logger.Warn("嵌套发现失败", zap.Error(err))
		} else {
			for _, s := range nested {
				if err := f.registry.Register(s); err != nil {
					f.logger.Warn("注册嵌套 skill 失败", zap.String("name", s.Metadata.Name), zap.Error(err))
				}
			}
		}
	}

	// 远程 skill 发现
	if f.discovery != nil && len(f.remoteURLs) > 0 {
		for _, url := range f.remoteURLs {
			dirs, err := f.discovery.Pull(context.Background(), url)
			if err != nil {
				f.logger.Warn("远程 skill 拉取失败",
					zap.String("url", url), zap.Error(err))
				continue
			}
			for _, dir := range dirs {
				// 将远程 skill 目录的父目录作为搜索路径进行发现
				remoteSkills, err := f.discoverInPath(filepath.Dir(dir))
				if err != nil {
					f.logger.Warn("远程 skill 发现失败",
						zap.String("dir", dir), zap.Error(err))
					continue
				}
				for _, s := range remoteSkills {
					if err := f.registry.Register(s); err != nil {
						f.logger.Warn("注册远程 skill 失败", zap.String("name", s.Metadata.Name), zap.Error(err))
					}
				}
			}
		}
	}

	f.logger.Info("skill 发现完成", zap.Int("count", f.registry.Count()))
	return nil
}

// DiscoverNested 递归遍历根目录下的目录树，寻找 .claude/skills/ 子目录，
// 并在每个目录中发现 skill
func (f *Finder) DiscoverNested(root string) ([]*Skill, error) {
	var discovered []*Skill

	skipDirs := map[string]bool{
		"node_modules": true,
		".git":         true,
		"vendor":       true,
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible directories
		}
		if !d.IsDir() {
			return nil
		}

		// Skip known non-interesting directories
		if skipDirs[d.Name()] {
			return filepath.SkipDir
		}

		// Check if this directory contains .claude/skills/
		skillsDir := filepath.Join(path, ".claude", "skills")
		if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
			// Check that this isn't one of our main search paths (avoid double-discovery)
			absSkillsDir, _ := filepath.Abs(skillsDir)
			isDuplicate := false
			for _, sp := range f.searchPaths {
				absSP, _ := filepath.Abs(sp)
				if absSkillsDir == absSP {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				skills, err := f.discoverInPath(skillsDir)
				if err != nil {
					f.logger.Warn("嵌套发现错误", zap.String("path", skillsDir), zap.Error(err))
				} else {
					discovered = append(discovered, skills...)
				}
			}
		}

		return nil
	})
	if err != nil {
		return discovered, errs.Wrap(errs.CodeSkillLoadFailed, "nested discovery walk", err)
	}

	return discovered, nil
}

// SearchPaths 返回配置的搜索路径
func (f *Finder) SearchPaths() []string {
	return f.searchPaths
}

// discoverMetadataOnly 解析 SKILL.md 文件但仅提取 frontmatter 元数据。
// Content 保持为空（Level 1 — 仅元数据）
func discoverMetadataOnly(filePath string, skillDir string) (*Skill, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("read %s", filePath), err)
	}

	content := string(data)
	metadata, _, err := parseFrontmatter(content)
	if err != nil {
		return nil, errs.Wrap(errs.CodeSkillLoadFailed, fmt.Sprintf("parse frontmatter in %s", filePath), err)
	}

	if metadata.Name == "" {
		metadata.Name = filepath.Base(skillDir)
	}

	return &Skill{
		Metadata: metadata,
		Content:  "", // Level 1: 仅元数据
		Path:     skillDir,
		Loaded:   LevelMetadataOnly,
	}, nil
}

// parseFrontmatter 将 SKILL.md 文件分割为 YAML frontmatter 和 markdown 正文。
// Frontmatter 由文件开头的 "---" 行分隔
func parseFrontmatter(content string) (SkillMetadata, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		// No frontmatter — 将整个内容视为正文
		return SkillMetadata{}, content, nil
	}

	// Find the closing "---"
	rest := content[3:] // skip opening "---"
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return SkillMetadata{}, content, errs.New(errs.CodeSkillLoadFailed, "closing frontmatter delimiter not found")
	}

	frontmatterRaw := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // skip "\n---"

	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatterRaw), &metadata); err != nil {
		return SkillMetadata{}, "", errs.Wrap(errs.CodeSkillLoadFailed, "unmarshal frontmatter YAML", err)
	}

	return metadata, body, nil
}
