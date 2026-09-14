package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// The disambiguation rule: a CRUD word counts only when a NAME follows it;
// list/schema take none; `show NAME` bare is the lister's own report.
func TestCommandsRegistryArgsDisambiguation(t *testing.T) {
	cases := []struct {
		in       []string
		registry bool
		out      []string
	}{
		{[]string{"rm"}, false, []string{"rm"}},            // show rm's record
		{[]string{"set"}, false, []string{"set"}},          // the builtin's record
		{[]string{"rm", "gl"}, true, []string{"rm", "gl"}}, // CRUD
		{[]string{"set", "gl", "--set", "x=1"}, true, []string{"set", "gl", "--set", "x=1"}},
		{[]string{"rm", "--json"}, false, []string{"rm", "--json"}}, // a flag is not a name
		{[]string{"add", "gl", "--set", "script=x"}, true, nil},
		{[]string{"list"}, true, nil},
		{[]string{"schema", "--json"}, true, nil},
		{[]string{"show", "gl"}, false, []string{"gl"}},                            // ≡ commands gl
		{[]string{"show", "gl", "--yaml"}, true, []string{"show", "gl", "--yaml"}}, // the record
		{[]string{"show", "gl", "--field", "synopsis"}, true, nil},
		{[]string{"verify"}, false, []string{"verify"}}, // the hidden alias's record
		{[]string{"verify", "gl"}, true, nil},
		{[]string{"--json"}, false, []string{"--json"}},
		{[]string{"ls"}, false, []string{"ls"}},
		{nil, false, nil},
	}
	for _, tc := range cases {
		got, registry := commandsRegistryArgs(tc.in)
		if registry != tc.registry {
			t.Errorf("%v: registry = %v, want %v", tc.in, registry, tc.registry)
		}
		if tc.out != nil && !slices.Equal(got, tc.out) {
			t.Errorf("%v: args = %v, want %v", tc.in, got, tc.out)
		}
	}
}

// The listing, the one-command report, the JSON contract and the origin view
// all see a populated ring — and a shadowed ring entry is reported, not listed.
func TestCommandsListsRegisteredRing(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	dir := ringDir(t)
	writeRecord(t, fleet.Command{Name: "gl", Synopsis: "compact log", Script: "git log", Effects: []string{"read"}})
	writeRecord(t, fleet.Command{Name: "hid", Hidden: true, Script: "true", Effects: []string{"pure"}})
	if err := os.WriteFile(filepath.Join(dir, "cat.yaml"), []byte("name: cat\nkind: command\nexec: [/bin/sh]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resetRegisteredIndex()

	out, code := captureCommands(t)
	if code != 0 || !strings.Contains(out, "registered — yours") || !strings.Contains(out, "gl") {
		t.Errorf("default listing lacks the registered block:\n%s", out)
	}
	if !strings.Contains(out, "shadowed by this bashy") || !strings.Contains(out, "cat (") {
		t.Errorf("default listing lacks the shadowed note:\n%s", out)
	}
	if strings.Contains(out, " hid ") || strings.Contains(out, " hid\n") {
		t.Errorf("hidden registered command listed by default:\n%s", out)
	}
	all, _ := captureCommands(t, "--all")
	if !strings.Contains(all, "hid") {
		t.Errorf("--all must list the hidden registered command:\n%s", all)
	}

	out, code = captureCommands(t, "gl")
	if code != 0 || !strings.Contains(out, "registered") || !strings.Contains(out, "compact log") {
		t.Errorf("commands gl:\n%s", out)
	}
	out, _ = captureCommands(t, "show", "gl")
	if !strings.Contains(out, "compact log") {
		t.Errorf("commands show gl must be the same report:\n%s", out)
	}
	out, code = captureCommands(t, "gl", "--features")
	var info map[string]any
	if err := json.Unmarshal([]byte(out), &info); err != nil || code != 0 {
		t.Fatalf("--features: %v %d %s", err, code, out)
	}
	for k, want := range map[string]any{"resolver": "bashy-registered", "origin": "registered", "mode": "script", "ring": "local", "class": "verb"} {
		if info[k] != want {
			t.Errorf("features[%s] = %v, want %v", k, info[k], want)
		}
	}
	if _, ok := info["path"]; !ok {
		t.Errorf("features lacks the record path: %v", info)
	}
	out, _ = captureCommands(t, "cat")
	if strings.Contains(out, "registered") {
		t.Errorf("a shadowed ring entry must report as the applet:\n%s", out)
	}

	out, _ = captureCommands(t, "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	reg, _ := got["registered"].([]any)
	if len(reg) != 1 || reg[0] != "gl" {
		t.Errorf("registered = %v", got["registered"])
	}
	verbs, _ := got["verbs"].([]any)
	if !slices.Contains(verbs, any("gl")) || slices.Contains(verbs, any("hid")) || slices.Contains(verbs, any("cat")) {
		t.Errorf("verbs = %v", verbs)
	}

	out, _ = captureCommands(t, "--view", "origin")
	if !strings.Contains(out, "registered") || !strings.Contains(out, "gl") {
		t.Errorf("--view origin lacks the registered block:\n%s", out)
	}
	if !slices.Contains(atlasCommandNamesLive(), "gl") {
		t.Error("liveAtlas must carry the registered record")
	}
}

func atlasCommandNamesLive() []string {
	var out []string
	for _, r := range liveAtlas(false) {
		out = append(out, r.Name)
	}
	return out
}

// The CRUD path through the front door: add → the name resolves; rm → it
// does not; the collision filter refuses shipped names by holder.
func TestCommandsCRUDThroughDispatch(t *testing.T) {
	ringDir(t)
	if _, code := captureCommands(t, "add", "gl2", "--set", "script=git log", "--set", "effects.0=read"); code != 0 {
		t.Fatalf("add exit = %d", code)
	}
	if _, ok := registeredLookup("gl2"); !ok {
		t.Error("add did not register gl2")
	}
	if out, code := captureCommands(t, "list"); code != 0 || !strings.Contains(out, "gl2") {
		t.Errorf("list: %d %s", code, out)
	}
	if out, code := captureCommands(t, "schema"); code != 0 || !strings.Contains(out, "download.sha256.<key>") {
		t.Errorf("schema: %d %s", code, out)
	}
	for _, bad := range []string{"ls", "set", "weave", "add", "command", "doctl"} {
		if _, code := captureCommands(t, "add", bad, "--set", "exec.0=/bin/sh"); code == 0 {
			t.Errorf("add %s must be refused", bad)
		}
	}
	if _, code := captureCommands(t, "rm", "gl2"); code != 0 {
		t.Error("rm failed")
	}
	if _, ok := registeredLookup("gl2"); ok {
		t.Error("rm did not unregister gl2")
	}
}
