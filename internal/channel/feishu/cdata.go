package feishu

import "strings"

// cdata 将字符串包装为 XML CDATA 块，防止注入。
// 飞书文档标题/父消息正文可能包含 <、>、& 等特殊字符，必须转义。
func cdata(s string) string {
	if s == "" {
		return ""
	}
	// CDATA 转义规则：将 ]]> 替换为 ]]]]><![CDATA[>
	return "<![CDATA[" + strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>") + "]]>"
}
