package desk

import (
	"encoding/json"
	"strings"
)

// registerScripts are the desk tools that ship an installer. The shipped
// installer must take the same app-add route as an external shaped app, and a
// principal can then activate and undo a personal revision of what it
// registered. TestDeskToolsForkAndRollback proves both for these tools in one
// pass: it registers through register.sh instead of a bare app add and then
// runs the fork and rollback on that registration, so each revision is
// verified once rather than by a second, identical round trip.
var registerScripts = map[string]bool{"notepad": true, "form": true}

// register runs the installer in root against f's isolated home and checks it
// left a registered shaped app whose managed source is repo.
func (f *forkHome) register(name, root, repo string) {
	f.t.Helper()
	f.env = append(f.env, "BASHY_BIN="+f.bin)
	f.run(root, "bash", "register.sh")

	var app struct {
		Name, Type, Source string
	}
	out := f.bashy("app", "show", name, "--json")
	if err := json.Unmarshal([]byte(out), &app); err != nil {
		f.t.Fatalf("app show: %v\n%s", err, out)
	}
	if app.Name != name || app.Type != "shaped" || app.Source != repo {
		f.t.Fatalf("registered app = %+v, want shaped managed source %s", app, repo)
	}
	if list := f.bashy("app", "list"); !strings.Contains(list, name+" ") || !strings.Contains(list, "registered") {
		f.t.Fatalf("app list lacks registered shaped app %s:\n%s", name, list)
	}
}
