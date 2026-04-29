package skills

import (
	"testing"

	"go.uber.org/zap"
)

// overlayFixture builds an OverlayRegistry with at most one skill per layer for a given (name, userID) pair.
//
//	layers to seed:
//	  personalDB:   (name, userID, revision) → UpsertDB(name, userID, ...)
//	  personalFS:   Register( Skill{Scope:personal, UserID} )
//	  publicDB:     UpsertDB(name, "", ...)
//	  publicFS:     Register( Skill{Scope:public} )
func newOverlayFixture(t *testing.T) *OverlayRegistry {
	t.Helper()
	logger := zap.NewNop()
	return NewOverlayRegistry(logger)
}

// TestOverlay_FourLayerPriority — with all 4 layers populated for "nuwa" and user=alice,
// Get must return the personal DB copy; then progressively strip layers to verify fallback order.
func TestOverlay_FourLayerPriority(t *testing.T) {
	o := newOverlayFixture(t)
	const name = "nuwa"
	const alice = "alice"

	// Layer 4 (lowest): public FS
	pubFS := newScopedSkill(name, ScopePublic, "", "1.0.0")
	pubFS.Content = "public-fs"
	if err := o.Registry.Register(pubFS); err != nil {
		t.Fatalf("seed public FS: %v", err)
	}
	// Layer 3: public DB (frontmatter-free content body)
	o.UpsertDB(name, "", "public-db body", "/db/public/nuwa", 3)
	// Layer 2: personal FS
	persFS := newScopedSkill(name, ScopePersonal, alice, "2.0.0")
	persFS.Content = "personal-fs"
	if err := o.Registry.Register(persFS); err != nil {
		t.Fatalf("seed personal FS: %v", err)
	}
	// Layer 1 (highest): personal DB
	o.UpsertDB(name, alice, "personal-db body", "/db/personal/alice/nuwa", 5)

	got, err := o.Get(name, alice)
	if err != nil {
		t.Fatalf("Get(alice): %v", err)
	}
	if got.Content != "personal-db body" {
		t.Errorf("L1 priority broken: expected personal DB, got %q", got.Content)
	}

	// Strip L1 → L2 (personal FS) wins
	o.DeleteDB(name, alice)
	got, err = o.Get(name, alice)
	if err != nil {
		t.Fatalf("after L1 strip: %v", err)
	}
	if got.Content != "personal-fs" {
		t.Errorf("L2 priority broken: expected personal FS, got %q", got.Content)
	}

	// Strip L2 (remove from registry map directly — no public unregister API needed)
	if err := o.Registry.Unregister(name, alice); err != nil {
		t.Fatalf("unregister personal FS: %v", err)
	}
	got, err = o.Get(name, alice)
	if err != nil {
		t.Fatalf("after L2 strip: %v", err)
	}
	if got.Content != "public-db body" {
		t.Errorf("L3 priority broken: expected public DB, got %q", got.Content)
	}

	// Strip L3 → public FS wins
	o.DeleteDB(name, "")
	got, err = o.Get(name, alice)
	if err != nil {
		t.Fatalf("after L3 strip: %v", err)
	}
	if got.Content != pubFS.Content {
		t.Errorf("L4 priority broken: expected public FS, got %q", got.Content)
	}
}

// TestOverlay_PersonalDBCrossTenantIsolation — alice's personal DB entry MUST NOT leak to bob.
// This is the core MAJOR 2 regression protection: the old dbCache[name] single-key design would
// stomp entries across tenants; the new {name, user_id} composite key prevents it.
func TestOverlay_PersonalDBCrossTenantIsolation(t *testing.T) {
	o := newOverlayFixture(t)
	const name = "nuwa"

	// Public FS baseline (both should see this as fallback)
	if err := o.Registry.Register(newScopedSkill(name, ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("public seed: %v", err)
	}
	o.UpsertDB(name, "alice", "alice-private-nuwa", "/db/alice/nuwa", 10)
	o.UpsertDB(name, "bob", "bob-private-nuwa", "/db/bob/nuwa", 11)

	gotA, err := o.Get(name, "alice")
	if err != nil {
		t.Fatalf("alice Get: %v", err)
	}
	if gotA.Content != "alice-private-nuwa" {
		t.Errorf("alice got wrong skill: %q", gotA.Content)
	}

	gotB, err := o.Get(name, "bob")
	if err != nil {
		t.Fatalf("bob Get: %v", err)
	}
	if gotB.Content != "bob-private-nuwa" {
		t.Errorf("bob got wrong skill: %q", gotB.Content)
	}

	// Carol (no personal entry) falls back to public FS
	gotC, err := o.Get(name, "carol")
	if err != nil {
		t.Fatalf("carol Get: %v", err)
	}
	if gotC.Metadata.UserID != "" {
		t.Errorf("carol should see public (UserID=\"\"), got %q", gotC.Metadata.UserID)
	}

	// Deleting alice's entry MUST NOT touch bob's
	o.DeleteDB(name, "alice")
	gotB2, err := o.Get(name, "bob")
	if err != nil {
		t.Fatalf("bob after alice delete: %v", err)
	}
	if gotB2.Content != "bob-private-nuwa" {
		t.Errorf("bob's entry was collateral-deleted: got %q", gotB2.Content)
	}
}

// TestOverlay_ListSummariesMarkOverride — ListSummaries for alice must flag
// OverriddenPublic=true on personal skills that shadow a public namesake.
func TestOverlay_ListSummariesMarkOverride(t *testing.T) {
	o := newOverlayFixture(t)
	// Public layer "shared"
	if err := o.Registry.Register(newScopedSkill("shared", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("public: %v", err)
	}
	// Public-only "pubonly"
	if err := o.Registry.Register(newScopedSkill("pubonly", ScopePublic, "", "1.0.0")); err != nil {
		t.Fatalf("pubonly: %v", err)
	}
	// Alice overrides "shared" with personal FS
	if err := o.Registry.Register(newScopedSkill("shared", ScopePersonal, "alice", "2.0.0")); err != nil {
		t.Fatalf("alice.shared: %v", err)
	}
	// Alice has personal-only "alicenew"
	if err := o.Registry.Register(newScopedSkill("alicenew", ScopePersonal, "alice", "1.0.0")); err != nil {
		t.Fatalf("alice.alicenew: %v", err)
	}

	sums := o.ListSummaries("alice")
	byName := make(map[string]SkillSummary, len(sums))
	for _, s := range sums {
		byName[s.Name] = s
	}

	if s, ok := byName["shared"]; !ok {
		t.Error("alice should see shared")
	} else if !s.OverriddenPublic {
		t.Errorf("shared should be marked OverriddenPublic for alice, got %+v", s)
	}
	if s, ok := byName["alicenew"]; !ok {
		t.Error("alice should see alicenew")
	} else if s.OverriddenPublic {
		t.Errorf("alicenew has no public counterpart, must not be OverriddenPublic: %+v", s)
	}
	if s, ok := byName["pubonly"]; !ok {
		t.Error("alice should see pubonly fallback")
	} else if s.OverriddenPublic {
		t.Errorf("pubonly has no personal counterpart, must not be OverriddenPublic: %+v", s)
	}
}
