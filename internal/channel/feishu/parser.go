package feishu

import (
	"encoding/json"
	"strings"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

// ParsedMessageType 标识解析后的消息分类。
type ParsedMessageType string

const (
	ParsedText         ParsedMessageType = "text"
	ParsedPost         ParsedMessageType = "post"
	ParsedImage        ParsedMessageType = "image"
	ParsedFile         ParsedMessageType = "file"
	ParsedAudio        ParsedMessageType = "audio"
	ParsedVideo        ParsedMessageType = "video"
	ParsedSticker      ParsedMessageType = "sticker"
	ParsedLocation     ParsedMessageType = "location"
	ParsedShareChat    ParsedMessageType = "share_chat"
	ParsedShareUser    ParsedMessageType = "share_user"
	ParsedMergeForward ParsedMessageType = "merge_forward"
	ParsedSystem       ParsedMessageType = "system"
	ParsedInteractive  ParsedMessageType = "interactive"
	ParsedUnknown      ParsedMessageType = "unknown"
)

// Attachment 是消息中可下载的资源引用。
type Attachment struct {
	Type     string `json:"type"`                // image / file / audio / video / media
	Key      string `json:"key"`                 // image_key / file_key / media_key
	FileName string `json:"file_name,omitempty"` // 文件名（仅 file 类型有）
}

// ParsedMessage 是 ParseInboundMessage 的结构化输出。
// TextContent 始终有值（人类可读摘要），Attachments 仅在消息含可下载资源时非空。
type ParsedMessage struct {
	Type        ParsedMessageType `json:"type"`
	TextContent string            `json:"text_content"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	References  []imctx.DocRef    `json:"references,omitempty"`
	RawJSON     string            `json:"raw_json,omitempty"`
}

// ParseInboundMessage 将飞书原始 messageType + contentJSON 解析为结构化 ParsedMessage。
// 未知类型保守降级为文本占位符，不 panic。
func ParseInboundMessage(messageType, contentJSON string) ParsedMessage {
	var parsed ParsedMessage
	switch messageType {
	case "text":
		parsed = parseText(contentJSON)
	case "post":
		parsed = parsePost(contentJSON)
	case "image":
		parsed = parseImage(contentJSON)
	case "file":
		parsed = parseFile(contentJSON)
	case "audio":
		parsed = parseAudio(contentJSON)
	case "video", "media":
		parsed = parseMedia(messageType, contentJSON)
	case "sticker":
		parsed = ParsedMessage{Type: ParsedSticker, TextContent: "[表情消息]"}
	case "location":
		parsed = parseLocation(contentJSON)
	case "share_chat":
		parsed = parseShareChat(contentJSON)
	case "share_user":
		parsed = parseShareUser(contentJSON)
	case "merge_forward":
		parsed = parseMergeForward(contentJSON)
	case "system":
		parsed = parseSystem(contentJSON)
	case "interactive":
		parsed = ParsedMessage{
			Type:        ParsedInteractive,
			TextContent: "[卡片消息]",
			References:  extractRefsFromAnyJSON(contentJSON, "card"),
			RawJSON:     contentJSON,
		}
	default:
		if messageType != "" {
			parsed = ParsedMessage{Type: ParsedUnknown, TextContent: "[" + messageType + " 消息]"}
		} else {
			parsed = ParsedMessage{Type: ParsedUnknown, TextContent: ""}
		}
	}

	// 通用兜底：无论消息类型为何，都再扫描一遍原始 JSON 里的字符串字段。
	// 这样 text/post 之外的新消息类型只要埋了标准飞书文档 URL，也能先识别出来。
	if fallbackRefs := extractRefsFromAnyJSON(contentJSON, string(parsed.Type)); len(fallbackRefs) > 0 {
		parsed.References = deduplicateRefs(append(parsed.References, fallbackRefs...))
	}
	if parsed.RawJSON == "" && contentJSON != "" && parsed.Type == ParsedInteractive {
		parsed.RawJSON = contentJSON
	}
	return parsed
}

func parseText(contentJSON string) ParsedMessage {
	var tc FeishuTextContent
	if json.Unmarshal([]byte(contentJSON), &tc) == nil {
		return ParsedMessage{Type: ParsedText, TextContent: tc.Text, References: extractRefsFromText(tc.Text)}
	}
	return ParsedMessage{Type: ParsedText, TextContent: contentJSON}
}

func parseImage(contentJSON string) ParsedMessage {
	var ic FeishuImageContent
	if json.Unmarshal([]byte(contentJSON), &ic) == nil && ic.ImageKey != "" {
		return ParsedMessage{
			Type:        ParsedImage,
			TextContent: "[图片消息 image_key=" + ic.ImageKey + "]",
			Attachments: []Attachment{{Type: "image", Key: ic.ImageKey}},
		}
	}
	return ParsedMessage{Type: ParsedImage, TextContent: "[图片消息]"}
}

func parseFile(contentJSON string) ParsedMessage {
	var fc FeishuFileContent
	if json.Unmarshal([]byte(contentJSON), &fc) == nil && fc.FileKey != "" {
		return ParsedMessage{
			Type:        ParsedFile,
			TextContent: "[文件: " + fc.FileName + "]",
			Attachments: []Attachment{{Type: "file", Key: fc.FileKey, FileName: fc.FileName}},
		}
	}
	return ParsedMessage{Type: ParsedFile, TextContent: "[文件消息]"}
}

func parseAudio(contentJSON string) ParsedMessage {
	var ac struct {
		FileKey  string `json:"file_key"`
		Duration int    `json:"duration"`
	}
	if json.Unmarshal([]byte(contentJSON), &ac) == nil && ac.FileKey != "" {
		return ParsedMessage{
			Type:        ParsedAudio,
			TextContent: "[语音消息]",
			Attachments: []Attachment{{Type: "audio", Key: ac.FileKey}},
		}
	}
	return ParsedMessage{Type: ParsedAudio, TextContent: "[语音消息]"}
}

func parseMedia(messageType, contentJSON string) ParsedMessage {
	var mc struct {
		FileKey  string `json:"file_key"`
		ImageKey string `json:"image_key"`
		FileName string `json:"file_name"`
	}
	if json.Unmarshal([]byte(contentJSON), &mc) == nil {
		var atts []Attachment
		if mc.FileKey != "" {
			atts = append(atts, Attachment{Type: "media", Key: mc.FileKey, FileName: mc.FileName})
		}
		if mc.ImageKey != "" {
			atts = append(atts, Attachment{Type: "image", Key: mc.ImageKey})
		}
		if len(atts) > 0 {
			return ParsedMessage{Type: ParsedVideo, TextContent: "[视频消息]", Attachments: atts}
		}
	}
	_ = messageType
	return ParsedMessage{Type: ParsedVideo, TextContent: "[视频消息]"}
}

func parsePost(contentJSON string) ParsedMessage {
	var wrapper FeishuPostWrapper
	if json.Unmarshal([]byte(contentJSON), &wrapper) != nil {
		return ParsedMessage{Type: ParsedPost, TextContent: "[富文本消息]"}
	}
	post := wrapper.ZhCN
	if post == nil {
		post = wrapper.EnUS
	}
	if post == nil {
		return ParsedMessage{Type: ParsedPost, TextContent: "[富文本消息]"}
	}

	var sb strings.Builder
	var atts []Attachment
	var refs []imctx.DocRef
	if post.Title != "" {
		sb.WriteString(post.Title)
		sb.WriteString("\n")
	}
	for _, line := range post.Content {
		for _, entry := range line {
			switch entry.Tag {
			case "text":
				sb.WriteString(entry.Text)
				refs = append(refs, extractRefsFromText(entry.Text)...)
			case "a":
				sb.WriteString(entry.Text)
				if entry.Href != "" {
					sb.WriteString("(" + entry.Href + ")")
					if ref, ok := parseDocURL(entry.Href); ok {
						refs = append(refs, ref)
					}
				}
			case "at":
				if entry.UserName != "" {
					sb.WriteString("@" + entry.UserName)
				}
			case "img":
				sb.WriteString("[图片]")
				if entry.ImageKey != "" {
					atts = append(atts, Attachment{Type: "image", Key: entry.ImageKey})
				}
			}
		}
		sb.WriteString("\n")
	}
	return ParsedMessage{
		Type:        ParsedPost,
		TextContent: strings.TrimSpace(sb.String()),
		Attachments: atts,
		References:  deduplicateRefs(refs),
	}
}

func parseLocation(contentJSON string) ParsedMessage {
	var loc struct {
		Name      string `json:"name"`
		Longitude string `json:"longitude"`
		Latitude  string `json:"latitude"`
	}
	if json.Unmarshal([]byte(contentJSON), &loc) == nil && loc.Name != "" {
		return ParsedMessage{Type: ParsedLocation, TextContent: "[位置: " + loc.Name + "]"}
	}
	return ParsedMessage{Type: ParsedLocation, TextContent: "[位置消息]"}
}

func parseShareChat(contentJSON string) ParsedMessage {
	var sc struct {
		ChatID string `json:"chat_id"`
	}
	if json.Unmarshal([]byte(contentJSON), &sc) == nil && sc.ChatID != "" {
		return ParsedMessage{
			Type:        ParsedShareChat,
			TextContent: "[分享群聊 chat_id=" + sc.ChatID + "]",
			References:  extractRefsFromAnyJSON(contentJSON, "share_chat"),
		}
	}
	return ParsedMessage{
		Type:        ParsedShareChat,
		TextContent: "[分享群聊]",
		References:  extractRefsFromAnyJSON(contentJSON, "share_chat"),
	}
}

func parseShareUser(contentJSON string) ParsedMessage {
	var su struct {
		UserID string `json:"user_id"`
	}
	if json.Unmarshal([]byte(contentJSON), &su) == nil && su.UserID != "" {
		return ParsedMessage{
			Type:        ParsedShareUser,
			TextContent: "[分享名片]",
			References:  extractRefsFromAnyJSON(contentJSON, "share_user"),
		}
	}
	return ParsedMessage{
		Type:        ParsedShareUser,
		TextContent: "[分享名片]",
		References:  extractRefsFromAnyJSON(contentJSON, "share_user"),
	}
}

func parseMergeForward(contentJSON string) ParsedMessage {
	var mf struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(contentJSON), &mf) == nil && mf.Title != "" {
		return ParsedMessage{
			Type:        ParsedMergeForward,
			TextContent: "[合并转发: " + mf.Title + "]",
			References:  extractRefsFromAnyJSON(contentJSON, "merge_forward"),
		}
	}
	return ParsedMessage{
		Type:        ParsedMergeForward,
		TextContent: "[合并转发消息]",
		References:  extractRefsFromAnyJSON(contentJSON, "merge_forward"),
	}
}

func parseSystem(contentJSON string) ParsedMessage {
	var sys struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(contentJSON), &sys) == nil && sys.Type != "" {
		return ParsedMessage{Type: ParsedSystem, TextContent: "[系统消息: " + sys.Type + "]"}
	}
	return ParsedMessage{Type: ParsedSystem, TextContent: "[系统消息]"}
}
