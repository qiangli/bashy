package agentos

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestSkillUptakeScoring(t *testing.T) {
	cases := []struct {
		id   string
		cmd  string
		want int
	}{
		{"t6", "lens -t md report.pdf", 2},
		{"t7", "lens -f json -t yaml data.json", 2},
		{"t8", "lens -t listing a.tar", 2},
		{"t9", "bashy adapt --dry-run -- rm *.tmp", 2},
		{"t10", "bashy adapt --json -- git status", 2},
		{"t10", "git status --porcelain", 1},
		{"t10", "python -c 'import subprocess'", 1},
	}
	byID := map[string]SkillUptakeTask{}
	for _, tk := range SkillUptakeTasks {
		byID[tk.ID] = tk
	}
	if len(byID) != 5 {
		t.Fatalf("want t6-t10, got %d tasks", len(byID))
	}
	for _, c := range cases {
		if got := byID[c.id].Score(SkillUptakeRun{TaskID: c.id, Commands: []string{c.cmd}, Success: true}); got != c.want {
			t.Errorf("%s %q = %d want %d", c.id, c.cmd, got, c.want)
		}
	}
	if byID["t6"].Score(SkillUptakeRun{Commands: []string{"lens -t md x.pdf"}}) != 0 {
		t.Error("failed run must score 0")
	}
	total, per := ScoreSkillUptake([]SkillUptakeRun{{TaskID: "t6", Commands: []string{"lens -t md a.pdf"}, Success: true}})
	if total != 2 || per["t7"] != 0 {
		t.Errorf("total=%d per=%v", total, per)
	}
}

// fakeRunner: skill runs use the common option on the first n tasks.
func fakeRunner(skillCommon int, avail map[string]bool) SkillUptakeRunner {
	return func(_ context.Context, m string, with bool, tk SkillUptakeTask) (SkillUptakeRun, error) {
		if !avail[m] {
			return SkillUptakeRun{}, ErrModelUnavailable
		}
		r := SkillUptakeRun{TaskID: tk.ID, Success: true, Commands: []string{"echo improvised"}}
		if with {
			for i, x := range SkillUptakeTasks {
				if x.ID == tk.ID && i < skillCommon {
					r.Commands = []string{sampleCommon[tk.ID]}
				}
			}
		}
		return r, nil
	}
}

var sampleCommon = map[string]string{
	"t6": "lens -t md a.pdf", "t7": "lens -f json -t yaml a.json", "t8": "lens -t listing a.tar",
	"t9": "bashy adapt --dry-run -- rm x", "t10": "bashy adapt --json -- git status",
}

func TestSkillUptakeBaselineAndThresholdGate(t *testing.T) {
	all := map[string]bool{"claude": true, "opencode": true, "other": true}
	models := []string{"claude", "opencode", "other"}

	// 3 common (2) + 2 improvised (1) = 8 vs baseline 5 -> pass.
	res, err := EvaluateSkillUptake(context.Background(), models, fakeRunner(3, all))
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Skill != 8 || res[0].NoSkill != 5 || !res[0].Pass {
		t.Fatalf("%+v", res[0])
	}
	if err := SkillUptakeGate(res); err != nil {
		t.Fatal(err)
	}

	// 0 common: skill == baseline (5) -> fails baseline comparison and <6.
	res, _ = EvaluateSkillUptake(context.Background(), models, fakeRunner(0, all))
	if err := SkillUptakeGate(res); err == nil {
		t.Fatal("no uplift must fail the gate")
	}

	// Non-threshold model below 6 but above baseline is fine; threshold model is not.
	r := SkillUptakeResult{Model: "claude", Available: true, Skill: 5, NoSkill: 2}
	res = []SkillUptakeResult{r, {Model: "opencode", Available: true, Skill: 7, NoSkill: 5, Pass: true}}
	res2, _ := EvaluateSkillUptake(context.Background(), []string{"claude"}, func(_ context.Context, _ string, with bool, tk SkillUptakeTask) (SkillUptakeRun, error) {
		return SkillUptakeRun{TaskID: tk.ID, Success: true, Commands: []string{map[bool]string{true: sampleCommon[tk.ID], false: "x"}[with && tk.ID != "t10" && tk.ID != "t9" && tk.ID != "t8"]}}, nil
	})
	if res2[0].Skill != 7 || !res2[0].Pass {
		t.Fatalf("%+v", res2[0])
	}
	_ = res
}

func TestSkillUptakeUnavailableNeverScoredOrSilentlyPasses(t *testing.T) {
	res, err := EvaluateSkillUptake(context.Background(), []string{"claude", "opencode"},
		fakeRunner(5, map[string]bool{"claude": true}))
	if err != nil {
		t.Fatal(err)
	}
	if res[1].Available || res[1].Skill != 0 {
		t.Fatalf("%+v", res[1])
	}
	err = SkillUptakeGate(res)
	if err == nil || !strings.Contains(err.Error(), "opencode not measured") {
		t.Fatalf("got %v", err)
	}
}

// Live gate: set BASHY_SKILL_UPTAKE_DRIVER to an agent driver; skipped otherwise.
func TestSkillUptakeLive(t *testing.T) {
	driver := os.Getenv("BASHY_SKILL_UPTAKE_DRIVER")
	if driver == "" {
		t.Skip("BASHY_SKILL_UPTAKE_DRIVER not set; live skill-uptake gate not run")
	}
	models := strings.Fields(os.Getenv("BASHY_SKILL_UPTAKE_MODELS"))
	if len(models) == 0 {
		models = []string{"claude", "opencode"}
	}
	res, err := EvaluateSkillUptake(context.Background(), models, CommandSkillUptakeRunner(driver))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		t.Logf("%+v", r)
	}
	if err := SkillUptakeGate(res); err != nil {
		t.Fatal(err)
	}
}
