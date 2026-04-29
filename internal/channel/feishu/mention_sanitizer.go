package feishu

import (
	"html"
	"regexp"
	"strings"
)

// SanitizeOutboundMentions 是 Phase 0 P0-#11 的最后一道闸门。
//
// 红队场景：
//  1. 用户向 bot 发包含 "@all" / "@_all" / 全角 ＠all / 零宽字符 / HTML 实体的内容，
//     如果 bot 把内容 echo 回群（甚至只是 quote），就触发"@所有人"全员 ping。
//  2. AI 模型生成跨平台 mention 语法（Slack `<!everyone>`、`<!channel>`、`<!here>`、
//     Discord `<@&role>`），即便飞书 UI 不解析，运维 / 报表 / 桥接的其它 IM 会解析。
//  3. AI 生成飞书原生 `<at user_id="all"></at>`，飞书会真的把它渲染为 @所有人。
//  4. 引用块 / fenced code block 内嵌入上述任意一种——markdown 渲染器在飞书卡片
//     里依然会执行 lark_md 解析，因此代码块不算 safe harbor。
//
// 白名单（透传）：仅 `<at user_id="ou_xxx"></at>` 这类指向具体 open_id 的精确 mention
//
//	放行；其余一律降级为不可触发 ping 的纯文本。
//
// 任何无法识别但形似 `<at ...>` 的 tag 走拒绝侧（fail-closed）：直接剥成纯文本，
// 因为我们宁可少 ping，也不能误 ping 全员。
func SanitizeOutboundMentions(content string) string {
	if content == "" {
		return content
	}

	// Step 1: 占位标准 `<at user_id="ou_xxx">` 先抽走，留 sentinel 等末尾还原。
	// 这一步必须在 normalize 之前——避免 normalize 误伤合法 user_id。
	content, restore := stashWhitelistedAtTags(content)

	// Step 2: 处理飞书原生 `<at user_id="all">` / `<at user_id="@all">` / `<at user_id="">` 等。
	// 这些都不是 ou_ 前缀的 open_id，全部降级为字面文本 `[mention removed]`。
	// 注意：替换文本绝不能包含 `@` + 任何黑名单词，否则产物会被自身再次扫到。
	content = atTagAnyPattern.ReplaceAllString(content, redactedMention)

	// Step 3: Slack/Discord 跨平台 mention 文本化。
	// `<!everyone>` / `<!channel>` / `<!here>` / `<@&role-id>` / `<@U12345>` etc.
	// 同上：替换串只用安全占位，不带 `@`。
	content = slackBroadcastPattern.ReplaceAllString(content, redactedBroadcast)
	content = discordRolePattern.ReplaceAllString(content, redactedRole)

	// Step 4: 关键归一化扫描。把 content 复制一份做 normalize：
	//   - 全角 ＠ → ASCII @
	//   - HTML 实体 &#64; / &#x40; / &commat; → @
	//   - 删除零宽 / BOM / 软连字符（避免 @\u200ball 绕过）
	// 然后在 normalized 文本上找黑名单关键词的位置。
	// 因为我们只对触发词做"整 token 替换"，先 normalize 整段，再用同样规则在原文上做替换更省事。
	normalized := normalizeForScan(content)

	// 黑名单关键词（不区分大小写）。匹配前后必须是非字母数字下划线的 boundary，避免误伤
	// "@allen" / "@everyone-else"。
	if !blacklistRegex.MatchString(normalized) {
		// 没击中黑名单就直接还原白名单 sentinel 返回。
		return restore(content)
	}

	// Step 5: 击中了——为了不让任何变体留活路，整段在归一化形态下做替换，并再做 HTML 实体 decode。
	// 这里的代价是：合法的 "@allen" 可能误伤（"@all" 是其严格前缀，但 boundary 限制能挡住）。
	// 设计选择：宁可 over-sanitize 不可漏。
	content = decodeAtEntities(content)
	content = stripZeroWidth(content)
	content = fullwidthAtToAscii(content)

	// 此时 content 已是"归一化"形态，再用 boundary regex 替换。
	// SubmatchIndex 才能精确替换"危险 token"本身、保留前后 boundary 字符。
	content = replaceBlacklistTokens(content)

	return restore(content)
}

// replaceBlacklistTokens 在已归一化的文本上做"按 token 替换"：
//   - 只替换捕获组 1 的 token（@all / @_all / @here / ...）
//   - 保留前后 boundary 字符（空格、标点、行首/行尾）
//   - 替换为 `[mention removed]`，串内不含 @ 和黑名单词，避免产物自撞
//
// 不停迭代直到没有命中，杜绝 "@@all" 这种"剥一层还有一层"的攻击。
func replaceBlacklistTokens(content string) string {
	for i := 0; i < 8; i++ { // 8 次迭代上限——正常 1~2 次足够，限上限避免病态正则把 CPU 拖死
		idx := blacklistRegex.FindStringSubmatchIndex(content)
		if idx == nil {
			return content
		}
		// idx[2:4] = group 1 范围（即 token 本身）
		tokStart, tokEnd := idx[2], idx[3]
		content = content[:tokStart] + redactedMention + content[tokEnd:]
	}
	// 极端兜底：上限触发时整段标记为不可信
	return redactedMention
}

// --- 实现细节 ---

// 安全替换串：必须不含 `@` 也不含任何黑名单关键词，避免产物自撞 blacklistRegex 触发死循环。
const (
	redactedMention   = "[mention redacted]"
	redactedBroadcast = "[broadcast redacted]"
	redactedRole      = "[role redacted]"
)

var (
	// 合法白名单：`<at user_id="ou_xxxxx"></at>` / `<at user_id="ou_xxx"/>`
	// open_id 飞书规范以 ou_ 开头；为容错也接受 on_ 前缀（open chat id；理论上不该出现在 user_id，
	// 但 SDK 偶有混用 — 这里收紧为只接 ou_）。
	whitelistAtPattern = regexp.MustCompile(`(?i)<at\s+user_id\s*=\s*"(ou_[A-Za-z0-9_\-]+)"\s*(?:>\s*</at>|/>)`)

	// 任意未匹配白名单的 `<at ...>` 标签；这一步在 stash 之后才跑，所以不会误吃白名单。
	atTagAnyPattern = regexp.MustCompile(`(?is)<at\b[^>]*>(?:\s*</at>)?`)

	// Slack 全员/频道/在线广播：<!everyone> <!channel> <!here>
	slackBroadcastPattern = regexp.MustCompile(`(?i)<!\s*(everyone|channel|here)\s*(?:\|[^>]*)?>`)

	// Discord 角色 mention：<@&123456789> （`<@!user>`、`<@user>` 也算）
	discordRolePattern = regexp.MustCompile(`<@[!&]?[0-9]{5,}>`)

	// 黑名单关键词正则。
	// 前导 boundary：必须不是 word char（字母/数字/下划线）。
	// 关键词：@_all / @all / @here / @channel / @everyone / @team / @group / @online
	// 后导 boundary：同上。
	// 注意：@_user_N 是飞书 mention 占位符（resolveMentions 已替换），但理论上 user_N 是数字 id，
	//       走 boundary 后 _user_1 不会匹配 _all。
	blacklistRegex = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(@_?(?:all|here|channel|everyone|team|group|online))(?:[^A-Za-z0-9_]|$)`)

	// 零宽 / 不可见字符黑名单。
	// U+200B Zero-Width Space, U+200C ZWNJ, U+200D ZWJ, U+2060 Word Joiner,
	// U+FEFF BOM, U+00AD Soft Hyphen, U+180E Mongolian Vowel Separator
	zeroWidthPattern = regexp.MustCompile("[\u200B\u200C\u200D\u2060\uFEFF\u00AD\u180E]")
)

// stashWhitelistedAtTags 把合法 `<at user_id="ou_xxx">` 替换为 sentinel，返回 restore fn。
// sentinel 选用不会在用户输入中自然出现的形态：`\u0000HIVE_AT_<idx>\u0000`。
// （\u0000 在文本消息里被 Go regexp 视作普通字符，飞书侧会被序列化层过滤——但我们最终 restore 回来，
// 落到 wire 时已是合法 `<at>` 标签。）
func stashWhitelistedAtTags(content string) (string, func(string) string) {
	stash := make([]string, 0, 4)
	rewritten := whitelistAtPattern.ReplaceAllStringFunc(content, func(match string) string {
		idx := len(stash)
		stash = append(stash, match)
		return atSentinel(idx)
	})
	restore := func(s string) string {
		for i, original := range stash {
			s = strings.ReplaceAll(s, atSentinel(i), original)
		}
		return s
	}
	return rewritten, restore
}

func atSentinel(i int) string {
	// `\x00` 包夹避免普通字符串撞车；i 用 ASCII。
	return "\x00HIVE_AT_" + itoaFast(i) + "\x00"
}

func itoaFast(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// normalizeForScan 复制一份做扫描。原文后续若命中黑名单会单独处理。
func normalizeForScan(content string) string {
	s := decodeAtEntities(content)
	s = stripZeroWidth(s)
	s = fullwidthAtToAscii(s)
	return s
}

// decodeAtEntities 只 decode @ 相关 HTML 实体（&#64; / &#x40; / &commat;），
// 故意不调 html.UnescapeString 全量 decode——避免破坏用户可能想保留的其它实体。
//
// 但为完整对抗，我们用 html.UnescapeString 兜底：实体 sanitizer 误伤一些 `&amp;`
// 退化为 `&`，比放过 `&#64;all` 安全得多。Phase 0 选 fail-closed。
func decodeAtEntities(content string) string {
	if !strings.Contains(content, "&") {
		return content
	}
	// 全量 unescape：决定性选择，详见上方注释。
	return html.UnescapeString(content)
}

// stripZeroWidth 删除所有零宽 / BOM 字符。
func stripZeroWidth(content string) string {
	return zeroWidthPattern.ReplaceAllString(content, "")
}

// fullwidthAtToAscii 把 U+FF20 全角 ＠ 转成 ASCII @。
func fullwidthAtToAscii(content string) string {
	if !strings.ContainsRune(content, '＠') {
		return content
	}
	return strings.ReplaceAll(content, "＠", "@")
}

// TODO(P0-#12 longconn.go owner): 入站侧若想观测黑名单击中率，可在
// internal/channel/feishu/longconn.go 第 ~175 行 ExtractMessageContent 之后
// 调用 SanitizeOutboundMentions(content) 的对偶检测函数（待补 ScanMentions），
// 把命中数打成 metric。本任务不直接动 longconn.go，避免与并行 owner 冲突。
