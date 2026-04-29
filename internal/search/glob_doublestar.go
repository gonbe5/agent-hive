package search

import (
	"context"
	"os"

	"github.com/bmatcuk/doublestar/v4"
)

// DoublestarGlob 使用 doublestar 库实现完整 ** 递归匹配。
type DoublestarGlob struct{}

func NewDoublestarGlob() *DoublestarGlob { return &DoublestarGlob{} }

func (d *DoublestarGlob) Glob(_ context.Context, pattern string, root string) ([]string, error) {
	if root == "" {
		root = "."
	}

	fsys := os.DirFS(root)
	matches, err := doublestar.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}

	// 将相对路径转换为基于 root 的路径，与 BasicGlob 行为一致
	for i, m := range matches {
		if root != "." {
			matches[i] = root + "/" + m
		}
	}
	return matches, nil
}
