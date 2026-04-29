// Package regression — session-scope-regression-matrix Phase 1.4 (R-3)
//
// Grep-based enforcement 红测：驱动 scripts/ci/check_session_scope.sh 脚本，
// 验证它对（A）干净仓库 exit 0，（B）注入的违例 exit 1 + 精确 file:line 输出。
// 脚本本身是 enforcement 的一线防御；本测试是对脚本行为的回归保证。
package regression

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot 返回仓库根目录（本文件位于 tests/regression/，根在上两级）
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func runCheckScript(t *testing.T, root string) (exitCode int, stderr string) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(root, "scripts/ci/check_session_scope.sh"))
	cmd.Env = append(os.Environ(), "REPO_ROOT="+root)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	stderr = stderrBuf.String()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), stderr
		}
		t.Fatalf("exec failed: %v", err)
	}
	return 0, stderr
}

func TestRedR3_CleanBaseline_ScriptExitsZero(t *testing.T) {
	root := repoRoot(t)
	exitCode, stderr := runCheckScript(t, root)
	assert.Equal(t, 0, exitCode, "干净仓库下 check_session_scope.sh 必须 exit 0，stderr=%q", stderr)
}

func TestRedR3_InjectedViolation_ScriptExitsNonZeroWithFileLine(t *testing.T) {
	root := repoRoot(t)
	probe := filepath.Join(root, "internal/master/_tmp_regression_probe.go")

	// 写入违例 probe（故意无 marker，必须被 flag）
	content := `// build ignore — tests/regression R-3 probe, auto-cleaned
package master

func _regressionProbeR1() {
	_ = func(m *Master) {
		m.eventBus.Broadcast(BroadcastMessage{Type: "test"})
	}
}

func _regressionProbeR2() {
	_ = func(m *Master) {
		m.eventBus.BroadcastGenericMessage(EventTypeAgentProgress, nil)
	}
}
`
	require.NoError(t, os.WriteFile(probe, []byte(content), 0o644))
	defer func() { _ = os.Remove(probe) }()

	exitCode, stderr := runCheckScript(t, root)
	assert.Equal(t, 1, exitCode, "注入 probe 后 script 必须 exit 1")
	assert.Contains(t, stderr, "_tmp_regression_probe.go", "stderr 必须精确指出 probe 文件名")
	assert.Contains(t, stderr, "[R-1]", "必须命中 R-1 pattern (raw Broadcast)")
	assert.Contains(t, stderr, "[R-2]", "必须命中 R-2 pattern (BroadcastGenericMessage session-scoped)")
	// 行号精确检查：R-1 在 probe 文件第 6 行，R-2 在第 12 行
	assert.Regexp(t, `_tmp_regression_probe\.go:6\b`, stderr)
	assert.Regexp(t, `_tmp_regression_probe\.go:12\b`, stderr)
}

func TestRedR3_JustifiedViolation_ScriptIgnores(t *testing.T) {
	root := repoRoot(t)
	probe := filepath.Join(root, "internal/master/_tmp_regression_probe_whitelist.go")

	content := `// build ignore — tests/regression R-3 whitelist probe, auto-cleaned
package master

func _regressionProbeWhitelisted() {
	_ = func(m *Master) {
		// no session scope by design
		m.eventBus.Broadcast(BroadcastMessage{Type: "test"})
	}
}
`
	require.NoError(t, os.WriteFile(probe, []byte(content), 0o644))
	defer func() { _ = os.Remove(probe) }()

	exitCode, stderr := runCheckScript(t, root)
	assert.Equal(t, 0, exitCode, "紧邻 `// no session scope by design` 的 Broadcast 必须豁免，stderr=%q", stderr)
	assert.False(t, strings.Contains(stderr, "_tmp_regression_probe_whitelist.go"),
		"白名单文件不应出现在 violation 报告里")
}
