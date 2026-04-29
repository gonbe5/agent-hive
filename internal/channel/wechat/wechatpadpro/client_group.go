package wechatpadpro

import (
	"context"
	"net/http"
)

// CreateGroup 创建群聊
func (c *HTTPClient) CreateGroup(ctx context.Context, wxIDs []string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/CreateChatRoom", CreateGroupReq{WxIDs: wxIDs})
	return err
}

// GetGroupList 获取群列表
func (c *HTTPClient) GetGroupList(ctx context.Context) ([]GroupInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/group/GetAllGroupList", nil)
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]GroupInfo](resp)
}

// GetGroupDetail 获取群详情
func (c *HTTPClient) GetGroupDetail(ctx context.Context, groupWxID string) (*GroupDetail, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/group/GetChatRoomInfo", GroupMembersReq{GroupWxID: groupWxID})
	if err != nil {
		return nil, err
	}
	return parseResponseData[*GroupDetail](resp)
}

// GetGroupMembers 获取群成员列表
func (c *HTTPClient) GetGroupMembers(ctx context.Context, groupWxID string) ([]GroupMember, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/group/GetChatroomMemberDetail", GroupMembersReq{GroupWxID: groupWxID})
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]GroupMember](resp)
}

// InviteToGroup 邀请成员加入群聊
func (c *HTTPClient) InviteToGroup(ctx context.Context, groupWxID string, wxIDs []string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/InviteChatroomMembers", GroupMembersReq{
		GroupWxID: groupWxID,
		WxIDs:    wxIDs,
	})
	return err
}

// RemoveFromGroup 从群聊中移除成员
func (c *HTTPClient) RemoveFromGroup(ctx context.Context, groupWxID string, wxIDs []string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/SendDelDelChatRoomMember", GroupMembersReq{
		GroupWxID: groupWxID,
		WxIDs:    wxIDs,
	})
	return err
}

// SetGroupName 设置群名称
func (c *HTTPClient) SetGroupName(ctx context.Context, groupWxID, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/SetChatroomName", SetGroupNameReq{
		GroupWxID: groupWxID,
		Name:     name,
	})
	return err
}

// SetGroupAnnouncement 设置群公告
func (c *HTTPClient) SetGroupAnnouncement(ctx context.Context, groupWxID, text string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/SetChatroomAnnouncement", SetGroupAnnouncementReq{
		GroupWxID: groupWxID,
		Content:  text,
	})
	return err
}

// GetGroupQRCode 获取群二维码
func (c *HTTPClient) GetGroupQRCode(ctx context.Context, groupWxID string) (string, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/group/GetChatroomQrCode", GroupMembersReq{GroupWxID: groupWxID})
	if err != nil {
		return "", err
	}
	return parseResponseData[string](resp)
}

// QuitGroup 退出群聊
func (c *HTTPClient) QuitGroup(ctx context.Context, groupWxID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/group/QuitChatroom", GroupMembersReq{GroupWxID: groupWxID})
	return err
}
