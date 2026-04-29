package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadInstructions(t *testing.T) {
	logger := zap.NewNop()

	t.Run("无指令文件返回空字符串", func(t *testing.T) {
		dir := t.TempDir()
		result := LoadInstructions(dir, logger)
		assert.Equal(t, "", result)
	})

	t.Run("加载单个 CLAUDE.md", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# 项目指令\n使用中文注释")

		result := LoadInstructions(dir, logger)
		assert.Equal(t, "# 项目指令\n使用中文注释", result)
	})

	t.Run(".claw/AGENTS.md 优先于 CLAUDE.md", func(t *testing.T) {
		dir := t.TempDir()

		clawDir := filepath.Join(dir, ".claw")
		require.NoError(t, os.MkdirAll(clawDir, 0755))

		writeFile(t, filepath.Join(clawDir, "AGENTS.md"), "AGENTS 指令")
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "CLAUDE 指令")

		result := LoadInstructions(dir, logger)
		assert.Equal(t, "AGENTS 指令", result)
	})

	t.Run("递归合并父子目录指令", func(t *testing.T) {
		// 创建目录结构: root/sub
		root := t.TempDir()
		sub := filepath.Join(root, "sub")
		require.NoError(t, os.MkdirAll(sub, 0755))

		writeFile(t, filepath.Join(root, "CLAUDE.md"), "根目录指令")
		writeFile(t, filepath.Join(sub, "CLAUDE.md"), "子目录指令")

		result := LoadInstructions(sub, logger)
		// 父目录在前，子目录在后
		assert.Contains(t, result, "根目录指令")
		assert.Contains(t, result, "子目录指令")

		// 确保顺序：根目录在前
		rootIdx := indexOf(result, "根目录指令")
		subIdx := indexOf(result, "子目录指令")
		assert.True(t, rootIdx < subIdx, "父目录指令应在子目录指令之前")
	})

	t.Run("跳过空文件", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "")

		result := LoadInstructions(dir, logger)
		assert.Equal(t, "", result)
	})

	t.Run("多级目录合并", func(t *testing.T) {
		// root/a/b
		root := t.TempDir()
		a := filepath.Join(root, "a")
		b := filepath.Join(a, "b")
		require.NoError(t, os.MkdirAll(b, 0755))

		writeFile(t, filepath.Join(root, "CLAUDE.md"), "L0")
		// a 目录没有指令文件
		writeFile(t, filepath.Join(b, "CLAUDE.md"), "L2")

		result := LoadInstructions(b, logger)
		assert.Contains(t, result, "L0")
		assert.Contains(t, result, "L2")
		assert.True(t, indexOf(result, "L0") < indexOf(result, "L2"))
	})
}

func TestFindInstructionFile(t *testing.T) {
	logger := zap.NewNop()

	t.Run("目录为空返回空", func(t *testing.T) {
		dir := t.TempDir()
		result := findInstructionFile(dir, logger)
		assert.Equal(t, "", result)
	})

	t.Run("找到 CLAUDE.md", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "指令内容")

		result := findInstructionFile(dir, logger)
		assert.Equal(t, "指令内容", result)
	})

	t.Run("找到 .claw/AGENTS.md", func(t *testing.T) {
		dir := t.TempDir()
		clawDir := filepath.Join(dir, ".claw")
		require.NoError(t, os.MkdirAll(clawDir, 0755))
		writeFile(t, filepath.Join(clawDir, "AGENTS.md"), "AGENTS 内容")

		result := findInstructionFile(dir, logger)
		assert.Equal(t, "AGENTS 内容", result)
	})
}

// indexOf 返回子串在字符串中的位置
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestLoadRemoteInstruction(t *testing.T) {
	logger := zap.NewNop()

	t.Run("成功加载远程指令", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "远程指令内容")
		}))
		defer server.Close()

		result := LoadRemoteInstruction(server.URL, logger)
		assert.Equal(t, "远程指令内容", result)
	})

	t.Run("服务器返回 404", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		result := LoadRemoteInstruction(server.URL, logger)
		assert.Equal(t, "", result)
	})

	t.Run("服务器返回空内容", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "   \n  ")
		}))
		defer server.Close()

		result := LoadRemoteInstruction(server.URL, logger)
		assert.Equal(t, "", result)
	})

	t.Run("超大内容被拒绝", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 写入超过 512KB 的内容
			fmt.Fprint(w, strings.Repeat("A", remoteInstructionMaxSize+100))
		}))
		defer server.Close()

		result := LoadRemoteInstruction(server.URL, logger)
		assert.Equal(t, "", result)
	})

	t.Run("无效 URL 返回空", func(t *testing.T) {
		result := LoadRemoteInstruction("http://invalid-host-that-does-not-exist.example.com/instructions", logger)
		assert.Equal(t, "", result)
	})
}

func TestLoadInstructionsWithRemote(t *testing.T) {
	logger := zap.NewNop()

	t.Run("无远程 URL 等同于 LoadInstructions", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "本地指令")

		result := LoadInstructionsWithRemote(dir, nil, logger)
		assert.Equal(t, "本地指令", result)
	})

	t.Run("远程内容追加在本地之后", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "CLAUDE.md"), "本地指令")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "远程指令")
		}))
		defer server.Close()

		result := LoadInstructionsWithRemote(dir, []string{server.URL}, logger)
		assert.Contains(t, result, "本地指令")
		assert.Contains(t, result, "远程指令")
		assert.True(t, indexOf(result, "本地指令") < indexOf(result, "远程指令"))
	})

	t.Run("仅远程内容（无本地文件）", func(t *testing.T) {
		dir := t.TempDir()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "仅远程")
		}))
		defer server.Close()

		result := LoadInstructionsWithRemote(dir, []string{server.URL}, logger)
		assert.Contains(t, result, "仅远程")
	})
}
