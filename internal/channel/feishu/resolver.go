package feishu

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/imctx"
)

// DefaultResolverTimeout 是 ContextResolver 每个 API 调用的默认超时。
const DefaultResolverTimeout = 2500 * time.Millisecond

// ContextResolver 实现 channel.InboundContextResolver，
// 在 dedup/debounce 之后、dispatchProcess 之前丰富飞书消息上下文。
//
// 核心职责：
//  1. 父消息解析：拉取 parent 正文 + refs，自反射防御（丢弃 bot 自己的回复）
//  2. wiki token 转换：wiki → 真实 obj_token + obj_type
//  3. 构建 SystemPromptPrefix（XML 格式）
type ContextResolver struct {
	client     *Client
	logger     *zap.Logger
	timeout    time.Duration // 每个 API 调用的超时
	userCache  *userCache
	nameLocale string
	region     string

	// 缓存 bot open_id，避免每次 Resolve 都调 API
	botOpenIDOnce sync.Once
	botOpenID     string
}

// NewContextResolver 创建飞书 ContextResolver。
func NewContextResolver(client *Client, logger *zap.Logger) *ContextResolver {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ContextResolver{
		client:     client,
		logger:     logger,
		timeout:    DefaultResolverTimeout,
		userCache:  newUserCache(5000, defaultUserCacheTTL),
		nameLocale: "zh-CN",
	}
}

// WithTimeout 设置每个 API 调用的超时时间。
func (r *ContextResolver) WithTimeout(d time.Duration) *ContextResolver {
	r.timeout = d
	return r
}

func (r *ContextResolver) WithIdentityConfig(cfg config.FeishuIdentityConfig) *ContextResolver {
	size := cfg.UserCacheSize
	if size <= 0 {
		size = 5000
	}
	ttl := time.Duration(cfg.UserCacheTTLSec) * time.Second
	if ttl <= 0 {
		ttl = defaultUserCacheTTL
	}
	r.userCache = newUserCache(size, ttl)
	return r
}

func (r *ContextResolver) WithNameLocale(locale string) *ContextResolver {
	if locale != "" {
		r.nameLocale = locale
	}
	return r
}

func (r *ContextResolver) WithRegion(region string) *ContextResolver {
	r.region = region
	if r.nameLocale == "" || r.nameLocale == "zh-CN" || r.nameLocale == "en-US" {
		r.nameLocale = localeForRegion(region)
	}
	return r
}

// cachedBotOpenID 懒加载 bot open_id。失败时返回空字符串（降级：不做自反射过滤）。
func (r *ContextResolver) cachedBotOpenID(ctx context.Context) string {
	r.botOpenIDOnce.Do(func() {
		_ = ctx
		id := r.client.BotOpenID()
		if id == "" {
			r.logger.Warn("resolver: 获取 bot open_id 失败，自反射防御降级")
			return
		}
		r.botOpenID = id
	})
	return r.botOpenID
}

// Resolve 实现 channel.InboundContextResolver 接口。
// 所有 API 调用失败都 degrade（warn + 继续），绝不阻断消息处理。
func (r *ContextResolver) Resolve(ctx context.Context, msg *channel.InboundMessage) (*imctx.IMMessageContext, error) {
	if r.userCache != nil {
		r.userCache.WithTenantKey(msg.TenantKey)
	}
	r.resolveUserIdentity(ctx, msg)

	out := &imctx.IMMessageContext{
		Platform:         imctx.Platform(msg.Platform),
		TenantKey:        msg.TenantKey,
		ChannelMessageID: msg.MessageID,
		ChatID:           msg.ChatID,
		SafeSenderID:     imctx.SafeSenderID(msg.SenderID),
		ReceivedAt:       msg.Timestamp,
		References:       copyRefs(msg.References),
		Mentions:         msg.Mentions,
		BotMentioned:     msg.BotMentioned,
	}

	// 1) 父消息解析
	if msg.ParentID != "" {
		r.resolveParent(ctx, msg.ParentID, msg.RootID, out)
	}

	// 2) wiki token 转换
	r.resolveWikiTokens(ctx, out)

	// 3) 构建 SystemPromptPrefix
	out.SystemPromptPrefix = buildSystemPromptPrefix(out)

	// 诊断观测：resolver 在成功路径下原本完全静默（只在失败 warn）。
	// 这就让"用户引用了 wiki 卡片但 ref 抽不到"这种诡异场景从日志里完全看不出来。
	// 这条 log 暴露 refs 数 / parent 是否拉到 / prefix 长度，方便排障。
	r.logger.Info("resolver: 完成上下文解析",
		zap.String("message_id", msg.MessageID),
		zap.Int("refs_count", len(out.References)),
		zap.Bool("parent_resolved", out.ParentMessageID != ""),
		zap.Int("parent_content_len", len(out.ParentContent)),
		zap.Int("prefix_len", len(out.SystemPromptPrefix)))

	return out, nil
}

func (r *ContextResolver) resolveUserIdentity(ctx context.Context, msg *channel.InboundMessage) {
	if msg == nil || r.userCache == nil || r.client == nil {
		return
	}
	if msg.SenderID != "" && (msg.SenderName == "" || msg.SenderName == msg.SenderID) {
		callCtx, cancel := context.WithTimeout(ctx, r.timeout)
		user, err := r.userCache.GetOrFetch(callCtx, msg.SenderID, r.client)
		cancel()
		if err == nil && user != nil {
			if name := r.displayName(user); name != "" {
				msg.SenderName = name
			}
		}
	}
	for i := range msg.Mentions {
		if msg.Mentions[i].OpenID == "" {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, r.timeout)
		user, err := r.userCache.GetOrFetch(callCtx, msg.Mentions[i].OpenID, r.client)
		cancel()
		if err == nil && user != nil {
			if name := r.displayName(user); name != "" {
				msg.Mentions[i].Name = name
			}
		}
	}
}

func (r *ContextResolver) displayName(user *UserDetail) string {
	if user == nil {
		return ""
	}
	if r.nameLocale == "en-US" && user.EnName != "" {
		return user.EnName
	}
	if user.Name != "" {
		return user.Name
	}
	return user.EnName
}

func localeForRegion(region string) string {
	switch region {
	case "intl", "lark", "international":
		return "en-US"
	default:
		return "zh-CN"
	}
}

// resolveParent 拉取父消息正文，填充 ParentMessageID / ParentContent / References。
// 失败 → warn + degrade；bot 自反射 → 丢弃。
func (r *ContextResolver) resolveParent(ctx context.Context, parentID, rootID string, out *imctx.IMMessageContext) {
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	parent, err := r.client.GetMessageContent(callCtx, parentID)
	if err != nil {
		r.logger.Warn("resolver: 父消息拉取失败，降级跳过",
			zap.String("parent_id", parentID),
			zap.Error(err))
		return
	}

	// 自反射防御：bot 自己的回复不当作用户 context
	botID := r.cachedBotOpenID(ctx)
	if botID != "" && parent.SenderOpenID == botID {
		r.logger.Debug("resolver: 丢弃父消息（bot 自反射）",
			zap.String("parent_id", parentID))
		return
	}

	out.ParentMessageID = parentID
	out.ParentContent = parent.Text
	// 合并父消息的 refs，统一标记 source="parent"
	for i := range parent.Refs {
		parent.Refs[i].Source = "parent"
	}
	out.References = appendUniqueRefs(out.References, parent.Refs...)

	r.logger.Info("resolver: 父消息解析完成",
		zap.String("parent_id", parentID),
		zap.String("root_id", rootID),
		zap.String("parent_msg_type", parent.MessageType),
		zap.Int("parent_raw_content_len", len(parent.RawContent)),
		zap.Int("parent_text_len", len(parent.Text)),
		zap.Int("parent_refs_count", len(parent.Refs)),
		zap.Strings("parent_refs_summary", formatRefsForLog(parent.Refs)))

	// 群里“引用消息”常出现两层结构：
	// parent_id 指向的是系统生成的引用摘要，root_id 才是原始那条带文档链接的消息。
	// 当 parent 没抽到 refs 且 root 与 parent 不同，继续补拉 root，避免把真实文档线索丢掉。
	if len(parent.Refs) == 0 && rootID != "" && rootID != parentID {
		r.resolveRootFallback(ctx, rootID, out)
	}
}

func (r *ContextResolver) resolveRootFallback(ctx context.Context, rootID string, out *imctx.IMMessageContext) {
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	root, err := r.client.GetMessageContent(callCtx, rootID)
	if err != nil {
		r.logger.Warn("resolver: 根消息拉取失败，跳过 root fallback",
			zap.String("root_id", rootID),
			zap.Error(err))
		return
	}

	for i := range root.Refs {
		root.Refs[i].Source = "root"
	}
	out.References = appendUniqueRefs(out.References, root.Refs...)

	r.logger.Info("resolver: 根消息 fallback 完成",
		zap.String("root_id", rootID),
		zap.String("root_msg_type", root.MessageType),
		zap.Int("root_raw_content_len", len(root.RawContent)),
		zap.Int("root_text_len", len(root.Text)),
		zap.Int("root_refs_count", len(root.Refs)),
		zap.Strings("root_refs_summary", formatRefsForLog(root.Refs)))
}

// resolveWikiTokens 遍历 References，将 wiki token 转换为真实 obj_token + obj_type。
func (r *ContextResolver) resolveWikiTokens(ctx context.Context, out *imctx.IMMessageContext) {
	for i := range out.References {
		ref := &out.References[i]
		if ref.Type != imctx.RefWiki || ref.Token == "" {
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, r.timeout)
		objToken, objType, err := r.client.GetWikiNodeInfo(callCtx, ref.Token)
		cancel()

		if err != nil {
			r.logger.Warn("resolver: wiki token 转换失败，保留原 token",
				zap.String("wiki_token", ref.Token),
				zap.Error(err))
			continue
		}

		ref.Token = objToken
		ref.Type = imctx.NormalizeDocType(objType)
	}
}

// copyRefs 浅拷贝 refs slice，避免修改原始 msg.References。
func copyRefs(refs []imctx.DocRef) []imctx.DocRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]imctx.DocRef, len(refs))
	copy(out, refs)
	return out
}

// appendUniqueRefs 将 extra refs 追加到 base，按 {Token, Type} 去重。
func appendUniqueRefs(base []imctx.DocRef, extra ...imctx.DocRef) []imctx.DocRef {
	if len(extra) == 0 {
		return base
	}
	type key struct {
		Token string
		Type  imctx.ReferenceType
	}
	seen := make(map[key]struct{}, len(base))
	for _, r := range base {
		seen[key{r.Token, r.Type}] = struct{}{}
	}
	for _, r := range extra {
		k := key{r.Token, r.Type}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		base = append(base, r)
	}
	return base
}
