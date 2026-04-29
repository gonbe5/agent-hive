package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVerifySignature_Valid(t *testing.T) {
	appSecret := "test-secret-key-12345"
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// 生成有效签名
	stringToSign := fmt.Sprintf("%s\n%s", timestamp, appSecret)
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	assert.True(t, VerifySignature(timestamp, sign, appSecret))
}

func TestVerifySignature_InvalidSign(t *testing.T) {
	appSecret := "test-secret-key-12345"
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	assert.False(t, VerifySignature(timestamp, "invalid-sign", appSecret))
}

func TestVerifySignature_ExpiredTimestamp(t *testing.T) {
	appSecret := "test-secret-key-12345"
	// 2小时前的时间戳
	timestamp := strconv.FormatInt(time.Now().Add(-2*time.Hour).UnixMilli(), 10)

	stringToSign := fmt.Sprintf("%s\n%s", timestamp, appSecret)
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	assert.False(t, VerifySignature(timestamp, sign, appSecret))
}

func TestVerifySignature_EmptyFields(t *testing.T) {
	assert.False(t, VerifySignature("", "sign", "secret"))
	assert.False(t, VerifySignature("123", "", "secret"))
	assert.False(t, VerifySignature("123", "sign", ""))
}

func TestVerifySignature_InvalidTimestamp(t *testing.T) {
	assert.False(t, VerifySignature("not-a-number", "sign", "secret"))
}
