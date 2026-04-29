package feishu

import (
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

// extractMentions 从飞书 SDK 的 MentionEvent 列表中提取结构化 mentions 和 botMentioned 标志。
// botOpenID 为空时降级为"任意 mention 即视为 bot mentioned"。
func extractMentions(sdkMentions []*larkim.MentionEvent, botOpenID string) ([]imctx.Mention, bool) {
	if len(sdkMentions) == 0 {
		return nil, false
	}
	var mentions []imctx.Mention
	botMentioned := false
	for _, m := range sdkMentions {
		if m == nil {
			continue
		}
		var name, openID string
		if m.Name != nil {
			name = *m.Name
		}
		if m.Id != nil && m.Id.OpenId != nil {
			openID = *m.Id.OpenId
		}
		isBot := false
		if botOpenID != "" && openID == botOpenID {
			isBot = true
			botMentioned = true
		}
		mentions = append(mentions, imctx.Mention{
			Name:   name,
			OpenID: openID,
			IsBot:  isBot,
		})
	}
	// 降级策略：botOpenID 未知时，任意 mention 都视为可能 @ 了机器人
	if botOpenID == "" && len(mentions) > 0 {
		botMentioned = true
	}
	return mentions, botMentioned
}
