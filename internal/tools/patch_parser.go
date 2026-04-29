package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// LineType 表示补丁行的类型
type LineType byte

const (
	LineContext LineType = ' ' // 上下文行（未修改）
	LineRemoved LineType = '-' // 删除的行
	LineAdded   LineType = '+' // 添加的行
)

// HunkLine 表示一行修改
type HunkLine struct {
	Type    LineType // 行类型
	Content string   // 行内容（不包含前导符号）
}

// Hunk 表示一个修改块
type Hunk struct {
	OldStart int        // 原文件起始行（1-based）
	OldLines int        // 原文件行数
	NewStart int        // 新文件起始行（1-based）
	NewLines int        // 新文件行数
	Lines    []HunkLine // 具体的修改行
}

// FilePatch 表示单个文件的补丁
type FilePatch struct {
	OldPath string // 原文件路径
	NewPath string // 新文件路径
	Hunks   []Hunk // 修改块
}

// Patch 表示一个完整的补丁
type Patch struct {
	Files []FilePatch
}

// hunk 头部正则：@@ -oldStart,oldLines +newStart,newLines @@
var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParsePatch 解析 unified diff 格式的补丁
func ParsePatch(patchText string) (*Patch, error) {
	if patchText == "" {
		return nil, errs.New(errs.CodeStoreParseFailed, "补丁内容为空")
	}

	lines := strings.Split(patchText, "\n")
	patch := &Patch{}

	i := 0
	for i < len(lines) {
		line := lines[i]

		// 查找文件头部 "--- " 和 "+++ "
		if strings.HasPrefix(line, "--- ") {
			if i+1 >= len(lines) {
				return nil, errs.New(errs.CodeStoreParseFailed, fmt.Sprintf("补丁格式错误：缺少 +++ 行（第 %d 行）", i+1))
			}

			nextLine := lines[i+1]
			if !strings.HasPrefix(nextLine, "+++ ") {
				return nil, errs.New(errs.CodeStoreParseFailed, fmt.Sprintf("补丁格式错误：期望 +++ 行但得到 %q（第 %d 行）", nextLine, i+2))
			}

			// 解析文件路径
			oldPath := strings.TrimPrefix(line, "--- ")
			newPath := strings.TrimPrefix(nextLine, "+++ ")

			// 移除 a/ 和 b/ 前缀（如果存在）
			oldPath = strings.TrimPrefix(oldPath, "a/")
			newPath = strings.TrimPrefix(newPath, "b/")

			// 创建 FilePatch
			fp := FilePatch{
				OldPath: oldPath,
				NewPath: newPath,
				Hunks:   []Hunk{},
			}

			// 解析该文件的所有 hunks
			i += 2
			for i < len(lines) {
				line := lines[i]

				// 遇到新文件头部或结束
				if strings.HasPrefix(line, "--- ") || i == len(lines)-1 {
					break
				}

				// 解析 hunk
				if strings.HasPrefix(line, "@@ ") {
					hunk, consumed, err := parseHunk(lines[i:])
					if err != nil {
						return nil, errs.Wrap(errs.CodeStoreParseFailed, fmt.Sprintf("解析 hunk 失败（第 %d 行）", i+1), err)
					}
					fp.Hunks = append(fp.Hunks, *hunk)
					i += consumed
				} else {
					i++
				}
			}

			patch.Files = append(patch.Files, fp)
		} else {
			i++
		}
	}

	if len(patch.Files) == 0 {
		return nil, errs.New(errs.CodeStoreParseFailed, "未找到有效的补丁文件")
	}

	return patch, nil
}

// parseHunk 解析单个 hunk（从 @@ 行开始）
func parseHunk(lines []string) (*Hunk, int, error) {
	if len(lines) == 0 {
		return nil, 0, errs.New(errs.CodeStoreParseFailed, "hunk 为空")
	}

	// 解析 hunk 头部
	header := lines[0]
	matches := hunkHeaderRE.FindStringSubmatch(header)
	if matches == nil {
		return nil, 0, errs.New(errs.CodeStoreParseFailed, fmt.Sprintf("无效的 hunk 头部: %q", header))
	}

	// 解析行号和行数；任何数字解析失败均视为格式错误并返回
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, 0, errs.New(errs.CodeStoreParseFailed,
			fmt.Sprintf("无效的 hunk 旧起始行号 %q: %v", matches[1], err))
	}
	oldLines := 1
	if matches[2] != "" {
		oldLines, err = strconv.Atoi(matches[2])
		if err != nil {
			return nil, 0, errs.New(errs.CodeStoreParseFailed,
				fmt.Sprintf("无效的 hunk 旧行数 %q: %v", matches[2], err))
		}
	}

	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, 0, errs.New(errs.CodeStoreParseFailed,
			fmt.Sprintf("无效的 hunk 新起始行号 %q: %v", matches[3], err))
	}
	newLines := 1
	if matches[4] != "" {
		newLines, err = strconv.Atoi(matches[4])
		if err != nil {
			return nil, 0, errs.New(errs.CodeStoreParseFailed,
				fmt.Sprintf("无效的 hunk 新行数 %q: %v", matches[4], err))
		}
	}

	hunk := &Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Lines:    []HunkLine{},
	}

	// 解析 hunk 内容行
	i := 1 // 跳过头部
	for i < len(lines) {
		line := lines[i]

		// 遇到新 hunk 或文件头部
		if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "--- ") {
			break
		}

		// 空行代表 diff 结束
		if line == "" {
			i++
			break
		}

		// 检查是否为有效的 diff 行
		if len(line) == 0 || (line[0] != ' ' && line[0] != '+' && line[0] != '-') {
			// 不是有效的 diff 行，结束当前 hunk
			break
		}

		lineType := LineType(line[0])
		content := ""
		if len(line) > 1 {
			content = line[1:]
		}

		hunk.Lines = append(hunk.Lines, HunkLine{
			Type:    lineType,
			Content: content,
		})

		i++
	}

	return hunk, i, nil
}

// Validate 验证 hunk 的行数是否匹配
func (h *Hunk) Validate() error {
	oldCount := 0
	newCount := 0

	for _, line := range h.Lines {
		switch line.Type {
		case LineContext:
			oldCount++
			newCount++
		case LineRemoved:
			oldCount++
		case LineAdded:
			newCount++
		}
	}

	if oldCount != h.OldLines {
		return errs.New(errs.CodeStoreParseFailed, fmt.Sprintf("hunk 原文件行数不匹配：期望 %d，实际 %d", h.OldLines, oldCount))
	}

	if newCount != h.NewLines {
		return errs.New(errs.CodeStoreParseFailed, fmt.Sprintf("hunk 新文件行数不匹配：期望 %d，实际 %d", h.NewLines, newCount))
	}

	return nil
}
