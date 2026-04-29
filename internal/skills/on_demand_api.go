package skills

import "context"

// SkillInstallRegistry 是 tools/skill_install 向 bootstrap 暴露的注册契约。
// *Registry 和 *OverlayRegistry 均已满足。独立声明是为了避免跨包反向依赖。
type SkillInstallRegistry interface {
	RegisterFromPath(ctx context.Context, path string, scope SkillScope, userID string) error
}

// SkillSearchLister 是 tools/skill_search 向 bootstrap 暴露的列表契约。
type SkillSearchLister interface {
	List(userID ...string) []SkillMetadata
}
