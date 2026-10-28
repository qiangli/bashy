package agentos

import (
	browsercmd "github.com/qiangli/yoke/cmds/browser"
	"github.com/qiangli/yoke/pkg/browser/guardask"
)

// `bashy browser skills` and the extract hint resolve site skills (Sprint
// 512) through the same catalog `bashy skill` uses, so a skill that `skill
// show` finds is the skill the browser names. Called lazily per lookup.
func init() {
	browsercmd.SkillOptions = skillsOptions
	// Sensitive-input guard (Sprint 512): the browser guard asks the human
	// through the bashy ask form and fills answers by reference.
	guardask.Install()
}
