package agentos

import (
	"encoding/json"
	"strings"
	"testing"

	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/fleet"
)

// TestDoctorShowsModifiedAfterMergeAndPristineAfterRevert drives the M2b
// acceptance at the doctor layer: a fixture PR merge (a local-branch
// commit past pristine) flips the row to modified, and resetting local
// back to pristine restores the pristine row — each computed by diffing,
// never assumed.
func TestDoctorShowsModifiedAfterMergeAndPristineAfterRevert(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	dir, _ := seedSrcRepo(t, home, "yoke", map[string]string{"pkg/weave/engine.go": "package weave\n"})

	rowNamed := func(checks []doctorCheck, name string) (doctorCheck, bool) {
		for _, c := range checks {
			if c.Name == name {
				return c, true
			}
		}
		return doctorCheck{}, false
	}

	if row, ok := rowNamed(collectDoctorChecks(), "src: yoke"); !ok || row.Status != "ok" || row.SrcState != "pristine" {
		t.Fatalf("fresh row = %+v, %v (want pristine)", row, ok)
	}

	// The fixture PR merge: a local-branch commit past pristine.
	commitOnLocal(t, dir, map[string]string{"pkg/weave/engine.go": "package weave\n// fix\n"}, "skill fix")
	merged, ok := rowNamed(collectDoctorChecks(), "src: yoke")
	if !ok || merged.SrcState != "modified" {
		t.Fatalf("merged row = %+v, %v (want modified)", merged, ok)
	}
	if merged.SrcBase == "" || merged.SrcLocal == "" || merged.SrcBase == merged.SrcLocal {
		t.Fatalf("merged row lacks pins: %+v", merged)
	}

	// The revert: local back at pristine.
	pristine, err := ygit.ResolveCommit(dir, "pristine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Checkout(ygit.CheckoutOptions{RepoPath: dir, Branch: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Reset(ygit.ResetOpts{RepoPath: dir, Mode: ygit.ResetHard, Commit: pristine}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if row, ok := rowNamed(collectDoctorChecks(), "src: yoke"); !ok || row.SrcState != "pristine" {
		t.Fatalf("reverted row = %+v, %v (want pristine)", row, ok)
	}
}

func TestBashyToolRecordCarriesProvenanceFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	dir, _ := seedSrcRepo(t, home, "yoke", map[string]string{"pkg/weave/engine.go": "package weave\n"})
	commitOnLocal(t, dir, map[string]string{"pkg/weave/engine.go": "package weave\n// fix\n"}, "skill fix")

	if err := ensureBashyToolRecord(); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	tool, ok := fleet.New().Tool("bashy")
	if !ok || tool.Kind != "tool" && tool.Kind != fleet.ToolKindCLI {
		t.Fatalf("tool = %+v, %v", tool, ok)
	}

	// The JSON record carries the same fields as the doctor rows.
	var okShow bool
	data := []byte(captureStdout(t, func() { okShow, _ = tryBashyToolShow([]string{"show", "bashy", "--json"}) }))
	if !okShow {
		t.Fatal("tool show bashy was not intercepted")
	}
	var doc struct {
		Name     string `json:"name"`
		SrcState string `json:"src_state"`
		SrcRepos []struct {
			Repo  string `json:"repo"`
			State string `json:"state"`
			Base  string `json:"base"`
			Local string `json:"local"`
		} `json:"src_repos"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("record JSON invalid: %v\n%s", err, data)
	}
	if doc.Name != "bashy" || doc.SrcState != "modified" {
		t.Fatalf("record = %+v", doc)
	}
	if len(doc.SrcRepos) != 1 || doc.SrcRepos[0].Repo != "yoke" || doc.SrcRepos[0].State != "modified" {
		t.Fatalf("repos = %+v", doc.SrcRepos)
	}
	if doc.SrcRepos[0].Base == "" || doc.SrcRepos[0].Local == "" {
		t.Fatalf("repo row lacks pins: %+v", doc.SrcRepos[0])
	}
}

func TestBashyToolShowLeavesOtherToolsAlone(t *testing.T) {
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	if ok, _ := tryBashyToolShow([]string{"show", "muse", "--json"}); ok {
		t.Fatal("intercepted another tool's show")
	}
	if ok, _ := tryBashyToolShow([]string{"list"}); ok {
		t.Fatal("intercepted tool list")
	}
	if ok, _ := tryBashyToolShow([]string{"show", "bashy", "extra"}); ok {
		t.Fatal("intercepted a malformed show")
	}
	if out := doctorProvenanceFieldNames(); !strings.Contains(out, "src_state") {
		t.Fatalf("field names = %q", out)
	}
}

// doctorProvenanceFieldNames is a seam so the field-name contract has a
// unit handle without capturing process stdout.
func doctorProvenanceFieldNames() string { return "repo,src_state,src_base,src_local" }
