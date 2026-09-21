//go:build e2e

package agentos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/yoke/pkg/binmgr"
)

// registryEnv is scratchStoreEnv plus an isolated binmgr cache and no cert
// profile, so a download record provisions into the test's own directory.
func registryEnv(t *testing.T) []string {
	t.Helper()
	return append(scratchStoreEnv(t), "BASHY_BIN_CACHE="+t.TempDir(), "VSC_PROFILE=", "BASHY_AGENTIC=")
}

// A script record re-enters the built bashy: $0 is the name, "$@" the
// args, in-shell and at the front door, --posix included; a non-zero body
// under `agentic` yields (exit 6) while an unregistered external keeps its
// exact status.
func TestE2ERegisteredScriptDispatch(t *testing.T) {
	bin := bashyBinary(t)
	env := registryEnv(t)
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "who0", "--set", `script=printf '%s:%s\n' "$0" "$1"`, "--set", "effects.0=pure"); code != 0 {
		t.Fatalf("add (exit %d): %s", code, stderr)
	}
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "fails", "--set", "script=exit 3", "--set", "effects.0=pure"); code != 0 {
		t.Fatalf("add fails (exit %d): %s", code, stderr)
	}
	for _, args := range [][]string{{"-c", "who0 x"}, {"--posix", "-c", "who0 x"}, {"who0", "x"}} {
		stdout, stderr, code := runBashyStdEnv(bin, env, args...)
		if code != 0 || strings.TrimSpace(stdout) != "who0:x" {
			t.Errorf("%v: exit %d stdout=%q stderr=%q", args, code, stdout, stderr)
		}
	}
	stdout, _, code := runBashyStdEnv(bin, env, "agentic", "fails")
	if code != 6 || !strings.Contains(stdout, `"input_required"`) {
		t.Errorf("agentic on a failing registered command: exit %d stdout=%s", code, stdout)
	}
	if _, _, code := runBashyStdEnv(bin, env, "agentic", "who0", "y"); code != 0 {
		t.Errorf("agentic on a passing registered command: exit %d", code)
	}
	if _, _, code := runBashyStdEnv(bin, env, "agentic", "/bin/sh", "-c", "exit 7"); code != 7 {
		t.Errorf("agentic on an unregistered external: exit %d, want 7", code)
	}
	// A certification run never sees the ring.
	if _, _, code := runBashyStdEnv(bin, append(env, "VSC_PROFILE=cert"), "--posix", "-c", "who0 x"); code != 127 {
		t.Errorf("cert profile: exit %d, want 127", code)
	}
	// rm unregisters in every mode.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "rm", "who0"); code != 0 {
		t.Fatalf("rm (exit %d): %s", code, stderr)
	}
	for _, args := range [][]string{{"-c", "who0 x"}, {"who0", "x"}} {
		if _, _, code := runBashyStdEnv(bin, env, args...); code != 127 {
			t.Errorf("%v after rm: exit %d, want 127", args, code)
		}
	}
}

// A persisted SCRIPT schema (`commands add … --set args.…`) is honoured by
// the built binary, in-shell and --posix alike: valid typed / enum /
// defaulted arguments reach the body already bound, an invalid call is
// refused with status 2 before the body is entered (the body's own marker
// never appears), a legacy record keeps its argv byte for byte, and an
// invalid schema is refused at add time. The `paint` fixture and the ported
// PowerShell cases are the ones registered_schema_test.go cites (sh
// interp/command_resolver_test.go @ ef726acd, yoke
// pkg/fleet/command_schema_test.go @ 8163aa1; upstream PowerShell/PowerShell,
// MIT) — here on a script record, through `bashy commands add`.
func TestE2ERegisteredScriptSchema(t *testing.T) {
	bin := bashyBinary(t)
	env := registryEnv(t)
	const body = `printf entered; for a; do printf '|%s' "$a"; done; echo`
	add := func(name string, sets ...string) {
		t.Helper()
		args := []string{"commands", "add", name, "--set", "script=" + body, "--set", "effects.0=pure"}
		for _, kv := range sets {
			args = append(args, "--set", kv)
		}
		if _, stderr, code := runBashyStdEnv(bin, env, args...); code != 0 {
			t.Fatalf("add %s (exit %d): %s", name, code, stderr)
		}
	}
	add("paint",
		"args.positionals.0.name=color", "args.positionals.0.required=true", "args.positionals.0.enum.0=red", "args.positionals.0.enum.1=blue",
		"args.positionals.1.name=level", "args.positionals.1.type=int", "args.positionals.1.default=3",
		"args.flags.0.name=mode", "args.flags.0.shorthand=m", "args.flags.0.default=fast", "args.flags.0.enum.0=fast", "args.flags.0.enum.1=slow",
		"args.flags.1.name=count", "args.flags.1.shorthand=c", "args.flags.1.type=int", "args.flags.1.required=true",
		"args.flags.2.name=verbose", "args.flags.2.shorthand=v", "args.flags.2.type=bool")
	add("legacy0")
	// The ported cases (provenance in registered_schema_test.go).
	add("get-testvalidatesetps4", "args.positionals.0.name=Param1", "args.positionals.0.required=true",
		"args.positionals.0.enum.0=Test1", "args.positionals.0.enum.1=TestString1", "args.positionals.0.enum.2=Test2")
	add("get-fook", "args.positionals.0.name=p", "args.positionals.0.required=true",
		"args.positionals.0.enum.0=A", "args.positionals.0.enum.1=B", "args.positionals.0.enum.2=C")
	add("test-singleintparameter", "args.flags.0.name=Parameter1", "args.flags.0.type=int")
	add("get-foo", "args.flags.0.name=b", "args.positionals.0.name=a", "args.positionals.0.required=true")
	add("get-fooa", "args.positionals.0.name=n", "args.positionals.0.type=int", "args.positionals.0.default=007")
	add("sw", "args.flags.0.name=Parameter1", "args.flags.0.type=bool")

	cases := []struct {
		src     string
		wantOut string
		wantErr []string
	}{
		{"paint --mode slow -c 007 blue", "entered|--mode=slow|--count=7|blue|3\n", nil},
		{"paint --count=2 --verbose red", "entered|--mode=fast|--count=2|--verbose=true|red|3\n", nil},
		{"paint red; echo rc=$?", "rc=2\n", []string{"missing required flag --count"}},
		{"paint --count 1 green; echo rc=$?", "rc=2\n", []string{`color must be one of red, blue, got "green"`}},
		{"paint --count nope red; echo rc=$?", "rc=2\n", []string{`--count expects int, got "nope"`}},
		{"legacy0 --count nope -x --mode=slow -- green extra", "entered|--count|nope|-x|--mode=slow|--|green|extra\n", nil},
		{"get-testvalidatesetps4 TestString1", "entered|TestString1\n", nil},
		{"get-testvalidatesetps4 TestStringWrong; echo rc=$?", "rc=2\n", []string{`Param1 must be one of Test1, TestString1, Test2, got "TestStringWrong"`}},
		{"get-fook 2; echo rc=$?", "rc=2\n", []string{`p must be one of A, B, C, got "2"`}},
		{"test-singleintparameter --Parameter1 exampleInvalidParam; echo rc=$?", "rc=2\n", []string{"Parameter1", `"exampleInvalidParam"`}},
		{"get-foo --b d c", "entered|--b=d|c\n", nil},
		{"get-foo c --b d", "entered|--b=d|c\n", nil},
		{"get-fooa", "entered|7\n", nil},
		{"sw", "entered\n", nil},
		{"sw --Parameter1", "entered|--Parameter1=true\n", nil},
	}
	for _, mode := range [][]string{{"-c"}, {"--posix", "-c"}} {
		for _, tc := range cases {
			args := append(append([]string(nil), mode...), tc.src)
			stdout, stderr, _ := runBashyStdEnv(bin, env, args...)
			if stdout != tc.wantOut {
				t.Errorf("%v: stdout=%q want %q (stderr=%q)", args, stdout, tc.wantOut, stderr)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(stderr, want) {
					t.Errorf("%v: stderr=%q want substring %q", args, stderr, want)
				}
			}
		}
	}
	if _, _, code := runBashyStdEnv(bin, env, "-c", "paint red"); code != 2 {
		t.Errorf("refused call as the last command: exit %d, want 2", code)
	}
	// The schema is a persisted fact of the record.
	if stdout, _, code := runBashyStdEnv(bin, env, "commands", "show", "paint", "--field", "args.flags.1.type"); code != 0 || strings.TrimSpace(stdout) != "int" {
		t.Errorf("show --field args.flags.1.type: exit %d stdout=%q", code, stdout)
	}
	// An invalid schema is refused when written (yoke's write-time half),
	// naming the set in declaration order; the ring does not gain the record.
	_, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "get-fook-bad", "--set", "script=true", "--set", "effects.0=pure",
		"--set", "args.flags.0.name=p", "--set", "args.flags.0.default=2",
		"--set", "args.flags.0.enum.0=A", "--set", "args.flags.0.enum.1=B", "--set", "args.flags.0.enum.2=C")
	if code == 0 || !strings.Contains(stderr, `"2" is not one of A|B|C`) {
		t.Errorf("add with an invalid schema: exit %d stderr=%q", code, stderr)
	}
	if _, _, code := runBashyStdEnv(bin, env, "-c", "command -v get-fook-bad"); code != 1 {
		t.Errorf("a refused record must not be in the ring: exit %d, want 1", code)
	}
	// A certification run sees neither the record nor its schema.
	if _, _, code := runBashyStdEnv(bin, append(env, "VSC_PROFILE=cert"), "--posix", "-c", "paint red"); code != 127 {
		t.Errorf("cert profile: exit %d, want 127", code)
	}
}

// A download record provisions on first use from the record's own digest,
// hits the cache afterwards, and is refused — nothing cached — when the
// digest is wrong or absent.
func TestE2ERegisteredDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the served artifact is a #!/bin/sh script")
	}
	bin := bashyBinary(t)
	env := registryEnv(t)
	body := []byte("#!/bin/sh\nprintf 'dl:%s\\n' \"$1\"\n")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tool" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Write(body)
	}))
	defer srv.Close()
	plat := binmgr.Platform()

	// No digest → add refuses, nothing is fetched.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dl0", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1"); code == 0 || !strings.Contains(stderr, "sha256") {
		t.Errorf("add without digest: exit %d stderr=%s", code, stderr)
	}
	if hits.Load() != 0 {
		t.Fatalf("add fetched %d times; a record is validated offline", hits.Load())
	}
	// Wrong digest → refused at first use, exit 126, nothing cached.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dlbad", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1", "--set", "download.sha256."+plat+"="+strings.Repeat("0", 64)); code != 0 {
		t.Fatalf("add dlbad (exit %d): %s", code, stderr)
	}
	if _, stderr, code := runBashyStdEnv(bin, env, "-c", "dlbad x"); code != 126 || !strings.Contains(strings.ToLower(stderr), "sha256") {
		t.Errorf("wrong digest: exit %d stderr=%s", code, stderr)
	}
	// Right digest → provisioned once, then served from the cache.
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "dl1", "--set", "download.url="+srv.URL+"/{version}/tool", "--set", "download.version=v1", "--set", "download.sha256."+plat+"="+digest); code != 0 {
		t.Fatalf("add dl1 (exit %d): %s", code, stderr)
	}
	before := hits.Load()
	stdout, stderr, code := runBashyStdEnv(bin, env, "-c", "dl1 first")
	if code != 0 || strings.TrimSpace(stdout) != "dl:first" {
		t.Fatalf("first run: exit %d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, _, code = runBashyStdEnv(bin, env, "dl1", "second")
	if code != 0 || strings.TrimSpace(stdout) != "dl:second" {
		t.Fatalf("second run (front door): exit %d stdout=%q", code, stdout)
	}
	if got := hits.Load() - before; got != 1 {
		t.Errorf("server hits across two runs = %d, want 1 (cache)", got)
	}
	// The record's os defaults to the platforms it carries a digest for.
	stdout, _, _ = runBashyStdEnv(bin, env, "commands", "show", "dl1", "--json")
	var rec map[string]any
	if err := json.Unmarshal([]byte(stdout), &rec); err != nil {
		t.Fatal(err)
	}
	if osList, _ := rec["os"].([]any); len(osList) != 1 || osList[0] != runtime.GOOS {
		t.Errorf("os = %v, want [%s]", rec["os"], runtime.GOOS)
	}
	// verify names the provisioned path.
	if stdout, _, code := runBashyStdEnv(bin, env, "commands", "verify", "dl1"); code != 0 || !strings.Contains(stdout, "provisioned") {
		t.Errorf("verify dl1: exit %d %s", code, stdout)
	}
	// The scratch cache holds exactly the good binary.
	cacheDir := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "BASHY_BIN_CACHE=") {
			cacheDir = strings.TrimPrefix(kv, "BASHY_BIN_CACHE=")
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "dl1", "v1", "dl1")); err != nil {
		t.Errorf("good binary not cached: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "dlbad", "v1", "dlbad")); err == nil {
		t.Error("a refused download left an executable in the cache")
	}
}

// What dispatch runs, introspection must see: a registered name answers
// `type`, `command -v` and `command -V` (in-shell, --posix included), where
// before Sprint 195 it RAN while `command -v` said not found. The file
// questions stay file questions (`type -P`, `hash`), and a certification run
// sees nothing.
func TestE2ERegisteredIntrospection(t *testing.T) {
	bin := bashyBinary(t)
	env := registryEnv(t)
	if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", "hi0", "--set", "script=echo hi", "--set", "effects.0=pure", "--set", "aliases.0=hi0a"); code != 0 {
		t.Fatalf("add (exit %d): %s", code, stderr)
	}
	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"-c", "type hi0"}, "hi0 is a bashy registered command (script)\n", 0},
		{[]string{"-c", "type -t hi0"}, "file\n", 0},
		{[]string{"-c", "command -v hi0"}, "hi0\n", 0},
		{[]string{"-c", "command -V hi0"}, "hi0 is a bashy registered command (script)\n", 0},
		{[]string{"-c", "type hi0a"}, "hi0a is a bashy registered command (script, alias of hi0)\n", 0},
		{[]string{"--posix", "-c", "command -v hi0 && type -t hi0"}, "hi0\nfile\n", 0},
		// The idiom every portable script uses before calling a command.
		{[]string{"-c", "command -v hi0 >/dev/null 2>&1 && hi0"}, "hi\n", 0},
		// File questions: no path to print, PATH search stays PATH search.
		{[]string{"-c", "type -p hi0; echo rc=$?"}, "rc=0\n", 0},
		{[]string{"-c", "type -P hi0"}, "", 1},
		{[]string{"-c", "command -v nosuch0"}, "", 1},
	}
	for _, tc := range cases {
		stdout, stderr, code := runBashyStdEnv(bin, env, tc.args...)
		if code != tc.code || stdout != tc.want {
			t.Errorf("%v: exit %d (want %d) stdout=%q (want %q) stderr=%q", tc.args, code, tc.code, stdout, tc.want, stderr)
		}
	}
	if stdout, _, code := runBashyStdEnv(bin, append(env, "VSC_PROFILE=cert"), "--posix", "-c", "command -v hi0"); code != 1 || stdout != "" {
		t.Errorf("cert profile: exit %d stdout=%q, want 1 and nothing", code, stdout)
	}
	if _, _, code := runBashyStdEnv(bin, env, "commands", "rm", "hi0"); code != 0 {
		t.Fatal("rm")
	}
	if _, _, code := runBashyStdEnv(bin, env, "-c", "command -v hi0"); code != 1 {
		t.Errorf("after rm: exit %d, want 1", code)
	}
}

// `--view shipped` and `--view registered` are the two sides of one split,
// in text and JSON: shipped never carries a registered record, registered
// carries nothing else, the footer counts agree, and the origin view's
// registered block title is printed once.
func TestE2ECommandsViewShippedRegistered(t *testing.T) {
	bin := bashyBinary(t)
	env := registryEnv(t)
	type envelope struct {
		View     string            `json:"view"`
		Filter   map[string]string `json:"filter"`
		Commands []struct {
			Name   string `json:"name"`
			Origin string `json:"origin"`
		} `json:"commands"`
	}
	decode := func(t *testing.T, view string) envelope {
		t.Helper()
		stdout, stderr, code := runBashyStdEnv(bin, env, "commands", "--view", view, "--json")
		if code != 0 {
			t.Fatalf("--view %s --json: exit %d: %s", view, code, stderr)
		}
		var e envelope
		if err := json.Unmarshal([]byte(stdout), &e); err != nil {
			t.Fatalf("--view %s --json: %v", view, err)
		}
		return e
	}
	// Empty ring first.
	if stdout, _, code := runBashyStdEnv(bin, env, "commands", "--view", "registered"); code != 0 || !strings.Contains(stdout, "registered — yours (0)") {
		t.Errorf("empty --view registered: exit %d stdout=%q", code, stdout)
	}
	if e := decode(t, "registered"); len(e.Commands) != 0 || e.Filter["shipped"] != "false" {
		t.Errorf("empty --view registered --json: %d commands, filter %v", len(e.Commands), e.Filter)
	}
	for _, n := range []string{"vw1", "vw2"} {
		if _, stderr, code := runBashyStdEnv(bin, env, "commands", "add", n, "--set", "script=echo "+n, "--set", "effects.0=pure"); code != 0 {
			t.Fatalf("add %s (exit %d): %s", n, code, stderr)
		}
	}
	shipped, registered := decode(t, "shipped"), decode(t, "registered")
	if shipped.Filter["shipped"] != "true" || registered.Filter["shipped"] != "false" {
		t.Errorf("filters: shipped=%v registered=%v", shipped.Filter, registered.Filter)
	}
	for _, c := range shipped.Commands {
		if c.Origin == "registered" {
			t.Errorf("--view shipped carries registered %q", c.Name)
		}
	}
	var names []string
	for _, c := range registered.Commands {
		if c.Origin != "registered" {
			t.Errorf("--view registered carries shipped %q (%s)", c.Name, c.Origin)
		}
		names = append(names, c.Name)
	}
	if strings.Join(names, " ") != "vw1 vw2" {
		t.Errorf("--view registered names = %v", names)
	}
	if len(shipped.Commands) == 0 {
		t.Error("--view shipped is empty")
	}
	text, _, _ := runBashyStdEnv(bin, env, "commands", "--view", "shipped")
	if !strings.Contains(text, "shipped — every command bashy ships") || !strings.Contains(text, "registered — yours, not shipped (2)") || strings.Contains(text, "vw1") {
		t.Errorf("--view shipped text:\n%s", text)
	}
	text, _, _ = runBashyStdEnv(bin, env, "commands", "--view", "registered")
	if !strings.Contains(text, "(2; ~ = hidden)") || !strings.Contains(text, "vw1 vw2") {
		t.Errorf("--view registered text:\n%s", text)
	}
	text, _, _ = runBashyStdEnv(bin, env, "commands", "--view", "origin")
	if strings.Contains(text, "registered — registered") || !strings.Contains(text, "  registered — added with bashy commands add (2):") {
		t.Errorf("--view origin registered block title:\n%s", text)
	}
	help, _, _ := runBashyStdEnv(bin, env, "commands", "--help")
	if !strings.Contains(help, "| shipped | registered") {
		t.Errorf("--help does not list the views:\n%s", help)
	}
}
