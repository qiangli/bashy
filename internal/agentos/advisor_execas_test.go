package agentos

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// A command run under another name (`exec -a NAME FILE`) is advised under
// that name: Claude Code's grep shim runs its own binary as "ugrep", whose
// exit 1 is an ordinary no-match, not a failure that feeds loop detection.
func TestAdvisorUsesExecAsName(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The fixture is a #!/bin/sh file with no extension; exec'ing it on
		// Windows hangs the whole agentos package (CI run 36337465550). The
		// advice naming under test is OS-independent.
		t.Skip("needs a directly executable shebang script")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "2.1.283")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(name string) string {
		var out, errOut bytes.Buffer
		a := &advisor{probe: healthyProbe(), mem: newTestMemory(t)}
		r, err := interp.New(interp.StdIO(nil, &out, &errOut), interp.ExecHandlers(advisorHandler(a)))
		if err != nil {
			t.Fatal(err)
		}
		script := strings.Repeat("( exec -a "+name+" '"+tool+"' ) || true\n", loopThreshold+1)
		prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Run(context.Background(), prog)
		return errOut.String()
	}
	if got := run("ugrep"); strings.Contains(got, "failed repeatedly") || strings.Contains(got, "2.1.283") {
		t.Fatalf("a ugrep no-match was advised as a failure:\n%s", got)
	}
	got := run("mytool")
	if !strings.Contains(got, "mytool has now failed repeatedly") || strings.Contains(got, "2.1.283") {
		t.Fatalf("the loop advice should name mytool, not the file:\n%s", got)
	}
}
