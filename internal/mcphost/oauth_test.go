package mcphost

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"go.uber.org/zap"
)

// mockTokenStore 模拟 token 存储
type mockTokenStore struct {
	tokens map[string]*OAuthToken
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{tokens: make(map[string]*OAuthToken)}
}

func (m *mockTokenStore) SaveToken(_ context.Context, serverURL string, token *OAuthToken) error {
	m.tokens[serverURL] = token
	return nil
}

func (m *mockTokenStore) LoadToken(_ context.Context, serverURL string) (*OAuthToken, error) {
	t, ok := m.tokens[serverURL]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockTokenStore) DeleteToken(_ context.Context, serverURL string) error {
	delete(m.tokens, serverURL)
	return nil
}

func TestGenerateCodeVerifier(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "生成有效的 code_verifier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier, err := generateCodeVerifier()
			if err != nil {
				t.Fatalf("generateCodeVerifier() 返回错误: %v", err)
			}

			// 检查长度范围: base64url(32 bytes) = 43 字符
			if len(verifier) < 43 || len(verifier) > 128 {
				t.Errorf("code_verifier 长度 %d 不在 43-128 范围内", len(verifier))
			}

			// 检查字符集: base64url 字符（A-Z, a-z, 0-9, -, _）
			for _, c := range verifier {
				if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
					(c >= '0' && c <= '9') || c == '-' || c == '_') {
					t.Errorf("code_verifier 包含无效字符: %c", c)
				}
			}
		})
	}
}

func TestGenerateCodeVerifier_Uniqueness(t *testing.T) {
	// 生成多个 verifier 确认唯一性
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		v, err := generateCodeVerifier()
		if err != nil {
			t.Fatalf("第 %d 次生成失败: %v", i, err)
		}
		if seen[v] {
			t.Errorf("检测到重复的 code_verifier")
		}
		seen[v] = true
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
	}{
		{
			name:     "标准 verifier 的 S256 challenge",
			verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
		},
		{
			name:     "自定义 verifier",
			verifier: "test_verifier_12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			challenge := generateCodeChallenge(tt.verifier)

			// 检查非空
			if challenge == "" {
				t.Fatal("generateCodeChallenge 返回空字符串")
			}

			// 检查是 base64url 编码（无填充）
			_, err := base64.RawURLEncoding.DecodeString(challenge)
			if err != nil {
				t.Errorf("challenge 不是有效的 base64url 编码: %v", err)
			}

			// SHA256 哈希 = 32 字节 = base64url 编码后 43 个字符
			if len(challenge) != 43 {
				t.Errorf("challenge 长度应为 43，实际为 %d", len(challenge))
			}

			// 验证确定性：相同输入应产生相同输出
			challenge2 := generateCodeChallenge(tt.verifier)
			if challenge != challenge2 {
				t.Error("相同 verifier 产生了不同的 challenge")
			}
		})
	}
}

func TestGenerateCodeChallenge_DifferentInputs(t *testing.T) {
	// 不同输入应产生不同输出
	c1 := generateCodeChallenge("verifier_a")
	c2 := generateCodeChallenge("verifier_b")
	if c1 == c2 {
		t.Error("不同 verifier 产生了相同的 challenge")
	}
}

func TestOAuthClient_GetAccessToken_Cached(t *testing.T) {
	store := newMockTokenStore()
	config := OAuthConfig{
		ClientID: "test-client",
		AuthURL:  "https://example.com/auth",
		TokenURL: "https://example.com/token",
	}
	client := NewOAuthClient(config, store, nil)
	// 使用 nop logger 避免 nil panic
	client.logger = noopLogger()

	// 预存一个未过期的 token
	ctx := context.Background()
	store.tokens["https://mcp.example.com"] = &OAuthToken{
		AccessToken: "cached-token-123",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	result, err := client.GetAccessToken(ctx, "https://mcp.example.com")
	if err != nil {
		t.Fatalf("GetAccessToken 返回错误: %v", err)
	}

	expected := "Bearer cached-token-123"
	if result != expected {
		t.Errorf("期望 %q，实际 %q", expected, result)
	}
}

func TestOAuthClient_GetAccessToken_NoToken(t *testing.T) {
	store := newMockTokenStore()
	config := OAuthConfig{
		ClientID: "test-client",
		AuthURL:  "https://example.com/auth",
		TokenURL: "https://example.com/token",
	}
	client := NewOAuthClient(config, store, nil)
	client.logger = noopLogger()

	// 无缓存 token 且没有真实服务器，应该启动 PKCE 流程然后超时
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetAccessToken(ctx, "https://mcp.example.com")
	if err == nil {
		t.Fatal("期望返回错误（无真实 OAuth 服务器），但没有返回错误")
	}
}

func TestMockTokenStore_CRUD(t *testing.T) {
	store := newMockTokenStore()
	ctx := context.Background()
	serverURL := "https://mcp.test.com"

	token := &OAuthToken{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scopes:       "read write",
	}

	// 保存
	if err := store.SaveToken(ctx, serverURL, token); err != nil {
		t.Fatalf("SaveToken 失败: %v", err)
	}

	// 加载
	loaded, err := store.LoadToken(ctx, serverURL)
	if err != nil {
		t.Fatalf("LoadToken 失败: %v", err)
	}
	if loaded.AccessToken != "access-123" {
		t.Errorf("AccessToken 不匹配: %q", loaded.AccessToken)
	}
	if loaded.RefreshToken != "refresh-456" {
		t.Errorf("RefreshToken 不匹配: %q", loaded.RefreshToken)
	}

	// 删除
	if err := store.DeleteToken(ctx, serverURL); err != nil {
		t.Fatalf("DeleteToken 失败: %v", err)
	}

	// 验证已删除
	deleted, err := store.LoadToken(ctx, serverURL)
	if err != nil {
		t.Fatalf("LoadToken 返回意外错误: %v", err)
	}
	if deleted != nil {
		t.Error("期望 token 已被删除，但仍存在")
	}
}

// noopLogger 创建一个不输出任何内容的 logger（用于测试）
func noopLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	if l == nil {
		l = zap.NewNop()
	}
	return l
}
