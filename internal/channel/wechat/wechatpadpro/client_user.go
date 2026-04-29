package wechatpadpro

import (
	"context"
	"net/http"
)

// GetProfile 获取当前登录用户资料
func (c *HTTPClient) GetProfile(ctx context.Context) (*UserProfile, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/user/GetProfile", nil)
	if err != nil {
		return nil, err
	}
	return parseResponseData[*UserProfile](resp)
}

// SetNickname 设置昵称
func (c *HTTPClient) SetNickname(ctx context.Context, name string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/user/SetNickName", SetNicknameReq{Nickname: name})
	return err
}

// SetSignature 设置个性签名
func (c *HTTPClient) SetSignature(ctx context.Context, sig string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/user/SetSignature", SetSignatureReq{Signature: sig})
	return err
}

// ModifyRemark 修改联系人备注
func (c *HTTPClient) ModifyRemark(ctx context.Context, wxID, remark string) error {
	_, err := c.doRequest(ctx, http.MethodPost, "/user/ModifyRemark", ModifyRemarkReq{
		WxID:   wxID,
		Remark: remark,
	})
	return err
}
