package gateway

import (
	"context"
	"encoding/json"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/errs"
)

func registerChannelMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "channel.status",
		Description: "各 IM 平台状态",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			platforms := []channel.Platform{
				channel.PlatformDingTalk, channel.PlatformFeishu, channel.PlatformWeCom,
			}
			status := make(map[string]bool)
			for _, p := range platforms {
				_, ok := deps.ChannelRouter.GetPlugin(p)
				status[string(p)] = ok
			}
			return json.Marshal(status)
		},
	})

	gw.Register(MethodDef{
		Name:        "channel.send",
		Description: "手动发送消息到 IM",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				Platform string `json:"platform"`
				ChatID   string `json:"chat_id"`
				Content  string `json:"content"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			plugin, ok := deps.ChannelRouter.GetPlugin(channel.Platform(p.Platform))
			if !ok {
				return nil, errs.New(errs.CodeChannelPlatformNotFound, "平台未注册: "+p.Platform)
			}
			err := plugin.Send(ctx, channel.OutboundMessage{
				Platform: channel.Platform(p.Platform),
				ChatID:   p.ChatID,
				Content:  p.Content,
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"status": "sent"})
		},
	})

	gw.Register(MethodDef{
		Name:        "channel.bind",
		Description: "绑定 IM 通道到会话",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var b channel.Binding
			if err := json.Unmarshal(params, &b); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			deps.ChannelRouter.Bind(b)
			return json.Marshal(map[string]string{"status": "bound"})
		},
	})
}

// wechatConfigurator 微信后端配置接口（本地定义避免循环依赖）
type wechatConfigurator interface {
	IsLoggedIn() bool
	Reconfigure(baseURL, key string)
}

// registerWechatMethods 注册微信 WeChatPadPro 专用方法
func registerWechatMethods(gw *Gateway, deps Deps) {
	gw.Register(MethodDef{
		Name:        "channel.wechat.config",
		Description: "更新微信 WeChatPadPro 后端配置",
		AuthScope:   "write",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			var p struct {
				BaseURL  string `json:"base_url"`
				Token    string `json:"token"`
				AdminKey string `json:"admin_key"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, errs.Wrap(errs.CodeInvalidArgument, "参数无效", err)
			}
			backend, ok := deps.WechatBackend.(wechatConfigurator)
			if !ok {
				return nil, errs.New(errs.CodeInvalidArgument, "微信后端未实现配置接口")
			}
			if p.BaseURL != "" && p.AdminKey != "" {
				backend.Reconfigure(p.BaseURL, p.AdminKey)
			}
			return json.Marshal(map[string]string{"status": "updated"})
		},
	})

	gw.Register(MethodDef{
		Name:        "channel.wechat.status",
		Description: "查询微信 WeChatPadPro 登录状态",
		AuthScope:   "read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			backend, ok := deps.WechatBackend.(wechatConfigurator)
			if !ok {
				return nil, errs.New(errs.CodeInvalidArgument, "微信后端未实现配置接口")
			}
			return json.Marshal(map[string]bool{"logged_in": backend.IsLoggedIn()})
		},
	})
}
