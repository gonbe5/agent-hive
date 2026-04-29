package feishu

import (
	"strings"
	"testing"
)

// 红队断言协议：
//   - mustSanitize：sanitizer 必须把"危险触发词"变成不会触发 ping 的形态。
//     判定方式：归一化后再扫，不能再撞黑名单 regex。
//   - mustPassthrough：合法精确 mention 必须 1:1 透传。
//
// 蓝军 mutation：把 SanitizeOutboundMentions 改 `return content`，
// 所有 mustSanitize 子测必须 fail——证据见 README/最终汇报。

func TestSanitizeOutboundMentions_DangerousVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"plain @all", "Hello @all, please review."},
		{"plain @here", "FYI @here"},
		{"plain @everyone", "@everyone come"},
		{"plain @channel", "ping @channel now"},
		{"feishu @_all SDK key", "Echoing user message: @_all 紧急"},
		{"uppercase @ALL", "@ALL stop"},
		{"mixed case @AlL", "@AlL hi"},
		{"fullwidth ＠all", "嘿 ＠all 注意"},
		{"fullwidth ＠here", "＠here 看一下"},
		{"zero-width inside @\\u200ball", "请看 @\u200ball 这条"},
		{"ZWJ inside @a\\u200dll", "提醒 @a\u200dll 小心"},
		{"BOM inside @\\uFEFFall", "test @\uFEFFall test"},
		{"soft hyphen inside @\\u00ADall", "soft @\u00ADall hyphen"},
		{"html numeric entity &#64;all", "AI 写了 &#64;all 全员"},
		{"html hex entity &#x40;all", "AI 写了 &#x40;all 全员"},
		{"html named entity &commat;all", "AI 写了 &commat;all"},
		{"slack <!everyone>", "桥接消息 <!everyone> 一下"},
		{"slack <!channel>", "<!channel> heads up"},
		{"slack <!here>", "<!here> 在线的"},
		{"slack <!everyone|label>", "<!everyone|label> hi"},
		{"feishu <at user_id=\"all\">", `请 <at user_id="all"></at> 注意`},
		{"feishu <at user_id=\"@all\">", `<at user_id="@all"></at>`},
		{"feishu <at user_id=\"\">empty", `<at user_id=""></at>`},
		{"discord role mention", "see <@&123456789>"},
		{"inside markdown code fence",
			"```\nbash\n# 注释\necho '@all 全员'\n```"},
		{"inside markdown blockquote", "> 引用：@all 看一下\n>\n> 第二行"},
		{"nested blockquote with entity", "> > &#64;everyone wake up"},
		{"mixed danger fullwidth + zero-width",
			"组合攻击 ＠\u200beveryone 应被拦截"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeOutboundMentions(tc.in)
			assertSafe(t, tc.in, out)
		})
	}
}

func TestSanitizeOutboundMentions_LegalMentionPassthrough(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"single legal at-tag", `你好 <at user_id="ou_abc123def456"></at> 请处理`},
		{"self-closing legal at-tag", `<at user_id="ou_xyz789"/> heads up`},
		{"two legal mentions", `<at user_id="ou_aaa"></at> 和 <at user_id="ou_bbb"></at> 一起`},
		{"legal mention near (but not inside) @allen text",
			`<at user_id="ou_allen001"></at> @allen 你好`}, // "@allen" boundary 测,allen 不应被替换
		{"legal mention with attribute spacing",
			`<at  user_id = "ou_spaced"  ></at>`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeOutboundMentions(tc.in)
			// 必须 1:1 包含合法 at 标签的核心 user_id
			if !strings.Contains(out, "ou_") {
				t.Fatalf("合法 mention 被剥离\n input=%q\noutput=%q", tc.in, out)
			}
			// 输出不允许出现 sanitize sentinel 残留
			if strings.Contains(out, "\x00HIVE_AT_") {
				t.Fatalf("sentinel 未还原\noutput=%q", out)
			}
			// 输出不允许撞黑名单 regex（合法 mention 永远不能触发拦截）
			normalized := normalizeForScan(out)
			if blacklistRegex.MatchString(normalized) {
				t.Fatalf("合法 mention 被误判为黑名单\noutput=%q", out)
			}
		})
	}
}

func TestSanitizeOutboundMentions_BoundaryNonTriggers(t *testing.T) {
	// 这些不应该被替换：词边界保护合法用户名/路径。
	cases := []struct {
		name string
		in   string
		want string // 期望与输入 1:1 一致
	}{
		{"username @allen", "Hi @allen, see PR.", "Hi @allen, see PR."},
		{"username @everyones", "@everyones thoughts?", "@everyones thoughts?"},
		{"path /everyone/", "Visit /everyone/list", "Visit /everyone/list"},
		{"email @host", "send to user@host.com", "send to user@host.com"},
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := SanitizeOutboundMentions(tc.in)
			if out != tc.want {
				t.Fatalf("误伤合法文本\n input=%q\noutput=%q\n  want=%q", tc.in, out, tc.want)
			}
		})
	}
}

func TestSanitizeOutboundMentions_RedTeamCombo(t *testing.T) {
	// 嵌套 + 多形态混合：bot echo 用户原话 + AI 自行加 @。
	in := strings.Join([]string{
		"用户问：",
		"> @all 这是引用块",
		"```",
		"// AI 生成的代码示例",
		"send_to(\"&#64;everyone\")",
		"```",
		`并触发 <at user_id="all"></at> 全员`,
		"跨平台 <!everyone> 也别漏",
		`合法的 <at user_id="ou_legitkeep"></at> 必须留下`,
	}, "\n")
	out := SanitizeOutboundMentions(in)
	assertSafe(t, in, out)
	if !strings.Contains(out, "ou_legitkeep") {
		t.Fatalf("合法 mention 在组合场景下被剥离\noutput=%q", out)
	}
}

// assertSafe 把 sanitizer 的输出再 normalize 一次,确认没有任何危险残留。
// 这是红队侧的"二次复核"——避免 sanitizer 自欺欺人（比如只在原文替换、normalize 后还能命中）。
func assertSafe(t *testing.T, in, out string) {
	t.Helper()
	normalized := normalizeForScan(out)

	// 1. 不允许残留任何黑名单关键词（@all / @here / @channel / @everyone / @_all 等）
	if blacklistRegex.MatchString(normalized) {
		t.Fatalf("sanitize 后仍命中黑名单\n input=%q\noutput=%q\nnormalized=%q",
			in, out, normalized)
	}
	// 2. 不允许残留 Slack/Discord broadcast tag
	if slackBroadcastPattern.MatchString(out) {
		t.Fatalf("sanitize 后仍含 Slack <!everyone|channel|here>\n input=%q\noutput=%q", in, out)
	}
	// 3. 不允许残留任何非白名单 <at ...>
	if atTagAnyPattern.MatchString(out) {
		// 但白名单 <at user_id="ou_xxx"> 是允许的——单独豁免
		residue := atTagAnyPattern.ReplaceAllStringFunc(out, func(m string) string {
			if whitelistAtPattern.MatchString(m) {
				return ""
			}
			return m
		})
		if strings.Contains(residue, "<at") {
			t.Fatalf("sanitize 后仍含未白名单 <at> 标签\n input=%q\noutput=%q\nresidue=%q",
				in, out, residue)
		}
	}
}

func TestSanitizeOutboundMentions_NormalizeHelpers(t *testing.T) {
	// 单独验证归一化辅助函数本身正确——避免组合测试覆盖盲区。
	t.Run("zero width strip", func(t *testing.T) {
		in := "a\u200bb\u200cc\u200dd\u2060e\ufefff\u00adg\u180eh"
		got := stripZeroWidth(in)
		if got != "abcdefgh" {
			t.Fatalf("stripZeroWidth got=%q", got)
		}
	})
	t.Run("fullwidth at", func(t *testing.T) {
		got := fullwidthAtToAscii("hello ＠world")
		if got != "hello @world" {
			t.Fatalf("fullwidthAtToAscii got=%q", got)
		}
	})
	t.Run("html @ entities", func(t *testing.T) {
		got := decodeAtEntities("&#64;a &#x40;b &commat;c")
		if got != "@a @b @c" {
			t.Fatalf("decodeAtEntities got=%q", got)
		}
	})
}
