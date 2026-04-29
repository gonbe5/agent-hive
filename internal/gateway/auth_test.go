package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuthManager(t *testing.T) {
	am := NewAuthManager([]string{"test-token"})

	// 验证有效 token
	token, err := am.Authenticate("Bearer test-token")
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.True(t, am.HasScope(token, "read"))
	assert.True(t, am.HasScope(token, "admin"))

	// 验证无效 token
	token2, err := am.Authenticate("Bearer invalid")
	assert.NoError(t, err)
	assert.Nil(t, token2)

	// 空 token
	token3, err := am.Authenticate("")
	assert.NoError(t, err)
	assert.Nil(t, token3)
}

func TestAuthManager_EmptyTokensFiltered(t *testing.T) {
	// 模拟 ${GATEWAY_TOKEN} 未设置时展开为空字符串的场景
	am := NewAuthManager([]string{""})

	// 空字符串不应被注册为有效 token
	token, err := am.Authenticate("")
	assert.NoError(t, err)
	assert.Nil(t, token, "空字符串 token 不应通过认证")

	// 带任何实际值的 token 也不应通过（空列表无有效 token）
	token2, err := am.Authenticate("Bearer some-token")
	assert.NoError(t, err)
	assert.Nil(t, token2, "不存在的 token 不应通过认证")
}

// ---- ExtractClientIP 安全测试 ----

// newReqWithHeaders 创建带指定头部的测试请求
func newReqWithHeaders(remoteAddr, xff, xrealip string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if xrealip != "" {
		req.Header.Set("X-Real-IP", xrealip)
	}
	return req
}

// TestExtractClientIP_NoTrustedProxy 验证：无受信任代理时，始终使用 RemoteAddr，
// 忽略 X-Forwarded-For / X-Real-IP，防止攻击者伪造 IP 绕过速率限制。
func TestExtractClientIP_NoTrustedProxy(t *testing.T) {
	rl := NewIPRateLimiterWithProxies(100, time.Minute, nil)
	defer rl.Stop()

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xrealip    string
		wantIP     string
	}{
		{
			name:       "直连请求，无代理头",
			remoteAddr: "1.2.3.4:5678",
			wantIP:     "1.2.3.4",
		},
		{
			name:       "攻击者伪造 X-Forwarded-For，应被忽略",
			remoteAddr: "1.2.3.4:5678",
			xff:        "127.0.0.1",
			wantIP:     "1.2.3.4",
		},
		{
			name:       "攻击者伪造 X-Real-IP，应被忽略",
			remoteAddr: "1.2.3.4:5678",
			xrealip:    "10.0.0.1",
			wantIP:     "1.2.3.4",
		},
		{
			name:       "同时伪造两个头，均应被忽略",
			remoteAddr: "5.6.7.8:9999",
			xff:        "127.0.0.1",
			xrealip:    "192.168.1.1",
			wantIP:     "5.6.7.8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newReqWithHeaders(tc.remoteAddr, tc.xff, tc.xrealip)
			got := rl.ExtractClientIP(req)
			assert.Equal(t, tc.wantIP, got)
		})
	}
}

// TestExtractClientIP_WithTrustedProxy 验证：RemoteAddr 在受信任代理列表中时，
// 才读取 X-Forwarded-For / X-Real-IP 获取真实客户端 IP。
func TestExtractClientIP_WithTrustedProxy(t *testing.T) {
	// 配置 10.0.0.1 为受信任代理
	rl := NewIPRateLimiterWithProxies(100, time.Minute, []string{"10.0.0.1"})
	defer rl.Stop()

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xrealip    string
		wantIP     string
	}{
		{
			name:       "受信任代理转发，读取 X-Forwarded-For",
			remoteAddr: "10.0.0.1:12345",
			xff:        "203.0.113.5",
			wantIP:     "203.0.113.5",
		},
		{
			name:       "受信任代理转发，X-Forwarded-For 多跳取第一个",
			remoteAddr: "10.0.0.1:12345",
			xff:        "203.0.113.5, 10.0.0.2",
			wantIP:     "203.0.113.5",
		},
		{
			name:       "受信任代理，无 X-Forwarded-For，读取 X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			xrealip:    "203.0.113.9",
			wantIP:     "203.0.113.9",
		},
		{
			name:       "受信任代理，无任何代理头，回退到代理自身 IP",
			remoteAddr: "10.0.0.1:12345",
			wantIP:     "10.0.0.1",
		},
		{
			name:       "非受信任代理发来的请求，忽略代理头",
			remoteAddr: "99.99.99.99:5000",
			xff:        "127.0.0.1",
			wantIP:     "99.99.99.99",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := newReqWithHeaders(tc.remoteAddr, tc.xff, tc.xrealip)
			got := rl.ExtractClientIP(req)
			assert.Equal(t, tc.wantIP, got)
		})
	}
}

// TestPackageLevelExtractClientIP 验证包级别便捷函数始终返回 RemoteAddr，不信任代理头
func TestPackageLevelExtractClientIP(t *testing.T) {
	req := newReqWithHeaders("8.8.8.8:1234", "1.1.1.1", "2.2.2.2")
	got := ExtractClientIP(req)
	assert.Equal(t, "8.8.8.8", got, "包级别函数不应信任代理头")
}

// TestRateLimiter_BypassPrevented 集成验证：攻击者伪造 X-Forwarded-For 无法绕过速率限制
func TestRateLimiter_BypassPrevented(t *testing.T) {
	// 无受信任代理，每个 IP 每窗口最多 3 次
	rl := NewIPRateLimiterWithProxies(3, time.Minute, nil)
	defer rl.Stop()

	// 模拟攻击者：RemoteAddr 固定为 attacker-ip，尝试通过修改 XFF 头来"换 IP"
	makeReq := func(xff string) *http.Request {
		return newReqWithHeaders("attacker:9000", xff, "")
	}

	// 前 3 次正常通过
	for i := 0; i < 3; i++ {
		// 每次伪造不同的 XFF，试图重置计数器
		ip := rl.ExtractClientIP(makeReq("10.20.30." + string(rune('0'+i))))
		assert.Equal(t, "attacker", ip, "应使用 RemoteAddr 而非伪造 IP")
		assert.True(t, rl.Allow(ip), "第 %d 次请求应通过", i+1)
	}

	// 第 4 次：伪造全新 XFF，但仍应被 RemoteAddr（attacker）限流
	ip := rl.ExtractClientIP(makeReq("1.2.3.4"))
	assert.Equal(t, "attacker", ip)
	assert.False(t, rl.Allow(ip), "超限请求应被拒绝，伪造 XFF 无效")
}
