package agentos

// Sprint: #322; Story: #1130; Story-ID: 4aac4f305b17
//
// Agent mode routes the Python provisioner names to bashy's own pinned
// toolchain, but an ACTIVATED environment (a virtualenv or conda env — the
// SWE-bench task images activate `testbed`) is the project's interpreter, with
// the project's dependencies: bare python/pip must reach it through PATH. Before
// this, genie in a task container ran `python tests/runtests.py` as
// `bashy python …`, which tried to download uv with no network, so no test
// could run at all.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func runAgentPreamble(t *testing.T, env []string, script string) string {
	t.Helper()
	// Never let a shim reach "bashy itself": in a test binary that is the test
	// binary, and a red run would re-execute it recursively.
	t.Setenv("BASHY_SELF", filepath.Join(t.TempDir(), "no-bashy-self"))
	prog, err := syntax.NewParser().Parse(strings.NewReader(PreambleFor(true)+script), "")
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	r, err := interp.New(interp.StdIO(nil, &out, &errOut), interp.Env(expand.ListEnviron(env...)))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), prog); err != nil {
		t.Fatalf("run: %v; stderr=%q", err, errOut.String())
	}
	return out.String()
}

func TestAgentModePythonUsesActivatedEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stand-in for an interpreter")
	}
	envBin := t.TempDir()
	for _, name := range []string{"python", "python3", "pip", "pip3"} {
		script := "#!/bin/sh\necho \"env-" + name + " $*\"\n"
		if err := os.WriteFile(filepath.Join(envBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := envBin + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin"
	for _, activation := range []string{"CONDA_PREFIX=" + filepath.Dir(envBin), "VIRTUAL_ENV=" + filepath.Dir(envBin)} {
		got := runAgentPreamble(t, []string{"PATH=" + path, activation},
			"python -V\npython3 -V\npip list\npip3 list\n")
		want := "env-python -V\nenv-python3 -V\nenv-pip list\nenv-pip3 list\n"
		if got != want {
			t.Errorf("%s: got %q, want the activated environment's interpreters %q", activation, got, want)
		}
	}
}

// Without an activated environment the shims still route to bashy's toolchain
// (checked on the text: taking that branch would exec bashy itself).
func TestAgentModePythonWithoutEnvironmentKeepsBashyToolchain(t *testing.T) {
	agent := PreambleFor(true)
	for _, name := range []string{"python", "python3", "pip", "pip3"} {
		start := strings.Index(agent, name+"() {")
		if start < 0 {
			t.Fatalf("agent preamble lacks %s()", name)
		}
		line := agent[start:]
		line = line[:strings.Index(line, "\n")]
		provisioner := agentModeShimAliases[name]
		if provisioner == "" {
			provisioner = name
		}
		if !strings.Contains(line, "' "+provisioner+" \"$@\"") {
			t.Errorf("%s shim no longer falls back to `bashy %s`: %s", name, provisioner, line)
		}
	}
}
