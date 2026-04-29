package dingtalk

// DingTalkCallback 钉钉回调请求体
type DingTalkCallback struct {
	MsgType            string   `json:"msgtype"`
	Text               *TextMsg `json:"text,omitempty"`
	ConversationID     string   `json:"conversationId"`
	ConversationType   string   `json:"conversationType"` // "1"=私聊, "2"=群聊
	SenderID           string   `json:"senderId"`
	SenderNick         string   `json:"senderNick"`
	ChatbotUserID      string   `json:"chatbotUserId"`
	MsgID              string   `json:"msgId"`
	IsInAtList         bool     `json:"isInAtList"`
	SessionWebhook     string   `json:"sessionWebhook"`
}

type TextMsg struct {
	Content string `json:"content"`
}

// DingTalkResponse 钉钉回复消息格式
type DingTalkResponse struct {
	MsgType string   `json:"msgtype"`
	Text    *TextMsg `json:"text"`
}
