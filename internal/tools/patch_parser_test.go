package tools

import (
	"testing"
)

func TestParsePatch_Simple(t *testing.T) {
	// 简化的测试用例（不包含空行）
	patchText := `--- a/file.go
+++ b/file.go
@@ -1,4 +1,5 @@
 package main
+import "fmt"
 func main() {
-    println("hello")
+    fmt.Println("hello, world")
 }
`

	patch, err := ParsePatch(patchText)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("期望 1 个文件，得到 %d", len(patch.Files))
	}

	fp := patch.Files[0]
	if fp.OldPath != "file.go" {
		t.Errorf("期望 oldPath='file.go'，得到 %q", fp.OldPath)
	}
	if fp.NewPath != "file.go" {
		t.Errorf("期望 newPath='file.go'，得到 %q", fp.NewPath)
	}

	if len(fp.Hunks) != 1 {
		t.Fatalf("期望 1 个 hunk，得到 %d", len(fp.Hunks))
	}

	hunk := fp.Hunks[0]
	if hunk.OldStart != 1 || hunk.OldLines != 4 {
		t.Errorf("期望 oldStart=1, oldLines=4，得到 %d,%d", hunk.OldStart, hunk.OldLines)
	}
	if hunk.NewStart != 1 || hunk.NewLines != 5 {
		t.Errorf("期望 newStart=1, newLines=5，得到 %d,%d", hunk.NewStart, hunk.NewLines)
	}

	// 验证行数
	if err := hunk.Validate(); err != nil {
		t.Errorf("hunk 验证失败: %v", err)
	}
}

func TestParsePatch_MultipleFiles(t *testing.T) {
	patchText := `--- a/file1.txt
+++ b/file1.txt
@@ -1,2 +1,2 @@
 hello
-world
+universe
--- a/file2.txt
+++ b/file2.txt
@@ -1,1 +1,2 @@
 first line
+second line
`

	patch, err := ParsePatch(patchText)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(patch.Files) != 2 {
		t.Fatalf("期望 2 个文件，得到 %d", len(patch.Files))
	}

	// 验证第一个文件
	fp1 := patch.Files[0]
	if fp1.OldPath != "file1.txt" || fp1.NewPath != "file1.txt" {
		t.Errorf("文件1 路径错误: old=%q, new=%q", fp1.OldPath, fp1.NewPath)
	}
	if len(fp1.Hunks) != 1 {
		t.Fatalf("文件1 期望 1 个 hunk，得到 %d", len(fp1.Hunks))
	}

	// 验证第二个文件
	fp2 := patch.Files[1]
	if fp2.OldPath != "file2.txt" || fp2.NewPath != "file2.txt" {
		t.Errorf("文件2 路径错误: old=%q, new=%q", fp2.OldPath, fp2.NewPath)
	}
	if len(fp2.Hunks) != 1 {
		t.Fatalf("文件2 期望 1 个 hunk，得到 %d", len(fp2.Hunks))
	}
}

func TestParsePatch_MultipleHunks(t *testing.T) {
	patchText := `--- a/multi.txt
+++ b/multi.txt
@@ -1,2 +1,3 @@
 line1
+inserted
 line2
@@ -10,1 +11,2 @@
 line10
+another insert
`

	patch, err := ParsePatch(patchText)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(patch.Files) != 1 {
		t.Fatalf("期望 1 个文件，得到 %d", len(patch.Files))
	}

	fp := patch.Files[0]
	if len(fp.Hunks) != 2 {
		t.Fatalf("期望 2 个 hunks，得到 %d", len(fp.Hunks))
	}

	// 验证第一个 hunk
	h1 := fp.Hunks[0]
	if h1.OldStart != 1 || h1.OldLines != 2 {
		t.Errorf("hunk1: 期望 oldStart=1, oldLines=2，得到 %d,%d", h1.OldStart, h1.OldLines)
	}

	// 验证第二个 hunk
	h2 := fp.Hunks[1]
	if h2.OldStart != 10 || h2.OldLines != 1 {
		t.Errorf("hunk2: 期望 oldStart=10, oldLines=1，得到 %d,%d", h2.OldStart, h2.OldLines)
	}
}

func TestParsePatch_EmptyPatch(t *testing.T) {
	_, err := ParsePatch("")
	if err == nil {
		t.Error("期望空补丁返回错误")
	}
}

func TestParsePatch_InvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		patch string
	}{
		{
			name:  "缺少 +++ 行",
			patch: "--- a/file.txt\nsome content",
		},
		{
			name:  "无效的 hunk 头部",
			patch: "--- a/file.txt\n+++ b/file.txt\n@@ invalid @@\n",
		},
		{
			name:  "没有文件头部",
			patch: "some random text\n+added line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePatch(tt.patch)
			if err == nil {
				t.Errorf("期望返回错误，但成功解析")
			}
		})
	}
}

func TestHunkValidate(t *testing.T) {
	tests := []struct {
		name      string
		hunk      Hunk
		expectErr bool
	}{
		{
			name: "有效的 hunk",
			hunk: Hunk{
				OldStart: 1,
				OldLines: 3,
				NewStart: 1,
				NewLines: 3,
				Lines: []HunkLine{
					{Type: LineContext, Content: "line1"},
					{Type: LineRemoved, Content: "old"},
					{Type: LineAdded, Content: "new"},
					{Type: LineContext, Content: "line3"},
				},
			},
			expectErr: false,
		},
		{
			name: "oldLines 不匹配",
			hunk: Hunk{
				OldStart: 1,
				OldLines: 5, // 错误：实际只有 3 行
				NewStart: 1,
				NewLines: 3,
				Lines: []HunkLine{
					{Type: LineContext, Content: "line1"},
					{Type: LineRemoved, Content: "old"},
					{Type: LineContext, Content: "line3"},
				},
			},
			expectErr: true,
		},
		{
			name: "newLines 不匹配",
			hunk: Hunk{
				OldStart: 1,
				OldLines: 2,
				NewStart: 1,
				NewLines: 5, // 错误：实际只有 2 行
				Lines: []HunkLine{
					{Type: LineContext, Content: "line1"},
					{Type: LineContext, Content: "line2"},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hunk.Validate()
			if tt.expectErr && err == nil {
				t.Error("期望验证失败，但成功")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("期望验证成功，但失败: %v", err)
			}
		})
	}
}

func TestParseHunk(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		expectErr bool
		consumed  int
	}{
		{
			name: "简单 hunk",
			lines: []string{
				"@@ -1,3 +1,3 @@",
				" context",
				"-removed",
				"+added",
			},
			expectErr: false,
			consumed:  4,
		},
		{
			name: "无效头部",
			lines: []string{
				"invalid header",
			},
			expectErr: true,
		},
		{
			name:      "空 hunk",
			lines:     []string{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunk, consumed, err := parseHunk(tt.lines)
			if tt.expectErr {
				if err == nil {
					t.Error("期望解析失败，但成功")
				}
				return
			}

			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}

			if consumed != tt.consumed {
				t.Errorf("期望消耗 %d 行，实际 %d", tt.consumed, consumed)
			}

			if hunk == nil {
				t.Error("期望非 nil hunk")
			}
		})
	}
}
