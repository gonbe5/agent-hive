package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestFinder_Discover(t *testing.T) {
	dir := t.TempDir()

	// Create skill directories with SKILL.md files
	skillADir := filepath.Join(dir, "skill-a")
	os.MkdirAll(skillADir, 0o755)
	os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte(`---
name: skill-a
description: Skill A description
---

Skill A content.
`), 0o644)

	skillBDir := filepath.Join(dir, "skill-b")
	os.MkdirAll(skillBDir, 0o755)
	os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte(`---
name: skill-b
description: Skill B description
allowed-tools:
  - Read
  - Grep
---

Skill B content with $ARGUMENTS placeholder.
`), 0o644)

	// Create a directory without SKILL.md (should be ignored)
	os.MkdirAll(filepath.Join(dir, "no-skill"), 0o755)

	// Create a file (should be ignored)
	os.WriteFile(filepath.Join(dir, "not-a-skill.txt"), []byte("hi"), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered skills, got %d", len(discovered))
	}

	names := map[string]bool{}
	for _, d := range discovered {
		names[d.Metadata.Name] = true
	}
	if !names["skill-a"] || !names["skill-b"] {
		t.Errorf("expected skill-a and skill-b, got %v", names)
	}

	// Level 1: content should be empty
	for _, d := range discovered {
		if d.Content != "" {
			t.Errorf("expected empty Content at Level 1 for %s, got %q", d.Metadata.Name, d.Content)
		}
		if d.Loaded != LevelMetadataOnly {
			t.Errorf("expected LevelMetadataOnly for %s, got %d", d.Metadata.Name, d.Loaded)
		}
	}
}

func TestFinder_DiscoverAndRegister(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: my-skill
description: A test skill
---

My skill content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	err := finder.DiscoverAndRegister()
	if err != nil {
		t.Fatalf("DiscoverAndRegister returned error: %v", err)
	}

	if reg.Count() != 1 {
		t.Fatalf("expected 1 registered skill, got %d", reg.Count())
	}

	skill, err := reg.Get("my-skill")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if skill.Metadata.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got %s", skill.Metadata.Description)
	}
	// Level 1 — content is empty until LoadContent/Invoke
	if skill.Content != "" {
		t.Errorf("expected empty content at Level 1, got %q", skill.Content)
	}
}

func TestFinder_DiscoverNonexistentPath(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{"/nonexistent/path"})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 discovered skills, got %d", len(discovered))
	}
}

func TestFinder_SearchPaths(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{"/a", "/b"})

	paths := finder.SearchPaths()
	if len(paths) != 2 || paths[0] != "/a" || paths[1] != "/b" {
		t.Errorf("unexpected search paths: %v", paths)
	}
}

func TestFinder_DiscoverNoFrontmatter(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "plain-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Just plain markdown content."), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered skill, got %d", len(discovered))
	}
	// Name should default to directory name
	if discovered[0].Metadata.Name != "plain-skill" {
		t.Errorf("expected name 'plain-skill', got %s", discovered[0].Metadata.Name)
	}
}

func TestFinder_DiscoverWithAllowedTools(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "tool-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: tool-skill
description: Skill with allowed tools
allowed-tools:
  - Read
  - Write
  - Bash
---

Content here.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered skill, got %d", len(discovered))
	}

	tools := discovered[0].Metadata.AllowedTools
	if len(tools) != 3 {
		t.Fatalf("expected 3 allowed tools, got %d", len(tools))
	}
	if tools[0] != "Read" || tools[1] != "Write" || tools[2] != "Bash" {
		t.Errorf("unexpected allowed tools: %v", tools)
	}
}

func TestFinder_DiscoverNameValidation(t *testing.T) {
	dir := t.TempDir()

	// Valid skill
	validDir := filepath.Join(dir, "valid-skill")
	os.MkdirAll(validDir, 0o755)
	os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(`---
name: valid-skill
description: valid
---

Content.
`), 0o644)

	// Invalid name (uppercase)
	invalidDir := filepath.Join(dir, "InvalidSkill")
	os.MkdirAll(invalidDir, 0o755)
	os.WriteFile(filepath.Join(invalidDir, "SKILL.md"), []byte(`---
name: InvalidSkill
description: invalid
---

Content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	// Only valid-skill should be discovered
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered skill (invalid name should be skipped), got %d", len(discovered))
	}
	if discovered[0].Metadata.Name != "valid-skill" {
		t.Errorf("expected valid-skill, got %s", discovered[0].Metadata.Name)
	}
}

func TestFinder_DiscoverNameMismatchDirectory(t *testing.T) {
	dir := t.TempDir()

	// Name doesn't match directory — frontmatter name takes precedence
	skillDir := filepath.Join(dir, "dir-name")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: different-name
description: name mismatch
---

Content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	// frontmatter name is the source of truth, directory name is just physical organization
	if len(discovered) != 1 {
		t.Fatalf("expected 1 skill (frontmatter name takes precedence over dir name), got %d", len(discovered))
	}
	if discovered[0].Metadata.Name != "different-name" {
		t.Errorf("expected name 'different-name', got %s", discovered[0].Metadata.Name)
	}
}

func TestFinder_NestedDiscovery(t *testing.T) {
	root := t.TempDir()

	// Create nested .claude/skills/ directory
	nestedSkillsDir := filepath.Join(root, "subproject", ".claude", "skills", "nested-skill")
	os.MkdirAll(nestedSkillsDir, 0o755)
	os.WriteFile(filepath.Join(nestedSkillsDir, "SKILL.md"), []byte(`---
name: nested-skill
description: A nested skill
---

Nested content.
`), 0o644)

	// Create top-level skills
	topSkillsDir := filepath.Join(root, "top-skills", "top-skill")
	os.MkdirAll(topSkillsDir, 0o755)
	os.WriteFile(filepath.Join(topSkillsDir, "SKILL.md"), []byte(`---
name: top-skill
description: A top skill
---

Top content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger,
		[]string{filepath.Join(root, "top-skills")},
		WithNestedDiscovery(root),
	)

	err := finder.DiscoverAndRegister()
	if err != nil {
		t.Fatalf("DiscoverAndRegister returned error: %v", err)
	}

	// Should find both top-level and nested skills
	if reg.Count() != 2 {
		t.Fatalf("expected 2 registered skills, got %d", reg.Count())
	}
	if _, err := reg.Get("top-skill"); err != nil {
		t.Errorf("expected top-skill to be registered: %v", err)
	}
	if _, err := reg.Get("nested-skill"); err != nil {
		t.Errorf("expected nested-skill to be registered: %v", err)
	}
}

func TestFinder_NestedDiscoverySkipsGitAndNodeModules(t *testing.T) {
	root := t.TempDir()

	// Create skill inside node_modules (should be skipped)
	nmSkillDir := filepath.Join(root, "node_modules", ".claude", "skills", "nm-skill")
	os.MkdirAll(nmSkillDir, 0o755)
	os.WriteFile(filepath.Join(nmSkillDir, "SKILL.md"), []byte(`---
name: nm-skill
description: Should be skipped
---

Content.
`), 0o644)

	// Create skill inside .git (should be skipped)
	gitSkillDir := filepath.Join(root, ".git", ".claude", "skills", "git-skill")
	os.MkdirAll(gitSkillDir, 0o755)
	os.WriteFile(filepath.Join(gitSkillDir, "SKILL.md"), []byte(`---
name: git-skill
description: Should be skipped
---

Content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{}, WithNestedDiscovery(root))

	err := finder.DiscoverAndRegister()
	if err != nil {
		t.Fatalf("DiscoverAndRegister returned error: %v", err)
	}

	if reg.Count() != 0 {
		t.Errorf("expected 0 skills (node_modules and .git should be skipped), got %d", reg.Count())
	}
}

func TestFinder_DiscoverWithStandardFields(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "std-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: std-skill
description: Skill with standard fields
license: MIT
compatibility: ">=1.0.0"
metadata:
  author: test
  version: "1.0"
model: claude-sonnet
context: fork
agent: research
---

Standard skill content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(discovered))
	}

	m := discovered[0].Metadata
	if m.License != "MIT" {
		t.Errorf("expected license MIT, got %s", m.License)
	}
	if m.Compatibility != ">=1.0.0" {
		t.Errorf("expected compatibility >=1.0.0, got %s", m.Compatibility)
	}
	if m.ExtraMetadata["author"] != "test" {
		t.Errorf("expected metadata author=test, got %v", m.ExtraMetadata)
	}
	if m.Model != "claude-sonnet" {
		t.Errorf("expected model claude-sonnet, got %s", m.Model)
	}
	if m.Context != "fork" {
		t.Errorf("expected context fork, got %s", m.Context)
	}
	if m.Agent != "research" {
		t.Errorf("expected agent research, got %s", m.Agent)
	}
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantBody string
		wantErr  bool
	}{
		{
			name:     "valid frontmatter",
			input:    "---\nname: test\ndescription: A test\n---\n\nBody content.",
			wantName: "test",
			wantBody: "Body content.",
		},
		{
			name:     "no frontmatter",
			input:    "Just plain markdown.",
			wantName: "",
			wantBody: "Just plain markdown.",
		},
		{
			name:    "unclosed frontmatter",
			input:   "---\nname: test\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := parseFrontmatter(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if meta.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, meta.Name)
			}
			if body != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, body)
			}
		})
	}
}

func TestFinder_ProgressiveDisclosure(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "prog-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: prog-skill
description: Progressive disclosure test
---

Full content here with $ARGUMENTS.
`), 0o644)

	// Add bundled files
	os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755)
	os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh"), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	err := finder.DiscoverAndRegister()
	if err != nil {
		t.Fatalf("DiscoverAndRegister error: %v", err)
	}

	// Step 1: After Discover, should be Level 1
	skill, err := reg.Get("prog-skill")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if skill.Loaded != LevelMetadataOnly {
		t.Errorf("expected LevelMetadataOnly after Discover, got %d", skill.Loaded)
	}
	if skill.Content != "" {
		t.Errorf("expected empty content at Level 1, got %q", skill.Content)
	}

	// Step 2: After Invoke, should be Level 2
	result, err := reg.Invoke("prog-skill", RenderContext{Arguments: "test-arg"})
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	if result != "Full content here with test-arg." {
		t.Errorf("unexpected render: %q", result)
	}
	if skill.Loaded != LevelFullContent {
		t.Errorf("expected LevelFullContent after Invoke, got %d", skill.Loaded)
	}

	// Step 3: After LoadBundledFiles, should be Level 3
	err = skill.LoadBundledFiles()
	if err != nil {
		t.Fatalf("LoadBundledFiles error: %v", err)
	}
	if skill.Loaded != LevelBundledFiles {
		t.Errorf("expected LevelBundledFiles, got %d", skill.Loaded)
	}
	if len(skill.Bundled.Scripts) != 1 {
		t.Errorf("expected 1 script, got %d", len(skill.Bundled.Scripts))
	}
}

func TestFinder_DiscoverDescriptionTooLong(t *testing.T) {
	dir := t.TempDir()

	// Create a skill with description > 1024 characters
	longDesc := strings.Repeat("a", 1025)
	skillDir := filepath.Join(dir, "long-desc")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(fmt.Sprintf(`---
name: long-desc
description: %s
---

Content.
`, longDesc)), 0o644)

	// Create a valid skill alongside
	validDir := filepath.Join(dir, "valid-skill")
	os.MkdirAll(validDir, 0o755)
	os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(`---
name: valid-skill
description: Short description
---

Content.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	// Only valid-skill should be discovered
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered skill (long description should be skipped), got %d", len(discovered))
	}
	if discovered[0].Metadata.Name != "valid-skill" {
		t.Errorf("expected valid-skill, got %s", discovered[0].Metadata.Name)
	}
}

func TestFinder_DiscoverEmptyDescription(t *testing.T) {
	dir := t.TempDir()

	skillDir := filepath.Join(dir, "no-desc")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: no-desc
---

Content without description.
`), 0o644)

	logger, _ := zap.NewDevelopment()
	reg := NewRegistry(logger)
	finder := NewFinder(reg, logger, []string{dir})

	discovered, err := finder.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered skill (empty description is valid), got %d", len(discovered))
	}
	if discovered[0].Metadata.Name != "no-desc" {
		t.Errorf("expected no-desc, got %s", discovered[0].Metadata.Name)
	}
}
