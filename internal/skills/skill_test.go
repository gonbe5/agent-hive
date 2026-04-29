package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillMetadata_IsUserInvocable_Default(t *testing.T) {
	m := SkillMetadata{Name: "test"}
	if !m.IsUserInvocable() {
		t.Error("expected IsUserInvocable to default to true")
	}
}

func TestSkillMetadata_IsUserInvocable_Explicit(t *testing.T) {
	f := false
	m := SkillMetadata{Name: "test", UserInvocable: &f}
	if m.IsUserInvocable() {
		t.Error("expected IsUserInvocable to be false")
	}

	tr := true
	m2 := SkillMetadata{Name: "test2", UserInvocable: &tr}
	if !m2.IsUserInvocable() {
		t.Error("expected IsUserInvocable to be true")
	}
}

func TestSkill_Render(t *testing.T) {
	s := &Skill{
		Content: "Review the code: $ARGUMENTS\n\nDone.",
	}

	result := s.Render(RenderContext{Arguments: "main.go"})
	expected := "Review the code: main.go\n\nDone."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSkill_Render_NoArguments(t *testing.T) {
	s := &Skill{
		Content: "No arguments placeholder here.",
	}

	result := s.Render(RenderContext{Arguments: "anything"})
	if result != "No arguments placeholder here." {
		t.Errorf("unexpected render result: %q", result)
	}
}

func TestSkill_Render_IndexedArguments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		args     string
		expected string
	}{
		{
			name:     "single indexed argument",
			content:  "File: $ARGUMENTS[0]",
			args:     "main.go",
			expected: "File: main.go",
		},
		{
			name:     "multiple indexed arguments",
			content:  "Source: $ARGUMENTS[0], Target: $ARGUMENTS[1]",
			args:     "src.go dst.go",
			expected: "Source: src.go, Target: dst.go",
		},
		{
			name:     "out of range index preserved",
			content:  "File: $ARGUMENTS[5]",
			args:     "main.go",
			expected: "File: $ARGUMENTS[5]",
		},
		{
			name:     "shorthand $N",
			content:  "File: $0, Target: $1",
			args:     "src.go dst.go",
			expected: "File: src.go, Target: dst.go",
		},
		{
			name:     "quoted argument",
			content:  "Path: $0",
			args:     `"hello world" second`,
			expected: "Path: hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Skill{Content: tt.content}
			result := s.Render(RenderContext{Arguments: tt.args})
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSkill_Render_SessionID(t *testing.T) {
	s := &Skill{
		Content: "Session: ${CLAUDE_SESSION_ID}",
	}

	result := s.Render(RenderContext{SessionID: "abc-123"})
	expected := "Session: abc-123"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSkill_Render_SkillDir(t *testing.T) {
	s := &Skill{
		Content: "Run: ${CLAUDE_SKILL_DIR}/scripts/validate.py",
	}

	result := s.Render(RenderContext{SkillDir: "/home/user/.claude/skills/my-skill"})
	expected := "Run: /home/user/.claude/skills/my-skill/scripts/validate.py"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSkill_Render_AllVariables(t *testing.T) {
	s := &Skill{
		Content: "File: $ARGUMENTS[0]\nShort: $0\nAll: $ARGUMENTS\nDir: ${CLAUDE_SKILL_DIR}\nSession: ${CLAUDE_SESSION_ID}",
	}

	result := s.Render(RenderContext{
		Arguments: "main.go",
		SessionID: "sess-42",
		SkillDir:  "/skills/test",
	})
	expected := "File: main.go\nShort: main.go\nAll: main.go\nDir: /skills/test\nSession: sess-42"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid simple", input: "my-skill", wantErr: false},
		{name: "valid single char", input: "a", wantErr: false},
		{name: "valid with numbers", input: "skill-123", wantErr: false},
		{name: "too long", input: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01234", wantErr: true},
		{name: "starts with hyphen", input: "-skill", wantErr: true},
		{name: "ends with hyphen", input: "skill-", wantErr: true},
		{name: "consecutive hyphens", input: "my--skill", wantErr: true},
		{name: "uppercase", input: "MySkill", wantErr: true},
		{name: "underscore", input: "my_skill", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSplitArguments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "simple", input: "a b c", expected: []string{"a", "b", "c"}},
		{name: "quoted", input: `"hello world" second`, expected: []string{"hello world", "second"}},
		{name: "empty", input: "", expected: nil},
		{name: "single", input: "arg", expected: []string{"arg"}},
		{name: "extra spaces", input: "a  b", expected: []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitArguments(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d parts, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("part[%d] = %q, want %q", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestSkill_LoadContent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: test-skill
description: test
---

Loaded content here.`), 0o644)

	s := &Skill{
		Metadata: SkillMetadata{Name: "test-skill"},
		Path:     dir,
		Loaded:   LevelMetadataOnly,
	}

	if err := s.LoadContent(); err != nil {
		t.Fatalf("LoadContent error: %v", err)
	}

	if s.Loaded != LevelFullContent {
		t.Errorf("expected LevelFullContent, got %d", s.Loaded)
	}
	if s.Content != "Loaded content here." {
		t.Errorf("unexpected content: %q", s.Content)
	}

	// Second call should be a no-op (sync.Once)
	if err := s.LoadContent(); err != nil {
		t.Fatalf("second LoadContent error: %v", err)
	}
}

func TestSkill_LoadBundledFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: test-skill
description: test
---

Content.`), 0o644)

	// Create subdirectories with files
	os.MkdirAll(filepath.Join(dir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(dir, "scripts", "setup.sh"), []byte("#!/bin/sh"), 0o644)
	os.WriteFile(filepath.Join(dir, "scripts", "teardown.sh"), []byte("#!/bin/sh"), 0o644)

	os.MkdirAll(filepath.Join(dir, "references"), 0o755)
	os.WriteFile(filepath.Join(dir, "references", "spec.md"), []byte("# Spec"), 0o644)

	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "logo.png"), []byte("PNG"), 0o644)

	s := &Skill{
		Metadata: SkillMetadata{Name: "test-skill"},
		Path:     dir,
		Loaded:   LevelMetadataOnly,
	}

	if err := s.LoadBundledFiles(); err != nil {
		t.Fatalf("LoadBundledFiles error: %v", err)
	}

	if s.Loaded != LevelBundledFiles {
		t.Errorf("expected LevelBundledFiles, got %d", s.Loaded)
	}
	if len(s.Bundled.Scripts) != 2 {
		t.Errorf("expected 2 scripts, got %d: %v", len(s.Bundled.Scripts), s.Bundled.Scripts)
	}
	if len(s.Bundled.References) != 1 {
		t.Errorf("expected 1 reference, got %d", len(s.Bundled.References))
	}
	if len(s.Bundled.Assets) != 1 {
		t.Errorf("expected 1 asset, got %d", len(s.Bundled.Assets))
	}
}

func TestSkillMetadata_DomainFields(t *testing.T) {
	raw := `---
name: roi-analysis
description: ROI 分析规范
domain: analytics
trigger_keywords:
  - ROI
  - 投资回报
  - 数据分析
priority: 7
complexity: medium
---

Content.`

	m, _, err := parseFrontmatter(raw)
	if err != nil {
		t.Fatalf("parseFrontmatter error: %v", err)
	}

	if m.Domain != "analytics" {
		t.Errorf("expected Domain %q, got %q", "analytics", m.Domain)
	}
	if m.Priority != 7 {
		t.Errorf("expected Priority 7, got %d", m.Priority)
	}
	if m.Complexity != "medium" {
		t.Errorf("expected Complexity %q, got %q", "medium", m.Complexity)
	}
	if len(m.TriggerKeywords) != 3 {
		t.Fatalf("expected 3 TriggerKeywords, got %d: %v", len(m.TriggerKeywords), m.TriggerKeywords)
	}
	if m.TriggerKeywords[0] != "ROI" {
		t.Errorf("expected TriggerKeywords[0] %q, got %q", "ROI", m.TriggerKeywords[0])
	}
	if m.TriggerKeywords[2] != "数据分析" {
		t.Errorf("expected TriggerKeywords[2] %q, got %q", "数据分析", m.TriggerKeywords[2])
	}
}

func TestSkill_LoadBundledFiles_NoSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: test-skill
description: test
---

Content.`), 0o644)

	s := &Skill{
		Metadata: SkillMetadata{Name: "test-skill"},
		Path:     dir,
		Loaded:   LevelMetadataOnly,
	}

	if err := s.LoadBundledFiles(); err != nil {
		t.Fatalf("LoadBundledFiles error: %v", err)
	}

	if s.Bundled.Scripts != nil {
		t.Errorf("expected nil scripts, got %v", s.Bundled.Scripts)
	}
	if s.Bundled.References != nil {
		t.Errorf("expected nil references, got %v", s.Bundled.References)
	}
	if s.Bundled.Assets != nil {
		t.Errorf("expected nil assets, got %v", s.Bundled.Assets)
	}
}
