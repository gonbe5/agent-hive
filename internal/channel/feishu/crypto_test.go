package feishu

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifySignature_Valid(t *testing.T) {
	timestamp := "1234567890"
	nonce := "test-nonce"
	encryptKey := "test-encrypt-key"
	body := `{"event":"test"}`

	// 计算正确签名
	toSign := timestamp + nonce + encryptKey + body
	h := sha256.New()
	h.Write([]byte(toSign))
	signature := fmt.Sprintf("%x", h.Sum(nil))

	assert.True(t, VerifySignature(timestamp, nonce, encryptKey, body, signature))
}

func TestVerifySignature_Invalid(t *testing.T) {
	assert.False(t, VerifySignature("ts", "nonce", "key", "body", "wrong-signature"))
}

func TestVerifySignature_EmptyParams(t *testing.T) {
	assert.False(t, VerifySignature("ts", "nonce", "", "body", "sig"))
	assert.False(t, VerifySignature("ts", "nonce", "key", "body", ""))
}
