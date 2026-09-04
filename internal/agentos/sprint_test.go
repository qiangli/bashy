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
	cmd.SetArgs([]string{"take", "1", "--owner", owner, "--watch"})
	err := cmd.Execute()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("take --watch: %v\n%s", err, out.String())
	}
	// The take names the runnable command and the standard procedure, rather
	// than reporting a readiness condition the agent would have to go and
	// arrange. Assert the COMMAND and the SKILL, never the prose around them.
	for _, want := range []string{
		"is now conductor",
		"bashy inbox --as " + owner,
		"bashy skills show inbox",
		"attached inbox stream",
		"wake the external manager",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("take --watch output missing %q:\n%s", want, out.String())
		}
	}
}

func TestExternalSprintStartWatchClaimsActiveSprintThenStreamsInbox(t *testing.T) {
	isolateUnifiedInbox(t)
	t.Setenv("BASHY_SPRINT_DIR", t.TempDir())
	const owner = "external-start-manager"
	if err := fleet.New().SaveAgent(fleet.Agent{Name: owner, Tool: "codex", Model: "gpt5.6-sol"}); err != nil {
		t.Fatal(err)
	}
	if err := bus.PostMessage(bus.Post{From: "human", To: owner, Body: "start-watch delivery"}); err != nil {
		t.Fatal(err)
	}

	add := newSprintCmd()
	add.SetOut(&bytes.Buffer{})
	add.SetErr(&bytes.Buffer{})
	add.SetArgs([]string{"add", "external start watch test"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	cmd := newSprintCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"start", "1", "--owner", owner, "--watch", "--for", "1h"})
	err := cmd.Execute()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("start --watch: %v\n%s", err, out.String())
	}
	for _, want := range []string{"started", "attached inbox stream", "start-watch delivery"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("start --watch output missing %q:\n%s", want, out.String())
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

// The embedded skill is the portable agent-facing adapter used by both the
// vendor-neutral .agents export and Claude's .claude export. Keep the owner
// decision outside Bashy and keep the existing-sprint path ownership-neutral.
func TestSprintSkillRequiresAnExplicitManagerAndReusesActiveOwnership(t *testing.T) {
	body, ok := skills.Body("sprint")
	if !ok {
		t.Fatal("sprint SKILL.md not embedded")
	}
	for _, want := range []string{
		"Never choose a default manager or guess",
		"bashy agents list",
		"ask the user to choose before mutating",
		"sprint start ID --owner NAME --instruction TEXT",
		"sprint instruct ID --instruction TEXT",
		"Do not supply or change an owner",
		"claim that work was dispatched unless Bashy confirms it.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sprint SKILL.md missing adapter contract %q", want)
		}
	}
}

// The attach banner said "retain and read this process" and stopped there,
// which left the agent to discover the rest by failing. It now names the loop,
// with the sprint id and owner substituted so nothing has to be assembled.
func TestSprintWatchNextStepsNameTheLoopAndTheCommands(t *testing.T) {
	var buf bytes.Buffer
	writeSprintWatchNextSteps(&buf, 99, "trestle")
	got := buf.String()

	for _, want := range []string{
		"bashy sprint inbox-ack 99 --as trestle",
		"bashy inbox --as trestle",
		// The procedure lives in the standard skill, not in a second copy here.
		"bashy skills show inbox",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("next steps omit the runnable command %q:\n%s", want, got)
		}
	}
	// It must say that nothing is lost by getting this wrong, because that is
	// true and because a warning implying otherwise is what makes an agent
	// treat a routine step as an emergency.
	if !strings.Contains(got, "never consumed") || !strings.Contains(got, "nothing is lost") {
		t.Errorf("next steps do not say unacked mail is safe:\n%s", got)
	}
	// The one genuine correctness point: two readers draining the same cursors.
	if !strings.Contains(got, "second") || !strings.Contains(got, "cursors") {
		t.Errorf("next steps do not warn against a second concurrent reader:\n%s", got)
	}
	// No penalties, because there are none.
	for _, gone := range []string{"EXITS NONZERO", "UNREACHABLE", "ages out"} {
		if strings.Contains(got, gone) {
			t.Errorf("next steps still threaten %q, which no longer happens:\n%s", gone, got)
		}
	}
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
