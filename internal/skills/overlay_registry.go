package skills

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// SkillOrigin 标记 skill 的来源层
type SkillOrigin string

const (
	OriginFS SkillOrigin = "fs" // 文件系统层（只读）
	OriginDB SkillOrigin = "db" // 数据库层（可覆盖 FS）
)

// OverlayItem 表示合并视图中的 skill 条目
type OverlayItem struct {
	Skill    *Skill
	Origin   SkillOrigin
	Revision int // 仅 DB 层有值
	UserID   string // personal skill 的 userID；public 为空
}

// dbCacheKey 是 DB 层的复合索引键：
//   - UserID = "" 表示 public scope DB skill
//   - UserID != "" 表示 personal scope DB skill
//
// 在 alice + bob 并发推同名 personal skill 时，分离 key 防止 cache 互相覆盖。
type dbCacheKey struct {
	Name   string
	UserID string
}

// OverlayRegistry 四层 skill 注册表（personal DB > personal FS > public DB > public FS）。
//
// 分层策略：
//   - FS 层（embedded Registry）：由 Finder/Watcher 管理，只读
//   - DB 层（dbCache）：由 SkillService 通过 pg_notify 热更新，按 {name, userID} 复合索引
//
// 租户隔离：alice 永远看不到 bob 的 personal skill；public skill 所有租户共享。
type OverlayRegistry struct {
	*Registry // FS 层（嵌入，Finder/Watcher 直接操作）

	mu      sync.RWMutex
	dbCache map[dbCacheKey]*dbEntry
	logger  *zap.Logger
}

type dbEntry struct {
	skill    *Skill
	revision int
}

// NewOverlayRegistry 创建四层注册表。
func NewOverlayRegistry(logger *zap.Logger) *OverlayRegistry {
	return &OverlayRegistry{
		Registry: NewRegistry(logger),
		dbCache:  make(map[dbCacheKey]*dbEntry),
		logger:   logger,
	}
}

// UpsertDB 将 DB skill 写入内存 dbCache（由 SkillService 调用，content 已从 DB 加载）。
// userID = "" 代表 public scope；非空代表 personal scope 归属该租户。
func (o *OverlayRegistry) UpsertDB(name, userID, content, path string, revision int) {
	meta, body := parseFrontmatterContent(content)
	skill := &Skill{
		Metadata: meta,
		Content:  body,
		Path:     path,
		Loaded:   LevelFullContent,
	}
	skill.Metadata.Name = name
	if userID != "" {
		skill.Metadata.Scope = ScopePersonal
		skill.Metadata.UserID = userID
	} else {
		skill.Metadata.Scope = ScopePublic
		skill.Metadata.UserID = ""
	}
	key := dbCacheKey{Name: name, UserID: userID}
	o.mu.Lock()
	o.dbCache[key] = &dbEntry{skill: skill, revision: revision}
	o.mu.Unlock()
	o.logger.Debug("DB skill 已加载到内存缓存",
		zap.String("name", name),
		zap.String("user_id", userID),
		zap.Int("revision", revision))
}

// DeleteDB 从内存 dbCache 中删除 DB skill。
func (o *OverlayRegistry) DeleteDB(name, userID string) {
	o.mu.Lock()
	delete(o.dbCache, dbCacheKey{Name: name, UserID: userID})
	o.mu.Unlock()
}

// Get 按四层优先级查找 skill：
//
//	personal DB (userID) → personal FS (userID) → public DB → public FS
//
// Go embedding 不自动 re-dispatch 新签名，必须显式覆盖以确保 personal 层在线上生效。
func (o *OverlayRegistry) Get(name string, userID ...string) (*Skill, error) {
	uid := firstUserID(userID...)

	// Layer 1: personal DB
	if uid != "" {
		o.mu.RLock()
		entry, ok := o.dbCache[dbCacheKey{Name: name, UserID: uid}]
		o.mu.RUnlock()
		if ok {
			return entry.skill, nil
		}
	}

	// Layer 2: personal FS（Registry.Get 的 uid 分支）
	if uid != "" {
		o.Registry.mu.RLock()
		if s, ok := o.Registry.skills[registryKey{Name: name, UserID: uid}]; ok {
			o.Registry.mu.RUnlock()
			return s, nil
		}
		o.Registry.mu.RUnlock()
	}

	// Layer 3: public DB
	o.mu.RLock()
	entry, ok := o.dbCache[dbCacheKey{Name: name, UserID: ""}]
	o.mu.RUnlock()
	if ok {
		return entry.skill, nil
	}

	// Layer 4: public FS
	return o.Registry.Get(name)
}

// List 返回合并元数据：personal DB + personal FS + public DB + public FS，
// 按名字去重，personal 优先于 public，DB 优先于 FS。
func (o *OverlayRegistry) List(userID ...string) []SkillMetadata {
	uid := firstUserID(userID...)

	fsList := o.Registry.List(userID...)

	o.mu.RLock()
	defer o.mu.RUnlock()

	// dedupKey 以 name 为键（跨层去重）
	seen := make(map[string]bool, len(fsList)+len(o.dbCache))
	result := make([]SkillMetadata, 0, len(fsList)+len(o.dbCache))

	// Personal DB 最高优先
	if uid != "" {
		for key, entry := range o.dbCache {
			if key.UserID == uid {
				result = append(result, entry.skill.Metadata)
				seen[key.Name] = true
			}
		}
	}

	// FS 层（personal + public 合并后结果，已在 Registry.List 里处理 personal 优先）
	for _, m := range fsList {
		if seen[m.Name] {
			continue
		}
		result = append(result, m)
		seen[m.Name] = true
	}

	// Public DB 兜底（对所有未覆盖的 name 填充）
	for key, entry := range o.dbCache {
		if key.UserID != "" {
			continue
		}
		if seen[key.Name] {
			continue
		}
		result = append(result, entry.skill.Metadata)
		seen[key.Name] = true
	}

	return result
}

// ListForModel 覆盖 Registry.ListForModel，走合并视图。
func (o *OverlayRegistry) ListForModel(userID ...string) []SkillMetadata {
	all := o.List(userID...)
	result := make([]SkillMetadata, 0, len(all))
	for _, m := range all {
		if !m.DisableModelInvocation {
			result = append(result, m)
		}
	}
	return result
}

// ListUserInvocable 覆盖 Registry.ListUserInvocable，走合并视图。
func (o *OverlayRegistry) ListUserInvocable(userID ...string) []SkillMetadata {
	all := o.List(userID...)
	result := make([]SkillMetadata, 0, len(all))
	for _, m := range all {
		if m.IsUserInvocable() {
			result = append(result, m)
		}
	}
	return result
}

// ListSummaries 覆盖 Registry.ListSummaries，走合并视图（携带 OverriddenPublic）。
func (o *OverlayRegistry) ListSummaries(userID ...string) []SkillSummary {
	all := o.List(userID...)
	summaries := make([]SkillSummary, 0, len(all))

	// 标记 personal 层覆盖 public 的情况
	uid := firstUserID(userID...)
	publicNames := make(map[string]bool)
	if uid != "" {
		o.Registry.mu.RLock()
		for key := range o.Registry.skills {
			if key.UserID == "" {
				publicNames[key.Name] = true
			}
		}
		o.Registry.mu.RUnlock()
		o.mu.RLock()
		for key := range o.dbCache {
			if key.UserID == "" {
				publicNames[key.Name] = true
			}
		}
		o.mu.RUnlock()
	}

	for _, m := range all {
		overridden := false
		if uid != "" && m.Scope == ScopePersonal && publicNames[m.Name] {
			overridden = true
		}
		summaries = append(summaries, SkillSummary{
			Name:             m.Name,
			Description:      m.Description,
			OverriddenPublic: overridden,
		})
	}
	return summaries
}

// Count 覆盖合并视图。
func (o *OverlayRegistry) Count(userID ...string) int {
	return len(o.List(userID...))
}

// RegisterFromPath 覆盖 Registry.RegisterFromPath，保留 embed 的能力同时显式声明签名。
// Go embedding 不会 re-dispatch 新签名，必须显式重写。
func (o *OverlayRegistry) RegisterFromPath(ctx context.Context, path string, scope SkillScope, userID string) error {
	return o.Registry.RegisterFromPath(ctx, path, scope, userID)
}

// ListOverlayItems 返回带来源信息的完整列表（Admin UI 用）。
func (o *OverlayRegistry) ListOverlayItems(userID ...string) []OverlayItem {
	uid := firstUserID(userID...)

	o.mu.RLock()
	defer o.mu.RUnlock()

	seen := make(map[string]bool)
	var items []OverlayItem

	// personal DB 优先
	if uid != "" {
		for key, entry := range o.dbCache {
			if key.UserID == uid {
				items = append(items, OverlayItem{
					Skill:    entry.skill,
					Origin:   OriginDB,
					Revision: entry.revision,
					UserID:   key.UserID,
				})
				seen[key.Name] = true
			}
		}
	}

	// personal FS
	if uid != "" {
		o.Registry.mu.RLock()
		for key, s := range o.Registry.skills {
			if key.UserID == uid && !seen[key.Name] {
				items = append(items, OverlayItem{
					Skill:  s,
					Origin: OriginFS,
					UserID: key.UserID,
				})
				seen[key.Name] = true
			}
		}
		o.Registry.mu.RUnlock()
	}

	// public DB
	for key, entry := range o.dbCache {
		if key.UserID != "" {
			continue
		}
		if seen[key.Name] {
			continue
		}
		items = append(items, OverlayItem{
			Skill:    entry.skill,
			Origin:   OriginDB,
			Revision: entry.revision,
		})
		seen[key.Name] = true
	}

	// public FS
	o.Registry.mu.RLock()
	for key, s := range o.Registry.skills {
		if key.UserID != "" {
			continue
		}
		if seen[key.Name] {
			continue
		}
		items = append(items, OverlayItem{
			Skill:  s,
			Origin: OriginFS,
		})
		seen[key.Name] = true
	}
	o.Registry.mu.RUnlock()

	return items
}

// GetWithOrigin 返回 skill 及其来源层。personal 优先，DB 优先。
func (o *OverlayRegistry) GetWithOrigin(name string, userID ...string) (*Skill, SkillOrigin, int, error) {
	uid := firstUserID(userID...)

	// personal DB
	if uid != "" {
		o.mu.RLock()
		entry, ok := o.dbCache[dbCacheKey{Name: name, UserID: uid}]
		o.mu.RUnlock()
		if ok {
			return entry.skill, OriginDB, entry.revision, nil
		}
	}
	// personal FS
	if uid != "" {
		o.Registry.mu.RLock()
		if s, ok := o.Registry.skills[registryKey{Name: name, UserID: uid}]; ok {
			o.Registry.mu.RUnlock()
			return s, OriginFS, 0, nil
		}
		o.Registry.mu.RUnlock()
	}
	// public DB
	o.mu.RLock()
	entry, ok := o.dbCache[dbCacheKey{Name: name, UserID: ""}]
	o.mu.RUnlock()
	if ok {
		return entry.skill, OriginDB, entry.revision, nil
	}
	// public FS
	s, err := o.Registry.Get(name)
	if err != nil {
		return nil, OriginFS, 0, err
	}
	return s, OriginFS, 0, nil
}

// FindBySpecRequirements 覆盖 Registry.FindBySpecRequirements，在四层合并视图上查找。
func (o *OverlayRegistry) FindBySpecRequirements(reqs []string, userID string) []*Skill {
	if len(reqs) == 0 {
		return nil
	}
	reqSet := make(map[string]bool, len(reqs))
	for _, r := range reqs {
		reqSet[r] = true
	}

	seen := make(map[string]bool)
	var matched []*Skill
	consider := func(name string, s *Skill) {
		if seen[name] {
			return
		}
		for _, p := range s.Metadata.ProvidesRequirements {
			if reqSet[p] {
				matched = append(matched, s)
				seen[name] = true
				return
			}
		}
	}

	// personal DB / FS
	if userID != "" {
		o.mu.RLock()
		for key, entry := range o.dbCache {
			if key.UserID == userID {
				consider(key.Name, entry.skill)
			}
		}
		o.mu.RUnlock()
		o.Registry.mu.RLock()
		for key, s := range o.Registry.skills {
			if key.UserID == userID {
				consider(key.Name, s)
			}
		}
		o.Registry.mu.RUnlock()
	}
	// public DB / FS
	o.mu.RLock()
	for key, entry := range o.dbCache {
		if key.UserID == "" {
			consider(key.Name, entry.skill)
		}
	}
	o.mu.RUnlock()
	o.Registry.mu.RLock()
	for key, s := range o.Registry.skills {
		if key.UserID == "" {
			consider(key.Name, s)
		}
	}
	o.Registry.mu.RUnlock()

	return matched
}

// parseFrontmatterContent 将完整 content（含 frontmatter）拆分为 metadata + body。
// 复用 finder.go 中的 parseFrontmatter 函数（同包，直接调用）。
func parseFrontmatterContent(content string) (SkillMetadata, string) {
	meta, body, _ := parseFrontmatter(content)
	return meta, body
}
