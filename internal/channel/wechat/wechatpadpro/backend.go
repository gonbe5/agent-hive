package wechatpadpro

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/channel/wechat"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/chef-guo/agents-hive/internal/errs"
)

// 编译期断言：Backend 实现 WechatOpsProvider 接口
var _ WechatOpsProvider = (*Backend)(nil)

// Backend WeChatPadPro 协议后端实现
type Backend struct {
	cfg      config.WeChatPadProInstanceConfig
	handler  wechat.MessageHandler
	logger   *zap.Logger
	httpCli  *HTTPClient
	wsCli    *WebSocketClient
	loggedIn atomic.Bool
	wxid     string // 当前登录的微信 ID
}

// New 创建 WeChatPadPro Backend
func New(cfg config.WeChatPadProInstanceConfig, logger *zap.Logger) *Backend {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	b := &Backend{
		cfg:    cfg,
		logger: logger,
		httpCli: NewHTTPClient(HTTPClientConfig{
			BaseURL: cfg.BaseURL,
			Key:     cfg.Token, // 配置中的 Token 字段对应 WeChatPadPro 的 Key
			Timeout: timeout,
			Logger:  logger,
		}),
	}
	return b
}

// Name 返回协议名称
func (b *Backend) Name() string {
	return "wechatpadpro"
}

// Start 启动协议连接
func (b *Backend) Start(ctx context.Context) error {
	b.logger.Info("启动 WeChatPadPro 连接",
		zap.String("base_url", b.cfg.BaseURL))

	// 0. 如果配置了 AdminKey 但没有 Token，自动生成
	if b.cfg.Token == "" && b.cfg.AdminKey != "" {
		b.logger.Info("Token 为空，使用 AdminKey 自动生成...")
		keyResp, err := b.httpCli.GenAuthKey(ctx, b.cfg.AdminKey, 30)
		if err != nil {
			return errs.Wrap(errs.CodeWeChatLoginFailed, "自动生成 Key 失败", err)
		}
		// 更新 HTTPClient 的 key
		b.httpCli.key = keyResp.Key
		b.cfg.Token = keyResp.Key
		b.logger.Info("自动生成 Key 成功", zap.String("key", keyResp.Key))
	}

	// 1. 检查登录状态
	status, err := b.httpCli.CheckLoginStatus(ctx)
	if err != nil {
		return errs.Wrap(errs.CodeWeChatLoginFailed, "检查登录状态失败", err)
	}

	if !status.IsLogin {
		// 2. 未登录，获取二维码
		qrCode, err := b.httpCli.GetQRCode(ctx)
		if err != nil {
			return errs.Wrap(errs.CodeWeChatLoginFailed, "获取登录二维码失败", err)
		}

		b.logger.Info("请扫描二维码登录",
			zap.String("qrcode_url", qrCode.QrCodeUrl))

		// 3. 等待登录（轮询，最多等待 2 分钟）
		timeout := time.After(2 * time.Minute)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				return errs.New(errs.CodeWeChatLoginFailed, "登录超时（2分钟）")
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				status, err = b.httpCli.CheckLoginStatus(ctx)
				if err != nil {
					b.logger.Warn("检查登录状态失败", zap.Error(err))
					continue
				}
				if status.IsLogin {
					goto LoggedIn
				}
			}
		}
	}

LoggedIn:
	b.wxid = status.WxID
	b.loggedIn.Store(true)
	b.logger.Info("微信登录成功",
		zap.String("wxid", status.WxID),
		zap.String("nickname", status.Nickname))

	// 4. 启动 WebSocket 消息接收
	// 创建适配器函数：将 Protocol handler (func(msg)) 适配为 WebSocket handler (func(*msg) error)
	wsHandler := func(msg *wechat.IncomingMessage) error {
		if b.handler != nil {
			b.handler(*msg) // 解指针并调用
		}
		return nil
	}

	// 使用默认重连配置
	reconnectCfg := DefaultReconnectConfig()
	b.wsCli = NewWebSocketClient(b.cfg.BaseURL, wsHandler, b.logger, reconnectCfg)
	if err := b.wsCli.Connect(); err != nil {
		return errs.Wrap(errs.CodeWeChatProtocolError, "连接 WebSocket 失败", err)
	}

	return nil
}

// Stop 停止协议连接
func (b *Backend) Stop() error {
	b.logger.Info("停止 WeChatPadPro 连接")

	b.loggedIn.Store(false)

	if b.wsCli != nil {
		if err := b.wsCli.Close(); err != nil {
			b.logger.Warn("关闭 WebSocket 失败", zap.Error(err))
		}
	}

	return nil
}

// SendText 发送文本消息
func (b *Backend) SendText(ctx context.Context, chatID, content string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录，无法发送消息")
	}

	err := b.httpCli.SendTextMessage(ctx, chatID, content)
	if err != nil {
		return errs.Wrap(errs.CodeChannelSendFailed, "发送微信消息失败", err)
	}

	b.logger.Debug("发送微信消息成功",
		zap.String("chat_id", chatID),
		zap.Int("content_len", len(content)))

	return nil
}

// SetMessageHandler 设置消息回调
func (b *Backend) SetMessageHandler(handler wechat.MessageHandler) {
	b.handler = handler

	// 如果 WebSocket 客户端已存在，更新其 handler
	if b.wsCli != nil {
		// 创建新的适配器函数
		wsHandler := func(msg *wechat.IncomingMessage) error {
			if b.handler != nil {
				b.handler(*msg)
			}
			return nil
		}
		b.wsCli.handler = wsHandler
	}
}

// IsLoggedIn 返回当前登录状态
func (b *Backend) IsLoggedIn() bool {
	return b.loggedIn.Load()
}

// ===== WechatOpsProvider 接口实现 =====
// Backend 委托给 HTTPClient 执行所有操作

// --- 消息 ---

// SendImageMessage 发送图片消息
func (b *Backend) SendImageMessage(ctx context.Context, toWxID, imageBase64 string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SendImageMessage(ctx, toWxID, imageBase64)
}

// SendFileMessage 发送文件消息
func (b *Backend) SendFileMessage(ctx context.Context, toWxID, fileBase64, fileName string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SendFileMessage(ctx, toWxID, fileBase64, fileName)
}

// SendEmojiMessage 发送表情消息
func (b *Backend) SendEmojiMessage(ctx context.Context, toWxID, emojiMD5 string, emojiLen int) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SendEmojiMessage(ctx, toWxID, emojiMD5, emojiLen)
}

// SendCardMessage 发送名片消息
func (b *Backend) SendCardMessage(ctx context.Context, toWxID, cardWxID string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SendCardMessage(ctx, toWxID, cardWxID)
}

// RevokeMessage 撤回消息
func (b *Backend) RevokeMessage(ctx context.Context, msgID, toWxID string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.RevokeMessage(ctx, msgID, toWxID)
}

// ForwardImage 转发图片消息
func (b *Backend) ForwardImage(ctx context.Context, toWxID, xml string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.ForwardImage(ctx, toWxID, xml)
}

// ForwardVideo 转发视频消息
func (b *Backend) ForwardVideo(ctx context.Context, toWxID, xml string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.ForwardVideo(ctx, toWxID, xml)
}

// --- 联系人 ---

// GetFriendList 获取好友列表
func (b *Backend) GetFriendList(ctx context.Context) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetFriendList(ctx)
}

// GetContactDetails 批量获取联系人详情
func (b *Backend) GetContactDetails(ctx context.Context, wxIDs []string) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetContactDetails(ctx, wxIDs)
}

// SearchContact 搜索联系人
func (b *Backend) SearchContact(ctx context.Context, keyword string) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SearchContact(ctx, keyword)
}

// AddFriend 添加好友
func (b *Backend) AddFriend(ctx context.Context, wxID, verifyMsg string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.AddFriend(ctx, wxID, verifyMsg)
}

// AcceptFriend 同意好友请求
func (b *Backend) AcceptFriend(ctx context.Context, encryptUser, ticket string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.AcceptFriend(ctx, encryptUser, ticket)
}

// DeleteFriend 删除联系人
func (b *Backend) DeleteFriend(ctx context.Context, wxID string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.DeleteFriend(ctx, wxID)
}

// --- 群管理 ---

// CreateGroup 创建群聊
func (b *Backend) CreateGroup(ctx context.Context, wxIDs []string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.CreateGroup(ctx, wxIDs)
}

// GetGroupList 获取群列表
func (b *Backend) GetGroupList(ctx context.Context) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetGroupList(ctx)
}

// GetGroupDetail 获取群详情
func (b *Backend) GetGroupDetail(ctx context.Context, groupWxID string) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetGroupDetail(ctx, groupWxID)
}

// GetGroupMembers 获取群成员列表
func (b *Backend) GetGroupMembers(ctx context.Context, groupWxID string) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetGroupMembers(ctx, groupWxID)
}

// InviteToGroup 邀请成员加入群聊
func (b *Backend) InviteToGroup(ctx context.Context, groupWxID string, wxIDs []string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.InviteToGroup(ctx, groupWxID, wxIDs)
}

// RemoveFromGroup 从群聊中移除成员
func (b *Backend) RemoveFromGroup(ctx context.Context, groupWxID string, wxIDs []string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.RemoveFromGroup(ctx, groupWxID, wxIDs)
}

// SetGroupName 设置群名称
func (b *Backend) SetGroupName(ctx context.Context, groupWxID, name string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SetGroupName(ctx, groupWxID, name)
}

// SetGroupAnnouncement 设置群公告
func (b *Backend) SetGroupAnnouncement(ctx context.Context, groupWxID, text string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SetGroupAnnouncement(ctx, groupWxID, text)
}

// GetGroupQRCode 获取群二维码
func (b *Backend) GetGroupQRCode(ctx context.Context, groupWxID string) (string, error) {
	if !b.loggedIn.Load() {
		return "", errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetGroupQRCode(ctx, groupWxID)
}

// QuitGroup 退出群聊
func (b *Backend) QuitGroup(ctx context.Context, groupWxID string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.QuitGroup(ctx, groupWxID)
}

// --- 用户 ---

// GetProfile 获取当前登录用户资料
func (b *Backend) GetProfile(ctx context.Context) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetProfile(ctx)
}

// SetNickname 设置昵称
func (b *Backend) SetNickname(ctx context.Context, name string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SetNickname(ctx, name)
}

// SetSignature 设置个性签名
func (b *Backend) SetSignature(ctx context.Context, sig string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.SetSignature(ctx, sig)
}

// ModifyRemark 修改联系人备注
func (b *Backend) ModifyRemark(ctx context.Context, wxID, remark string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.ModifyRemark(ctx, wxID, remark)
}

// --- 朋友圈 ---

// PostMoment 发布朋友圈
func (b *Backend) PostMoment(ctx context.Context, content string, images []string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.PostMoment(ctx, content, images)
}

// GetTimeline 获取朋友圈时间线
func (b *Backend) GetTimeline(ctx context.Context) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetTimeline(ctx)
}

// GetUserMoments 获取指定用户的朋友圈
func (b *Backend) GetUserMoments(ctx context.Context, wxID string) (any, error) {
	if !b.loggedIn.Load() {
		return nil, errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.GetUserMoments(ctx, wxID)
}

// LikeMoment 给朋友圈点赞
func (b *Backend) LikeMoment(ctx context.Context, snsID string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.LikeMoment(ctx, snsID)
}

// CommentMoment 评论朋友圈
func (b *Backend) CommentMoment(ctx context.Context, snsID, content string) error {
	if !b.loggedIn.Load() {
		return errs.New(errs.CodeWeChatNotLoggedIn, "微信未登录")
	}
	return b.httpCli.CommentMoment(ctx, snsID, content)
}

// ===== 热更新 =====

// Reconfigure 热更新连接配置
func (b *Backend) Reconfigure(baseURL, key string) {
	timeout := time.Duration(b.cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	b.httpCli = NewHTTPClient(HTTPClientConfig{
		BaseURL: baseURL,
		Key:     key,
		Timeout: timeout,
		Logger:  b.logger,
	})
	b.logger.Info("WeChatPadPro 配置已更新",
		zap.String("base_url", baseURL))
}
