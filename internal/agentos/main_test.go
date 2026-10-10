package agentos

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/cli"
)

// TestMain isolates the registered-command ring and the compiled host's
// attest sink for the WHOLE package: the catalog, listing, atlas and dispatch
// tests read the ring through registeredLookup/registeredNames, and a
// decorated call run through the compiled (shellrt) path resolves its attest
// sink lazily on first use (compiledAttestSink, attest.go) — both read a
// developer's real ~/.config/bashy directory unless redirected here first.
// Tests that need a populated ring, or want to assert on receipts, point
// BASHY_COMMANDS_DIR / BASHY_SKILLS_DIR at their own temp dir.
// toolFenceEchoEnv switches the test binary into a portable echo (polyglot_tool_row_test.go).
const toolFenceEchoEnv = "BASHY_TOOLFENCE_TEST_ECHO"

// isolationSentinel marks a process whose BASHY_SHIM_DIR the top-level TestMain
// already pointed at a throwaway dir. It is inherited across the package's
// `<self> -test.run=^TestXHelper$` re-execs, so those children do not allocate
// (and then, via os.Exit, leak) a second /tmp isolation dir.
const isolationSentinel = "BASHY_AGENTOS_TEST_ISOLATED"

// Isolation roots installed by the top-level TestMain and the real values they
// shadow, so the shim-isolation test can confirm a write never reaches the
// real ~/.bashy/shims (shim_isolation_test.go).
var (
	testIsolatedBase    string
	testIsolatedShimDir string
	testRealHome        string
	testRealHomeSet     bool
	testRealShimDir     string
	testRealShimSet     bool
)

// foreignReExec reports whether this test binary was launched as something
// other than `go test` — e.g. a bashy self-exec path that resolved
// os.Executable() back to this binary and handed it shell or subcommand
// arguments (`probeShell` runs `<shell> -c "echo ok"`, `hasDAGTarget` runs
// `<self> dag …`, the podman machine would run `<self> podman info`, a
// tool-row launch runs `<self> --model … <prompt>`). Running m.Run() in such
// a child re-executes the ENTIRE suite, which re-triggers the same self-exec
// and forks until the host dies (observed: `agentos.test -c echo ok` recursing
// 250+ levels deep). This is the generic backstop for the memory rule that
// tests must never reach code that execs the bashy self path.
//
// A genuine `go test` invocation always carries at least one of the testing
// framework's own -test.* flags: the `go test` runner adds -test.timeout /
// -test.paniconexit0, and this package's helper re-execs pass an explicit
// -test.run=^Foo$ (which may ALSO carry trailing `-- <arg>` positionals — so
// the signal is the PRESENCE of one -test.* flag, not the absence of any
// non-test arg). A bare `agentos.test` with no arguments is the legitimate
// run-everything case. So: foreign iff there are arguments and none is -test.*.
func foreignReExec() bool {
	args := os.Args[1:]
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-test.") {
			return false
		}
	}
	return true
}

func TestMain(m *testing.M) {
	// Carrier helpers re-exec this binary; intercept them before the generic
	// foreign-re-exec guard and before package test isolation.
	cli.MaybeRunJobCarrierHelper()
	// The supervisord integration tests re-exec this binary as a scripted
	// stand-in for the dag root (supervisord_test.go). Gated by its own env
	// var and checked first so its `dag … svc` argv reaches the child, not the
	// foreignReExec backstop below.
	if os.Getenv("BASHY_SUPERVISORD_TEST_CHILD") != "" {
		os.Exit(supervisordTestChild())
	}
	// Any invocation that is not `go test` — the toolFenceEchoEnv echo mode and
	// the generic foreignReExec backstop — echoes its arguments and exits 0
	// WITHOUT running the suite. Echoing (rather than just exiting) keeps the
	// toolFenceEchoEnv contract and also satisfies a probeShell that greps
	// stdout for "ok" in `<self> -c "echo ok"`. Placed before any isolation
	// setup so a re-exec never creates a /tmp dir.
	if os.Getenv(toolFenceEchoEnv) != "" || foreignReExec() {
		fmt.Println(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}

	// Isolate install-agent's shim writes from the real ~/.bashy/shims (seen
	// poisoned on a dev host 2026-10-08) by pointing BASHY_SHIM_DIR — which
	// shimDir() honors — at a throwaway /tmp dir. Done once in the top-level
	// process and inherited across the package's -test.run=^Helper$ re-execs
	// (isolationSentinel), so a helper that os.Exit()s never allocates and
	// leaks its own dir. /tmp keeps the path short; the dir only holds
	// shell-script shims. HOME and BASHY_HOME are deliberately left alone:
	// tests that resolve the real bashy under $HOME/.local/bin (remote
	// install) and the agent-roster / inbox identity stores depend on the real
	// home, and overriding it there is both unnecessary (shimDir already
	// honors BASHY_SHIM_DIR) and regressive.
	var shimBase string
	if os.Getenv(isolationSentinel) == "" {
		os.Setenv(isolationSentinel, "1")
		testRealShimDir, testRealShimSet = os.LookupEnv("BASHY_SHIM_DIR")
		if home, herr := os.UserHomeDir(); herr == nil {
			testRealHome, testRealHomeSet = home, true
		}
		if b, berr := os.MkdirTemp("/tmp", "bashy-agentos-"); berr == nil {
			shimBase = b
			testIsolatedBase = b
			testIsolatedShimDir = filepath.Join(b, "shims")
			_ = os.MkdirAll(testIsolatedShimDir, 0o755)
			_ = os.Setenv("BASHY_SHIM_DIR", testIsolatedShimDir)
		}
	}

	dir, err := os.MkdirTemp("", "bashy-agentos-commands-")
	if err != nil {
		panic(err)
	}
	os.Setenv("BASHY_COMMANDS_DIR", filepath.Join(dir, "commands"))
	os.Setenv("BASHY_COMMANDS_PATH", "")
	os.Setenv("BASHY_SKILLS_DIR", filepath.Join(dir, "skills"))
	// The self path is this test binary: a real ensure would re-run the whole
	// suite as `<test> podman info`.
	ensurePodmanMachineHook = func(string) error { return nil }
	code := m.Run()
	os.RemoveAll(dir)
	if shimBase != "" {
		// Explicit cleanup before os.Exit (which skips defers). A codex/shim
		// install may leave read-only files, so widen permissions first.
		_ = filepath.Walk(shimBase, func(p string, info os.FileInfo, werr error) error {
			if werr == nil && info != nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		os.RemoveAll(shimBase)
	}
	os.Exit(code)
}
