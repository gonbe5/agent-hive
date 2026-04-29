package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Claims JWT 声明
type Claims struct {
	jwt.RegisteredClaims
	Role             string   `json:"role"`
	Provider         string   `json:"provider"`
	Scopes           []string `json:"scopes,omitempty"`
	OriginalIssuedAt int64    `json:"oiat"` // 首次签发时间，refresh 时保留
}

type cachedUser struct {
	user      *User
	expiresAt time.Time
}

// JWTManager JWT 签发/验证管理器
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	maxTTL time.Duration
}

// NewJWTManager 创建 JWTManager
func NewJWTManager(secret string, ttl, maxTTL time.Duration) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
		maxTTL: maxTTL,
	}
}

// Issue 签发 JWT
func (m *JWTManager) Issue(userID, role, provider string) (string, error) {
	return m.IssueWithScopes(userID, role, provider, defaultScopesForRole(role))
}

// IssueWithScopes 使用显式 scope 集合签发 JWT。
func (m *JWTManager) IssueWithScopes(userID, role, provider string, scopes []string) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
		Role:             role,
		Provider:         provider,
		Scopes:           normalizeScopes(scopes),
		OriginalIssuedAt: now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Verify 验证 JWT 并返回 Claims
func (m *JWTManager) Verify(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// ParseSkipExpiry 解析 token 但跳过 exp 校验，用于 refresh 流程提取 subject
// 仍验证签名和算法，只是不拒绝已过期的 token
func (m *JWTManager) ParseSkipExpiry(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		return nil, fmt.Errorf("token 解析失败: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// Refresh 验证旧 token 并签发新 token，保留 OriginalIssuedAt
// 允许刷新已过期（但未超过 maxTTL）的 token
func (m *JWTManager) Refresh(tokenStr string) (string, error) {
	claims, err := m.ParseSkipExpiry(tokenStr)
	if err != nil {
		return "", err
	}
	// 检查绝对过期时间（maxTTL）
	if time.Since(time.Unix(claims.OriginalIssuedAt, 0)) > m.maxTTL {
		return "", fmt.Errorf("token 已超过最大有效期，请重新登录")
	}
	now := time.Now()
	newClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.Subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
		Role:             claims.Role,
		Provider:         claims.Provider,
		Scopes:           normalizeScopes(claims.Scopes),
		OriginalIssuedAt: claims.OriginalIssuedAt, // 保留原始签发时间
	}
	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	return newToken.SignedString(m.secret)
}

func defaultScopesForRole(role string) []string {
	scopes := []string{"read", "write"}
	if role == "admin" {
		scopes = append(scopes, "admin", "push:write")
	}
	return scopes
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

// ResolveJWTSecret 竞态安全地解析 JWT secret
func ResolveJWTSecret(ctx context.Context, pool *pgxpool.Pool, configSecret string) (string, error) {
	if configSecret != "" {
		return configSecret, nil
	}
	var dbSecret string
	err := pool.QueryRow(ctx, `SELECT value FROM configs WHERE key = 'auth.jwt_secret'`).Scan(&dbSecret)
	if err == nil && dbSecret != "" {
		return dbSecret, nil
	}
	newSecret := generateRandomSecret(32)
	_, err = pool.Exec(ctx,
		`INSERT INTO configs (key, value) VALUES ('auth.jwt_secret', $1) ON CONFLICT (key) DO NOTHING`,
		newSecret)
	if err != nil {
		return "", fmt.Errorf("写入 JWT secret 失败: %w", err)
	}
	err = pool.QueryRow(ctx, `SELECT value FROM configs WHERE key = 'auth.jwt_secret'`).Scan(&dbSecret)
	if err != nil {
		return "", fmt.Errorf("读取 JWT secret 失败: %w", err)
	}
	return dbSecret, nil
}

func generateRandomSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
