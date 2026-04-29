package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// LoadedCase 保留 fixture 的真实文件路径 + 解码后的 Case。
// harness 所有 required-set 判定、summary 输出、审计都基于 Path 而不是 Case.Name，
// 这样用户就算改了 JSON 里的 name 字段也骗不过 gate。Codex review 的 P0-3 红线修复。
type LoadedCase struct {
	Path string
	Case Case
}

// Base 返回文件名（不含目录），required-set 和日志都用它，不要再用 Case.Name。
func (lc LoadedCase) Base() string { return filepath.Base(lc.Path) }

// LoadCase 严格解码单个 fixture JSON：
// 1) DisallowUnknownFields — schema 漂移立即失败
// 2) 解码完成后再 Decode 一次验 EOF — 防御 trailing payload 绕过
// Codex review 指出的 "EOF 检查缺失" P1 修复。
func LoadCase(path string) (LoadedCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LoadedCase{}, fmt.Errorf("read fixture %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Case
	if err := dec.Decode(&c); err != nil {
		return LoadedCase{}, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	// EOF 检查：fixture 必须是单一顶层对象，不能是 {...}{...} 这样的 concatenated JSON
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return LoadedCase{}, fmt.Errorf("trailing data after JSON object in %s (got %v)", path, err)
	}
	return LoadedCase{Path: path, Case: c}, nil
}

// LoadAll 按文件名字典序加载目录下所有 *.json fixture。
// 顺序稳定 → 测试输出可复现,PR diff 友好。
func LoadAll(dir string) ([]LoadedCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read testdata dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	cases := make([]LoadedCase, 0, len(names))
	for _, n := range names {
		lc, err := LoadCase(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		cases = append(cases, lc)
	}
	return cases, nil
}
