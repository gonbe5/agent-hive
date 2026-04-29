package auth

import (
	"context"
	"time"
)

// UserInfo 第三方认证返回的用户信息（统一格式）
type UserInfo struct {
	ExternalID  string // 飞书 OpenID / 钉钉 UserID / LDAP DN
	DisplayName string
	Email       string
	AvatarURL   string
	Department  string
}

// OAuthProvider OAuth2 认证（飞书/钉钉）
type OAuthProvider interface {
	Type() string                                                 // "feishu" / "dingtalk"
	AuthCodeURL(state string) string                              // 生成授权 URL
	Exchange(ctx context.Context, code string) (*UserInfo, error) // 授权码换用户信息
}

// CredentialProvider 凭证认证（LDAP/本地账号）
type CredentialProvider interface {
	Type() string // "ldap"
	Authenticate(ctx context.Context, username, password string) (*UserInfo, error)
}

// User 系统用户
type User struct {
	ID           string     `json:"id"`
	ExternalID   string     `json:"external_id"`
	AuthProvider string     `json:"auth_provider"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email"`
	AvatarURL    string     `json:"avatar_url"`
	Department   string     `json:"department"`
	Role         string     `json:"role"`   // "user" / "admin"
	Status       string     `json:"status"` // "active" / "disabled"
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP  string     `json:"last_login_ip"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// LoginRecord 登录历史
type LoginRecord struct {
	ID           int64     `json:"id"`
	UserID       string    `json:"user_id"`
	AuthProvider string    `json:"auth_provider"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProviderInfo 对外暴露的 provider 信息（登录页用）
type ProviderInfo struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
}
