package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTIssueAndVerify(t *testing.T) {
	mgr := NewJWTManager("test-secret", time.Hour, 7*24*time.Hour)
	token, err := mgr.Issue("user-123", "admin", "feishu")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := mgr.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "admin", claims.Role)
	assert.Equal(t, "feishu", claims.Provider)
	assert.NotZero(t, claims.OriginalIssuedAt)
	assert.Contains(t, claims.Scopes, "admin")
}

func TestJWTVerifyExpired(t *testing.T) {
	mgr := NewJWTManager("test-secret", -time.Second, 7*24*time.Hour)
	token, err := mgr.Issue("user-123", "user", "feishu")
	require.NoError(t, err)

	_, err = mgr.Verify(token)
	assert.Error(t, err)
}

func TestJWTVerifyTampered(t *testing.T) {
	mgr := NewJWTManager("test-secret", time.Hour, 7*24*time.Hour)
	token, err := mgr.Issue("user-123", "user", "feishu")
	require.NoError(t, err)

	tampered := token + "x"
	_, err = mgr.Verify(tampered)
	assert.Error(t, err)
}

func TestJWTVerifyWrongSecret(t *testing.T) {
	mgr1 := NewJWTManager("secret-1", time.Hour, 7*24*time.Hour)
	mgr2 := NewJWTManager("secret-2", time.Hour, 7*24*time.Hour)

	token, err := mgr1.Issue("user-123", "user", "feishu")
	require.NoError(t, err)

	_, err = mgr2.Verify(token)
	assert.Error(t, err)
}

func TestJWTRefreshNormal(t *testing.T) {
	mgr := NewJWTManager("test-secret", time.Hour, 7*24*time.Hour)
	token, err := mgr.Issue("user-123", "admin", "feishu")
	require.NoError(t, err)

	oldClaims, err := mgr.Verify(token)
	require.NoError(t, err)

	newToken, err := mgr.Refresh(token)
	require.NoError(t, err)
	require.NotEmpty(t, newToken)

	newClaims, err := mgr.Verify(newToken)
	require.NoError(t, err)
	assert.Equal(t, "user-123", newClaims.Subject)
	assert.Equal(t, oldClaims.OriginalIssuedAt, newClaims.OriginalIssuedAt, "OriginalIssuedAt 应保持不变")
}

func TestJWTRefreshExceedsMaxTTL(t *testing.T) {
	// maxTTL 设为负数，模拟已超过绝对过期时间
	mgr := NewJWTManager("test-secret", time.Hour, -time.Second)
	token, err := mgr.Issue("user-123", "user", "feishu")
	require.NoError(t, err)

	_, err = mgr.Refresh(token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "最大有效期")
}

func TestJWTIssueAndRefreshPreserveScopes(t *testing.T) {
	mgr := NewJWTManager("test-secret", time.Hour, 7*24*time.Hour)

	token, err := mgr.IssueWithScopes("user-123", "user", "feishu", []string{"read", "push:write"})
	require.NoError(t, err)

	claims, err := mgr.Verify(token)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"read", "push:write"}, claims.Scopes)

	refreshed, err := mgr.Refresh(token)
	require.NoError(t, err)

	refreshedClaims, err := mgr.Verify(refreshed)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"read", "push:write"}, refreshedClaims.Scopes)
}
