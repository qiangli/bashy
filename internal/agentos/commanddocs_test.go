// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/bashy/skills"
	"github.com/qiangli/yoke/pkg/fleet"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

// Sprint 406: conductor, steward, supervisor, inbox, sprint,
// knowledge-transfer and force-agent-shell lost their own top-level skill
// folder — their bodies moved into skills/bashy/commands/*.md. commandDocSource
// bridges the names back into the skill catalog so `bashy skill show/run NAME`
// and the bare `bashy NAME` fallback keep working.

func TestCommandDocSourceNames(t *testing.T) {
	// Sorted, exact: a new commands/<name>.md (feed shipped with the S431 feed
	// registry, Sprint 9999 maintenance) must be added here deliberately.
	want := []string{"adapt", "common-options", "conductor", "feed", "feed-setup", "force-agent-shell", "inbox", "knowledge-transfer", "lens", "sprint", "steward", "supervisor", "transcribe"}
	got, err := commandDocSource{}.Names()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestCommandDocSourceBodyAndFiles(t *testing.T) {
	src := commandDocSource{}

	body, ok := src.Body("conductor")
	if !ok || !strings.HasPrefix(string(body), "---\nname: conductor\n") {
		t.Fatalf("Body(conductor) = %q, ok=%v", body, ok)
	}

	files, err := src.Files("conductor")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"SKILL.md", "reference.md", "skill.dhnt"}) {
		t.Fatalf("Files(conductor) = %v", files)
	}
	for _, f := range files {
		if _, ok := src.File("conductor", f); !ok {
			t.Errorf("File(conductor, %q) absent", f)
		}
	}

	// sprint has no reference.md or skill.dhnt — Files must not claim them.
	files, err = src.Files("sprint")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"SKILL.md"}) {
		t.Fatalf("Files(sprint) = %v, want [SKILL.md]", files)
	}

	// feed shipped its command doc without a reference.md or skill.dhnt: the
	// bridge must surface its body and claim only SKILL.md.
	body, ok = src.Body("feed")
	if !ok || !strings.HasPrefix(string(body), "---\nname: feed\n") {
		t.Fatalf("Body(feed) = %q, ok=%v", body, ok)
	}
	files, err = src.Files("feed")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"SKILL.md"}) {
		t.Fatalf("Files(feed) = %v, want [SKILL.md]", files)
	}
	if _, ok := src.File("feed", "SKILL.md"); !ok {
		t.Error(`File(feed, "SKILL.md") absent`)
	}

	if _, ok := src.Body("no-such-command"); ok {
		t.Fatal("Body(no-such-command) should be absent")
	}
}

// The bridge must resolve through the SAME catalog the "skill" front door
// uses (skillsOptions), not just through the Source in isolation — that is
// what makes `bashy skill show conductor` and `bashy skill run
// force-agent-shell` keep working after their folder folded into bashy's.
func TestCommandDocsResolveThroughSkillsCatalog(t *testing.T) {
	cat := coreskills.Catalog{Sources: skillsSourcesForTest()}
	for _, name := range []string{"conductor", "steward", "supervisor", "inbox", "sprint", "knowledge-transfer", "force-agent-shell", "feed"} {
		sk, _, ok := cat.Get(name)
		if !ok {
			t.Errorf("%s: not found through the skill catalog", name)
			continue
		}
		if sk.Name != name {
			t.Errorf("%s: catalog entry name = %q", name, sk.Name)
		}
	}
	if sk, _, ok := cat.Get("force-agent-shell"); !ok || !sk.HasDhnt {
		t.Errorf("force-agent-shell: HasDhnt = %v, ok=%v — `skill run` needs its skill.dhnt", sk.HasDhnt, ok)
	}
}

// skillsSourcesForTest mirrors skillsOptions' embedded-ring wiring without
// the host-local/shared store plumbing a unit test has no business touching.
func skillsSourcesForTest() []coreskills.Source {
	return []coreskills.Source{
		coreskills.EmbedSource(skills.FS, coreskills.RingEmbedded),
		commandDocSource{},
	}
}

func TestIsSkillDocName(t *testing.T) {
	for _, name := range []string{"bashy", "conductor", "steward", "supervisor", "inbox", "sprint", "knowledge-transfer", "force-agent-shell", "feed"} {
		if !isSkillDocName(name) {
			t.Errorf("isSkillDocName(%q) = false, want true", name)
		}
	}
	if isSkillDocName("not-a-real-command") {
		t.Error(`isSkillDocName("not-a-real-command") = true, want false`)
	}
}

// lookupEntry is what dispatchFormattedHelp uses instead of a bare
// atlas.Lookup, so a pseudo-command with only a local bashyOwnedVerbAtlas row
// (no shared-atlas entry of its own) still renders `--help --format skill`
// instead of refusing with "no atlas entry".
func TestLookupEntryFallsBackToLocalTable(t *testing.T) {
	for _, name := range []string{"supervisor", "knowledge-transfer", "force-agent-shell"} {
		e, ok := lookupEntry(name)
		if !ok {
			t.Fatalf("lookupEntry(%q) not found", name)
		}
		if e.Group == "" || e.Tier == "" {
			t.Errorf("lookupEntry(%q) = %+v, missing Group/Tier", name, e)
		}
	}
	// sprint has a real shared-atlas entry; lookupEntry must still resolve it
	// (the local table is a fallback, never a shadow, on a shared-atlas hit).
	if _, ok := lookupEntry("sprint"); !ok {
		t.Fatal(`lookupEntry("sprint") not found`)
	}
}

func TestLookupEntryRegisteredCommandMCP(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{
		Name: "registered-help-probe", Synopsis: "registered help probe",
		Script: "echo ok", Effects: []string{"pure"},
	})
	entry, ok := lookupEntry("registered-help-probe")
	if !ok || entry.Group == "" || entry.Tier == "" {
		t.Fatalf("registered atlas entry = %+v, ok=%v", entry, ok)
	}
	definition, err := detailedHelpMCPTool("registered-help-probe", entry)
	if err != nil || definition == nil || definition.Name != "registered-help-probe" {
		t.Fatalf("registered MCP help = %+v, err=%v", definition, err)
	}
}

func TestCommandLongDocStripsFrontmatter(t *testing.T) {
	got := commandLongDoc("sprint")
	if got == "" {
		t.Fatal("commandLongDoc(sprint) is empty")
	}
	if strings.HasPrefix(got, "---") {
		t.Fatalf("commandLongDoc(sprint) still carries frontmatter: %q", firstLine(got))
	}
	if !strings.Contains(got, "Translate the user's request into Bashy sprint commands") {
		t.Fatalf("commandLongDoc(sprint) missing its own prose: %q", got)
	}
	if got := commandLongDoc("no-such-command"); got != "" {
		t.Fatalf("commandLongDoc(no-such-command) = %q, want empty", got)
	}
}
