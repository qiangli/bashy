package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Skill-uptake bench slice t6-t10 (Sprint 446, S10). A run is scored from the
// commands the agent issued: 2 = used the skill's common option/lens/adapt,
// 1 = completed the task by improvising, 0 = failed.

type SkillUptakeTask struct {
	ID     string
	Prompt string
	// Common matches commands that use the skill's common option/lens/adapt.
	Common *regexp.Regexp
}

var SkillUptakeTasks = []SkillUptakeTask{
	{"t6", "Extract one fact from a PDF as markdown.", regexp.MustCompile(`\blens\b.*(?:-t(?:=|\s+)md|--to(?:=|\s+)md)\b`)},
	{"t7", "Edit a value in a JSON file and emit YAML.", regexp.MustCompile(`\blens\b.*-f(?:=|\s+)json\b.*-t(?:=|\s+)yaml\b`)},
	{"t8", "Find the largest file inside an archive without extracting it.", regexp.MustCompile(`\blens\b.*-t(?:=|\s+)listing\b`)},
	{"t9", "Delete files after previewing what would be deleted.", regexp.MustCompile(`(?:\brm\b.*--dry-run|--dry-run.*\brm\b)`)},
	{"t10", "Report git status as JSON.", regexp.MustCompile(`(?:\bgit\s+status\b.*--json|--json.*\bgit\s+status\b)`)},
}

const (
	SkillUptakeMaxScore = 2 * 5
	SkillUptakeMinPass  = 6 // Claude/opencode threshold out of 10
)

// SkillUptakeRun is the observed behaviour for one task.
type SkillUptakeRun struct {
	TaskID   string   `json:"task"`
	Commands []string `json:"commands"`
	Success  bool     `json:"success"`
}

func (t SkillUptakeTask) Score(r SkillUptakeRun) int {
	if !r.Success {
		return 0
	}
	for _, c := range r.Commands {
		if t.Common.MatchString(c) {
			return 2
		}
	}
	return 1
}

// ScoreSkillUptake totals a model's runs; a missing task scores 0.
func ScoreSkillUptake(runs []SkillUptakeRun) (total int, per map[string]int) {
	per = map[string]int{}
	byID := map[string]SkillUptakeRun{}
	for _, r := range runs {
		byID[r.TaskID] = r
	}
	for _, t := range SkillUptakeTasks {
		s := 0
		if r, ok := byID[t.ID]; ok {
			s = t.Score(r)
		}
		per[t.ID] = s
		total += s
	}
	return total, per
}

type SkillUptakeResult struct {
	Model     string `json:"model"`
	Available bool   `json:"available"`
	Skill     int    `json:"skill"`
	NoSkill   int    `json:"no_skill"`
	Pass      bool   `json:"pass"`
	Reason    string `json:"reason,omitempty"`
}

// SkillUptakeRunner executes one task for a model with or without the skill.
// It returns ErrModelUnavailable when the account/CLI is absent.
type SkillUptakeRunner func(ctx context.Context, model string, withSkill bool, t SkillUptakeTask) (SkillUptakeRun, error)

var ErrModelUnavailable = errors.New("model unavailable")

// SkillUptakeThresholdModels must reach SkillUptakeMinPass and beat baseline.
var SkillUptakeThresholdModels = map[string]bool{"claude": true, "opencode": true}

// EvaluateSkillUptake runs the model x {skill, no-skill} matrix. Unavailable
// models are reported as such and never given a score.
func EvaluateSkillUptake(ctx context.Context, models []string, run SkillUptakeRunner) ([]SkillUptakeResult, error) {
	var out []SkillUptakeResult
	for _, m := range models {
		res := SkillUptakeResult{Model: m, Available: true}
		var sk, ns []SkillUptakeRun
		for _, t := range SkillUptakeTasks {
			for _, with := range []bool{true, false} {
				r, err := run(ctx, m, with, t)
				if errors.Is(err, ErrModelUnavailable) {
					res.Available, res.Reason = false, err.Error()
					break
				}
				if err != nil {
					return nil, fmt.Errorf("%s %s: %w", m, t.ID, err)
				}
				if with {
					sk = append(sk, r)
				} else {
					ns = append(ns, r)
				}
			}
			if !res.Available {
				break
			}
		}
		if res.Available {
			res.Skill, _ = ScoreSkillUptake(sk)
			res.NoSkill, _ = ScoreSkillUptake(ns)
			res.Pass = res.Skill > res.NoSkill
			if SkillUptakeThresholdModels[m] {
				res.Pass = res.Pass && res.Skill >= SkillUptakeMinPass
			}
			if !res.Pass {
				res.Reason = "skill score must beat no-skill baseline (and be >= 6/10 for claude/opencode)"
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// SkillUptakeGate fails if any available model fails, or if a threshold model
// was never measured (so an absent account cannot silently pass the gate).
func SkillUptakeGate(results []SkillUptakeResult) error {
	seen := map[string]bool{}
	for _, r := range results {
		if r.Available {
			seen[r.Model] = true
			if !r.Pass {
				return fmt.Errorf("skill uptake gate: %s skill=%d no-skill=%d: %s", r.Model, r.Skill, r.NoSkill, r.Reason)
			}
		}
	}
	for m := range SkillUptakeThresholdModels {
		if !seen[m] {
			return fmt.Errorf("skill uptake gate: %s not measured (account unavailable)", m)
		}
	}
	return nil
}

// CommandSkillUptakeRunner adapts an external agent driver: the program in
// BASHY_SKILL_UPTAKE_DRIVER is invoked as `driver MODEL skill|noskill TASKID`
// with the prompt on stdin, and must print a SkillUptakeRun JSON object.
// Exit status 3 means the model/account is unavailable.
func CommandSkillUptakeRunner(driver string) SkillUptakeRunner {
	return func(ctx context.Context, model string, withSkill bool, t SkillUptakeTask) (SkillUptakeRun, error) {
		mode := "noskill"
		if withSkill {
			mode = "skill"
		}
		cmd := exec.CommandContext(ctx, driver, model, mode, t.ID)
		cmd.Stdin = strings.NewReader(t.Prompt)
		cmd.Stderr = os.Stderr
		b, err := cmd.Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 3 {
			return SkillUptakeRun{}, ErrModelUnavailable
		}
		if err != nil {
			return SkillUptakeRun{}, err
		}
		var r SkillUptakeRun
		if err := json.Unmarshal(b, &r); err != nil {
			return SkillUptakeRun{}, err
		}
		r.TaskID = t.ID
		return r, nil
	}
}
