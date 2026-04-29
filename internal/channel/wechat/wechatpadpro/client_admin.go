package wechatpadpro

import (
	"context"
	"net/http"
)

// GenAuthKey 通过管理接口生成授权码
func (c *HTTPClient) GenAuthKey(ctx context.Context, adminKey string, days int) (*GenAuthKeyResp, error) {
	resp, err := c.doRequestWithKey(ctx, http.MethodPost, "/admin/GenAuthKey1", GenAuthKeyReq{Days: days}, adminKey)
	if err != nil {
		return nil, err
	}
	return parseResponseData[*GenAuthKeyResp](resp)
}
