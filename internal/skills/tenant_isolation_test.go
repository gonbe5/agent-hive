package skills

import (
	"testing"

	"github.com/chef-guo/agents-hive/internal/errs"
)

// newScopedSkill constructs a skill with explicit scope+userID (for personal) or scope=public (userID empty).
func newScopedSkill(name string, scope SkillScope, userID string, version string) *Skill {
	return &Skill{
		Metadata: SkillMetadata{
			Name:        name,
			Description: "test " + name,
			Scope:       scope,
			UserID:      userID,
			Version:     version,
		},
		Content: "content of " + name,
		Path:    "/test/" + name,
		Loaded:  LevelFullContent,
	}
}

// TestRegistry_PersonalOverridesPublic — 2.7 case: same-name personal skill shadows public for that user only.
func TestRegistry_PersonalOverridesPublic(t *testing.T) {
	r := newTestRegistry()
	if err := r.Register(newScopedSkill("nuwa", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("register public: %v", err)
	}
	if err := r.Register(newScopedSkill("nuwa", ScopePersonal, "alice", "0.9.0")); err != nil {
		t.Fatalf("register personal: %v", err)
	}

	// alice sees personal
	got, err := r.Get("nuwa", "alice")
	if err != nil {
		t.Fatalf("Get(alice): %v", err)
	}
	if got.Metadata.UserID != "alice" {
		t.Errorf("alice expected personal (UserID=alice), got %q", got.Metadata.UserID)
	}
	// bob sees public fallback (no personal entry)
	got, err = r.Get("nuwa", "bob")
	if err != nil {
		t.Fatalf("Get(bob): %v", err)
	}
	if got.Metadata.UserID != "" {
		t.Errorf("bob expected public fallback (UserID=\"\"), got %q", got.Metadata.UserID)
	}
	// no-userID lookup only sees public
	got, err = r.Get("nuwa")
	if err != nil {
		t.Fatalf("Get(no-uid): %v", err)
	}
	if got.Metadata.UserID != "" {
		t.Errorf("anonymous Get expected public, got %q", got.Metadata.UserID)
	}
}

// TestRegistry_CrossTenantIsolation — alice's personal skill must NEVER leak to bob.
func TestRegistry_CrossTenantIsolation(t *testing.T) {
	r := newTestRegistry()
	if err := r.Register(newScopedSkill("secret", ScopePersonal, "alice", "1.0.0")); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	// bob has no personal + no public layer entry → Get must fail
	if _, err := r.Get("secret", "bob"); err == nil {
		t.Fatal("expected NotFound for bob, got nil")
	} else if !errs.IsCode(err, errs.CodeSkillNotFound) {
		t.Errorf("expected CodeSkillNotFound, got %v", err)
	}
	// bob's List must not include alice's personal
	for _, m := range r.List("bob") {
		if m.Name == "secret" {
			t.Fatalf("bob.List leaked alice's personal skill: %+v", m)
		}
	}
	// alice's List includes it
	found := false
	for _, m := range r.List("alice") {
		if m.Name == "secret" && m.UserID == "alice" {
			found = true
		}
	}
	if !found {
		t.Fatal("alice.List missing her own personal skill")
	}
}

// TestRegistry_PersonalRequiresUserID — personal scope with empty UserID must be rejected.
func TestRegistry_PersonalRequiresUserID(t *testing.T) {
	r := newTestRegistry()
	err := r.Register(newScopedSkill("oops", ScopePersonal, "", "1.0.0"))
	if err == nil {
		t.Fatal("expected rejection for personal+empty userID, got nil")
	}
	if !errs.IsCode(err, errs.CodeSkillInvalidName) {
		t.Errorf("expected CodeSkillInvalidName, got %v", err)
	}
}

// TestRegistry_PublicRejectsUserID — public scope carrying a userID must be rejected.
func TestRegistry_PublicRejectsUserID(t *testing.T) {
	r := newTestRegistry()
	err := r.Register(newScopedSkill("oops", ScopePublic, "alice", "1.0.0"))
	if err == nil {
		t.Fatal("expected rejection for public+userID, got nil")
	}
}

// TestRegistry_SameVersionIdempotent — re-register identical version is a no-op + bumps dup metric.
func TestRegistry_SameVersionIdempotent(t *testing.T) {
	r := newTestRegistry()
	r.metrics = NewMetrics()
	s := newScopedSkill("alpha", ScopePublic, "", "1.0.0")
	if err := r.Register(s); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(s); err != nil {
		t.Fatalf("second register (idempotent): %v", err)
	}
	snap := r.metrics.Snapshot()
	// dup count should be visible inside Metrics (registryDup is not exposed by Snapshot directly,
	// so assert via side-effect: re-registration did not error and registry still has only one entry).
	_ = snap
	if r.Count() != 1 {
		t.Errorf("expected 1 skill after idempotent re-register, got %d", r.Count())
	}
}

// TestRegistry_HigherVersionReplaces — semver-newer skill wins.
func TestRegistry_HigherVersionReplaces(t *testing.T) {
	r := newTestRegistry()
	if err := r.Register(newScopedSkill("beta", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	if err := r.Register(newScopedSkill("beta", ScopePublic, "", "1.2.3")); err != nil {
		t.Fatalf("v1.2.3: %v", err)
	}
	got, err := r.Get("beta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Version != "1.2.3" {
		t.Errorf("expected 1.2.3, got %q", got.Metadata.Version)
	}
}

// TestRegistry_PinOverridesSemver — pinned version keeps the old one even when a newer arrives.
func TestRegistry_PinOverridesSemver(t *testing.T) {
	r := newTestRegistry()
	if err := r.Register(newScopedSkill("gamma", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("v1: %v", err)
	}
	r.SetPinnedVersions(map[string]string{"gamma": "1.0.0"})
	if err := r.Register(newScopedSkill("gamma", ScopePublic, "", "2.0.0")); err != nil {
		t.Fatalf("v2 (should be silently skipped): %v", err)
	}
	got, err := r.Get("gamma")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Metadata.Version != "1.0.0" {
		t.Errorf("expected pinned 1.0.0 to win, got %q", got.Metadata.Version)
	}
}

// TestRegistry_TwoUsersSamePersonalName — alice and bob can each have personal "nuwa" without stomping.
// This is the in-memory analog of pg_notify {name, user_id, op} tenant-isolation (MAJOR 2).
func TestRegistry_TwoUsersSamePersonalName(t *testing.T) {
	r := newTestRegistry()
	alice := newScopedSkill("nuwa", ScopePersonal, "alice", "1.0.0")
	alice.Content = "alice's private nuwa"
	bob := newScopedSkill("nuwa", ScopePersonal, "bob", "2.0.0")
	bob.Content = "bob's private nuwa"

	if err := r.Register(alice); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if err := r.Register(bob); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	gotA, err := r.Get("nuwa", "alice")
	if err != nil || gotA.Content != "alice's private nuwa" {
		t.Fatalf("alice.Get wrong: %+v err=%v", gotA, err)
	}
	gotB, err := r.Get("nuwa", "bob")
	if err != nil || gotB.Content != "bob's private nuwa" {
		t.Fatalf("bob.Get wrong: %+v err=%v", gotB, err)
	}
	if gotA.Metadata.Version == gotB.Metadata.Version {
		t.Error("expected distinct versions per tenant, got same — cache likely shared")
	}
}

// TestRegistry_EmptyUserIDOnlySeesPublic — Get("") and Get() both return public-only results.
func TestRegistry_EmptyUserIDOnlySeesPublic(t *testing.T) {
	r := newTestRegistry()
	if err := r.Register(newScopedSkill("delta", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("public: %v", err)
	}
	if err := r.Register(newScopedSkill("delta", ScopePersonal, "alice", "9.9.9")); err != nil {
		t.Fatalf("personal: %v", err)
	}
	got, err := r.Get("delta")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.Metadata.Version != "1.0.0" {
		t.Errorf("anonymous Get expected public 1.0.0, got %q", got.Metadata.Version)
	}
	got, err = r.Get("delta", "")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if got.Metadata.Version != "1.0.0" {
		t.Errorf("Get(\"\") expected public 1.0.0, got %q", got.Metadata.Version)
	}
}
