package harnessrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

func workspaceWithFiles(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"pkg/a.py", "pkg/sub/b.py", "notes.txt"} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func compileIn(t *testing.T, dir, script string) Intent {
	t.Helper()
	intent, err := CompileIntent(compileRequestInCwd(t, script, dir))
	if err != nil {
		t.Fatalf("CompileIntent(%q): %v", script, err)
	}
	return intent
}

func TestFileRefinersProveWorkspaceTargets(t *testing.T) {
	dir := workspaceWithFiles(t)
	cases := []struct {
		script string
		kind   string
		target string
	}{
		{`cat pkg/a.py`, atlas.EffRead, "pkg/a.py"},
		{`head -n 20 pkg/a.py`, atlas.EffRead, "pkg/a.py"},
		{`tail -5 notes.txt`, atlas.EffRead, "notes.txt"},
		{`grep -rn 'format="html"' pkg`, atlas.EffRead, "pkg"},
		{`grep -e needle -e hay pkg/a.py`, atlas.EffRead, "pkg/a.py"},
		{`ls -la pkg`, atlas.EffRead, "pkg"},
		{`ls`, atlas.EffRead, "."},
		{`find pkg -name '*.py' -type f`, atlas.EffRead, "pkg"},
		{`sed -n '1,20p' pkg/a.py`, atlas.EffRead, "pkg/a.py"},
		{`sed -i 's/x/y/g' pkg/a.py`, atlas.EffWrite, "pkg/a.py"},
		{`wc -l pkg/a.py notes.txt`, atlas.EffRead, "notes.txt"},
		{`git status`, atlas.EffRead, "."},
		{`git diff pkg/a.py`, atlas.EffRead, "."},
		{`git apply fix.patch`, atlas.EffWrite, "."},
		{`cd pkg && cat sub/b.py`, atlas.EffRead, "pkg/sub/b.py"},
		{`cat > pkg/new.py <<'EOF'
print(1)
EOF`, atlas.EffWrite, "pkg/new.py"},
	}
	for _, tc := range cases {
		intent := compileIn(t, dir, tc.script)
		if !intent.Complete || len(intent.Unsupported) != 0 {
			t.Errorf("%q incomplete: %+v", tc.script, intent.Unsupported)
			continue
		}
		want := canonicalTarget(filepath.Join(dir, tc.target))
		found := false
		for _, e := range intent.Effects {
			if e.Kind == atlas.EffPure {
				continue
			}
			if e.Scope != atlas.TierWorkspace {
				t.Errorf("%q: effect %+v not in workspace scope", tc.script, e)
			}
			if e.Kind == tc.kind && e.Target == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: no %s effect on %s in %+v", tc.script, tc.kind, want, intent.Effects)
		}
	}
}

func TestFileRefinersLabelOutsideTargetsUserland(t *testing.T) {
	dir := workspaceWithFiles(t)
	for _, script := range []string{`cat /etc/hosts`, `cd / && cat etc/hosts`, `echo x > /tmp/escape.txt`, `grep -r x ../`} {
		intent := compileIn(t, dir, script)
		outside := false
		for _, e := range intent.Effects {
			if e.Kind != atlas.EffPure && e.Scope != atlas.TierWorkspace {
				outside = true
			}
		}
		if !outside {
			t.Errorf("%q: expected a non-workspace effect, got %+v", script, intent.Effects)
		}
	}
}

func TestFileRefinersStayFailClosed(t *testing.T) {
	dir := workspaceWithFiles(t)
	for _, script := range []string{
		`find . -name '*.pyc' -delete`,
		`find . -exec rm {} \;`,
		`sed 's/x/y/w out.txt' pkg/a.py`,
		`sed -e '1e date' pkg/a.py`,
		`sed -f script.sed pkg/a.py`,
		`sort -o out.txt notes.txt`,
		`git -c alias.x='!rm -rf /' x`,
		`git push origin main`,
		`git commit -m msg`,
		`git stash drop`,
		`git diff --output=/tmp/x`,
		`git grep -O vim needle`,
		`cd`,
		`cd -`,
		`cat "$FILE"`,
	} {
		intent := compileIn(t, dir, script)
		if intent.Complete {
			t.Errorf("%q: expected incomplete, got complete with %+v", script, intent.Effects)
		}
	}
}

func effectKinds(intent Intent) map[string]string {
	kinds := map[string]string{}
	for _, e := range intent.Effects {
		if e.Kind != atlas.EffPure {
			kinds[e.Kind] = e.Scope
		}
	}
	return kinds
}

func TestFunctionsCompileAtTheirCallSites(t *testing.T) {
	dir := workspaceWithFiles(t)
	intent := compileIn(t, dir, "show() { cat pkg/a.py; }\nshow")
	if !intent.Complete {
		t.Fatalf("provable function incomplete: %+v", intent.Unsupported)
	}
	if kinds := effectKinds(intent); kinds[atlas.EffRead] != atlas.TierWorkspace {
		t.Fatalf("function body effects not merged: %+v", intent.Effects)
	}
	undecorated := compileIn(t, dir, "tests() { python3 -m pytest -q; }\ntests")
	if undecorated.Complete {
		t.Fatalf("an undecorated interpreter call must stay unsupported: %+v", undecorated.Effects)
	}
}

func TestDecoratedEnvelopeBoundsInterpreters(t *testing.T) {
	dir := workspaceWithFiles(t)
	contained := compileIn(t, dir, `@effects("read,write,exec")
@contain(net: "deny")
function tests() { python3 -m pytest -q tests/; }
tests`)
	if !contained.Complete {
		t.Fatalf("declared, contained envelope incomplete: %+v", contained.Unsupported)
	}
	kinds := effectKinds(contained)
	if kinds[atlas.EffExec] != ScopeDeclared || kinds[atlas.EffWrite] != ScopeDeclared {
		t.Fatalf("envelope not recorded as declared: %+v", contained.Effects)
	}
	if _, net := kinds[atlas.EffNet]; net {
		t.Fatalf("contained envelope must not carry net: %+v", contained.Effects)
	}
	open := compileIn(t, dir, `@effects("read,write,exec")
function tests() { python3 -m pytest -q; }
tests`)
	if _, net := effectKinds(open)[atlas.EffNet]; !net {
		t.Fatalf("an uncontained declaration without net must keep a possible net effect: %+v", open.Effects)
	}
	exceeds := compileIn(t, dir, `@guard("read")
@effects("read,write")
function tests() { python3 -m pytest; }
tests`)
	if exceeds.Complete {
		t.Fatalf("a declaration exceeding its guard must be unsupported")
	}
	precise := compileIn(t, dir, `@effects("read")
function show() { cat pkg/a.py; }
show`)
	if !precise.Complete || effectKinds(precise)[atlas.EffRead] != atlas.TierWorkspace {
		t.Fatalf("a provable decorated body must keep its precise effects: %+v", precise.Effects)
	}
}
