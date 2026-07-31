// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"mvdan.cc/sh/v3/interp"
)

// THE BUG THIS PINS. bashy shims its front-door verbs as shell functions, so
// `kubectl …` reaches the ExecHandler as `/path/to/bashy kubectl …`. Every
// managed CLI was therefore invisible to the learning hook — and invisibly so:
// they never taught anything and never offered anything, which is exactly what
// having no facts looks like.
func TestCommandArgv_StripsTheFrontDoor(t *testing.T) {
	self := bashySelfPath()
	cases := []struct {
		in, want []string
	}{
		{[]string{self, "kubectl", "--context", "prod", "get", "pods"},
			[]string{"kubectl", "--context", "prod", "get", "pods"}},
		// The docker shim REPLACES the command rather than re-spelling it, so
		// podman is the name that actually arrives.
		{[]string{self, "podman", "-H", "tcp://build-host:2375", "ps"},
			[]string{"podman", "-H", "tcp://build-host:2375", "ps"}},
		// A real binary is untouched.
		{[]string{"ssh", "-p", "2222", "remote-host"},
			[]string{"ssh", "-p", "2222", "remote-host"}},
		// bashy operating on ITSELF is not a front-door verb; a flag is the tell.
		{[]string{self, "-c", "echo hi"}, []string{self, "-c", "echo hi"}},
		{[]string{self}, []string{self}},
		{nil, nil},
	}
	for _, c := range cases {
		if got := commandArgv(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("commandArgv(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// "The command ran and failed" and "the command never ran" are different
// claims, and only the first is evidence. The narrow interp.ExitStatus check
// classified every self-dispatched command as the second.
func TestObservedExitOf(t *testing.T) {
	if st, ok := observedExitOf(nil); st != 0 || !ok {
		t.Errorf("nil = (%d,%v), want (0,true)", st, ok)
	}
	if st, ok := observedExitOf(interp.ExitStatus(3)); st != 3 || !ok {
		t.Errorf("ExitStatus(3) = (%d,%v)", st, ok)
	}
	// Wrapped, which is how it arrives through several middleware layers.
	if st, ok := observedExitOf(fmt.Errorf("running: %w", interp.ExitStatus(1))); st != 1 || !ok {
		t.Errorf("wrapped ExitStatus = (%d,%v)", st, ok)
	}
	// A command bashy dispatched itself returns an *exec.ExitError.
	ee := exec.Command("sh", "-c", "exit 7").Run()
	var target *exec.ExitError
	if !errors.As(ee, &target) {
		t.Skip("no *exec.ExitError available in this environment")
	}
	if st, ok := observedExitOf(ee); st != 7 || !ok {
		t.Errorf("ExitError = (%d,%v), want (7,true)", st, ok)
	}
	// An unclassifiable error is NOT an observation.
	if _, ok := observedExitOf(errors.New("interrupted")); ok {
		t.Error("an unclassifiable error was treated as an observed exit")
	}
}

// THE INVARIANT THAT MATTERS MORE THAN THE LADDER ITSELF: every consumer of
// the skills store must resolve the SAME directory.
//
// Four call sites used to spell the ladder out separately — the catalog, the
// craft graph, the learning hook, and the repo-hint marker. The moment one
// grows a rung the others lack, the catalog and its own history point at
// different places, and it splits SILENTLY: facts get recorded and then are
// simply not there when read back.
func TestBashySkillsDir_Ladder(t *testing.T) {
	t.Setenv("BASHY_SKILLS_DIR", "")
	t.Setenv("BASHY_HOME", "")

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	want := filepath.Join(home, ".config", "bashy", "skills")
	if got := bashySkillsDir(); got != want {
		t.Errorf("default = %q, want %q — changing it would strand every existing store", got, want)
	}

	// $BASHY_HOME was the missing rung: it moved the audit log and the foreman
	// state but left facts writing to the real home, which is a store that
	// looks isolated and is not.
	t.Setenv("BASHY_HOME", filepath.Join("/tmp", "bashy-home"))
	if got, want := bashySkillsDir(), filepath.Join("/tmp", "bashy-home", "skills"); got != want {
		t.Errorf("BASHY_HOME = %q, want %q", got, want)
	}

	// The specific override stays most precise, matching audit and foreman.
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join("/tmp", "explicit"))
	if got, want := bashySkillsDir(), filepath.Join("/tmp", "explicit"); got != want {
		t.Errorf("BASHY_SKILLS_DIR = %q, want %q", got, want)
	}

	// And the whole point: the hook's writer and the graph's reader agree.
	if craftStoreDir() != bashySkillsDir() {
		t.Errorf("craftStoreDir()=%q diverged from bashySkillsDir()=%q", craftStoreDir(), bashySkillsDir())
	}
}
