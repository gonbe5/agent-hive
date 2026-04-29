package feishu

import (
	"encoding/json"
	"strings"
)

// FeishuEvent 飞书事件回调请求体
type FeishuEvent struct {
	Schema    string        `json:"schema"`
	Header    *EventHeader  `json:"header"`
	Event     *MessageEvent `json:"event,omitempty"`
	Challenge string        `json:"challenge,omitempty"` // URL 验证
	Token     string        `json:"token,omitempty"`
	Type      string        `json:"type,omitempty"` // "url_verification"
}

// EventHeader 事件头
type EventHeader struct {
	EventID    string `json:"event_id"`
	Token      string `json:"token"`
	CreateTime string `json:"create_time"`
	EventType  string `json:"event_type"`
}

// MessageEvent 消息事件
type MessageEvent struct {
	Sender  *Sender  `json:"sender"`
	Message *Message `json:"message"`
}

// Sender 发送者
type Sender struct {
	SenderID   *SenderID `json:"sender_id"`
	SenderType string    `json:"sender_type"`
}

// SenderID 发送者 ID
type SenderID struct {
	OpenID string `json:"open_id"`
}

// Message 消息
type Message struct {
	MessageID   string `json:"message_id"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"` // "p2p" 或 "group"
	Content     string `json:"content"`   // JSON 字符串
	MessageType string `json:"message_type"`
}

// FeishuTextContent 飞书文本消息内容（JSON 解析后）
type FeishuTextContent struct {
	Text string `json:"text"`
}

// FeishuImageContent 飞书图片消息内容
type FeishuImageContent struct {
	ImageKey string `json:"image_key"`
}

// FeishuFileContent 飞书文件消息内容
type FeishuFileContent struct {
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
}

// FeishuPostContent 飞书富文本消息内容
type FeishuPostContent struct {
	Title   string              `json:"title"`
	Content [][]FeishuPostEntry `json:"content"`
}

// FeishuPostEntry 富文本消息的单个元素
type FeishuPostEntry struct {
	Tag      string `json:"tag"`
	Text     string `json:"text,omitempty"`
	Href     string `json:"href,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	UserName string `json:"user_name,omitempty"`
	ImageKey string `json:"image_key,omitempty"`
}

// FeishuPostWrapper 富文本消息的多语言包装
type FeishuPostWrapper struct {
	ZhCN *FeishuPostContent `json:"zh_cn,omitempty"`
	EnUS *FeishuPostContent `json:"en_us,omitempty"`
}

// FeishuReply 飞书回复消息格式
type FeishuReply struct {
	MsgType string `json:"msg_type"`
	Content string `json:"content"` // JSON 字符串
}

// --- 飞书 Tool 相关类型 ---

// DocItem 文档搜索结果
type DocItem struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	DocToken   string `json:"docs_token"`
	DocType    string `json:"docs_type"` // doc/docx/sheet/bitable/mindnote/wiki
	OwnerID    string `json:"owner_id"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

// ContactItem 通讯录搜索结果
type ContactItem struct {
	UserID     string `json:"user_id"`
	OpenID     string `json:"open_id"`
	Name       string `json:"name"`
	Department string `json:"department"`
	Email      string `json:"email"`
	Mobile     string `json:"mobile"`
	Avatar     string `json:"avatar"`
	Status     string `json:"status"`
}

// UserDetail 用户详细信息
type UserDetail struct {
	UserID        string   `json:"user_id"`
	OpenID        string   `json:"open_id"`
	Name          string   `json:"name"`
	EnName        string   `json:"en_name"`
	Email         string   `json:"email"`
	Mobile        string   `json:"mobile"`
	Avatar        string   `json:"avatar"`
	DepartmentIDs []string `json:"department_ids"`
	JobTitle      string   `json:"job_title"`
	WorkStation   string   `json:"work_station"`
	City          string   `json:"city"`
	EmployeeType  int      `json:"employee_type"`
}

// CalendarEvent 日历事件
type CalendarEvent struct {
	EventID     string   `json:"event_id"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	Location    string   `json:"location"`
	Organizer   string   `json:"organizer"`
	Status      string   `json:"status"` // tentative/confirmed/cancelled
	Attendees   []string `json:"attendees"`
}

// ExtractMessageContent 从飞书消息中提取文本内容
// 支持 text、post（富文本）、image、file、audio、video、sticker 等类型
// 非文本消息转换为描述性文本，确保 AI 能理解上下文
func ExtractMessageContent(messageType, contentJSON string) string {
	switch messageType {
	case "text":
		var tc FeishuTextContent
		if json.Unmarshal([]byte(contentJSON), &tc) == nil {
			return tc.Text
		}
	case "post":
		return extractPostContent(contentJSON)
	case "image":
		var ic FeishuImageContent
		if json.Unmarshal([]byte(contentJSON), &ic) == nil {
			return "[图片消息 image_key=" + ic.ImageKey + "]"
		}
	case "file":
		var fc FeishuFileContent
		if json.Unmarshal([]byte(contentJSON), &fc) == nil {
			return "[文件: " + fc.FileName + "]"
		}
	case "audio":
		return "[语音消息]"
	case "video":
		return "[视频消息]"
	case "sticker":
		return "[表情消息]"
	case "interactive":
		return "[卡片消息]"
	case "share_chat":
		return "[分享群聊]"
	case "share_user":
		return "[分享名片]"
	case "system":
		return "[系统消息]"
	case "merge_forward":
		return extractMergeForward(contentJSON)
	}
	// 未知类型兜底
	if messageType != "" {
		return "[" + messageType + " 消息]"
	}
	return ""
}

// extractPostContent 从富文本消息中提取纯文本
func extractPostContent(contentJSON string) string {
	var wrapper FeishuPostWrapper
	if json.Unmarshal([]byte(contentJSON), &wrapper) != nil {
		return "[富文本消息]"
	}

	// 优先中文，其次英文
	post := wrapper.ZhCN
	if post == nil {
		post = wrapper.EnUS
	}
	if post == nil {
		return "[富文本消息]"
	}

	var sb strings.Builder
	if post.Title != "" {
		sb.WriteString(post.Title)
		sb.WriteString("\n")
	}
	for _, line := range post.Content {
		for _, entry := range line {
			switch entry.Tag {
			case "text":
				sb.WriteString(entry.Text)
			case "a":
				sb.WriteString(entry.Text)
				if entry.Href != "" {
					sb.WriteString("(" + entry.Href + ")")
				}
			case "at":
				if entry.UserName != "" {
					sb.WriteString("@" + entry.UserName)
				}
			case "img":
				sb.WriteString("[图片]")
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// extractMergeForward 从合并转发消息中提取概要
func extractMergeForward(contentJSON string) string {
	var mf struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(contentJSON), &mf) == nil && mf.Title != "" {
		return "[合并转发: " + mf.Title + "]"
	}
	return "[合并转发消息]"
}
