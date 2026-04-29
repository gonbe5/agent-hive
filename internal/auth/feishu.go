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

// FeishuAuthConfig 飞书 OAuth 配置
type FeishuAuthConfig struct {
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	RedirectURL string `json:"redirect_url"`
}

// FeishuProvider 飞书 OAuth2 provider
type FeishuProvider struct {
	appID       string
	appSecret   string
	redirectURL string
}

// NewFeishuProvider 创建飞书 provider
func NewFeishuProvider(cfg FeishuAuthConfig) *FeishuProvider {
	return &FeishuProvider{
		appID:       cfg.AppID,
		appSecret:   cfg.AppSecret,
		redirectURL: cfg.RedirectURL,
	}
}

func (p *FeishuProvider) Type() string { return "feishu" }

func (p *FeishuProvider) AuthCodeURL(state string) string {
	params := url.Values{}
	params.Set("app_id", p.appID)
	params.Set("redirect_uri", p.redirectURL)
	params.Set("state", state)
	return "https://open.feishu.cn/open-apis/authen/v1/authorize?" + params.Encode()
}

func (p *FeishuProvider) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	// Step 1: 用 code 换 user_access_token
	body, _ := json.Marshal(map[string]string{
		"grant_type": "authorization_code",
		"code":       code,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/authen/v1/oidc/access_token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	// 使用 app_access_token 作为 Authorization（飞书 OIDC 要求）
	appToken, err := p.getAppAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取 app_access_token 失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("换取 user_access_token 失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析 token 响应失败: %w", err)
	}
	if tokenResp.Code != 0 {
		return nil, fmt.Errorf("飞书返回错误: %s", tokenResp.Msg)
	}

	// Step 2: 用 user_access_token 获取用户信息
	return p.getUserInfo(ctx, tokenResp.Data.AccessToken)
}

func (p *FeishuProvider) getAppAccessToken(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"app_id":     p.appID,
		"app_secret": p.appSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/app_access_token/internal", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Code           int    `json:"code"`
		AppAccessToken string `json:"app_access_token"`
		Msg            string `json:"msg"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	if result.Code != 0 {
		return "", fmt.Errorf("获取 app_access_token 失败: %s", result.Msg)
	}
	return result.AppAccessToken, nil
}

func (p *FeishuProvider) getUserInfo(ctx context.Context, userAccessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取用户信息失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Code int `json:"code"`
		Data struct {
			OpenID      string `json:"open_id"`
			Name        string `json:"name"`
			Email       string `json:"email"`
			AvatarURL   string `json:"avatar_url"`
			Department  string `json:"department"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析用户信息失败: %w", err)
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("飞书返回错误: %s", result.Msg)
	}
	return &UserInfo{
		ExternalID:  result.Data.OpenID,
		DisplayName: result.Data.Name,
		Email:       result.Data.Email,
		AvatarURL:   result.Data.AvatarURL,
		Department:  result.Data.Department,
	}, nil
}
