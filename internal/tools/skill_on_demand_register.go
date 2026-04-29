package tools

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/skills"
)

var errSkillInstallRegistryMissing = errors.New("skill_install: registry not configured")

// RegisterSkillInstallPublic 是 skill_install 对外暴露的注册入口。bootstrap 侧
// 在 cfg.Agent.Skills.OnDemandEnabled=true 时调用，内部走已测 handleSkillInstall。
//
// emitter 形参类型为 any 以回避 mcphost ↔ master 循环导入：bootstrap 会传入
// *mcphost.Host（实现了 hitlEmitter 接口），在此处做一次 type-assert。
// adminChecker / broadcaster 为 nil 时按 "公共 scope 拒绝 / 不广播" 处理。
func RegisterSkillInstallPublic(
	host *mcphost.Host,
	logger *zap.Logger,
	registry skills.SkillInstallRegistry,
	discovery *skills.Discovery,
	broadcaster SkillInstallBroadcasterPublic,
	adminChecker skills.AdminChecker,
	emitter any,
) {
	var em hitlEmitter
	if emitter != nil {
		if he, ok := emitter.(hitlEmitter); ok {
			em = he
		}
	}
	deps := skillInstallDeps{
		Logger:       logger,
		Registry:     registryAdapter{r: registry},
		Discovery:    discovery,
		Broadcaster:  broadcasterAdapter{b: broadcaster},
		AdminChecker: adminChecker,
		Emitter:      em,
	}
	registerSkillInstall(host, deps)
}

// RegisterSkillSearchPublic 是 skill_search 对外暴露的注册入口。
// registry 形参接受 *skills.Registry / *skills.OverlayRegistry 或任何满足
// List(userID ...) 的实现。
func RegisterSkillSearchPublic(
	host *mcphost.Host,
	logger *zap.Logger,
	registry skills.SkillSearchLister,
	discovery *skills.Discovery,
) {
	registerSkillSearch(host, logger, searchRegistryAdapter{r: registry}, discovery)
}

// SkillInstallBroadcasterPublic 是 bootstrap 到 tools 的广播契约。
// *master.Master 已实现 BroadcastGenericMessage(msgType, payload) 原型。
type SkillInstallBroadcasterPublic interface {
	BroadcastGenericMessage(msgType string, payload any)
}

// ──────────────────────────────────────────────────────────────────────
// 内部 adapter：把 skills 包暴露出的 public 接口转成 tools 包内的 unexported 接口
// ──────────────────────────────────────────────────────────────────────

type registryAdapter struct{ r skills.SkillInstallRegistry }

func (a registryAdapter) RegisterFromPath(ctx context.Context, path string, scope skills.SkillScope, userID string) error {
	if a.r == nil {
		return errSkillInstallRegistryMissing
	}
	return a.r.RegisterFromPath(ctx, path, scope, userID)
}

type broadcasterAdapter struct{ b SkillInstallBroadcasterPublic }

func (a broadcasterAdapter) BroadcastGenericMessage(msgType string, payload interface{}) {
	if a.b == nil {
		return
	}
	a.b.BroadcastGenericMessage(msgType, payload)
}

type searchRegistryAdapter struct{ r skills.SkillSearchLister }

func (a searchRegistryAdapter) List(userID ...string) []skills.SkillMetadata {
	if a.r == nil {
		return nil
	}
	return a.r.List(userID...)
}
