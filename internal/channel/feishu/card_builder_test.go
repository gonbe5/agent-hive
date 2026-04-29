package feishu

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// parseCard 把 BuildCardJSON 输出反序列化成可断言的 map。
// 飞书卡片是纯 JSON 字符串；所有测试都通过这个 helper 做结构化断言，避免脆的字符串匹配。
func parseCard(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("BuildCardJSON 输出不是合法 JSON：%v\npayload=%s", err, raw)
	}
	return m
}

func headerTitle(t *testing.T, card map[string]any) string {
	t.Helper()
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("card.header 缺失或类型不对：%+v", card["header"])
	}
	title, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("card.header.title 缺失：%+v", header)
	}
	content, _ := title["content"].(string)
	return content
}

func headerTemplate(t *testing.T, card map[string]any) string {
	t.Helper()
	header, _ := card["header"].(map[string]any)
	tpl, _ := header["template"].(string)
	return tpl
}

func elementsOf(t *testing.T, card map[string]any) []any {
	t.Helper()
	els, ok := card["elements"].([]any)
	if !ok {
		t.Fatalf("card.elements 缺失或类型不对：%+v", card["elements"])
	}
	return els
}

// TestBuildCardJSON_StatusTitles 覆盖三种 status 的标题 + 颜色模板。
// 契约：generating/空 → 🤖 blue；done → ✅ green；error → ❌ red。
func TestBuildCardJSON_StatusTitles(t *testing.T) {
	tests := []struct {
		name      string
		status    CardStatus
		wantTitle string
		wantTpl   string
	}{
		{"generating", CardStatusGenerating, "🤖 生成中…", "blue"},
		{"empty-default-to-generating", "", "🤖 生成中…", "blue"},
		{"thinking", CardStatusThinking, "💭 思考中…", "blue"},
		{"done", CardStatusDone, "✅ 完成", "green"},
		{"error", CardStatusError, "❌ 失败", "red"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := BuildCardJSON(CardState{Status: tt.status, Body: "hello"})
			card := parseCard(t, raw)
			if got := headerTitle(t, card); got != tt.wantTitle {
				t.Errorf("title = %q, want %q", got, tt.wantTitle)
			}
			if got := headerTemplate(t, card); got != tt.wantTpl {
				t.Errorf("template = %q, want %q", got, tt.wantTpl)
			}
		})
	}
}

// TestBuildCardJSON_CustomTitleOverride 用户提供 Title 时优先生效。
func TestBuildCardJSON_CustomTitleOverride(t *testing.T) {
	raw := BuildCardJSON(CardState{
		Title:  "自定义标题",
		Status: CardStatusDone, // 应被覆盖
		Body:   "x",
	})
	if got := headerTitle(t, parseCard(t, raw)); got != "自定义标题" {
		t.Errorf("Title 覆盖失效：got %q", got)
	}
}

// TestBuildCardJSON_ToolLines 断言 start/success/error 三种状态的图标、duration、summary 都渲染到正文。
func TestBuildCardJSON_ToolLines(t *testing.T) {
	state := CardState{
		Status: CardStatusGenerating,
		Body:   "这是正文",
		ToolLines: []ToolLine{
			{ToolName: "bash", Status: "start", Summary: "ls -la"},
			{ToolName: "grep", Status: "success", Duration: 230 * time.Millisecond, Summary: "匹配 3 行"},
			{ToolName: "edit", Status: "error", Duration: 1 * time.Second, Summary: "文件不存在"},
		},
	}
	card := parseCard(t, BuildCardJSON(state))
	els := elementsOf(t, card)

	// elements 布局：body markdown → hr → tool markdown
	if len(els) != 3 {
		t.Fatalf("elements 数量 = %d，want 3 (body+hr+tool)", len(els))
	}
	toolMd, ok := els[2].(map[string]any)
	if !ok || toolMd["tag"] != "markdown" {
		t.Fatalf("elements[2] 应为 tool markdown：%+v", els[2])
	}
	content, _ := toolMd["content"].(string)

	// 图标 + 工具名
	for _, want := range []string{"🔧", "**bash**", "✅", "**grep**", "❌", "**edit**"} {
		if !strings.Contains(content, want) {
			t.Errorf("tool markdown 缺失 %q\nfull=%s", want, content)
		}
	}
	// duration 只在 success/error 出现
	if strings.Contains(content, "`0s`") {
		t.Error("start 态不应渲染 duration")
	}
	// summary
	for _, want := range []string{"ls -la", "匹配 3 行", "文件不存在"} {
		if !strings.Contains(content, want) {
			t.Errorf("tool markdown 缺失 summary %q", want)
		}
	}
}

// TestBuildCardJSON_HITLButtons 断言 approve/reject 按钮渲染：type 映射 + value.request_id 透传。
func TestBuildCardJSON_HITLButtons(t *testing.T) {
	state := CardState{
		Status: CardStatusGenerating,
		Body:   "需要你确认",
		HITLButtons: []HITLButton{
			{Label: "✅ 批准", Action: "approve", RequestID: "req-123"},
			{Label: "❌ 拒绝", Action: "reject", RequestID: "req-123"},
		},
	}
	card := parseCard(t, BuildCardJSON(state))
	els := elementsOf(t, card)

	// 最后一个 element 必是 action
	last, ok := els[len(els)-1].(map[string]any)
	if !ok || last["tag"] != "action" {
		t.Fatalf("最后一个 element 应为 action：%+v", els[len(els)-1])
	}
	actions, ok := last["actions"].([]any)
	if !ok || len(actions) != 2 {
		t.Fatalf("actions 数量 = %d，want 2", len(actions))
	}

	approve, _ := actions[0].(map[string]any)
	if approve["type"] != "primary" {
		t.Errorf("approve.type = %v, want primary", approve["type"])
	}
	val, _ := approve["value"].(map[string]any)
	if val["action"] != "approve" || val["request_id"] != "req-123" {
		t.Errorf("approve.value = %+v，want action=approve/request_id=req-123", val)
	}

	reject, _ := actions[1].(map[string]any)
	if reject["type"] != "danger" {
		t.Errorf("reject.type = %v, want danger", reject["type"])
	}
}

// TestBuildCardJSON_HITLOnly_NoOrphanHR 只有 HITL 按钮、没有 body/tool 的场景，
// 卡片顶部不能出现孤儿 hr——reviewer 发现的真实问题，不加测试会回归。
func TestBuildCardJSON_HITLOnly_NoOrphanHR(t *testing.T) {
	state := CardState{
		Status: CardStatusGenerating,
		HITLButtons: []HITLButton{
			{Label: "✅", Action: "approve", RequestID: "r1"},
		},
	}
	card := parseCard(t, BuildCardJSON(state))
	els := elementsOf(t, card)

	// 预期元素：[action]，不是 [hr, action]
	if len(els) != 1 {
		t.Fatalf("HITL-only 应只产生 1 个 element，got %d: %+v", len(els), els)
	}
	first, _ := els[0].(map[string]any)
	if first["tag"] != "action" {
		t.Errorf("HITL-only 的第 1 个 element 应为 action，got %v", first["tag"])
	}
}

// TestBuildCardJSON_EmptyElementsFallback 全空 state 必须产出 elements ≥1 的合法卡片（飞书 400 防御）。
func TestBuildCardJSON_EmptyElementsFallback(t *testing.T) {
	raw := BuildCardJSON(CardState{})
	card := parseCard(t, raw)
	els := elementsOf(t, card)
	if len(els) < 1 {
		t.Fatalf("空 state 也必须至少 1 个 element，got %d", len(els))
	}
}

// TestBuildCardJSON_ConfigFlags 断言 update_multi=true，允许后续 PatchMessage 多次整卡替换。
func TestBuildCardJSON_ConfigFlags(t *testing.T) {
	raw := BuildCardJSON(CardState{Status: CardStatusGenerating, Body: "x"})
	card := parseCard(t, raw)
	cfg, ok := card["config"].(map[string]any)
	if !ok {
		t.Fatalf("card.config 缺失")
	}
	if cfg["update_multi"] != true {
		t.Errorf("config.update_multi = %v，want true（否则 PatchMessage 失效）", cfg["update_multi"])
	}
}
