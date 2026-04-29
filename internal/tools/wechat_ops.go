package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/chef-guo/agents-hive/internal/mcphost"
)

// WechatOpsProvider 微信操作能力接口（本地定义避免循环依赖）
// 由 wechatpadpro.Backend 实现，方法签名与 wechatpadpro.WechatOpsProvider 一致
// 返回值使用 interface{} 避免导入 wechatpadpro 包产生循环依赖
type WechatOpsProvider interface {
	// 状态
	IsLoggedIn() bool

	// 消息
	SendImageMessage(ctx context.Context, toWxID, imageBase64 string) error
	SendFileMessage(ctx context.Context, toWxID, fileBase64, fileName string) error
	SendEmojiMessage(ctx context.Context, toWxID, emojiMD5 string, emojiLen int) error
	SendCardMessage(ctx context.Context, toWxID, cardWxID string) error
	RevokeMessage(ctx context.Context, msgID, toWxID string) error
	ForwardImage(ctx context.Context, toWxID, xml string) error
	ForwardVideo(ctx context.Context, toWxID, xml string) error

	// 联系人
	GetFriendList(ctx context.Context) (interface{}, error)
	GetContactDetails(ctx context.Context, wxIDs []string) (interface{}, error)
	SearchContact(ctx context.Context, keyword string) (interface{}, error)
	AddFriend(ctx context.Context, wxID, verifyMsg string) error
	AcceptFriend(ctx context.Context, encryptUser, ticket string) error
	DeleteFriend(ctx context.Context, wxID string) error

	// 群管理
	CreateGroup(ctx context.Context, wxIDs []string) error
	GetGroupList(ctx context.Context) (interface{}, error)
	GetGroupDetail(ctx context.Context, groupWxID string) (interface{}, error)
	GetGroupMembers(ctx context.Context, groupWxID string) (interface{}, error)
	InviteToGroup(ctx context.Context, groupWxID string, wxIDs []string) error
	RemoveFromGroup(ctx context.Context, groupWxID string, wxIDs []string) error
	SetGroupName(ctx context.Context, groupWxID, name string) error
	SetGroupAnnouncement(ctx context.Context, groupWxID, text string) error
	GetGroupQRCode(ctx context.Context, groupWxID string) (string, error)
	QuitGroup(ctx context.Context, groupWxID string) error

	// 用户
	GetProfile(ctx context.Context) (interface{}, error)
	SetNickname(ctx context.Context, name string) error
	SetSignature(ctx context.Context, sig string) error
	ModifyRemark(ctx context.Context, wxID, remark string) error

	// 朋友圈
	PostMoment(ctx context.Context, content string, images []string) error
	GetTimeline(ctx context.Context) (interface{}, error)
	GetUserMoments(ctx context.Context, wxID string) (interface{}, error)
	LikeMoment(ctx context.Context, snsID string) error
	CommentMoment(ctx context.Context, snsID, content string) error
}

// RegisterWechatOpsTools 注册微信操作工具集（从 server 启动时调用）
// 返回注册的工具数量
func RegisterWechatOpsTools(host *mcphost.Host, logger *zap.Logger, provider interface{}) int {
	p, ok := provider.(WechatOpsProvider)
	if !ok {
		logger.Warn("WechatOpsProvider 类型断言失败，跳过微信操作工具注册")
		return 0
	}
	return registerWechatOps(host, logger, p)
}

// registerWechatOps 注册所有微信操作 MCP 工具
func registerWechatOps(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) int {
	registerWechatRichMessage(host, logger, provider)
	registerWechatContacts(host, logger, provider)
	registerWechatGroups(host, logger, provider)
	registerWechatProfile(host, logger, provider)
	registerWechatMoments(host, logger, provider)
	registerWechatStatus(host, logger, provider)
	logger.Info("微信操作工具已注册", zap.Int("count", 6))
	return 6
}

// checkWechatLogin 检查微信登录状态，未登录则返回错误结果
func checkWechatLogin(provider WechatOpsProvider) *mcphost.ToolResult {
	if !provider.IsLoggedIn() {
		return errorResult("微信未登录，请先完成登录")
	}
	return nil
}

// marshalResult 将数据序列化为 JSON 文本结果
func marshalResult(data interface{}) *mcphost.ToolResult {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return errorResult("序列化结果失败: " + err.Error())
	}
	return textResult(string(b))
}

// ===== 1. wechat_send_rich_message =====

// registerWechatRichMessage 注册微信富媒体消息工具
// 支持操作：send_image/send_file/send_emoji/send_card/revoke/forward_image/forward_video
func registerWechatRichMessage(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"send_image", "send_file", "send_emoji", "send_card", "revoke", "forward_image", "forward_video"},
				"description": "操作类型",
			},
			"to_wxid":      map[string]any{"type": "string", "description": "接收者微信 ID"},
			"image_base64": map[string]any{"type": "string", "description": "图片 Base64 编码（send_image）"},
			"file_base64":  map[string]any{"type": "string", "description": "文件 Base64 编码（send_file）"},
			"file_name":    map[string]any{"type": "string", "description": "文件名（send_file）"},
			"emoji_md5":    map[string]any{"type": "string", "description": "表情 MD5（send_emoji）"},
			"emoji_len":    map[string]any{"type": "integer", "description": "表情长度（send_emoji）"},
			"card_wxid":    map[string]any{"type": "string", "description": "名片对应的微信 ID（send_card）"},
			"msg_id":       map[string]any{"type": "string", "description": "消息 ID（revoke）"},
			"xml":          map[string]any{"type": "string", "description": "转发消息 XML（forward_image/forward_video）"},
		},
		"required": []string{"action", "to_wxid"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_send_rich_message",
			Description: "发送微信富媒体消息（图片/文件/表情/名片/撤回/转发）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			if r := checkWechatLogin(provider); r != nil {
				return r, nil
			}

			var p struct {
				Action      string `json:"action"`
				ToWxID      string `json:"to_wxid"`
				ImageBase64 string `json:"image_base64"`
				FileBase64  string `json:"file_base64"`
				FileName    string `json:"file_name"`
				EmojiMD5    string `json:"emoji_md5"`
				EmojiLen    int    `json:"emoji_len"`
				CardWxID    string `json:"card_wxid"`
				MsgID       string `json:"msg_id"`
				XML         string `json:"xml"`
			}
			if err := json.Unmarshal(input, &p); err != nil {
				return errorResult("参数解析失败: " + err.Error()), nil
			}

			var err error
			switch p.Action {
			case "send_image":
				err = provider.SendImageMessage(ctx, p.ToWxID, p.ImageBase64)
			case "send_file":
				err = provider.SendFileMessage(ctx, p.ToWxID, p.FileBase64, p.FileName)
			case "send_emoji":
				err = provider.SendEmojiMessage(ctx, p.ToWxID, p.EmojiMD5, p.EmojiLen)
			case "send_card":
				err = provider.SendCardMessage(ctx, p.ToWxID, p.CardWxID)
			case "revoke":
				err = provider.RevokeMessage(ctx, p.MsgID, p.ToWxID)
			case "forward_image":
				err = provider.ForwardImage(ctx, p.ToWxID, p.XML)
			case "forward_video":
				err = provider.ForwardVideo(ctx, p.ToWxID, p.XML)
			default:
				return errorResult(fmt.Sprintf("未知操作: %s", p.Action)), nil
			}

			if err != nil {
				logger.Error("微信富媒体消息操作失败",
					zap.String("action", p.Action),
					zap.String("to_wxid", p.ToWxID),
					zap.Error(err))
				return errorResult(fmt.Sprintf("%s 失败: %v", p.Action, err)), nil
			}
			return textResult(fmt.Sprintf("%s 操作成功", p.Action)), nil
		},
	)
}

// ===== 2. wechat_contacts =====

// registerWechatContacts 注册微信联系人管理工具
// 支持操作：list/detail/search/add/accept/delete
func registerWechatContacts(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "detail", "search", "add", "accept", "delete"},
				"description": "操作类型：list=好友列表, detail=联系人详情, search=搜索, add=添加好友, accept=同意好友请求, delete=删除",
			},
			"wxids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "微信 ID 列表（detail）"},
			"keyword":      map[string]any{"type": "string", "description": "搜索关键词（search）"},
			"wxid":         map[string]any{"type": "string", "description": "目标微信 ID（add/delete）"},
			"verify_msg":   map[string]any{"type": "string", "description": "验证消息（add）"},
			"encrypt_user": map[string]any{"type": "string", "description": "加密用户名（accept）"},
			"ticket":       map[string]any{"type": "string", "description": "好友请求 ticket（accept）"},
		},
		"required": []string{"action"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_contacts",
			Description: "微信联系人管理（列表/详情/搜索/添加/删除好友）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			if r := checkWechatLogin(provider); r != nil {
				return r, nil
			}

			var p struct {
				Action      string   `json:"action"`
				WxIDs       []string `json:"wxids"`
				Keyword     string   `json:"keyword"`
				WxID        string   `json:"wxid"`
				VerifyMsg   string   `json:"verify_msg"`
				EncryptUser string   `json:"encrypt_user"`
				Ticket      string   `json:"ticket"`
			}
			if err := json.Unmarshal(input, &p); err != nil {
				return errorResult("参数解析失败: " + err.Error()), nil
			}

			switch p.Action {
			case "list":
				data, err := provider.GetFriendList(ctx)
				if err != nil {
					return errorResult("获取好友列表失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "detail":
				if len(p.WxIDs) == 0 {
					return errorResult("wxids 参数不能为空"), nil
				}
				data, err := provider.GetContactDetails(ctx, p.WxIDs)
				if err != nil {
					return errorResult("获取联系人详情失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "search":
				if p.Keyword == "" {
					return errorResult("keyword 参数不能为空"), nil
				}
				data, err := provider.SearchContact(ctx, p.Keyword)
				if err != nil {
					return errorResult("搜索联系人失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "add":
				if p.WxID == "" {
					return errorResult("wxid 参数不能为空"), nil
				}
				if err := provider.AddFriend(ctx, p.WxID, p.VerifyMsg); err != nil {
					return errorResult("添加好友失败: " + err.Error()), nil
				}
				return textResult("好友请求已发送"), nil

			case "accept":
				if p.EncryptUser == "" || p.Ticket == "" {
					return errorResult("encrypt_user 和 ticket 参数不能为空"), nil
				}
				if err := provider.AcceptFriend(ctx, p.EncryptUser, p.Ticket); err != nil {
					return errorResult("同意好友请求失败: " + err.Error()), nil
				}
				return textResult("已同意好友请求"), nil

			case "delete":
				if p.WxID == "" {
					return errorResult("wxid 参数不能为空"), nil
				}
				if err := provider.DeleteFriend(ctx, p.WxID); err != nil {
					return errorResult("删除联系人失败: " + err.Error()), nil
				}
				return textResult("联系人已删除"), nil

			default:
				return errorResult(fmt.Sprintf("未知操作: %s", p.Action)), nil
			}
		},
	)
}

// ===== 3. wechat_groups =====

// registerWechatGroups 注册微信群管理工具
// 支持操作：list/create/detail/members/invite/remove/set_name/set_announcement/qrcode/quit
func registerWechatGroups(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "create", "detail", "members", "invite", "remove", "set_name", "set_announcement", "qrcode", "quit"},
				"description": "操作类型",
			},
			"group_wxid":   map[string]any{"type": "string", "description": "群微信 ID"},
			"wxids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "微信 ID 列表（create/invite/remove）"},
			"name":         map[string]any{"type": "string", "description": "群名称（set_name）"},
			"announcement": map[string]any{"type": "string", "description": "群公告内容（set_announcement）"},
		},
		"required": []string{"action"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_groups",
			Description: "微信群管理（列表/创建/详情/成员/邀请/移除/改名/公告/二维码/退群）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			if r := checkWechatLogin(provider); r != nil {
				return r, nil
			}

			var p struct {
				Action       string   `json:"action"`
				GroupWxID    string   `json:"group_wxid"`
				WxIDs        []string `json:"wxids"`
				Name         string   `json:"name"`
				Announcement string   `json:"announcement"`
			}
			if err := json.Unmarshal(input, &p); err != nil {
				return errorResult("参数解析失败: " + err.Error()), nil
			}

			switch p.Action {
			case "list":
				data, err := provider.GetGroupList(ctx)
				if err != nil {
					return errorResult("获取群列表失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "create":
				if len(p.WxIDs) < 2 {
					return errorResult("创建群聊至少需要 2 个成员"), nil
				}
				if err := provider.CreateGroup(ctx, p.WxIDs); err != nil {
					return errorResult("创建群聊失败: " + err.Error()), nil
				}
				return textResult("群聊创建成功"), nil

			case "detail":
				if p.GroupWxID == "" {
					return errorResult("group_wxid 参数不能为空"), nil
				}
				data, err := provider.GetGroupDetail(ctx, p.GroupWxID)
				if err != nil {
					return errorResult("获取群详情失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "members":
				if p.GroupWxID == "" {
					return errorResult("group_wxid 参数不能为空"), nil
				}
				data, err := provider.GetGroupMembers(ctx, p.GroupWxID)
				if err != nil {
					return errorResult("获取群成员失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "invite":
				if p.GroupWxID == "" || len(p.WxIDs) == 0 {
					return errorResult("group_wxid 和 wxids 参数不能为空"), nil
				}
				if err := provider.InviteToGroup(ctx, p.GroupWxID, p.WxIDs); err != nil {
					return errorResult("邀请成员失败: " + err.Error()), nil
				}
				return textResult("邀请已发送"), nil

			case "remove":
				if p.GroupWxID == "" || len(p.WxIDs) == 0 {
					return errorResult("group_wxid 和 wxids 参数不能为空"), nil
				}
				if err := provider.RemoveFromGroup(ctx, p.GroupWxID, p.WxIDs); err != nil {
					return errorResult("移除成员失败: " + err.Error()), nil
				}
				return textResult("成员已移除"), nil

			case "set_name":
				if p.GroupWxID == "" || p.Name == "" {
					return errorResult("group_wxid 和 name 参数不能为空"), nil
				}
				if err := provider.SetGroupName(ctx, p.GroupWxID, p.Name); err != nil {
					return errorResult("设置群名失败: " + err.Error()), nil
				}
				return textResult("群名已更新"), nil

			case "set_announcement":
				if p.GroupWxID == "" {
					return errorResult("group_wxid 参数不能为空"), nil
				}
				if err := provider.SetGroupAnnouncement(ctx, p.GroupWxID, p.Announcement); err != nil {
					return errorResult("设置群公告失败: " + err.Error()), nil
				}
				return textResult("群公告已更新"), nil

			case "qrcode":
				if p.GroupWxID == "" {
					return errorResult("group_wxid 参数不能为空"), nil
				}
				qr, err := provider.GetGroupQRCode(ctx, p.GroupWxID)
				if err != nil {
					return errorResult("获取群二维码失败: " + err.Error()), nil
				}
				return textResult(qr), nil

			case "quit":
				if p.GroupWxID == "" {
					return errorResult("group_wxid 参数不能为空"), nil
				}
				if err := provider.QuitGroup(ctx, p.GroupWxID); err != nil {
					return errorResult("退出群聊失败: " + err.Error()), nil
				}
				return textResult("已退出群聊"), nil

			default:
				return errorResult(fmt.Sprintf("未知操作: %s", p.Action)), nil
			}
		},
	)
}

// ===== 4. wechat_profile =====

// registerWechatProfile 注册微信个人资料管理工具
// 支持操作：get/set_nickname/set_signature/remark
func registerWechatProfile(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"get", "set_nickname", "set_signature", "remark"},
				"description": "操作类型：get=获取资料, set_nickname=设置昵称, set_signature=设置签名, remark=修改备注",
			},
			"nickname":  map[string]any{"type": "string", "description": "新昵称（set_nickname）"},
			"signature": map[string]any{"type": "string", "description": "新签名（set_signature）"},
			"wxid":      map[string]any{"type": "string", "description": "目标微信 ID（remark）"},
			"remark":    map[string]any{"type": "string", "description": "新备注（remark）"},
		},
		"required": []string{"action"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_profile",
			Description: "微信个人资料管理（查看资料/设置昵称/设置签名/修改备注）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			if r := checkWechatLogin(provider); r != nil {
				return r, nil
			}

			var p struct {
				Action    string `json:"action"`
				Nickname  string `json:"nickname"`
				Signature string `json:"signature"`
				WxID      string `json:"wxid"`
				Remark    string `json:"remark"`
			}
			if err := json.Unmarshal(input, &p); err != nil {
				return errorResult("参数解析失败: " + err.Error()), nil
			}

			switch p.Action {
			case "get":
				data, err := provider.GetProfile(ctx)
				if err != nil {
					return errorResult("获取资料失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "set_nickname":
				if p.Nickname == "" {
					return errorResult("nickname 参数不能为空"), nil
				}
				if err := provider.SetNickname(ctx, p.Nickname); err != nil {
					return errorResult("设置昵称失败: " + err.Error()), nil
				}
				return textResult("昵称已更新"), nil

			case "set_signature":
				if err := provider.SetSignature(ctx, p.Signature); err != nil {
					return errorResult("设置签名失败: " + err.Error()), nil
				}
				return textResult("签名已更新"), nil

			case "remark":
				if p.WxID == "" || p.Remark == "" {
					return errorResult("wxid 和 remark 参数不能为空"), nil
				}
				if err := provider.ModifyRemark(ctx, p.WxID, p.Remark); err != nil {
					return errorResult("修改备注失败: " + err.Error()), nil
				}
				return textResult("备注已更新"), nil

			default:
				return errorResult(fmt.Sprintf("未知操作: %s", p.Action)), nil
			}
		},
	)
}

// ===== 5. wechat_moments =====

// registerWechatMoments 注册微信朋友圈操作工具
// 支持操作：post/timeline/user_page/like/comment
func registerWechatMoments(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"post", "timeline", "user_page", "like", "comment"},
				"description": "操作类型：post=发朋友圈, timeline=时间线, user_page=查看用户朋友圈, like=点赞, comment=评论",
			},
			"content": map[string]any{"type": "string", "description": "朋友圈文本内容（post/comment）"},
			"images":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "图片 Base64 列表（post）"},
			"wxid":    map[string]any{"type": "string", "description": "目标用户微信 ID（user_page）"},
			"sns_id":  map[string]any{"type": "string", "description": "朋友圈 ID（like/comment）"},
		},
		"required": []string{"action"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_moments",
			Description: "微信朋友圈操作（发布/浏览/点赞/评论）",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			if r := checkWechatLogin(provider); r != nil {
				return r, nil
			}

			var p struct {
				Action  string   `json:"action"`
				Content string   `json:"content"`
				Images  []string `json:"images"`
				WxID    string   `json:"wxid"`
				SnsID   string   `json:"sns_id"`
			}
			if err := json.Unmarshal(input, &p); err != nil {
				return errorResult("参数解析失败: " + err.Error()), nil
			}

			switch p.Action {
			case "post":
				if p.Content == "" && len(p.Images) == 0 {
					return errorResult("content 或 images 至少需要一个"), nil
				}
				if err := provider.PostMoment(ctx, p.Content, p.Images); err != nil {
					return errorResult("发布朋友圈失败: " + err.Error()), nil
				}
				return textResult("朋友圈已发布"), nil

			case "timeline":
				data, err := provider.GetTimeline(ctx)
				if err != nil {
					return errorResult("获取朋友圈时间线失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "user_page":
				if p.WxID == "" {
					return errorResult("wxid 参数不能为空"), nil
				}
				data, err := provider.GetUserMoments(ctx, p.WxID)
				if err != nil {
					return errorResult("获取用户朋友圈失败: " + err.Error()), nil
				}
				return marshalResult(data), nil

			case "like":
				if p.SnsID == "" {
					return errorResult("sns_id 参数不能为空"), nil
				}
				if err := provider.LikeMoment(ctx, p.SnsID); err != nil {
					return errorResult("点赞失败: " + err.Error()), nil
				}
				return textResult("已点赞"), nil

			case "comment":
				if p.SnsID == "" {
					return errorResult("sns_id 参数不能为空"), nil
				}
				if err := provider.CommentMoment(ctx, p.SnsID, p.Content); err != nil {
					return errorResult("评论失败: " + err.Error()), nil
				}
				return textResult("评论已发送"), nil

			default:
				return errorResult(fmt.Sprintf("未知操作: %s", p.Action)), nil
			}
		},
	)
}

// ===== 6. wechat_status =====

// registerWechatStatus 注册微信状态查询工具
func registerWechatStatus(host *mcphost.Host, logger *zap.Logger, provider WechatOpsProvider) {
	schema, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"login_status"},
				"description": "操作类型",
			},
		},
		"required": []string{"action"},
	})

	host.RegisterTool(
		mcphost.ToolDefinition{
			Name:        "wechat_status",
			Description: "查询微信登录状态",
			InputSchema: schema,
		},
		func(ctx context.Context, input json.RawMessage) (*mcphost.ToolResult, error) {
			status := map[string]any{
				"logged_in": provider.IsLoggedIn(),
			}
			return marshalResult(status), nil
		},
	)
}
