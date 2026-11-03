package desk

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The shipped installers must take the same app-add route as an external
// shaped app. A principal can then activate and undo a personal revision.
func TestDeskRegisterScriptsUsePublicShapedAppPath(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("runs the desk installers and the product CLI")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for a revision round trip")
	}
	bin := productBashy(t)
	for _, name := range []string{"notepad", "form"} {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.Abs(name)
			if err != nil {
				t.Fatal(err)
			}
			f := newForkHome(t, bin, gitBin)
			f.env = append(f.env, "BASHY_BIN="+bin)
			f.run(root, "bash", "register.sh")

			var app struct {
				Name, Type, Source string
			}
			out := f.bashy("app", "show", name, "--json")
			if err := json.Unmarshal([]byte(out), &app); err != nil {
				t.Fatalf("app show: %v\n%s", err, out)
			}
			repo := filepath.Join(f.home, "src", name)
			if app.Name != name || app.Type != "shaped" || app.Source != repo {
				t.Fatalf("registered app = %+v, want shaped managed source %s", app, repo)
			}
			if list := f.bashy("app", "list"); !strings.Contains(list, name+" ") || !strings.Contains(list, "registered") {
				t.Fatalf("app list lacks registered shaped app %s:\n%s", name, list)
			}

			first := f.fork(name, repo, "personal-edit-1", "first personal edit")
			f.bashy("src", "activate", name, first, "--principal", alice)
			if got := f.active(name); got != first {
				t.Fatalf("active revision = %s, want %s", got, first)
			}
			f.bashy("src", "pr", "merge", f.lastPR)
			second := f.fork(name, repo, "personal-edit-2", "second personal edit")
			f.bashy("src", "activate", name, second, "--principal", alice)
			if got := f.active(name); got != second {
				t.Fatalf("active revision = %s, want %s", got, second)
			}
			f.bashy("src", "undo", name, "--principal", alice)
			if got := f.active(name); got != first {
				t.Fatalf("after undo active = %s, want %s", got, first)
			}
		})
	}
}
