package dingtalk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"time"
)

// VerifySignature 验证钉钉回调签名
// timestamp: 请求头中的时间戳（毫秒）
// sign: 请求头中的签名
// appSecret: 机器人的 AppSecret
func VerifySignature(timestamp, sign, appSecret string) bool {
	if timestamp == "" || sign == "" || appSecret == "" {
		return false
	}

	// 验证时间戳，防止重放攻击（1小时内有效）
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	now := time.Now().UnixMilli()
	if math.Abs(float64(now-ts)) > 3600000 { // 1小时
		return false
	}

	// 计算签名: Base64(HmacSHA256(timestamp + "\n" + appSecret))
	stringToSign := fmt.Sprintf("%s\n%s", timestamp, appSecret)
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write([]byte(stringToSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(sign))
}
