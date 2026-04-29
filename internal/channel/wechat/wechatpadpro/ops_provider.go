package wechatpadpro

import "context"

// WechatOpsProvider 微信操作能力接口
// 由 Backend 实现，注入到工具层避免循环依赖
// 返回数据的方法使用 any 类型，避免工具层导入具体类型产生循环依赖
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
	GetFriendList(ctx context.Context) (any, error)
	GetContactDetails(ctx context.Context, wxIDs []string) (any, error)
	SearchContact(ctx context.Context, keyword string) (any, error)
	AddFriend(ctx context.Context, wxID, verifyMsg string) error
	AcceptFriend(ctx context.Context, encryptUser, ticket string) error
	DeleteFriend(ctx context.Context, wxID string) error

	// 群管理
	CreateGroup(ctx context.Context, wxIDs []string) error
	GetGroupList(ctx context.Context) (any, error)
	GetGroupDetail(ctx context.Context, groupWxID string) (any, error)
	GetGroupMembers(ctx context.Context, groupWxID string) (any, error)
	InviteToGroup(ctx context.Context, groupWxID string, wxIDs []string) error
	RemoveFromGroup(ctx context.Context, groupWxID string, wxIDs []string) error
	SetGroupName(ctx context.Context, groupWxID, name string) error
	SetGroupAnnouncement(ctx context.Context, groupWxID, text string) error
	GetGroupQRCode(ctx context.Context, groupWxID string) (string, error)
	QuitGroup(ctx context.Context, groupWxID string) error

	// 用户
	GetProfile(ctx context.Context) (any, error)
	SetNickname(ctx context.Context, name string) error
	SetSignature(ctx context.Context, sig string) error
	ModifyRemark(ctx context.Context, wxID, remark string) error

	// 朋友圈
	PostMoment(ctx context.Context, content string, images []string) error
	GetTimeline(ctx context.Context) (any, error)
	GetUserMoments(ctx context.Context, wxID string) (any, error)
	LikeMoment(ctx context.Context, snsID string) error
	CommentMoment(ctx context.Context, snsID, content string) error
}
