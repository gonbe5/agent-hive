package feishu

import "testing"

func TestCdata(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "空字符串",
			input: "",
			want:  "",
		},
		{
			name:  "普通字符串",
			input: "Hello World",
			want:  "<![CDATA[Hello World]]>",
		},
		{
			name:  "包含 ]]> 的字符串",
			input: "text with ]]> inside",
			want:  "<![CDATA[text with ]]]]><![CDATA[> inside]]>",
		},
		{
			name:  "包含 <>&",
			input: "<tag>content & more</tag>",
			want:  "<![CDATA[<tag>content & more</tag>]]>",
		},
		{
			name:  "多个 ]]>",
			input: "a]]>b]]>c",
			want:  "<![CDATA[a]]]]><![CDATA[>b]]]]><![CDATA[>c]]>",
		},
		{
			name:  "只有 ]]>",
			input: "]]>",
			want:  "<![CDATA[]]]]><![CDATA[>]]>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cdata(tt.input)
			if got != tt.want {
				t.Errorf("cdata(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
