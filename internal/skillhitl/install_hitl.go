// Package skillhitl registers HITL choice_types owned by the skill subsystem.
//
// This package exists to solve an import-cycle constraint: the choice_type for
// skill_install HITL (`skill_install_confirmation`) cannot live in:
//   - internal/master — spec `hitl-choice-type-registry` forbids listing
//     downstream choice_types in Master's init() built-in set.
//   - internal/skills — master transitively imports skills
//     (master → a2abridge → subagent → skills), so skills → master creates a
//     cycle.
//   - internal/tools — master transitively imports tools, same cycle problem.
//
// This is a leaf package: it imports master only, and nothing imports it except
// the binary entry points (bootstrap / cmd) via blank-import. At process start,
// blank-import triggers init(), which registers the choice_type before any
// handler can emit. See tasks 6.0 in openspec/changes/hive-skill-on-demand.
//
// When the full skill_install handler lands (tasks 6.1–6.6), it references the
// exported `ChoiceTypeSkillInstallConfirmation` constant from here rather than
// duplicating the string literal.
package skillhitl

import (
	"github.com/chef-guo/agents-hive/internal/master"
)

// ChoiceTypeSkillInstallConfirmation is the HITL choice_type name that
// skill_install emits to ask the user for approval before downloading +
// registering a new skill. External callers must reference this constant to
// keep spec drift detectable.
const ChoiceTypeSkillInstallConfirmation = "skill_install_confirmation"

func init() {
	master.MustRegisterChoiceType(master.ChoiceTypeSpec{
		Name:        ChoiceTypeSkillInstallConfirmation,
		Description: "User approves or declines an on-demand skill installation",
		PayloadHint: map[string]string{
			"name":           "skill name being installed",
			"scope":          "personal|public",
			"source":         "marketplace URL",
			"admin_required": "bool — whether scope=public required admin",
		},
	})
}
