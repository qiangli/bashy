// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureInspectAdvice runs `bashy inspect advice ARGS…` with stdout captured.
func captureInspectAdvice(t *testing.T, args ...string) (string, int) {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer r.Close()
	os.Stdout = w
	var sb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&sb, r)
		done <- err
	}()
	code := dispatchInspect(append([]string{"advice"}, args...))
	w.Close()
	os.Stdout = prev
	if err := <-done; err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return sb.String(), code
}

// adviceJSON runs the aspect with --json and decodes the envelope plus the
// advice payload in one step.
func adviceJSON(t *testing.T) (inspectAdviceReport, string, int) {
	t.Helper()
	out, code := captureInspectAdvice(t, "--json")
	if code != 0 {
		return inspectAdviceReport{}, out, code
	}
	var doc struct {
		SchemaVersion string              `json:"schema_version"`
		Aspect        string              `json:"aspect"`
		Rows          inspectAdviceReport `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if doc.SchemaVersion != inspectSchemaVersion || doc.Aspect != "advice" {
		t.Fatalf("envelope = %s/%s\n%s", doc.SchemaVersion, doc.Aspect, out)
	}
	return doc.Rows, out, code
}

// writeAdviceFile writes a rules file into a scratch dir and returns its path.
func writeAdviceFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// clearAdviceSignals removes the two inputs the advice decision reads, so a
// case starts from the off-by-default state.
func clearAdviceSignals(t *testing.T) {
	t.Helper()
	for _, env := range []string{"BASHY_ADVICE", "VSC_PROFILE"} {
		t.Setenv(env, "")
		os.Unsetenv(env)
	}
}

const adviceValidFile = `{
  "schema": "bashy-advice-v1",
  "rules": [
    {"id": "trace-deploy", "name": "deploy_*", "decorator": "trace"},
    {"name": "*", "file": "src/*.sh", "decorator": "guard", "args": {"effects": "read,net"}},
    {"agentic": true, "decorator": "trace"},
    {"name": "boot_*", "exclude": [], "decorator": "trace"}
  ]
}`

// TestInspectAdviceOffDefault: BASHY_ADVICE unset is the opt-in default —
// state off, exit 0, and the payload says which signal decided it, in both
// formats. "Off" is an answer, not an error and not an empty rule list.
func TestInspectAdviceOffDefault(t *testing.T) {
	clearAdviceSignals(t)
	rep, _, code := adviceJSON(t)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (off is the default, not an error)", code)
	}
	if rep.State != "off" || len(rep.Rules) != 0 {
		t.Fatalf("report = %+v, want state off with no rules", rep)
	}
	if !strings.Contains(rep.DecidedBy, "BASHY_ADVICE") || !strings.Contains(rep.DecidedBy, "opt-in") {
		t.Errorf("decided_by=%q does not name the opt-in default", rep.DecidedBy)
	}
	out, code := captureInspectAdvice(t, "--plain")
	if code != 0 || !strings.Contains(out, "off") || !strings.Contains(out, "BASHY_ADVICE") {
		t.Errorf("plain (exit %d):\n%s", code, out)
	}
}

// TestInspectAdviceCertNeverOpensTheFile: under VSC_PROFILE=cert the decision
// is off BEFORE the file is ever opened — so a BASHY_ADVICE naming a
// nonexistent or invalid file still exits 0, state off, cert named as the
// decider. This is the property that lets a cert run not vary on a developer
// host's rules file.
func TestInspectAdviceCertNeverOpensTheFile(t *testing.T) {
	for name, path := range map[string]string{
		"nonexistent file": filepath.Join(t.TempDir(), "no-such-rules.json"),
		"invalid file":     writeAdviceFile(t, `{"schema": "bashy-advice-v1", "rules": [{"name": "*", "decorator": "retry"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("VSC_PROFILE", "cert")
			t.Setenv("BASHY_ADVICE", path)
			rep, _, code := adviceJSON(t)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (cert must not open the file)", code)
			}
			if rep.State != "off" || len(rep.Rules) != 0 {
				t.Fatalf("report = %+v, want state off with no rules", rep)
			}
			if !strings.Contains(rep.DecidedBy, "VSC_PROFILE=cert") {
				t.Errorf("decided_by=%q does not name cert", rep.DecidedBy)
			}
			if !strings.Contains(rep.DecidedBy, "never opened") {
				t.Errorf("decided_by=%q does not state that the file is never opened", rep.DecidedBy)
			}
			out, code := captureInspectAdvice(t, "--plain")
			if code != 0 || !strings.Contains(out, "cert") {
				t.Errorf("plain (exit %d):\n%s", code, out)
			}
		})
	}
	// Cert with BASHY_ADVICE unset is off the same way, decided by cert alone.
	t.Run("cert with advice unset", func(t *testing.T) {
		clearAdviceSignals(t)
		t.Setenv("VSC_PROFILE", "cert")
		rep, _, code := adviceJSON(t)
		if code != 0 || rep.State != "off" || !strings.Contains(rep.DecidedBy, "VSC_PROFILE=cert") {
			t.Fatalf("report = %+v (exit %d)", rep, code)
		}
	})
}

// TestInspectAdviceValidFile: a configured valid file reports every active
// rule — stable IDs (explicit and derived), the selectors, and the decorator
// args — in file order, in both formats.
func TestInspectAdviceValidFile(t *testing.T) {
	clearAdviceSignals(t)
	t.Setenv("BASHY_ADVICE", writeAdviceFile(t, adviceValidFile))
	rep, _, code := adviceJSON(t)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if rep.State != "on" || len(rep.Rules) != 4 {
		t.Fatalf("report = %s/%d rules, want on with 4", rep.State, len(rep.Rules))
	}
	if !strings.Contains(rep.DecidedBy, "BASHY_ADVICE=") {
		t.Errorf("decided_by=%q does not name the rules file", rep.DecidedBy)
	}
	// Rule 1: the explicit ID, a name selector, no args.
	r1 := rep.Rules[0]
	if r1.ID != "trace-deploy" || r1.Name != "deploy_*" || r1.File != "" || r1.Agentic != nil {
		t.Errorf("rule 1 = %+v", r1)
	}
	if r1.Decorator != "trace" || len(r1.Args) != 0 || r1.Spec != "@trace()" {
		t.Errorf("rule 1 spec = %q args = %+v", r1.Spec, r1.Args)
	}
	// Rule 2: the derived stable ID (r-<12 hex>), name+file selectors, the
	// guard arg with its decorator-line spelling.
	r2 := rep.Rules[1]
	if !strings.HasPrefix(r2.ID, "r-") || len(r2.ID) != 14 {
		t.Errorf("rule 2 id = %q, want a derived r-<12 hex> id", r2.ID)
	}
	if r2.Name != "*" || r2.File != "src/*.sh" {
		t.Errorf("rule 2 selectors = name %q file %q", r2.Name, r2.File)
	}
	if r2.Spec != `@guard(effects: "read,net")` || len(r2.Args) != 1 || r2.Args[0].Name != "effects" || r2.Args[0].Value != `"read,net"` {
		t.Errorf("rule 2 = spec %q args %+v", r2.Spec, r2.Args)
	}
	// Rule 3: the agentic selector reports its value.
	r3 := rep.Rules[2]
	if r3.Agentic == nil || !*r3.Agentic {
		t.Errorf("rule 3 agentic = %v, want true", r3.Agentic)
	}
	// Rule 4: "exclude": [] opts the preamble in — stated, not defaulted away.
	r4 := rep.Rules[3]
	if r4.PreambleExcluded || !r1.PreambleExcluded {
		t.Errorf("preamble flags: rule 4 = %v (want false, opted in), rule 1 = %v (want true, default)", r4.PreambleExcluded, r1.PreambleExcluded)
	}
	// Plain renders the same facts: IDs, selectors, decorator lines.
	out, code := captureInspectAdvice(t, "--plain")
	if code != 0 {
		t.Fatalf("plain exit = %d", code)
	}
	for _, want := range []string{"4 rules", "trace-deploy", "deploy_*", `@guard(effects: "read,net")`, "src/*.sh", "agentic=true", "preamble in scope"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain output lacks %q:\n%s", want, out)
		}
	}
}

// TestInspectAdviceInvalidFile: a configured file that fails to load is a
// nonzero error in both formats — a broken policy must be loud, never a
// silent off.
func TestInspectAdviceInvalidFile(t *testing.T) {
	clearAdviceSignals(t)
	for name, path := range map[string]string{
		"unparsable":                writeAdviceFile(t, `{"schema": "bashy-advice-v1", "rules": [`),
		"never-advisable decorator": writeAdviceFile(t, `{"schema": "bashy-advice-v1", "rules": [{"name": "*", "decorator": "memo"}]}`),
		"nonexistent":               filepath.Join(t.TempDir(), "gone.json"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("BASHY_ADVICE", path)
			for _, mode := range []string{"--json", "--plain"} {
				if _, code := captureInspectAdvice(t, mode); code == 0 {
					t.Errorf("%s: exit 0, want nonzero for a configured file that fails to load", mode)
				}
			}
		})
	}
}

// TestInspectAdviceDispatch: the aspect rides the shared argument handling —
// --help exits 0, an unknown option exits 2, and the unknown-aspect hint and
// the index both name it.
func TestInspectAdviceDispatch(t *testing.T) {
	clearAdviceSignals(t)
	if code := dispatchInspect([]string{"advice", "--help"}); code != 0 {
		t.Errorf("--help exited %d", code)
	}
	if code := dispatchInspect([]string{"advice", "--bogus"}); code != 2 {
		t.Errorf("unknown option exited %d, want 2", code)
	}
	for _, r := range inspectIndex {
		if strings.Contains(r.Command, "advice") && r.Verb != "inspect" {
			t.Errorf("index row %q names verb %q", r.Command, r.Verb)
		}
	}
	// Deterministic: two runs of the same configured state agree byte-for-byte.
	t.Setenv("BASHY_ADVICE", writeAdviceFile(t, adviceValidFile))
	a, _ := captureInspectAdvice(t, "--json")
	b, _ := captureInspectAdvice(t, "--json")
	if a != b {
		t.Errorf("two runs differ:\n%s\n%s", a, b)
	}
}
