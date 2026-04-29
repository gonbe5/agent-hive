package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// VerifySignature 验证企业微信回调签名
func VerifySignature(token, timestamp, nonce, encrypt, signature string) bool {
	strs := []string{token, timestamp, nonce, encrypt}
	sort.Strings(strs)
	joined := strings.Join(strs, "")
	h := sha1.New()
	h.Write([]byte(joined))
	computed := fmt.Sprintf("%x", h.Sum(nil))
	return computed == signature
}

// DecryptMessage 解密企业微信消息
// encodingAESKey 为配置中的 AES Key（Base64 编码，43字符）
func DecryptMessage(encodingAESKey, encrypted string) ([]byte, error) {
	// AES Key = Base64Decode(encodingAESKey + "=")
	aesKey, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, "解码 AES Key 失败", err)
	}

	cipherText, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInvalidInput, "解码密文失败", err)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "创建 AES cipher 失败", err)
	}

	if len(cipherText) < aes.BlockSize {
		return nil, errs.New(errs.CodeInvalidInput, "密文太短")
	}

	iv := aesKey[:aes.BlockSize]
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(cipherText, cipherText)

	// PKCS#7 去填充
	pad := int(cipherText[len(cipherText)-1])
	if pad > aes.BlockSize || pad == 0 {
		return nil, errs.New(errs.CodeInvalidInput, "无效的 PKCS7 填充")
	}
	cipherText = cipherText[:len(cipherText)-pad]

	// 格式：随机16字节 + 4字节消息长度 + 消息内容 + 接收方ID
	if len(cipherText) < 20 {
		return nil, errs.New(errs.CodeInvalidInput, "解密后数据太短")
	}

	msgLen := binary.BigEndian.Uint32(cipherText[16:20])
	if uint32(len(cipherText)) < 20+msgLen {
		return nil, errs.New(errs.CodeInvalidInput, "消息长度不匹配")
	}

	return cipherText[20 : 20+msgLen], nil
}
