package skills

import (
	"context"
	"errors"
	"testing"
)

// mockLocalFinder records Find calls (for method-split enforcement) and returns pre-set skills.
type mockLocalFinder struct {
	callCount int
	lastReqs  []string
	lastUID   string
	result    []*Skill
}

func (m *mockLocalFinder) FindBySpecRequirements(reqs []string, userID string) []*Skill {
	m.callCount++
	m.lastReqs = reqs
	m.lastUID = userID
	return m.result
}

// mockRemoteFinder records Resolve calls (for method-split enforcement) and returns pre-set results.
type mockRemoteFinder struct {
	callCount int
	lastReqs  []string
	result    []*ResolvedSkill
	err       error
}

func (m *mockRemoteFinder) ResolveByRequirements(_ context.Context, reqs []string) ([]*ResolvedSkill, error) {
	m.callCount++
	m.lastReqs = reqs
	return m.result, m.err
}

// TestSpecResolver_LocalHitShortCircuitsRemote — 本地命中必须直接返回，远程不应被调。
func TestSpecResolver_LocalHitShortCircuitsRemote(t *testing.T) {
	local := &mockLocalFinder{result: []*Skill{{Metadata: SkillMetadata{Name: "local-skill"}}}}
	remote := &mockRemoteFinder{}
	r := NewSpecSkillResolver(local, remote, func() bool { return true })

	got, err := r.Resolve(context.Background(), []string{"req-a"}, "alice")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 1 || got.Local[0].Metadata.Name != "local-skill" {
		t.Errorf("expected local hit, got %+v", got.Local)
	}
	if len(got.Remote) != 0 {
		t.Errorf("remote must be empty on local hit, got %+v", got.Remote)
	}
	if got.Suggested != nil {
		t.Errorf("Suggested must be nil on local hit")
	}
	if remote.callCount != 0 {
		t.Errorf("remote called %d times on local hit, expected 0", remote.callCount)
	}
}

// TestSpecResolver_LocalMissRemoteHit — 本地 miss + flag on → 远程命中返回带 SuggestedAction。
func TestSpecResolver_LocalMissRemoteHit(t *testing.T) {
	local := &mockLocalFinder{result: nil}
	remote := &mockRemoteFinder{
		result: []*ResolvedSkill{
			{Entry: SkillIndexEntry{Name: "remote-skill", ScopeHint: "personal"}, Source: "https://mp/"},
		},
	}
	r := NewSpecSkillResolver(local, remote, func() bool { return true })

	got, err := r.Resolve(context.Background(), []string{"req-x"}, "alice")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Remote) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(got.Remote))
	}
	if got.Suggested == nil || got.Suggested.Tool != "skill_install" {
		t.Errorf("expected SuggestedAction{tool: skill_install}, got %+v", got.Suggested)
	}
	if got.Suggested.Args["name"] != "remote-skill" {
		t.Errorf("suggested args wrong: %+v", got.Suggested.Args)
	}
	if got.Suggested.Args["scope"] != "personal" {
		t.Errorf("userID set → scope should be personal, got %v", got.Suggested.Args["scope"])
	}
	if remote.callCount != 1 {
		t.Errorf("remote should be called exactly once, got %d", remote.callCount)
	}
}

// TestSpecResolver_LocalMissRemoteFlagOff — flag 关时绝不调 remote。
func TestSpecResolver_LocalMissRemoteFlagOff(t *testing.T) {
	local := &mockLocalFinder{}
	remote := &mockRemoteFinder{result: []*ResolvedSkill{{Entry: SkillIndexEntry{Name: "x"}, Source: "u"}}}
	r := NewSpecSkillResolver(local, remote, func() bool { return false })

	got, err := r.Resolve(context.Background(), []string{"req"}, "alice")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 0 || len(got.Remote) != 0 || got.Suggested != nil {
		t.Errorf("expected empty result on flag off, got %+v", got)
	}
	if remote.callCount != 0 {
		t.Errorf("remote must NOT be called when flag off, got %d calls", remote.callCount)
	}
}

// TestSpecResolver_RemoteError — remote 错误透传；Local/Suggested 保持空。
func TestSpecResolver_RemoteError(t *testing.T) {
	local := &mockLocalFinder{}
	remote := &mockRemoteFinder{err: errors.New("network down")}
	r := NewSpecSkillResolver(local, remote, func() bool { return true })

	_, err := r.Resolve(context.Background(), []string{"req"}, "alice")
	if err == nil {
		t.Fatal("expected remote error to propagate, got nil")
	}
}

// TestSpecResolver_MethodSplitGuard — local finder 只能收 FindBySpecRequirements，
// remote finder 只能收 ResolveByRequirements（mock 两边都只实现一个方法，类型系统守卫）。
// 这里通过调用 counter 核对：每路径只命中自己该调的一边。
func TestSpecResolver_MethodSplitGuard(t *testing.T) {
	// 路径 A：本地命中
	localA := &mockLocalFinder{result: []*Skill{{Metadata: SkillMetadata{Name: "a"}}}}
	remoteA := &mockRemoteFinder{}
	rA := NewSpecSkillResolver(localA, remoteA, func() bool { return true })
	_, _ = rA.Resolve(context.Background(), []string{"x"}, "u")
	if localA.callCount != 1 || remoteA.callCount != 0 {
		t.Errorf("local-hit path: local=%d remote=%d (want 1,0)", localA.callCount, remoteA.callCount)
	}

	// 路径 B：本地 miss + 远程命中
	localB := &mockLocalFinder{result: nil}
	remoteB := &mockRemoteFinder{result: []*ResolvedSkill{{Entry: SkillIndexEntry{Name: "b"}, Source: "u"}}}
	rB := NewSpecSkillResolver(localB, remoteB, func() bool { return true })
	_, _ = rB.Resolve(context.Background(), []string{"x"}, "u")
	if localB.callCount != 1 || remoteB.callCount != 1 {
		t.Errorf("local-miss+remote-hit: local=%d remote=%d (want 1,1)", localB.callCount, remoteB.callCount)
	}
}

// TestSpecResolver_EmptyReqs — 空 reqs 直接返回空结果，不调任何底层。
func TestSpecResolver_EmptyReqs(t *testing.T) {
	local := &mockLocalFinder{}
	remote := &mockRemoteFinder{}
	r := NewSpecSkillResolver(local, remote, func() bool { return true })
	got, err := r.Resolve(context.Background(), nil, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Local != nil || got.Remote != nil || got.Suggested != nil {
		t.Errorf("expected empty, got %+v", got)
	}
	if local.callCount+remote.callCount != 0 {
		t.Errorf("empty reqs must short-circuit, got local=%d remote=%d", local.callCount, remote.callCount)
	}
}

// TestSpecResolver_AnonymousDowngradesToPublic — userID="" 时 scope 降级为 public。
func TestSpecResolver_AnonymousDowngradesToPublic(t *testing.T) {
	local := &mockLocalFinder{}
	remote := &mockRemoteFinder{result: []*ResolvedSkill{
		{Entry: SkillIndexEntry{Name: "pub", ScopeHint: "personal"}, Source: "u"},
	}}
	r := NewSpecSkillResolver(local, remote, func() bool { return true })
	got, err := r.Resolve(context.Background(), []string{"x"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Suggested == nil || got.Suggested.Args["scope"] != "public" {
		t.Errorf("anonymous should downgrade to public, got %+v", got.Suggested)
	}
}

// TestRegistry_FindBySpecRequirements_TenantAware — MAJOR 1 stub 的租户隔离契约：
// alice 的 personal skill 对 bob 不可见，public 对所有人可见。
func TestRegistry_FindBySpecRequirements_TenantAware(t *testing.T) {
	r := newTestRegistry()
	pub := newScopedSkill("pub", ScopePublic, "", "1.0.0")
	pub.Metadata.ProvidesRequirements = []string{"cap-a"}
	if err := r.Register(pub); err != nil {
		t.Fatalf("register pub: %v", err)
	}
	priv := newScopedSkill("priv", ScopePersonal, "alice", "1.0.0")
	priv.Metadata.ProvidesRequirements = []string{"cap-a"}
	if err := r.Register(priv); err != nil {
		t.Fatalf("register priv: %v", err)
	}

	// alice 能看到自己的 personal + public
	aliceHits := r.FindBySpecRequirements([]string{"cap-a"}, "alice")
	aliceNames := skillNames(aliceHits)
	if len(aliceHits) != 2 || !aliceNames["priv"] || !aliceNames["pub"] {
		t.Errorf("alice should see both priv+pub, got %v", aliceNames)
	}

	// bob 只看到 public（绝不泄漏 alice 的 priv）
	bobHits := r.FindBySpecRequirements([]string{"cap-a"}, "bob")
	for _, s := range bobHits {
		if s.Metadata.Name == "priv" {
			t.Fatalf("bob leaked alice's priv: %+v", s.Metadata)
		}
	}
}

func skillNames(ss []*Skill) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s.Metadata.Name] = true
	}
	return m
}
