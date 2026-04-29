package command

import (
	"testing"
)

func TestInfo_Render(t *testing.T) {
	tests := []struct {
		name     string
		template string
		args     []string
		want     string
	}{
		{
			name:     "$ARGUMENTS 替换",
			template: "请审查以下内容: $ARGUMENTS",
			args:     []string{"foo", "bar", "baz"},
			want:     "请审查以下内容: foo bar baz",
		},
		{
			name:     "位置参数 $1/$2",
			template: "第一个: $1，第二个: $2",
			args:     []string{"alpha", "beta"},
			want:     "第一个: alpha，第二个: beta",
		},
		{
			name:     "混合 $1 和 $ARGUMENTS",
			template: "第一个: $1，全部: $ARGUMENTS",
			args:     []string{"x", "y"},
			want:     "第一个: x，全部: x y",
		},
		{
			name:     "空参数",
			template: "无参数: $ARGUMENTS",
			args:     []string{},
			want:     "无参数: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Info{
				Name:     "test",
				Template: tt.template,
				Source:   SourceConfig,
			}
			got := cmd.Render(tt.args)
			if got != tt.want {
				t.Errorf("期望 %q，得到 %q", tt.want, got)
			}
		})
	}
}

func TestExtractHints(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     []string
	}{
		{
			name:     "仅 $ARGUMENTS",
			template: "部署到 $ARGUMENTS",
			want:     []string{"$ARGUMENTS"},
		},
		{
			name:     "位置参数 $1 $2",
			template: "比较 $1 和 $2",
			want:     []string{"$1", "$2"},
		},
		{
			name:     "混合占位符",
			template: "第一个: $1，全部: $ARGUMENTS",
			want:     []string{"$1", "$ARGUMENTS"},
		},
		{
			name:     "无占位符",
			template: "固定命令",
			want:     nil,
		},
		{
			name:     "重复占位符去重",
			template: "$1 和 $1 再 $2",
			want:     []string{"$1", "$2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHints(tt.template)
			if len(got) != len(tt.want) {
				t.Fatalf("期望 %d 个提示，得到 %d: %v", len(tt.want), len(got), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("提示[%d]: 期望 %q，得到 %q", i, tt.want[i], got[i])
				}
			}
		})
	}
}
