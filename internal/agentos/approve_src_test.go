package agentos

import (
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/ask"
	"github.com/qiangli/yoke/pkg/src"
)

// raiseSrcEdit raises a protected-edit approval through the ask store and
// returns its ID. t.Setenv keeps the store and src home isolated.
func raiseSrcEdit(t *testing.T, argv ...string) string {
	t.Helper()
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_ASK_DIR", t.TempDir())
	r, err := ask.Raise(ask.ApprovalSpec{Source: ask.SourceConfirm, Argv: argv})
	if err != nil {
		t.Fatalf("Raise: %v", err)
	}
	return r.ID
}

func decide(t *testing.T, id, verdict string) {
	t.Helper()
	if _, err := ask.Decide(id, ask.Decision{Verdict: verdict}, ask.DecideOpts{}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
}

func TestApproveSrcRefusesWithoutHumanAnswer(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/ask/store.go")
	if _, err := approveSrcRecord("yoke", []string{"pkg/ask/store.go"}, id, ""); err == nil {
		t.Fatal("approved without a recorded human answer (self-approve)")
	} else if !strings.Contains(err.Error(), "cannot self-approve") {
		t.Fatalf("wrong refusal: %v", err)
	}
	if recs, _ := src.ListApprovals("", "yoke"); len(recs) != 0 {
		t.Fatalf("a refusal wrote %d approval record(s)", len(recs))
	}
}

func TestApproveSrcRefusesDeny(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/ask/store.go")
	decide(t, id, ask.VerdictDeny)
	if _, err := approveSrcRecord("yoke", []string{"pkg/ask/store.go"}, id, ""); err == nil {
		t.Fatal("approved on a recorded deny")
	}
}

func TestApproveSrcWritesAfterHumanAllow(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/ask/store.go")
	decide(t, id, ask.VerdictAllow)
	got, err := approveSrcRecord("yoke", []string{"pkg/ask/store.go"}, id, "test")
	if err != nil {
		t.Fatalf("refused a recorded human allow: %v", err)
	}
	recs, err := src.ListApprovals("", "yoke")
	if err != nil || len(recs) != 1 {
		t.Fatalf("approvals = %v, %v", recs, err)
	}
	if recs[0].ID != got || len(recs[0].Paths) != 1 || recs[0].Paths[0] != "pkg/ask/store.go" {
		t.Fatalf("record = %+v", recs[0])
	}
	if recs[0].Approver == "" {
		t.Fatal("record has no approver")
	}
}

func TestApproveSrcRefusesMismatchedEdit(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/ask/store.go")
	decide(t, id, ask.VerdictAllow)
	// An allow for one edit cannot authorize another repo or other paths.
	for _, tc := range []struct {
		repo  string
		paths []string
	}{
		{"yoke", []string{"pkg/policy/x.go"}},
		{"sh", []string{"pkg/ask/store.go"}},
	} {
		if _, err := approveSrcRecord(tc.repo, tc.paths, id, ""); err == nil {
			t.Fatalf("approved mismatched edit %v/%v", tc.repo, tc.paths)
		}
	}
}

func TestApproveSrcRefusesCorePaths(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/src/state.go")
	decide(t, id, ask.VerdictAllow)
	if _, err := approveSrcRecord("yoke", []string{"pkg/src/state.go"}, id, ""); err == nil {
		t.Fatal("approved a core path")
	} else if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("wrong refusal: %v", err)
	}
}

func TestApproveSrcRefusesSkillOnlyPaths(t *testing.T) {
	id := raiseSrcEdit(t, "src", "protected-edit", "yoke", "pkg/weave/engine.go")
	decide(t, id, ask.VerdictAllow)
	if _, err := approveSrcRecord("yoke", []string{"pkg/weave/engine.go"}, id, ""); err == nil {
		t.Fatal("wrote an approval no merge requires")
	}
}
