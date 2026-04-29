package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// DingTalkAuthConfig 钉钉 OAuth 配置
type DingTalkAuthConfig struct {
	AppKey      string `json:"app_key"`
	AppSecret   string `json:"app_secret"`
	RedirectURL string `json:"redirect_url"`
}

// DingTalkProvider 钉钉 OAuth2 provider
type DingTalkProvider struct {
	appKey      string
	appSecret   string
	redirectURL string
}

// NewDingTalkProvider 创建钉钉 provider
func NewDingTalkProvider(cfg DingTalkAuthConfig) *DingTalkProvider {
	return &DingTalkProvider{
		appKey:      cfg.AppKey,
		appSecret:   cfg.AppSecret,
		redirectURL: cfg.RedirectURL,
	}
}

func (p *DingTalkProvider) Type() string { return "dingtalk" }

func (p *DingTalkProvider) AuthCodeURL(state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.appKey)
	params.Set("redirect_uri", p.redirectURL)
	params.Set("scope", "openid")
	params.Set("state", state)
	params.Set("prompt", "consent")
	return "https://login.dingtalk.com/oauth2/auth?" + params.Encode()
}

func (p *DingTalkProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	// Step 1: 换取 user_access_token
	body, _ := json.Marshal(map[string]string{
		"clientId":     p.appKey,
		"clientSecret": p.appSecret,
		"code":         code,
		"grantType":    "authorization_code",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.dingtalk.com/v1.0/oauth2/userAccessToken", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("换取钉钉 user_access_token 失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken string `json:"accessToken"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析钉钉 token 响应失败: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("钉钉返回错误: %s", tokenResp.ErrMsg)
	}

	// Step 2: 获取用户信息
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.dingtalk.com/v1.0/contact/users/me", nil)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("x-acs-dingtalk-access-token", tokenResp.AccessToken)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("获取钉钉用户信息失败: %w", err)
	}
	defer resp2.Body.Close()
	data2, _ := io.ReadAll(resp2.Body)

	var userResp struct {
		UnionID   string `json:"unionId"`
		Nick      string `json:"nick"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatarUrl"`
	}
	if err := json.Unmarshal(data2, &userResp); err != nil {
		return nil, fmt.Errorf("解析钉钉用户信息失败: %w", err)
	}
	return &UserInfo{
		ExternalID:  userResp.UnionID,
		DisplayName: userResp.Nick,
		Email:       userResp.Email,
		AvatarURL:   userResp.AvatarURL,
	}, nil
}
