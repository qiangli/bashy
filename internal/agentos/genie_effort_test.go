package agentos

// Sprint: #412; Story: #896; Story-ID: 1ce7b1d56527

import (
	"os"
	"strings"
	"testing"
)

func TestGenieEffortFlag(t *testing.T) {
	for _, c := range []struct {
		args   []string
		effort string
		rest   []string
	}{
		{nil, "", nil},
		{[]string{"--effort", "high", "fix", "it"}, "high", []string{"fix", "it"}},
		{[]string{"--effort=XHigh", "task"}, "xhigh", []string{"task"}},
		{[]string{"-m", "door-claude-opus5", "--effort", "low", "task"}, "low", []string{"-m", "door-claude-opus5", "task"}},
		{[]string{"--effort", "none", "web"}, "none", []string{"web"}},
		{[]string{"explain", "--effort", "high"}, "", []string{"explain", "--effort", "high"}},
		{[]string{"--", "--effort", "high"}, "", []string{"--", "--effort", "high"}},
		{[]string{"-m", "x", "--", "--effort=low"}, "", []string{"-m", "x", "--", "--effort=low"}},
	} {
		effort, rest, err := genieEffortFlag(c.args)
		if err != nil || effort != c.effort || strings.Join(rest, "|") != strings.Join(c.rest, "|") {
			t.Errorf("%q: effort %q rest %q err %v; want %q %q", c.args, effort, rest, err, c.effort, c.rest)
		}
	}
}

func TestGenieEffortFlagRejects(t *testing.T) {
	for _, args := range [][]string{
		{"--effort"},
		{"--effort", ""},
		{"--effort", "turbo", "task"},
		{"--effort=", "task"},
		{"--effort", "high;rm", "task"},
	} {
		if _, _, err := genieEffortFlag(args); err == nil {
			t.Errorf("%q: want an error", args)
		}
	}
}

func TestGenieApplyEffortExportsEnv(t *testing.T) {
	t.Setenv("GENIE_EFFORT", "")
	os.Unsetenv("GENIE_EFFORT")

	rest, err := genieApplyEffort([]string{"--effort", "medium", "-m", "x", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("GENIE_EFFORT"); got != "medium" {
		t.Fatalf("GENIE_EFFORT = %q, want medium", got)
	}
	if strings.Join(rest, "|") != "-m|x|hi" {
		t.Fatalf("rest = %q", rest)
	}
}

func TestGenieApplyEffortKeepsLaunchEnvWhenNoFlag(t *testing.T) {
	t.Setenv("GENIE_EFFORT", "high")
	rest, err := genieApplyEffort([]string{"-m", "x", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("GENIE_EFFORT"); got != "high" {
		t.Fatalf("GENIE_EFFORT = %q, want the launch-supplied high", got)
	}
	if strings.Join(rest, "|") != "-m|x|hi" {
		t.Fatalf("rest = %q", rest)
	}
}

func TestGenieApplyEffortFlagBeatsEnv(t *testing.T) {
	t.Setenv("GENIE_EFFORT", "high")
	if _, err := genieApplyEffort([]string{"--effort", "low", "hi"}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("GENIE_EFFORT"); got != "low" {
		t.Fatalf("GENIE_EFFORT = %q, want the flag's low", got)
	}
}

func TestGenieApplyEffortRejectsBadEnv(t *testing.T) {
	t.Setenv("GENIE_EFFORT", "ludicrous")
	if _, err := genieApplyEffort([]string{"hi"}); err == nil {
		t.Fatal("an invalid GENIE_EFFORT must be refused, not forwarded to the model request")
	}
}

func TestGenieApplyEffortEmptyEnvIsUnset(t *testing.T) {
	t.Setenv("GENIE_EFFORT", "  ")
	if _, err := genieApplyEffort([]string{"hi"}); err != nil {
		t.Fatalf("blank GENIE_EFFORT means undeclared: %v", err)
	}
	if got, set := os.LookupEnv("GENIE_EFFORT"); set {
		t.Fatalf("blank GENIE_EFFORT must be unset, got %q", got)
	}
}

func TestDispatchGenieRefusesBadEffort(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("GENIE_BAR", "")
	t.Setenv("GENIE_EFFORT", "")
	for _, args := range [][]string{
		{"--effort", "turbo", "hi"},
		{"--effort"},
		{"-m", "x", "--effort=nope", "hi"},
	} {
		if code := dispatchGenie(args); code != 2 {
			t.Errorf("%q: exit %d, want 2", args, code)
		}
	}
}
