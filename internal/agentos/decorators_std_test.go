// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "x")
}

// @timeout: past the limit the call is 124 and says so; under it the body's
// own status stands.
func TestTimeoutDecoratorPastLimitIs124(t *testing.T) {
	script := `@timeout("150ms")
function slow() { sleep 3; echo never; }
slow
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if status, ok := exitStatusOf(err); !ok || status != exitTimeout {
		t.Fatalf("want exit 124, got %v (stdout %q stderr %q)", err, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "slow: timeout after 150ms") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if strings.Contains(out.String(), "never") {
		t.Error("body ran past the deadline")
	}
}

func TestTimeoutDecoratorUnderLimitKeepsStatus(t *testing.T) {
	script := `@timeout(d: "5s")
function quick() { return 3; }
quick
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if status, ok := exitStatusOf(err); !ok || status != 3 {
		t.Fatalf("want the body's 3, got %v (stderr %q)", err, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("no diagnostic under the limit; got %q", errOut.String())
	}
}

// Inside @retry every attempt gets its own deadline: the body starts n times.
func TestTimeoutDecoratorReArmsUnderRetry(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	script := `@retry(n: 2, backoff: "1ms")
@timeout("100ms")
function slow() { echo x >> "$COUNT"; sleep 3; }
slow
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"COUNT": count})
	if status, ok := exitStatusOf(err); !ok || status != exitTimeout {
		t.Fatalf("want 124 after two timed-out attempts, got %v (stderr %q)", err, errOut.String())
	}
	if got := countLines(t, count); got != 2 {
		t.Fatalf("body started %d times, want 2", got)
	}
}

func TestTimeoutDecoratorRefusesBadArgument(t *testing.T) {
	for _, deco := range []string{`@timeout()`, `@timeout("-1s")`, `@timeout(n: "1s")`, `@timeout("soon")`} {
		script := deco + "\nfunction f() { :; }\nf\n"
		err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
		if err == nil || !strings.Contains(errOut.String(), "timeout") {
			t.Errorf("%s: want a refusal naming timeout, got err=%v stderr=%q", deco, err, errOut.String())
		}
	}
}

// @memo: the second call with the same arguments returns the cached result
// without running the body; different arguments miss; a failure is not cached.
func TestMemoDecoratorCachesResultPerArgs(t *testing.T) {
	memoStore = *new(memoStoreType)
	count := filepath.Join(t.TempDir(), "count")
	script := `@memo()
func triple(n int) int { echo x >> "$COUNT"; return n * 3 }
a := triple(2)
b := triple(2)
c := triple(3)
echo "$a $b $c"
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"COUNT": count})
	if err != nil {
		t.Fatalf("want pass, got %v (stderr %q)", err, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "6 6 9" {
		t.Fatalf("results = %q", got)
	}
	if got := countLines(t, count); got != 2 {
		t.Fatalf("body ran %d times, want 2 (one per distinct argument)", got)
	}
}

func TestMemoDecoratorDoesNotCacheFailure(t *testing.T) {
	memoStore = *new(memoStoreType)
	count := filepath.Join(t.TempDir(), "count")
	script := `@memo("1h")
function flaky() { echo x >> "$COUNT"; return 1; }
flaky; flaky
`
	err, _, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"COUNT": count})
	if status, ok := exitStatusOf(err); !ok || status != 1 {
		t.Fatalf("want 1, got %v", err)
	}
	if got := countLines(t, count); got != 2 {
		t.Fatalf("a failure was cached: body ran %d times, want 2", got)
	}
}

func TestMemoDecoratorTTLExpires(t *testing.T) {
	memoStore = *new(memoStoreType)
	count := filepath.Join(t.TempDir(), "count")
	script := `@memo(ttl: "50ms")
function tick() { echo x >> "$COUNT"; }
tick; tick; sleep 0.2; tick
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"COUNT": count})
	if err != nil {
		t.Fatalf("want pass, got %v (stderr %q)", err, errOut.String())
	}
	if got := countLines(t, count); got != 2 {
		t.Fatalf("body ran %d times, want 2 (hit, then expired)", got)
	}
}

// @auth: via decides, once per process; its first line is the principal the
// body sees; as: must match; failure is 77.
func TestAuthDecoratorViaOnceAndPrincipal(t *testing.T) {
	authStore = *new(authStoreType)
	count := filepath.Join(t.TempDir(), "count")
	script := `@auth(via: 'echo x >> "$COUNT"; printf "alice\nextra\n"')
function whoami_guarded() { echo "as $BASHY_PRINCIPAL"; }
whoami_guarded; whoami_guarded
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"COUNT": count})
	if err != nil {
		t.Fatalf("want pass, got %v (stderr %q)", err, errOut.String())
	}
	if got := out.String(); got != "as alice\nas alice\n" {
		t.Fatalf("stdout = %q (the via output must not leak; the principal must bind)", got)
	}
	if got := countLines(t, count); got != 1 {
		t.Fatalf("via ran %d times, want once per process", got)
	}
}

func TestAuthDecoratorDeniesWith77(t *testing.T) {
	authStore = *new(authStoreType)
	script := `@auth(via: "false")
function secret() { echo leaked; }
secret
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if status, ok := exitStatusOf(err); !ok || status != exitNoPerm {
		t.Fatalf("want 77, got %v", err)
	}
	if strings.Contains(out.String(), "leaked") || !strings.Contains(errOut.String(), "secret: not authenticated (via: false)") {
		t.Errorf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestAuthDecoratorAsMustMatch(t *testing.T) {
	authStore = *new(authStoreType)
	script := `@auth(via: "echo bob", as: "alice")
function secret() { echo leaked; }
secret
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if status, ok := exitStatusOf(err); !ok || status != exitNoPerm {
		t.Fatalf("want 77, got %v", err)
	}
	if strings.Contains(out.String(), "leaked") || !strings.Contains(errOut.String(), `secret: principal "bob", want "alice"`) {
		t.Errorf("stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

// @effects at the call boundary: a caller guarded to read cannot call a
// function that declares write — denied before the body, naming both.
func TestEffectsDecoratorBoundaryDenied(t *testing.T) {
	leak := filepath.Join(t.TempDir(), "leak")
	script := `@effects("write")
function deploy() { touch "$LEAK"; }
@guard("read")
function caller() { deploy; echo "deploy -> $?"; }
caller
`
	_, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"LEAK": leak})
	if !strings.Contains(out.String(), "deploy -> 126") {
		t.Fatalf("want the boundary denial 126; stdout=%q stderr=%q", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "deploy: declared effects write exceed the guard (write not allowed by read)") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if _, err := os.Stat(leak); err == nil {
		t.Fatal("the body ran past the boundary denial")
	}
}

// @effects vouches for a command the atlas does not know: an unknown tool
// on PATH is denied under a bare @guard(exec) but runs inside an
// @effects("exec") function called under the same guard.
func TestEffectsDecoratorVouchesForUnknownTool(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "bin")
	if err := os.MkdirAll(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	// Use a real native executable: a shebang fixture cannot be launched on Windows.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	name := "notinatlas"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(shim, name), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_TEST_UNKNOWN_TOOL", "1")
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	bare := `@guard(effects: "exec")
function direct() { notinatlas -test.run=^TestEffectsUnknownToolProcess$; echo "direct -> $?"; }
direct
`
	_, out, _ := runDecorated(t, context.Background(), syntax.LangBashPP, bare, nil)
	if strings.Contains(out.String(), "ran-unknown") {
		t.Fatalf("an atlas-unknown tool must be denied under a bare guard: %q", out.String())
	}

	vouched := `@effects("exec")
function build() { notinatlas -test.run=^TestEffectsUnknownToolProcess$; }
@guard(effects: "exec")
function via() { build; echo "build -> $?"; }
via
`
	_, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, vouched, nil)
	if !strings.Contains(out.String(), "ran-unknown") || !strings.Contains(out.String(), "build -> 0") {
		t.Fatalf("the author's declaration must classify the unknown tool: stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

// Inside an @effects function the declaration is the cap: a command over it
// is denied as under @guard.
func TestEffectsDecoratorIsItsOwnCap(t *testing.T) {
	leak := filepath.Join(t.TempDir(), "leak")
	script := `@effects("read")
function honest() { touch "$LEAK"; echo "touch -> $?"; }
honest
`
	_, out, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"LEAK": leak})
	if strings.Contains(out.String(), "touch -> 0") {
		t.Fatalf("a write inside an @effects(read) function must be denied: %q", out.String())
	}
	if _, err := os.Stat(leak); err == nil {
		t.Fatal("the denied write happened")
	}
}

func TestEffectsDecoratorRefusesBadDeclaration(t *testing.T) {
	for _, deco := range []string{`@effects()`, `@effects("teleport")`, `@effects(cap: "read")`, `@effects("read", "write")`} {
		script := deco + "\nfunction f() { :; }\nf\n"
		err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
		if err == nil || !strings.Contains(errOut.String(), "effects") {
			t.Errorf("%s: want a refusal naming effects, got err=%v stderr=%q", deco, err, errOut.String())
		}
	}
}

// TestEffectsUnknownToolProcess is selected only by the native fixture child.
func TestEffectsUnknownToolProcess(t *testing.T) {
	if os.Getenv("BASHY_TEST_UNKNOWN_TOOL") != "1" {
		return
	}
	fmt.Println("ran-unknown")
	os.Exit(0)
}
