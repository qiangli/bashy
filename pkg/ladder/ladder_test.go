package ladder

import "testing"

type fakeCommands map[string]bool

func (f fakeCommands) Known(name string) bool          { return f[name] }
func (f fakeCommands) GlobMatches(pattern string) bool { return pattern == "file?" }

func (f fakeCommands) Names() []string {
	var out []string
	for n := range f {
		out = append(out, n)
	}
	return out
}

func TestResolve(t *testing.T) {
	cmds := fakeCommands{"git": true, "grep": true, "ls": true, "make": true, "who": true, "echo": true, "status": false}
	for _, tc := range []struct {
		line string
		rung Rung
		want string
	}{
		{"ls -la", Literal, "ls -la"},
		{"  git status", Literal, "  git status"},
		{"echo 'what's", Literal, "echo 'what's"},
		{"X=1 printenv X", Literal, "X=1 printenv X"},
		{"./run.sh", Literal, "./run.sh"},
		{"(cd /tmp)", Literal, "(cd /tmp)"},
		{"for i in 1 2; do", Literal, "for i in 1 2; do"},
		{"", Literal, ""},
		{"gti status", Repair, "git status"},
		{"grpe -r foo .", Repair, "grep -r foo ."},
		{"gti|wc", Repair, "git|wc"},
		{"what does this repo do?", Free, "what does this repo do?"},
		{"hwo does this work?", Free, "hwo does this work?"},
		{"explain the build", Free, "explain the build"},
		{"sl", Free, "sl"},
		{"what's in here", Free, "what's in here"},
		{"who is logged in?", Free, "who is logged in?"},
		{"echo really\\?", Literal, "echo really\\?"},
		{"echo 'why?'", Literal, "echo 'why?'"},
		{"ls file?", Literal, "ls file?"},
		{"which file has the bug? answer in one sentence", Free, "which file has the bug? answer in one sentence"},
		{"make sense of this?!", Free, "make sense of this?!"},
	} {
		got := Resolve(tc.line, cmds)
		if got.Rung != tc.rung || got.Line != tc.want {
			t.Errorf("Resolve(%q) = %v %q, want %v %q", tc.line, got.Rung, got.Line, tc.rung, tc.want)
		}
	}
}

func TestRepairRefusesTies(t *testing.T) {
	// "cst" is one edit from both cat and cut.
	cmds := fakeCommands{"cat": true, "cut": true}
	if got := Resolve("cst file", cmds); got.Rung != Free {
		t.Fatalf("tie repaired: %+v", got)
	}
}

// On a full PATH a typo is often one edit from several names; the one with
// the same letters (a transposition) wins, and a question is never repaired.
func TestRepairOnACrowdedPath(t *testing.T) {
	cmds := fakeCommands{"git": true, "gtr": true, "gt": true, "chat": true, "grep": true, "gerp": true}
	if got := Resolve("gti --version", cmds); got.Rung != Repair || got.Line != "git --version" {
		t.Fatalf("gti: %+v", got)
	}
	if got := Resolve("what does stats.py compute? one sentence", cmds); got.Rung != Free {
		t.Fatalf("a question was repaired: %+v", got)
	}
	// Two transpositions at the same distance: no guess.
	if got := Resolve("abc x", fakeCommands{"bac": true, "acb": true}); got.Rung != Free {
		t.Fatalf("ambiguous anagram repaired: %+v", got)
	}
}

func TestDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		d    int
	}{{"gti", "git", 1}, {"grpe", "grep", 1}, {"kitten", "sitting", 3}, {"", "ab", 2}} {
		if got := distance(tc.a, tc.b); got != tc.d {
			t.Errorf("distance(%q,%q)=%d want %d", tc.a, tc.b, got, tc.d)
		}
	}
}
