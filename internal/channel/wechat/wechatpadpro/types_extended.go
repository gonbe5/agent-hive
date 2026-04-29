package wechatpadpro

// ===== 联系人相关 =====

// ContactInfo 联系人基础信息
type ContactInfo struct {
	WxID     string `json:"wxid"`
	Nickname string `json:"nickname"`
	Remark   string `json:"remark,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
}

// ContactDetail 联系人详细信息
type ContactDetail struct {
	WxID      string `json:"wxid"`
	Nickname  string `json:"nickname"`
	Remark    string `json:"remark,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	Sex       int    `json:"sex"` // 0=未知 1=男 2=女
	Province  string `json:"province,omitempty"`
	City      string `json:"city,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// SearchResult 联系人搜索结果
type SearchResult struct {
	WxID     string `json:"wxid"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar,omitempty"`
	Sex      int    `json:"sex"`
	V1       string `json:"v1,omitempty"` // 加好友用的加密用户名
	V2       string `json:"v2,omitempty"` // 加好友用的 ticket
}

// ===== 群管理相关 =====

// GroupInfo 群基础信息
type GroupInfo struct {
	GroupWxID   string `json:"group_wxid"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count"`
	Avatar      string `json:"avatar,omitempty"`
}

// GroupDetail 群详细信息
type GroupDetail struct {
	GroupWxID    string `json:"group_wxid"`
	Name         string `json:"name"`
	Owner        string `json:"owner"`
	MemberCount  int    `json:"member_count"`
	Announcement string `json:"announcement,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
}

// GroupMember 群成员信息
type GroupMember struct {
	WxID        string `json:"wxid"`
	Nickname    string `json:"nickname"`
	DisplayName string `json:"display_name,omitempty"` // 群内昵称
	Avatar      string `json:"avatar,omitempty"`
}

// ===== 用户资料相关 =====

// UserProfile 用户资料
type UserProfile struct {
	WxID      string `json:"wxid"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar,omitempty"`
	Sex       int    `json:"sex"`
	Province  string `json:"province,omitempty"`
	City      string `json:"city,omitempty"`
	Signature string `json:"signature,omitempty"`
}

// ===== 朋友圈相关 =====

// MomentItem 朋友圈条目
type MomentItem struct {
	SnsID      string `json:"sns_id"`
	UserName   string `json:"user_name"`
	Content    string `json:"content,omitempty"`
	CreateTime int64  `json:"create_time"`
}

// ===== 消息相关 =====

// SendImageReq 发送图片消息请求
type SendImageReq struct {
	ToWxID      string `json:"ToWxID"`
	ImageBase64 string `json:"ImageContent"` // Base64 编码的图片
}

// SendFileReq 发送文件消息请求
type SendFileReq struct {
	ToWxID   string `json:"ToWxID"`
	Content  string `json:"Content"`  // 文件 base64
	FileName string `json:"FileName"`
}

// SendEmojiReq 发送表情消息请求
type SendEmojiReq struct {
	ToWxID   string `json:"ToWxID"`
	EmojiMD5 string `json:"EmojiMd5"`
	EmojiLen int    `json:"EmojiLen"`
}

// ShareCardReq 发送名片消息请求
type ShareCardReq struct {
	ToWxID   string `json:"ToWxID"`
	CardWxID string `json:"CardWxid"`
}

// RevokeMsgReq 撤回消息请求
type RevokeMsgReq struct {
	MsgID  string `json:"ClientMsgId"`
	ToWxID string `json:"ToWxID"`
}

// ForwardMessageReq 转发消息请求
type ForwardMessageReq struct {
	ToWxID string `json:"ToWxID"`
	XML    string `json:"Xml"`
}

// ===== 管理接口相关 =====

// GenAuthKeyReq 生成授权码请求
type GenAuthKeyReq struct {
	Days int `json:"Days"` // 有效天数
}

// GenAuthKeyResp 生成授权码响应
type GenAuthKeyResp struct {
	Key        string `json:"key"`
	ExpireTime string `json:"expire_time,omitempty"`
}

// ===== 联系人操作请求 =====

// GetContactDetailsReq 批量获取联系人详情请求
type GetContactDetailsReq struct {
	WxIDs []string `json:"wxids"`
}

// SearchContactReq 搜索联系人请求
type SearchContactReq struct {
	Keyword string `json:"Keyword"`
}

// VerifyUserReq 添加好友请求
type VerifyUserReq struct {
	WxID      string `json:"Wxid"`
	VerifyMsg string `json:"VerifyContent,omitempty"`
}

// AgreeAddReq 同意好友请求
type AgreeAddReq struct {
	EncryptUser string `json:"EncryptUserName"`
	Ticket      string `json:"Ticket"`
}

// DelContactReq 删除联系人请求
type DelContactReq struct {
	WxID string `json:"Wxid"`
}

// ===== 群操作请求 =====

// CreateGroupReq 创建群聊请求
type CreateGroupReq struct {
	WxIDs []string `json:"Wxids"`
}

// GroupMembersReq 群成员操作请求（邀请/移除/查询）
type GroupMembersReq struct {
	GroupWxID string   `json:"ChatRoomName"`
	WxIDs     []string `json:"Wxids,omitempty"`
}

// SetGroupNameReq 设置群名请求
type SetGroupNameReq struct {
	GroupWxID string `json:"ChatRoomName"`
	Name      string `json:"Name"`
}

// SetGroupAnnouncementReq 设置群公告请求
type SetGroupAnnouncementReq struct {
	GroupWxID string `json:"ChatRoomName"`
	Content   string `json:"Content"`
}

// ===== 用户操作请求 =====

// SetNicknameReq 设置昵称请求
type SetNicknameReq struct {
	Nickname string `json:"NickName"`
}

// SetSignatureReq 设置签名请求
type SetSignatureReq struct {
	Signature string `json:"Signature"`
}

// ModifyRemarkReq 修改备注请求
type ModifyRemarkReq struct {
	WxID   string `json:"Wxid"`
	Remark string `json:"Remark"`
}

// ===== 朋友圈操作请求 =====

// PostMomentReq 发朋友圈请求
type PostMomentReq struct {
	Content string   `json:"Content"`
	Images  []string `json:"Images,omitempty"` // 图片 base64 列表
}

// GetUserMomentsReq 获取用户朋友圈请求
type GetUserMomentsReq struct {
	WxID string `json:"Wxid"`
}

// SnsCommentReq 朋友圈互动请求（点赞/评论）
type SnsCommentReq struct {
	SnsID   string `json:"SnsId"`
	Type    int    `json:"Type"`              // 1=点赞 2=评论
	Content string `json:"Content,omitempty"` // 评论内容
}
