package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type SkillScope string

const (
	ScopePublic   SkillScope = "public"
	ScopePersonal SkillScope = "personal"
)

func (s SkillScope) String() string {
	if s == "" {
		return string(ScopePublic)
	}
	return string(s)
}

func (s SkillScope) Valid() bool {
	return s == ScopePublic || s == ScopePersonal || s == ""
}

func ParseScope(raw string) (SkillScope, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	switch v {
	case "", "public":
		return ScopePublic, nil
	case "personal":
		return ScopePersonal, nil
	default:
		return "", fmt.Errorf("invalid skill scope %q: must be public|personal", raw)
	}
}

func (s *SkillScope) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("scope: expected string, got %v", value.Kind)
	}
	parsed, err := ParseScope(value.Value)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}
