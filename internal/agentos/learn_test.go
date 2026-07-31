// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"os/exec"
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
