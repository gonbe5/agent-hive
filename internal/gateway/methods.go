package gateway

import (
	"sync"

	"github.com/chef-guo/agents-hive/internal/acpclient"
	"github.com/chef-guo/agents-hive/internal/airouter"
	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/command"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/master"
	"github.com/chef-guo/agents-hive/internal/mcphost"
	"github.com/chef-guo/agents-hive/internal/plugin"
	"github.com/chef-guo/agents-hive/internal/skills"
	"github.com/chef-guo/agents-hive/internal/store"
)

// SkillLister 是 gateway 需要的最小 skill 注册表接口，*skills.Registry 和 *skills.OverlayRegistry 均满足。
// 变参 userID 可选：不传 = public 层；传 userID = personal + public 合并（personal 优先）。
type SkillLister interface {
	List(userID ...string) []skills.SkillMetadata
}

// Deps 方法依赖注入
type Deps struct {
	Master          *master.Master
	SkillRegistry   SkillLister
	CommandRegistry *command.Registry        // 可为 nil
	ChannelRouter   *channel.Router          // 可为 nil
	PluginLoader    *plugin.Manager          // 可为 nil
	MCPHost         *mcphost.Host            // 可为 nil
	WechatBackend   interface{}              // 可为 nil, 实际类型为 wechatpadpro.Backend
	ACPClientPool   *acpclient.ACPClientPool // 可为 nil, 远程 ACP Agent 连接池
	Config          *config.Config           // 可为 nil, 用于 config.save/reload RPC
	ConfigMu        *sync.RWMutex            // 保护 Config 并发访问
	ConfigPath      string                   // 配置文件路径
	Store           store.Store              // 可为 nil, 统一存储后端（PG）
	AIRouter        *airouter.Router         // 可为 nil, AI 服务路由器（热重载用）

	// 热重载回调（由 bootstrap 注入实际逻辑）
	ReloadChannelFunc func(platform string) error   // 重载 IM 通道插件
	ReloadMCPFunc     func(serverName string) error // 重载 MCP 服务端连接
	ReloadConfigFunc  func()                        // 从 DB 重载所有运行时配置到 Config
}

// RegisterAllMethods 注册所有 RPC 方法
func RegisterAllMethods(gw *Gateway, deps Deps) {
	registerHealthMethods(gw, deps)
	registerSessionMethods(gw, deps)
	registerAgentMethods(gw, deps)
	registerSkillMethods(gw, deps)
	registerHITLMethods(gw, deps)
	// 必须同时提供 Config 和 ConfigMu，否则 RLock/Lock 会触发空指针 panic
	if deps.Config != nil && deps.ConfigMu != nil {
		registerConfigMethods(gw, deps)
	}
	if deps.CommandRegistry != nil {
		registerCommandMethods(gw, deps.CommandRegistry, deps)
	}
	if deps.ChannelRouter != nil {
		registerChannelMethods(gw, deps)
	}
	if deps.PluginLoader != nil {
		registerPluginMethods(gw, deps)
	}
	if deps.MCPHost != nil {
		registerMCPMethods(gw, deps)
	}
	if deps.Store != nil {
		registerResourceMethods(gw, deps)
	}
	if deps.WechatBackend != nil {
		registerWechatMethods(gw, deps)
	}
	if deps.ACPClientPool != nil {
		registerRemoteAgentMethods(gw, deps.ACPClientPool)
	}
}
