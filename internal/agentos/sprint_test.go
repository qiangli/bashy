package agentos

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bashy/skills"
	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// TestSprintHelpCarriesOwnerAccountability asserts the essential help contract:
// `bashy sprint --help` must summarize the ACTIVE owner-accountability of a
// sprint's appointed owner/conductor, not only the plan/handoff mechanics. This
// keeps the requirement durable — a human should not have to restate it — by
// pinning it to the command's own rendered help.
func TestSprintHelpCarriesOwnerAccountability(t *testing.T) {
	cmd := newSprintCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sprint --help: %v", err)
	}
	help := out.String()

	// The short line must already frame ownership as active accountability.
	if !strings.Contains(cmd.Short, "accountability") {
		t.Errorf("sprint Short does not mention owner accountability: %q", cmd.Short)
	}

	// The rendered help must state the non-passive contract and each of the
	// essential owner obligations, so an agent reading `--help` sees them.
	essentials := []string{
		"not a passive lease holder",
		"VERIFIED",        // accountable through verified completion
		"REACHABLE",       // reachable identity + inbox/host monitoring
		"inbox",           // monitor inbox
		"SUPERVISE",       // proactive worker supervision
		"reassign",        // steer / interrupt / salvage / reassign
		"salvage",         //
		"rerun the gates", // independent gate inspection/rerun
		"exit status",     // never trust worker prose or exit status
		"sequentially",    // serialize reviews and merges
		"pins",            // push subproject commits + parent pins
		"PRESERVE",        // preserve unrelated work
		"clean",           // cleanup of integrated workspaces/branches/temp
		"CONTINUITY",      // checkpoint continuity
		"safety policy",   // cleanup/deletion authority bounded by assignment + safety
		"Do not wait passively while actionable work remains",
	}
	for _, want := range essentials {
		if !strings.Contains(help, want) {
			t.Errorf("sprint --help missing owner-accountability element %q", want)
		}
	}
}

func TestExternalSprintTakeWatchClaimsThenStreamsInbox(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_SPRINT_DIR", t.TempDir())
	const owner = "external-sprint-manager"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: owner, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEAVE_CONDUCTOR", owner)

	add := newSprintCmd()
	add.SetOut(&bytes.Buffer{})
	add.SetErr(&bytes.Buffer{})
	add.SetArgs([]string{"add", "external watch test"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: owner, Body: "wake the external manager"}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cmd := newSprintCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"take", "1", "--as", owner, "--watch"})
	err := cmd.Execute()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("take --watch: %v\n%s", err, out.String())
	}
	for _, want := range []string{"is now conductor", "READY: attached inbox delivery", "attached inbox stream", "wake the external manager"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("take --watch output missing %q:\n%s", want, out.String())
		}
	}
}

// TestConductorSkillCarriesOwnerChecklist keeps the same contract durable in the
// conductor skill (embedded into the bashy binary), so `bashy skills show
// conductor` presents the concise active-owner checklist and the reference
// companion expands it. Reads through the coreutils skills loader used by
// `bashy skills`, exercising the same path an agent would.
func TestConductorSkillCarriesOwnerChecklist(t *testing.T) {
	body, ok := skills.Body("conductor")
	if !ok {
		t.Fatal("conductor SKILL.md not embedded")
	}
	for _, want := range []string{
		"Owner accountability",
		"not a passive lease holder",
		"reachable",
		"Supervise",
		"reassign",
		"rerun",
		"sequentially",
		"pins",
		"Preserve",
		"continuity",
		"safety policy",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("conductor SKILL.md missing owner-checklist element %q", want)
		}
	}

	ref, ok := skills.Reference("conductor")
	if !ok {
		t.Fatal("conductor reference.md not embedded")
	}
	for _, want := range []string{
		"Owner accountability",
		"passive lease holder",
		"safety policy",
	} {
		if !strings.Contains(ref, want) {
			t.Errorf("conductor reference.md missing owner-accountability element %q", want)
		}
	}
}

// THE ATTACH BANNER WAS TRUE AND INSUFFICIENT.
//
// "retain and read this process until handoff" does not tell an agent that an
// explicit ack is owed, and an agent that does not ack loses the seat to the
// fail-closed fuse with nothing visible until the process exits nonzero. That
// happened twice in one session on this project's own conductor seat, so the
// loop is now printed at the moment it becomes the agent's responsibility.
func TestSprintWatchNextStepsNamesTheLoopAndItsConsequences(t *testing.T) {
	var buf bytes.Buffer
	writeSprintWatchNextSteps(&buf, 99, "trestle")
	got := buf.String()

	// The exact commands, already substituted: an agent must not have to
	// assemble one from a template while a lease is ticking.
	for _, want := range []string{
		"bashy sprint inbox-ack 99 --as trestle",
		"bashy sprint take 99 --as trestle --watch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("next steps omit the runnable command %q:\n%s", want, got)
		}
	}
	// Each step carries its CONSEQUENCE. An instruction with no stated cost is
	// one an agent deprioritises under load, which is how the contract already
	// in `sprint --help` came to be unread.
	for _, want := range []string{"EXITS NONZERO", "UNREACHABLE"} {
		if !strings.Contains(got, want) {
			t.Errorf("next steps omit the consequence %q:\n%s", want, got)
		}
	}
	// The stream IS the inbox. A second reader would race it for the same
	// cursors, and "start a watch" is the obvious wrong inference from the
	// instruction to keep reading.
	if !strings.Contains(got, "second") || !strings.Contains(got, "race") {
		t.Errorf("next steps do not warn against a second inbox reader:\n%s", got)
	}
	// Compact enough to be read rather than skimmed past.
	if n := strings.Count(got, "\n"); n > 9 {
		t.Errorf("next steps are %d lines; keep them short enough to act on:\n%s", n, got)
	}
}

// The block must go to STDERR: stdout carries the NDJSON delivery stream, and a
// reader parsing it must not have to skip prose.
func TestSprintWatchNextStepsStayOffTheDeliveryStream(t *testing.T) {
	cmd := newSprintCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	writeSprintWatchNextSteps(cmd.ErrOrStderr(), 99, "trestle")
	if out.Len() != 0 {
		t.Fatalf("next steps polluted the NDJSON stream: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "NEXT STEPS") {
		t.Fatalf("next steps did not reach stderr: %q", errOut.String())
	}
}
