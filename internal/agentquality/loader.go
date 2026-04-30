package agentquality

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LoadedCase struct {
	Path string
	Case Case
}

func LoadCases(dir string) ([]LoadedCase, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []LoadedCase
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "sample_gate_summary.json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var c Case
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, LoadedCase{Path: path, Case: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func ValidateCase(c Case) error {
	if c.ID == "" {
		return fmt.Errorf("id missing")
	}
	if c.Name == "" {
		return fmt.Errorf("%s: name missing", c.ID)
	}
	if c.Route == "" {
		return fmt.Errorf("%s: route missing", c.ID)
	}
	if c.Input == "" {
		return fmt.Errorf("%s: input missing", c.ID)
	}
	switch c.ExpectedStatus {
	case StatusPass, StatusFail, StatusBlocked, StatusNeedsUser:
	default:
		return fmt.Errorf("%s: invalid expected_status %q", c.ID, c.ExpectedStatus)
	}
	if len(c.ExpectedTools) > 0 && len(c.AllowedTools) > 0 {
		return fmt.Errorf("%s: expected_tools and allowed_tools are mutually exclusive", c.ID)
	}
	if c.Risk != "" {
		switch c.Risk {
		case "safe", "dangerous":
		default:
			return fmt.Errorf("%s: invalid risk %q", c.ID, c.Risk)
		}
	}
	if c.Risk == "safe" && c.ExpectedStatus == StatusNeedsUser {
		return fmt.Errorf("%s: safe case must not require user approval", c.ID)
	}
	return nil
}
