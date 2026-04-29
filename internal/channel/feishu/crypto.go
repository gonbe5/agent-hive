package feishu

import (
	"crypto/sha256"
	"fmt"
)

// VerifySignature 验证飞书事件回调签名
// 签名算法: SHA256(timestamp + nonce + encryptKey + body)
func VerifySignature(timestamp, nonce, encryptKey, body, signature string) bool {
	if signature == "" || encryptKey == "" {
		return false
	}

	// 拼接待签名字符串
	toSign := timestamp + nonce + encryptKey + body
	h := sha256.New()
	h.Write([]byte(toSign))
	computed := fmt.Sprintf("%x", h.Sum(nil))

	return computed == signature
}
