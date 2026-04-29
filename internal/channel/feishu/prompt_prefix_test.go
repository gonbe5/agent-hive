package feishu

import (
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

func TestBuildSystemPromptPrefix(t *testing.T) {
	tests := []struct {
		name string
		ctx  *imctx.IMMessageContext
		want []string // 必须包含的子串
	}{
		{
			name: "空 context",
			ctx:  nil,
			want: []string{""},
		},
		{
			name: "只有父消息",
			ctx: &imctx.IMMessageContext{
				ParentContent: "这是父消息内容",
			},
			want: []string{
				"<im_context>",
				"<parent_message><![CDATA[这是父消息内容]]></parent_message>",
				"</im_context>",
			},
		},
		{
			name: "父消息包含特殊字符",
			ctx: &imctx.IMMessageContext{
				ParentContent: "<script>alert('xss')</script>",
			},
			want: []string{
				"<parent_message><![CDATA[<script>alert('xss')</script>]]></parent_message>",
			},
		},
		{
			name: "只有 refs",
			ctx: &imctx.IMMessageContext{
				References: []imctx.DocRef{
					{
						Type:  imctx.RefDocx,
						Token: "doccnABC123",
						URL:   "https://example.feishu.cn/docx/doccnABC123",
						Title: "测试文档",
					},
					{
						Type:  imctx.RefSheet,
						Token: "shtcnXYZ789",
					},
				},
			},
			want: []string{
				"<references>",
				`<ref type="docx" token="doccnABC123" url="https://example.feishu.cn/docx/doccnABC123"><![CDATA[测试文档]]></ref>`,
				`<ref type="sheet" token="shtcnXYZ789"/>`,
				"</references>",
				"如果用户要求你分析/总结/解释上面引用的文档，你必须先调用 feishu_api 读取正文，再进行回答；不要在未读取正文前凭猜测作答。",
			},
		},
		{
			name: "只有 mentions",
			ctx: &imctx.IMMessageContext{
				Mentions: []imctx.Mention{
					{Name: "张三", IsBot: false},
					{Name: "Bot助手", IsBot: true},
				},
			},
			want: []string{
				"<mentions>",
				`<m name="张三" is_bot="false"/>`,
				`<m name="Bot助手" is_bot="true"/>`,
				"</mentions>",
			},
		},
		{
			name: "全部字段",
			ctx: &imctx.IMMessageContext{
				ParentContent: "父消息 with <tags>",
				References: []imctx.DocRef{
					{
						Type:  imctx.RefBitable,
						Token: "bascnDEF456",
						Title: "多维表格]]>注入测试",
					},
				},
				Mentions: []imctx.Mention{
					{Name: "李四", IsBot: false},
				},
			},
			want: []string{
				"<im_context>",
				"<parent_message><![CDATA[父消息 with <tags>]]></parent_message>",
				"<references>",
				`<ref type="bitable" token="bascnDEF456"><![CDATA[多维表格]]]]><![CDATA[>注入测试]]></ref>`,
				"</references>",
				"<mentions>",
				`<m name="李四" is_bot="false"/>`,
				"</mentions>",
				"</im_context>",
				"你可以用 feishu_api 工具按 ref 的 type 和 token 拉取正文:",
			},
		},
		{
			name: "空 IMMessageContext 对象",
			ctx:  &imctx.IMMessageContext{},
			want: []string{
				"<im_context>",
				"</im_context>",
				"你可以用 feishu_api 工具按 ref 的 type 和 token 拉取正文:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSystemPromptPrefix(tt.ctx)

			// 空 context 特殊处理
			if tt.ctx == nil {
				if got != "" {
					t.Errorf("buildSystemPromptPrefix(nil) = %q, want empty string", got)
				}
				return
			}

			for _, substr := range tt.want {
				if !strings.Contains(got, substr) {
					t.Errorf("buildSystemPromptPrefix() 缺少子串:\n期望包含: %q\n实际输出:\n%s", substr, got)
				}
			}
		})
	}
}
