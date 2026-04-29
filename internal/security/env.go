package security

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
)

// EnvValidator 环境变量完整性校验器
// 在启动时记录关键环境变量的 SHA256 指纹，运行时检测是否被篡改
type EnvValidator struct {
	snapshot map[string]string // key → sha256(value)
}

// NewEnvValidator 创建环境变量校验器
func NewEnvValidator() *EnvValidator {
	return &EnvValidator{
		snapshot: make(map[string]string),
	}
}

// Snapshot 快照指定环境变量
func (v *EnvValidator) Snapshot(keys []string) {
	for _, key := range keys {
		val := os.Getenv(key)
		if val != "" {
			v.snapshot[key] = hashValue(val)
		}
	}
}

// Validate 检查环境变量是否被篡改
// 返回被篡改的变量名列表
func (v *EnvValidator) Validate() []string {
	var tampered []string
	for key, expectedHash := range v.snapshot {
		currentVal := os.Getenv(key)
		if currentVal == "" {
			tampered = append(tampered, key+" (已删除)")
			continue
		}
		if hashValue(currentVal) != expectedHash {
			tampered = append(tampered, key+" (已修改)")
		}
	}
	sort.Strings(tampered)
	return tampered
}

// Fingerprint 返回所有快照变量的综合指纹
func (v *EnvValidator) Fingerprint() string {
	var parts []string
	for k, h := range v.snapshot {
		parts = append(parts, k+"="+h)
	}
	sort.Strings(parts)
	return hashValue(strings.Join(parts, ";"))
}

func hashValue(val string) string {
	h := sha256.Sum256([]byte(val))
	return fmt.Sprintf("%x", h)
}
