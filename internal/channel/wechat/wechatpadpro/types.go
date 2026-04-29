package wechatpadpro

// APIResponse WeChatPadPro API 统一响应格式
// 所有 API 返回格式：{"Code": 200, "Data": {...}, "Text": "..."}
type APIResponse struct {
	Code int         `json:"Code"` // 200 表示成功
	Text string      `json:"Text"` // 响应消息
	Data interface{} `json:"Data,omitempty"`
}

// LoginStatusData 登录状态查询响应数据
type LoginStatusData struct {
	IsLogin  bool   `json:"isLogin"`          // 是否已登录
	WxID     string `json:"wxid,omitempty"`   // 微信 ID
	Nickname string `json:"nickname,omitempty"` // 昵称
}

// QRCodeData 二维码响应数据
type QRCodeData struct {
	Key           string `json:"Key"`           // 授权 Key
	QrCodeUrl     string `json:"QrCodeUrl"`     // 二维码图片 URL
	QrLink        string `json:"QrLink"`        // 微信登录链接
	QrCodeBase64  string `json:"qrCodeBase64"`  // Base64 编码的二维码
	ExpiredTime   int    `json:"expiredTime"`   // 过期时间（秒）
}

// GetLoginQrCodeModel 获取登录二维码请求参数
type GetLoginQrCodeModel struct {
	Proxy string `json:"Proxy,omitempty"` // socks代理，例如：socks5://username:password@ipv4:port
	Check bool   `json:"Check,omitempty"` // 是否发送检测代理请求
}

// MessageItem 消息体
type MessageItem struct {
	ToUserName   string   `json:"ToUserName"`             // 接收者 wxid
	TextContent  string   `json:"TextContent,omitempty"`  // 文本消息内容
	ImageContent string   `json:"ImageContent,omitempty"` // 图片 base64
	MsgType      int      `json:"MsgType"`                // 1=文本, 2=图片
	AtWxIDList   []string `json:"AtWxIDList,omitempty"`   // @的 wxid 列表
}

// SendMessageModel 发送消息请求参数
type SendMessageModel struct {
	MsgItem []MessageItem `json:"MsgItem"` // 消息体数组
}

// WebSocketMessage WebSocket 推送消息格式（如果使用 WebSocket）
type WebSocketMessage struct {
	Type      string     `json:"type"`      // "message" | "login" | "logout"
	Timestamp int64      `json:"timestamp"`
	Data      *WSMsgData `json:"data,omitempty"`
}

// WSMsgData WebSocket 消息数据
type WSMsgData struct {
	MsgID      string `json:"msg_id"`
	MsgType    int    `json:"msg_type"`    // 1=文本, 3=图片, ...
	FromWxID   string `json:"from_wxid"`   // 发送者 ID
	FromName   string `json:"from_name"`   // 发送者昵称
	RoomWxID   string `json:"room_wxid"`   // 群聊 ID（私聊为空）
	Content    string `json:"content"`     // 消息内容
	CreateTime int64  `json:"create_time"` // 时间戳
}

// IsTextMessage 判断是否为文本消息
func (m *WSMsgData) IsTextMessage() bool {
	return m.MsgType == 1
}

// IsGroupMessage 判断是否为群聊消息
func (m *WSMsgData) IsGroupMessage() bool {
	return m.RoomWxID != ""
}

// GetChatID 获取聊天 ID（群聊返回群 ID，私聊返回发送者 ID）
func (m *WSMsgData) GetChatID() string {
	if m.RoomWxID != "" {
		return m.RoomWxID
	}
	return m.FromWxID
}
