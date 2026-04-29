package wechat

import (
	"context"
	"net/http"
)

// Protocol 微信协议后端抽象
// 不同协议（openwechat / gewechat / wcferry / wechaty）实现此接口
type Protocol interface {
	// Name 返回协议名称
	Name() string

	// Start 启动协议连接（登录、监听消息等）
	Start(ctx context.Context) error

	// Stop 停止协议连接，释放资源
	Stop() error

	// SendText 向指定聊天发送文本消息
	SendText(ctx context.Context, chatID, content string) error

	// SetMessageHandler 设置消息回调，协议收到消息时调用
	SetMessageHandler(handler MessageHandler)

	// IsLoggedIn 返回当前是否已登录
	IsLoggedIn() bool
}

// MessageHandler 协议层消息回调函数类型
type MessageHandler func(msg IncomingMessage)

// WebhookProvider 可选接口，仅部分协议（如 GeweChat）需要 HTTP 回调
// 未实现此接口的协议，Plugin 的 WebhookHandler() 返回 405
type WebhookProvider interface {
	// WebhookHandler 返回处理外部回调的 HTTP handler
	WebhookHandler() http.HandlerFunc
}
