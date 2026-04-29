package search

import (
	"context"
	"os"
	"path/filepath"
)

// BasicGlob 使用 filepath.Walk + filepath.Match 实现基础 glob 匹配。
// 仅支持 basename 匹配，向后兼容现有行为。
type BasicGlob struct{}

func NewBasicGlob() *BasicGlob { return &BasicGlob{} }

func (b *BasicGlob) Glob(_ context.Context, pattern string, root string) ([]string, error) {
	if root == "" {
		root = "."
	}

	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过权限错误
		}
		matched, matchErr := filepath.Match(filepath.Base(pattern), filepath.Base(path))
		if matchErr != nil {
			return nil
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}
