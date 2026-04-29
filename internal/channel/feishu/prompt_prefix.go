package feishu

import (
	"fmt"
	"strings"

	"github.com/chef-guo/agents-hive/internal/imctx"
)

// buildSystemPromptPrefix 根据 IMMessageContext 构建 XML 格式的 system prompt 前缀。
// 按 M1 spec 第 9 节格式生成，包含父消息、文档引用、mentions 信息。
func buildSystemPromptPrefix(ctx *imctx.IMMessageContext) string {
	if ctx == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<im_context>\n")

	// 父消息
	if ctx.ParentContent != "" {
		sb.WriteString("  <parent_message>")
		sb.WriteString(cdata(ctx.ParentContent))
		sb.WriteString("</parent_message>\n")
	}

	// 文档引用
	if len(ctx.References) > 0 {
		sb.WriteString("  <references>\n")
		for _, ref := range ctx.References {
			sb.WriteString(fmt.Sprintf("    <ref type=\"%s\" token=\"%s\"", ref.Type, ref.Token))
			if ref.URL != "" {
				sb.WriteString(fmt.Sprintf(" url=\"%s\"", ref.URL))
			}
			if ref.Title != "" {
				sb.WriteString(">")
				sb.WriteString(cdata(ref.Title))
				sb.WriteString("</ref>\n")
			} else {
				sb.WriteString("/>\n")
			}
		}
		sb.WriteString("  </references>\n")
	}

	// mentions
	if len(ctx.Mentions) > 0 {
		sb.WriteString("  <mentions>\n")
		for _, m := range ctx.Mentions {
			sb.WriteString(fmt.Sprintf("    <m name=\"%s\" is_bot=\"%t\"/>\n", m.Name, m.IsBot))
		}
		sb.WriteString("  </mentions>\n")
	}

	sb.WriteString("</im_context>\n")
	if len(ctx.References) > 0 {
		sb.WriteString("如果用户要求你分析/总结/解释上面引用的文档，你必须先调用 feishu_api 读取正文，再进行回答；不要在未读取正文前凭猜测作答。\n")
	}
	sb.WriteString("你可以用 feishu_api 工具按 ref 的 type 和 token 拉取正文:\n")
	sb.WriteString("- docx/doc/mindnote/file → action=get_doc_content(document_id=token)\n")
	sb.WriteString("- sheet → action=read_sheet(spreadsheet_token=token, range=\"A1:Z1000\")，工具会自动定位默认工作表\n")
	sb.WriteString("- bitable → action=list_bitable_tables(app_token=token) 先列表再 list_bitable_records\n")
	sb.WriteString("- wiki → action=wiki_get_node(node_token=token) 先解析真实 obj_type/obj_token，再按真实类型读取；不要要求用户补 space_id\n")

	return sb.String()
}
