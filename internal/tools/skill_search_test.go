package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chef-guo/agents-hive/internal/auth"
	"github.com/chef-guo/agents-hive/internal/skills"
)

// stubSkillListRegistry — 内存版，List(userID...) 按 userID 过滤 personal。
type stubSkillListRegistry struct {
	public   []skills.SkillMetadata
	personal map[string][]skills.SkillMetadata
}

func (s *stubSkillListRegistry) List(userID ...string) []skills.SkillMetadata {
	out := append([]skills.SkillMetadata(nil), s.public...)
	if len(userID) > 0 && userID[0] != "" {
		if per := s.personal[userID[0]]; len(per) > 0 {
			out = append(out, per...)
		}
	}
	return out
}

func unmarshalHits(t *testing.T, raw string) []skillSearchHit {
	t.Helper()
	var env struct {
		Count   int              `json:"count"`
		Results []skillSearchHit `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return env.Results
}

// TestSkillSearch_LocalHit — query 命中本地公共 skill。
func TestSkillSearch_LocalHit(t *testing.T) {
	reg := &stubSkillListRegistry{
		public: []skills.SkillMetadata{
			{Name: "hello-world", Description: "greet", Scope: skills.ScopePublic},
			{Name: "farewell", Description: "bye", Scope: skills.ScopePublic},
		},
	}
	in, _ := json.Marshal(skillSearchInput{Query: "hello", IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(context.Background(), reg, nil, in)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.DecodeContent())
	}
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 1 || hits[0].Name != "hello-world" {
		t.Errorf("want 1 hit hello-world, got %+v", hits)
	}
	if hits[0].Source != "local-public" {
		t.Errorf("source tag wrong: %q", hits[0].Source)
	}
}

// TestSkillSearch_TenantIsolation — bob 不应看到 alice 的 personal skill。
func TestSkillSearch_TenantIsolation(t *testing.T) {
	reg := &stubSkillListRegistry{
		personal: map[string][]skills.SkillMetadata{
			"alice": {{Name: "alice-private", Description: "secret", Scope: skills.ScopePersonal, UserID: "alice"}},
		},
	}
	bobCtx := auth.WithUser(context.Background(), &auth.User{ID: "bob", Role: "user", Status: "active"})
	in, _ := json.Marshal(skillSearchInput{Query: "private", IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(bobCtx, reg, nil, in)
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 0 {
		t.Errorf("bob must not see alice's personal skill; got %+v", hits)
	}

	// sanity: alice 自己能看到
	aliceCtx := auth.WithUser(context.Background(), &auth.User{ID: "alice", Role: "user", Status: "active"})
	res2, _ := handleSkillSearch(aliceCtx, reg, nil, in)
	hits2 := unmarshalHits(t, res2.DecodeContent())
	if len(hits2) != 1 || hits2[0].Name != "alice-private" {
		t.Errorf("alice should see her own personal skill; got %+v", hits2)
	}
	if hits2[0].Source != "local-personal" {
		t.Errorf("source tag wrong: %q", hits2[0].Source)
	}
}

// TestSkillSearch_RequirementFilter — requirements 命中过滤。
func TestSkillSearch_RequirementFilter(t *testing.T) {
	reg := &stubSkillListRegistry{
		public: []skills.SkillMetadata{
			{Name: "html-renderer", Description: "html", ProvidesRequirements: []string{"html.render"}, Scope: skills.ScopePublic},
			{Name: "markdown-renderer", Description: "md", ProvidesRequirements: []string{"md.render"}, Scope: skills.ScopePublic},
		},
	}
	in, _ := json.Marshal(skillSearchInput{Requirements: []string{"html.render"}, IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(context.Background(), reg, nil, in)
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 1 || hits[0].Name != "html-renderer" {
		t.Errorf("requirement filter wrong: %+v", hits)
	}
}

// TestSkillSearch_ScopeFilter — 只查 personal 时公共结果被过滤。
func TestSkillSearch_ScopeFilter(t *testing.T) {
	reg := &stubSkillListRegistry{
		public:   []skills.SkillMetadata{{Name: "pub-x", Scope: skills.ScopePublic}},
		personal: map[string][]skills.SkillMetadata{"alice": {{Name: "pri-x", Scope: skills.ScopePersonal, UserID: "alice"}}},
	}
	aliceCtx := auth.WithUser(context.Background(), &auth.User{ID: "alice", Role: "user", Status: "active"})
	in, _ := json.Marshal(skillSearchInput{Scope: "personal", IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(aliceCtx, reg, nil, in)
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 1 || hits[0].Name != "pri-x" {
		t.Errorf("scope=personal filter wrong: %+v", hits)
	}
}

// TestSkillSearch_Scoring — 精确 name 命中 score > description 命中。
func TestSkillSearch_Scoring(t *testing.T) {
	reg := &stubSkillListRegistry{
		public: []skills.SkillMetadata{
			{Name: "greeter", Description: "says hello to people", Scope: skills.ScopePublic},
			{Name: "hello", Description: "misc", Scope: skills.ScopePublic},
		},
	}
	in, _ := json.Marshal(skillSearchInput{Query: "hello", IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(context.Background(), reg, nil, in)
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %d", len(hits))
	}
	if hits[0].Name != "hello" {
		t.Errorf("exact name match must sort first; got %q", hits[0].Name)
	}
	if hits[0].Score <= hits[1].Score {
		t.Errorf("score ordering wrong: %+v vs %+v", hits[0], hits[1])
	}
}

// TestSkillSearch_Limit — limit N 截断结果。
func TestSkillSearch_Limit(t *testing.T) {
	reg := &stubSkillListRegistry{
		public: []skills.SkillMetadata{
			{Name: "a1", Description: "alpha", Scope: skills.ScopePublic},
			{Name: "a2", Description: "alpha", Scope: skills.ScopePublic},
			{Name: "a3", Description: "alpha", Scope: skills.ScopePublic},
		},
	}
	in, _ := json.Marshal(skillSearchInput{Query: "alpha", Limit: 2, IncludeRemote: ptrBool(false)})
	res, _ := handleSkillSearch(context.Background(), reg, nil, in)
	hits := unmarshalHits(t, res.DecodeContent())
	if len(hits) != 2 {
		t.Errorf("limit=2 not applied; got %d hits", len(hits))
	}
}

func ptrBool(b bool) *bool { return &b }
