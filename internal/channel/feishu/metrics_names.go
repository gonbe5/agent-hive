package feishu

import "github.com/chef-guo/agents-hive/internal/channel"

const (
	MetricResolverDuration      = channel.MetricFeishuResolverDuration
	MetricInboundRefsCount      = channel.MetricFeishuInboundRefsCount
	MetricUserCacheHit          = "feishu.user_cache.hit"
	MetricUserCacheMiss         = "feishu.user_cache.miss"
	MetricBotDegraded           = "feishu.bot.degraded"
	MetricOutboundRejected      = "feishu.outbound.rejected"
	MetricOutboundDeadLetter    = "feishu.outbound.dead_letter"
	MetricWebhookSecurityReject = "feishu.webhook.security_reject"
	MetricLifecycleEventCount   = "feishu.lifecycle.event_count"
	MetricLifecycleWelcomeSent  = "feishu.lifecycle.welcome_sent"
	MetricHITLCallbackStatus    = "feishu.hitl.callback_status"
)
