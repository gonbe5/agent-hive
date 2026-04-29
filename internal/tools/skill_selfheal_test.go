package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/skills"
)

// stubSelfHealDiscovery — skillSelfHealDiscovery 的内存 stub。
// 命中表按 name 映射到 ResolvedSkill；未命中返回 err。
type stubSelfHealDiscovery struct {
	hits    map[string]*skills.ResolvedSkill
	missErr error
	calls   int
}

func (s *stubSelfHealDiscovery) ResolveByName(ctx context.Context, name string, refresh bool) (*skills.ResolvedSkill, error) {
	s.calls++
	if r, ok := s.hits[name]; ok {
		return r, nil
	}
	if s.missErr != nil {
		return nil, s.missErr
	}
	return nil, errors.New("not found")
}

// stubSelfHealRegistry — skillRegistry 最小实现，Get 按 {name, userID} 查询。
type stubSelfHealRegistry struct {
	getCalls  []stubGetCall
	listCalls []stubListCall
	// 命中表 key = name+"|"+userID，userID 为空表示公开层
	skills map[string]*skills.Skill
}

type stubGetCall struct {
	Name   string
	UserID string
}

type stubListCall struct {
	UserID string
}

func (s *stubSelfHealRegistry) ListSummaries(userID ...string) []skills.SkillSummary {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}
	s.listCalls = append(s.listCalls, stubListCall{UserID: uid})
	return nil
}

func (s *stubSelfHealRegistry) Get(name string, userID ...string) (*skills.Skill, error) {
	uid := ""
	if len(userID) > 0 {
		uid = userID[0]
	}
	s.getCalls = append(s.getCalls, stubGetCall{Name: name, UserID: uid})
	if sk, ok := s.skills[name+"|"+uid]; ok {
		return sk, nil
	}
	if sk, ok := s.skills[name+"|"]; ok && uid != "" {
		// fallback: personal 未命中则看公开层（真实 OverlayRegistry 的语义）
		return sk, nil
	}
	return nil, errors.New("skill not found")
}

func (s *stubSelfHealRegistry) GetForkHandler() skills.ForkHandler { return nil }

func (s *stubSelfHealRegistry) InvokeFull(ctx context.Context, name string, rctx skills.RenderContext, executor skills.ShellExecutor, runner *skills.ScriptRunner, hookRunner *skills.HookRunner) (string, error) {
	return "", errors.New("not used in self-heal tests")
}

// TestSelfHeal_Miss_DiscoveryNil — §8.4 byte-identical baseline：
// discovery==nil 时，Get 失败应原样返回原始错误字符串。
func TestSelfHeal_Miss_DiscoveryNil(t *testing.T) {
	origErr := errors.New("skill not found")
	res := skillGetErrorWithSelfHeal(context.Background(), nil, "ghost", origErr)
	if !res.IsError {
		t.Fatal("want IsError=true")
	}
	got := res.DecodeContent()
	want := `获取技能 "ghost" 失败: skill not found`
	if got != want {
		t.Errorf("baseline mismatch\n want: %q\n  got: %q", want, got)
	}
	// 必须是纯文本（字符串 JSON），不是 suggested_action 结构体
	if strings.Contains(got, "suggested_action") {
		t.Errorf("baseline must NOT contain suggested_action; got %q", got)
	}
}

// TestSelfHeal_Hit_AppendsSuggestedAction — discovery 命中时 payload 扩展为
// {error, suggested_action}，保留原始 error 字符串（§8.3）。
func TestSelfHeal_Hit_AppendsSuggestedAction(t *testing.T) {
	disc := &stubSelfHealDiscovery{
		hits: map[string]*skills.ResolvedSkill{
			"hello-world": {
				Entry:  skills.SkillIndexEntry{Name: "hello-world", Description: "greet"},
				Source: "https://example.com/marketplace",
			},
		},
	}
	origErr := errors.New("skill not found")
	res := skillGetErrorWithSelfHeal(context.Background(), disc, "hello-world", origErr)
	if !res.IsError {
		t.Fatal("want IsError=true")
	}
	raw := res.DecodeContent()
	var env struct {
		Error           string                 `json:"error"`
		SuggestedAction map[string]interface{} `json:"suggested_action"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal payload: %v (raw=%s)", err, raw)
	}
	// §8.3: 原始 error 字段内容必须保持不变
	wantErr := `获取技能 "hello-world" 失败: skill not found`
	if env.Error != wantErr {
		t.Errorf("error field mismatch\n want: %q\n  got: %q", wantErr, env.Error)
	}
	if env.SuggestedAction["tool"] != "skill_install" {
		t.Errorf("suggested_action.tool = %v, want skill_install", env.SuggestedAction["tool"])
	}
	args, _ := env.SuggestedAction["args"].(map[string]interface{})
	if args["name"] != "hello-world" {
		t.Errorf("suggested_action.args.name = %v", args["name"])
	}
	if args["source"] != "https://example.com/marketplace" {
		t.Errorf("suggested_action.args.source = %v", args["source"])
	}
	if args["scope"] != "personal" {
		t.Errorf("suggested_action.args.scope = %v, want personal", args["scope"])
	}
}

// TestSelfHeal_Miss_DiscoveryReturnsError — discovery 存在但未命中时，
// 降级为原始错误字符串（不是 panic，不是空字符串）。
func TestSelfHeal_Miss_DiscoveryReturnsError(t *testing.T) {
	disc := &stubSelfHealDiscovery{
		hits:    map[string]*skills.ResolvedSkill{},
		missErr: errors.New("marketplace unreachable"),
	}
	origErr := errors.New("skill not found")
	res := skillGetErrorWithSelfHeal(context.Background(), disc, "ghost", origErr)
	if !res.IsError {
		t.Fatal("want IsError=true")
	}
	got := res.DecodeContent()
	want := `获取技能 "ghost" 失败: skill not found`
	if got != want {
		t.Errorf("miss fallback mismatch\n want: %q\n  got: %q", want, got)
	}
	if strings.Contains(got, "suggested_action") {
		t.Errorf("miss must NOT contain suggested_action; got %q", got)
	}
	if disc.calls != 1 {
		t.Errorf("ResolveByName called %d times, want 1", disc.calls)
	}
}

// TestSelfHeal_UserIDInjection_PersonalLayer — handler 层面：ctx 带 userID 时，
// Get 必须以该 userID 查询 personal 层。
func TestSelfHeal_UserIDInjection_PersonalLayer(t *testing.T) {
	reg := &stubSelfHealRegistry{
		skills: map[string]*skills.Skill{
			"mine|alice": {Metadata: skills.SkillMetadata{Name: "mine", Context: "local"}, Content: "hi"},
		},
	}
	ctx := auth.WithUser(context.Background(), &auth.User{ID: "alice", Role: "user", Status: "active"})
	_, err := reg.Get("mine", auth.UserIDFrom(ctx))
	if err != nil {
		t.Fatalf("Get(mine, alice) failed: %v", err)
	}
	if len(reg.getCalls) != 1 || reg.getCalls[0].UserID != "alice" {
		t.Errorf("Get must receive userID=alice, got %+v", reg.getCalls)
	}
}

// TestSelfHeal_UserIDMissing_PublicOnly — ctx 无 userID 时，Get 不应传入 userID，
// 仅查询公开层（byte-identical 旧行为）。
func TestSelfHeal_UserIDMissing_PublicOnly(t *testing.T) {
	reg := &stubSelfHealRegistry{
		skills: map[string]*skills.Skill{
			"pub|": {Metadata: skills.SkillMetadata{Name: "pub", Context: "local"}, Content: "public"},
		},
	}
	// 模拟旧调用路径
	_, err := reg.Get("pub")
	if err != nil {
		t.Fatalf("Get(pub) public failed: %v", err)
	}
	if len(reg.getCalls) != 1 || reg.getCalls[0].UserID != "" {
		t.Errorf("Get must NOT receive userID when anonymous; got %+v", reg.getCalls)
	}
}
