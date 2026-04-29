package feishu

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// 飞书 / Lark Open API endpoint。
//
// SDK 默认连 open.feishu.cn(中国大陆飞书租户),无需显式 OpenBaseUrl。
// Lark 海外租户(Lark international / Lark Suite)必须切到 open.larksuite.com,
// 否则 SDK 走默认路径会拿到 401 / 跨区拒绝。
const feishuOpenBaseURLLark = "https://open.larksuite.com"

// feishuClientOptionsForRegion 按 cfg.Region 返回 lark SDK 创建客户端的额外 options。
//
// Phase 7 P0:Region=intl/lark/international 必须传 lark.WithOpenBaseUrl 切端点,
// 否则 Lark 客户的 webhook / token 拉取全部失败。
//
// 与 config.IdentityNameLocaleResolved 的 region 取值口径保持一致(intl/lark/international)。
// 未识别的 region(空 / cn / 其他)走默认 open.feishu.cn,SDK 不需额外 option。
//
// 返回 nil slice 是合法的:NewClient(..., nil...) 等价于不传额外 options。
func feishuClientOptionsForRegion(region string) []lark.ClientOptionFunc {
	switch region {
	case "intl", "lark", "international":
		return []lark.ClientOptionFunc{lark.WithOpenBaseUrl(feishuOpenBaseURLLark)}
	default:
		return nil
	}
}
