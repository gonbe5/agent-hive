package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// 指令文件名优先级列表（按查找顺序）
var instructionFileNames = []string{
	".claw/AGENTS.md",
	"CLAUDE.md",
}

// maxParentLevels 向上递归查找的最大层级数
const maxParentLevels = 10

// LoadInstructions 从 workDir 向上递归查找指令文件并合并返回。
//
// 查找逻辑：
//   - 从 workDir 开始，依次向上查找父目录
//   - 在每个目录中查找 .claw/AGENTS.md 或 CLAUDE.md（优先级按此顺序）
//   - 最多向上查找 maxParentLevels 级目录
//   - 子目录的指令优先级更高（放在最后，覆盖父目录）
//
// 返回合并后的指令文本，各级指令之间以分隔线分隔。
func LoadInstructions(workDir string, logger *zap.Logger) string {
	absDir, err := filepath.Abs(workDir)
	if err != nil {
		logger.Warn("无法解析工作目录绝对路径", zap.String("workDir", workDir), zap.Error(err))
		return ""
	}

	// 收集各级目录找到的指令（从根到子的顺序）
	type instruction struct {
		dir     string
		content string
	}
	var instructions []instruction

	currentDir := absDir
	for level := 0; level < maxParentLevels; level++ {
		content := findInstructionFile(currentDir, logger)
		if content != "" {
			instructions = append(instructions, instruction{
				dir:     currentDir,
				content: content,
			})
		}

		// 向上一级
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// 已经到达根目录
			break
		}
		currentDir = parent
	}

	if len(instructions) == 0 {
		return ""
	}

	// 反转顺序：父目录在前，子目录在后（子目录优先级更高）
	reversed := make([]string, len(instructions))
	for i, inst := range instructions {
		reversed[len(instructions)-1-i] = inst.content
	}

	result := strings.Join(reversed, "\n\n---\n\n")
	logger.Info("加载指令文件完成",
		zap.Int("层级数", len(instructions)),
		zap.String("workDir", absDir),
	)
	return result
}

// findInstructionFile 在指定目录中查找指令文件，返回文件内容。
// 按优先级查找：.claw/AGENTS.md > CLAUDE.md
// 找到第一个即返回。
func findInstructionFile(dir string, logger *zap.Logger) string {
	for _, name := range instructionFileNames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				logger.Warn("读取指令文件失败",
					zap.String("path", path),
					zap.Error(err),
				)
			}
			continue
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		logger.Debug("找到指令文件",
			zap.String("path", path),
			zap.Int("size", len(content)),
		)
		return content
	}
	return ""
}

// remoteInstructionTimeout HTTP 远程指令加载超时时间
const remoteInstructionTimeout = 5 * time.Second

// remoteInstructionMaxSize 远程指令内容最大大小（512KB）
const remoteInstructionMaxSize = 512 * 1024

// LoadRemoteInstruction 从 HTTP URL 加载远程指令内容
// 超时 5s，最大 512KB，失败时静默返回空字符串
func LoadRemoteInstruction(url string, logger *zap.Logger) string {
	client := &http.Client{Timeout: remoteInstructionTimeout}
	resp, err := client.Get(url)
	if err != nil {
		logger.Warn("加载远程指令失败", zap.String("url", url), zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Warn("远程指令返回非 200 状态码",
			zap.String("url", url),
			zap.Int("status", resp.StatusCode),
		)
		return ""
	}

	// 限制读取大小
	limited := io.LimitReader(resp.Body, int64(remoteInstructionMaxSize)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		logger.Warn("读取远程指令内容失败", zap.String("url", url), zap.Error(err))
		return ""
	}

	if len(data) > remoteInstructionMaxSize {
		logger.Warn("远程指令内容超过大小限制",
			zap.String("url", url),
			zap.Int("size", len(data)),
			zap.Int("maxSize", remoteInstructionMaxSize),
		)
		return ""
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}

	logger.Info("加载远程指令完成",
		zap.String("url", url),
		zap.Int("size", len(content)),
	)
	return content
}

// LoadInstructionsWithRemote 加载本地指令文件并追加远程 URL 内容
// 远程指令优先级低于本地文件（追加在最后）
func LoadInstructionsWithRemote(workDir string, urls []string, logger *zap.Logger) string {
	local := LoadInstructions(workDir, logger)

	if len(urls) == 0 {
		return local
	}

	var remoteParts []string
	for _, u := range urls {
		content := LoadRemoteInstruction(u, logger)
		if content != "" {
			remoteParts = append(remoteParts, fmt.Sprintf("<!-- remote: %s -->\n%s", u, content))
		}
	}

	if len(remoteParts) == 0 {
		return local
	}

	remote := strings.Join(remoteParts, "\n\n---\n\n")
	if local == "" {
		return remote
	}
	return local + "\n\n---\n\n" + remote
}
