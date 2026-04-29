package security

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvValidator(t *testing.T) {
	// 设置测试环境变量
	os.Setenv("TEST_ENV_VAR", "test_value")
	defer os.Unsetenv("TEST_ENV_VAR")

	v := NewEnvValidator()
	v.Snapshot([]string{"TEST_ENV_VAR", "NON_EXISTENT_VAR"})

	// 未修改时应通过验证
	tampered := v.Validate()
	assert.Empty(t, tampered)

	// 修改环境变量后应检测到篡改
	os.Setenv("TEST_ENV_VAR", "modified_value")
	tampered = v.Validate()
	assert.Len(t, tampered, 1)
	assert.Contains(t, tampered[0], "已修改")

	// 恢复
	os.Setenv("TEST_ENV_VAR", "test_value")
	tampered = v.Validate()
	assert.Empty(t, tampered)
}

func TestFingerprint(t *testing.T) {
	os.Setenv("TEST_FP_VAR", "value1")
	defer os.Unsetenv("TEST_FP_VAR")

	v := NewEnvValidator()
	v.Snapshot([]string{"TEST_FP_VAR"})

	fp1 := v.Fingerprint()
	assert.NotEmpty(t, fp1)

	// 相同输入应产生相同指纹
	fp2 := v.Fingerprint()
	assert.Equal(t, fp1, fp2)
}
