package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/chef-guo/agents-hive/internal/channel"
	"github.com/chef-guo/agents-hive/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestPluginPlatform(t *testing.T) {
	p := &Plugin{
		sessionWebhooks: make(map[string]string),
	}
	assert.Equal(t, channel.PlatformDingTalk, p.Platform())
}

func TestCacheWebhook(t *testing.T) {
	p := &Plugin{
		sessionWebhooks: make(map[string]string),
	}
	p.CacheWebhook("chat-1", "https://example.com/webhook")

	p.mu.RLock()
	url := p.sessionWebhooks["chat-1"]
	p.mu.RUnlock()
	assert.Equal(t, "https://example.com/webhook", url)
}

func TestVerify_WithAppSecret(t *testing.T) {
	appSecret := "test-secret"
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	stringToSign := fmt.Sprintf("%s\n%s", timestamp, appSecret)
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	p := &Plugin{
		cfg:             config.DingTalkConfig{AppSecret: appSecret},
		sessionWebhooks: make(map[string]string),
	}

	// Valid signature
	req := httptest.NewRequest("POST", "/webhook", nil)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("sign", sign)
	assert.True(t, p.Verify(req))

	// Invalid signature
	req2 := httptest.NewRequest("POST", "/webhook", nil)
	req2.Header.Set("timestamp", timestamp)
	req2.Header.Set("sign", "wrong")
	assert.False(t, p.Verify(req2))
}

func TestVerify_WithoutAppSecret(t *testing.T) {
	p := &Plugin{
		cfg:             config.DingTalkConfig{},
		sessionWebhooks: make(map[string]string),
	}
	req := httptest.NewRequest("POST", "/webhook", nil)
	assert.True(t, p.Verify(req)) // 未配置时跳过验证
}
