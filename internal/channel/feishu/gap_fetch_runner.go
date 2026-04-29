package feishu

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/imctx"
	"github.com/chef-guo/agents-hive/internal/master"
)

type gapFetchPageSource interface {
	ListGapMessages(ctx context.Context, req GapFetchRequest) (GapFetchPageResponse, error)
}

type gapFetchRunner struct {
	router *channel.Router
	client gapFetchPageSource
	logger *zap.Logger
}

func newGapFetchRunner(router *channel.Router, client gapFetchPageSource, logger *zap.Logger) *gapFetchRunner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &gapFetchRunner{
		router: router,
		client: client,
		logger: logger,
	}
}

func (r *gapFetchRunner) ReplayWindow(ctx context.Context, tenantKey string, req GapFetchRequest) error {
	if r == nil || r.router == nil || r.client == nil {
		return nil
	}
	if err := req.Validate(); err != nil {
		return err
	}

	walker := NewGapFetchWalker(r.client)
	pages, err := walker.FetchAll(ctx, req)
	if err != nil {
		return err
	}

	for _, page := range pages {
		for _, item := range page.Items {
			if err := r.replayMessage(ctx, tenantKey, item.Raw); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *gapFetchRunner) replayMessage(ctx context.Context, tenantKey string, msg *larkim.Message) error {
	inbound, claimKey, ok := buildInboundFromGapFetchMessage(msg)
	if !ok {
		return nil
	}
	inbound.Platform = channel.PlatformFeishu
	inbound.TenantKey = tenantKey
	inbound.NoDebounce = true

	claimer := r.router.EventClaimer()
	var claimToken master.ClaimToken
	if claimer != nil {
		tok, claimed := claimer.ClaimEvent(claimKey, master.DefaultClaimLease)
		if !claimed {
			return nil
		}
		claimToken = tok
	}

	if err := r.router.HandleMessage(ctx, inbound); err != nil {
		return err
	}
	if claimer != nil && claimToken.EventID != "" {
		if err := claimer.CompleteEvent(claimToken); err != nil {
			r.logger.Warn("gap fetch CompleteEvent 失败",
				zap.String("claim_key", claimKey),
				zap.Error(err))
		}
	}
	return nil
}

func buildInboundFromGapFetchMessage(msg *larkim.Message) (channel.InboundMessage, string, bool) {
	if msg == nil || msg.MessageId == nil || msg.ChatId == nil || msg.MsgType == nil || msg.Body == nil || msg.Body.Content == nil {
		return channel.InboundMessage{}, "", false
	}

	messageType := *msg.MsgType
	contentJSON := *msg.Body.Content
	parsed := ParseInboundMessage(messageType, contentJSON)

	content := parsed.TextContent
	if messageType == "text" {
		content = resolveListMentions(content, msg.Mentions)
	}

	var attachments []channel.Attachment
	for _, att := range parsed.Attachments {
		attachments = append(attachments, channel.Attachment{
			Type:     att.Type,
			Key:      att.Key,
			FileName: att.FileName,
		})
	}

	senderID, senderName := buildGapFetchSender(msg.Sender)
	mentions, botMentioned := extractListMentions(msg.Mentions, "")

	createdAt := time.Now()
	if msg.CreateTime != nil {
		if millis, err := strconv.ParseInt(*msg.CreateTime, 10, 64); err == nil && millis > 0 {
			createdAt = time.UnixMilli(millis)
		}
	}

	rawData, _ := json.Marshal(msg)

	inbound := channel.InboundMessage{
		MessageID:    *msg.MessageId,
		ChatID:       *msg.ChatId,
		ChatType:     inferGapFetchChatType(msg),
		SenderID:     senderID,
		SenderName:   senderName,
		Content:      content,
		MessageType:  messageType,
		Attachments:  attachments,
		References:   parsed.References,
		ParentID:     strDeref(msg.ParentId),
		RootID:       strDeref(msg.RootId),
		Mentions:     mentions,
		BotMentioned: botMentioned,
		RawData:      rawData,
		Timestamp:    createdAt,
	}
	return inbound, "feishu_gap_fetch:" + inbound.MessageID, true
}

func buildGapFetchSender(sender *larkim.Sender) (string, string) {
	if sender == nil {
		return "", ""
	}
	senderID := strDeref(sender.Id)
	return senderID, senderID
}

func inferGapFetchChatType(msg *larkim.Message) channel.ChatType {
	if msg == nil || msg.ChatId == nil {
		return channel.ChatGroup
	}
	if strings.HasPrefix(*msg.ChatId, "ou_") {
		return channel.ChatDirect
	}
	return channel.ChatGroup
}

func extractListMentions(sdkMentions []*larkim.Mention, botOpenID string) ([]imctx.Mention, bool) {
	if len(sdkMentions) == 0 {
		return nil, false
	}
	mentions := make([]imctx.Mention, 0, len(sdkMentions))
	botMentioned := false
	for _, m := range sdkMentions {
		if m == nil {
			continue
		}
		openID := strDeref(m.Id)
		isBot := botOpenID != "" && openID == botOpenID
		if isBot {
			botMentioned = true
		}
		mentions = append(mentions, imctx.Mention{
			Name:   strDeref(m.Name),
			OpenID: openID,
			IsBot:  isBot,
		})
	}
	if botOpenID == "" && len(mentions) > 0 {
		botMentioned = true
	}
	return mentions, botMentioned
}

func resolveListMentions(text string, mentions []*larkim.Mention) string {
	for _, m := range mentions {
		if m == nil || m.Key == nil || m.Name == nil {
			continue
		}
		text = strings.ReplaceAll(text, *m.Key, "@"+*m.Name)
	}
	return text
}
