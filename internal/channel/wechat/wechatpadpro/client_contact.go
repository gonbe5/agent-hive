package wechatpadpro

import (
	"context"
	"net/http"
)

// GetContactList 获取通讯录列表
func (c *HTTPClient) GetContactList(ctx context.Context) ([]ContactInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/friend/GetContactList", nil)
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]ContactInfo](resp)
}

// GetFriendList 获取好友列表
func (c *HTTPClient) GetFriendList(ctx context.Context) ([]ContactInfo, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/friend/GetFriendList", nil)
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]ContactInfo](resp)
}

// GetContactDetails 批量获取联系人详情
func (c *HTTPClient) GetContactDetails(ctx context.Context, wxIDs []string) ([]ContactDetail, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/friend/GetContactDetailsList", GetContactDetailsReq{WxIDs: wxIDs})
	if err != nil {
		return nil, err
	}
	return parseResponseData[[]ContactDetail](resp)
}

// SearchContact 搜索联系人
func (c *HTTPClient) SearchContact(ctx context.Context, keyword string) (*SearchResult, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/friend/SearchContact", SearchContactReq{Keyword: keyword})
	if err != nil {
		return nil, err
	}
	return parseResponseData[*SearchResult](resp)
}

// AddFriend 添加好友
func (c *HTTPClient) AddFriend(ctx context.Context, wxID, verifyMsg string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/friend/VerifyUser", VerifyUserReq{
		WxID:      wxID,
		VerifyMsg: verifyMsg,
	})
	return err
}

// AcceptFriend 同意好友请求
func (c *HTTPClient) AcceptFriend(ctx context.Context, encryptUser, ticket string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/friend/AgreeAdd", AgreeAddReq{
		EncryptUser: encryptUser,
		Ticket:      ticket,
	})
	return err
}

// DeleteFriend 删除联系人
func (c *HTTPClient) DeleteFriend(ctx context.Context, wxID string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/friend/DelContact", DelContactReq{WxID: wxID})
	return err
}
