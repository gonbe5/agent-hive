package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

// encryptedPrefix 是加密数据的唯一前缀标识符。
// 解密时以此前缀区分密文与明文，彻底规避旧版冒号歧义问题：
// 合法的 OAuth token（如 URL 形式）可能包含 ":"，而不会以 "enc:" 开头。
const encryptedPrefix = "enc:"

// tokenCrypto 是包级别的加密器单例，在首次调用时初始化。
// 如果环境变量 OAUTH_ENCRYPTION_KEY 未配置，则回退到明文模式（向后兼容）。
var (
	tokenCryptoOnce sync.Once
	tokenCryptoKey  []byte // 32 字节 AES-256-GCM 密钥，nil 表示明文模式
)

// initTokenCrypto 初始化 token 加密密钥（仅执行一次）。
// 密钥来源: 环境变量 OAUTH_ENCRYPTION_KEY（64 字符 hex 编码的 32 字节密钥）。
// 若未配置，tokenCryptoKey 为 nil，所有加解密操作均为明文直通模式。
func initTokenCrypto() {
	tokenCryptoOnce.Do(func() {
		raw := os.Getenv("OAUTH_ENCRYPTION_KEY")
		if raw == "" {
			// 未配置密钥，使用明文模式（向后兼容）
			return
		}

		key, err := hex.DecodeString(raw)
		if err != nil || len(key) != 32 {
			// 密钥格式不合法，回退明文模式并记录到 stderr（此时 logger 未必可用）
			tokenCryptoKey = nil
			return
		}

		tokenCryptoKey = key
	})
}

// encryptToken 使用 AES-256-GCM 加密 plaintext。
// 返回格式: "enc:" + hex(nonce) + ":" + hex(ciphertext)。
// 固定前缀 "enc:" 用于 decryptToken 可靠区分密文与明文，
// 解决旧版仅依赖冒号导致含 ":" 的合法 OAuth token 被误判的安全缺陷。
// 若密钥未配置，直接返回原文（明文模式）。
func encryptToken(plaintext string) (string, error) {
	initTokenCrypto()

	if tokenCryptoKey == nil {
		// 明文模式直通
		return plaintext, nil
	}

	block, err := aes.NewCipher(tokenCryptoKey)
	if err != nil {
		return "", errors.New("创建 AES cipher 失败: " + err.Error())
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("创建 AES-GCM 失败: " + err.Error())
	}

	// 生成随机 nonce（12 字节）
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.New("生成随机 nonce 失败: " + err.Error())
	}

	ciphertext := aesGCM.Seal(nil, nonce, []byte(plaintext), nil)

	// 编码为 "enc:hex(nonce):hex(ciphertext)" 格式
	return encryptedPrefix + hex.EncodeToString(nonce) + ":" + hex.EncodeToString(ciphertext), nil
}

// decryptToken 解密 encryptToken 返回的密文。
// 判断逻辑：以 "enc:" 前缀开头 → 为密文，执行解密；否则视为明文直接返回（向后兼容）。
// 此方式彻底避免旧版冒号歧义：含 ":" 的明文 OAuth token（如 URL）不会被误判为密文。
func decryptToken(stored string) (string, error) {
	initTokenCrypto()

	if tokenCryptoKey == nil {
		// 明文模式直通
		return stored, nil
	}

	// 不以 "enc:" 开头 → 旧明文数据，直接返回（向后兼容）
	if !strings.HasPrefix(stored, encryptedPrefix) {
		return stored, nil
	}

	// 去除前缀后解析 "hex(nonce):hex(ciphertext)"
	body := stored[len(encryptedPrefix):]
	colonIdx := strings.Index(body, ":")
	if colonIdx < 0 {
		// 格式损坏，保守地返回原始字符串避免数据丢失
		return stored, nil
	}

	nonceHex := body[:colonIdx]
	dataHex := body[colonIdx+1:]

	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", errors.New("解码 nonce 失败: " + err.Error())
	}

	data, err := hex.DecodeString(dataHex)
	if err != nil {
		return "", errors.New("解码密文失败: " + err.Error())
	}

	block, err := aes.NewCipher(tokenCryptoKey)
	if err != nil {
		return "", errors.New("创建 AES cipher 失败: " + err.Error())
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("创建 AES-GCM 失败: " + err.Error())
	}

	if len(nonce) != aesGCM.NonceSize() {
		return "", errors.New("nonce 长度不匹配，密文可能已损坏")
	}

	plaintext, err := aesGCM.Open(nil, nonce, data, nil)
	if err != nil {
		return "", errors.New("AES-GCM 解密失败，密文可能已篡改或损坏: " + err.Error())
	}

	return string(plaintext), nil
}

// isEncryptionEnabled 返回当前是否处于加密模式（密钥已配置）。
// 可用于日志警告判断。
func isEncryptionEnabled() bool {
	initTokenCrypto()
	return tokenCryptoKey != nil
}
