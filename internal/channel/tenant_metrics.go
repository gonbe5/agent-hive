package channel

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	MetricFeishuResolverDuration = "feishu.resolver.duration_ms"
	MetricFeishuInboundRefsCount = "feishu.inbound.refs_count"
)

func TenantKeyHashLabel(tenantKey string) string {
	if tenantKey == "" {
		tenantKey = defaultTenantKey
	}
	if tenantKey == defaultTenantKey {
		return defaultTenantKey
	}
	sum := sha256.Sum256([]byte(tenantKey))
	return "tk_" + hex.EncodeToString(sum[:4])
}
