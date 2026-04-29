package wechat

import "time"

// MsgType 微信消息类型
type MsgType int

const (
	MsgText    MsgType = 1     // 文本消息
	MsgImage   MsgType = 3     // 图片消息
	MsgVoice   MsgType = 34    // 语音消息
	MsgVideo   MsgType = 43    // 视频消息
	MsgEmoticon MsgType = 47   // 表情消息
	MsgLocation MsgType = 48   // 位置消息
	MsgLink    MsgType = 49    // 链接/文件消息
	MsgSystem  MsgType = 10000 // 系统消息
)

// IncomingMessage 从微信协议层收到的统一消息
type IncomingMessage struct {
	MsgID      string    `json:"msg_id"`
	MsgType    MsgType   `json:"msg_type"`
	FromUser   string    `json:"from_user"`   // 发送者微信 ID
	FromGroup  string    `json:"from_group"`  // 群聊 ID（私聊为空）
	Content    string    `json:"content"`
	SenderName string    `json:"sender_name"` // 发送者昵称
	Timestamp  time.Time `json:"timestamp"`
}

// IsGroup 判断是否为群聊消息
func (m IncomingMessage) IsGroup() bool {
	return m.FromGroup != ""
}

// ChatID 返回用于路由的聊天 ID（群聊返回群 ID，私聊返回发送者 ID）
func (m IncomingMessage) ChatID() string {
	if m.FromGroup != "" {
		return m.FromGroup
	}
	return m.FromUser
}
