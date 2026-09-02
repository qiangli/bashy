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
