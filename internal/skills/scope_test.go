package skills

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestParseScope(t *testing.T) {
	cases := []struct {
		in      string
		want    SkillScope
		wantErr bool
	}{
		{"", ScopePublic, false},
		{"public", ScopePublic, false},
		{"Public", ScopePublic, false},
		{"personal", ScopePersonal, false},
		{"PERSONAL", ScopePersonal, false},
		{"private", "", true},
		{"org", "", true},
	}
	for _, c := range cases {
		got, err := ParseScope(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseScope(%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseScope(%q) unexpected err: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseScope(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSkillScope_String(t *testing.T) {
	if SkillScope("").String() != "public" {
		t.Errorf("empty scope should default to public in String()")
	}
	if ScopePersonal.String() != "personal" {
		t.Errorf("personal scope String() wrong")
	}
}

// writeSkill 在 dir 下建 name 目录 + SKILL.md
func writeSkill(t *testing.T, dir, name, frontmatter string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" + frontmatter + "---\nbody\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFinder_PathInferredScope(t *testing.T) {
	tmp := t.TempDir()
	publicDir := filepath.Join(tmp, "public")
	personalRoot := filepath.Join(tmp, "users")

	writeSkill(t, publicDir, "shared-tool",
		"name: shared-tool\ndescription: public one\n")

	aliceDir := filepath.Join(personalRoot, "alice")
	writeSkill(t, aliceDir, "alice-note",
		"name: alice-note\ndescription: alice personal\n")

	reg := NewRegistry(zap.NewNop())
	f := NewFinder(reg, zap.NewNop(), nil,
		WithPublicSkillsDir(publicDir),
		WithPersonalSkillsRoot(personalRoot),
	)
	skills, err := f.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	byName := map[string]*Skill{}
	for _, s := range skills {
		byName[s.Metadata.Name] = s
	}
	if byName["shared-tool"].Metadata.Scope != ScopePublic {
		t.Errorf("shared-tool scope should be public, got %q", byName["shared-tool"].Metadata.Scope)
	}
	if byName["shared-tool"].Metadata.UserID != "" {
		t.Errorf("public skill should not carry userID, got %q", byName["shared-tool"].Metadata.UserID)
	}
	if byName["alice-note"].Metadata.Scope != ScopePersonal {
		t.Errorf("alice-note scope should be personal")
	}
	if byName["alice-note"].Metadata.UserID != "alice" {
		t.Errorf("alice-note userID want alice, got %q", byName["alice-note"].Metadata.UserID)
	}
}

func TestFinder_FrontmatterScopeOverride(t *testing.T) {
	tmp := t.TempDir()
	publicDir := filepath.Join(tmp, "public")
	// frontmatter 声明 personal，但 path 推断是 public 且无 userID → 必须被拒绝
	writeSkill(t, publicDir, "sneaky",
		"name: sneaky\ndescription: claims personal in public path\nscope: personal\n")

	reg := NewRegistry(zap.NewNop())
	f := NewFinder(reg, zap.NewNop(), nil, WithPublicSkillsDir(publicDir))
	skills, err := f.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("personal-in-public-path without userID must be rejected, got %d skills", len(skills))
	}
}

func TestFinder_LegacyPathsStayPublic(t *testing.T) {
	tmp := t.TempDir()
	writeSkill(t, tmp, "legacy",
		"name: legacy\ndescription: legacy path\n")

	reg := NewRegistry(zap.NewNop())
	f := NewFinder(reg, zap.NewNop(), []string{tmp})
	skills, err := f.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Metadata.Scope != ScopePublic {
		t.Errorf("legacy path must default to public, got %q", skills[0].Metadata.Scope)
	}
}
