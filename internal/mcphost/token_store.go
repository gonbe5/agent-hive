package mcphost

import (
	"context"
	"strings"
	"time"

	"github.com/chef-guo/agents-hive/internal/store"
)

// DBTokenStore 基于数据库的 token 存储适配器
type DBTokenStore struct {
	store oauthStore
}

// oauthStore 定义所需的底层存储方法
type oauthStore interface {
	SaveOAuthToken(ctx context.Context, token *store.OAuthTokenRecord) error
	LoadOAuthToken(ctx context.Context, serverURL string) (*store.OAuthTokenRecord, error)
	DeleteOAuthToken(ctx context.Context, serverURL string) error
}

// NewDBTokenStore 创建数据库 token 存储适配器
func NewDBTokenStore(s oauthStore) *DBTokenStore {
	return &DBTokenStore{store: s}
}

// SaveToken 保存 OAuth token
func (s *DBTokenStore) SaveToken(ctx context.Context, serverURL string, token *OAuthToken) error {
	record := &store.OAuthTokenRecord{
		ServerURL:    serverURL,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scopes:       token.Scopes,
	}
	if !token.ExpiresAt.IsZero() {
		record.ExpiresAt = token.ExpiresAt.Format(time.RFC3339)
	}
	return s.store.SaveOAuthToken(ctx, record)
}

// LoadToken 加载 OAuth token
func (s *DBTokenStore) LoadToken(ctx context.Context, serverURL string) (*OAuthToken, error) {
	record, err := s.store.LoadOAuthToken(ctx, serverURL)
	if err != nil {
		return nil, err
	}

	token := &OAuthToken{
		AccessToken:  record.AccessToken,
		RefreshToken: record.RefreshToken,
		TokenType:    record.TokenType,
		Scopes:       record.Scopes,
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if record.ExpiresAt != "" {
		// 尝试多种时间格式解析
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", time.RFC3339Nano} {
			if t, parseErr := time.Parse(layout, strings.TrimSpace(record.ExpiresAt)); parseErr == nil {
				token.ExpiresAt = t
				break
			}
		}
	}

	return token, nil
}

// DeleteToken 删除 OAuth token
func (s *DBTokenStore) DeleteToken(ctx context.Context, serverURL string) error {
	return s.store.DeleteOAuthToken(ctx, serverURL)
}
