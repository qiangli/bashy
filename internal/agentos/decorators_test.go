// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// decorators_test.go — positive evidence for the native Bash++ decorators
// (trace · guard · retry) and the registration-time advice wiring, through
// the same wireExec seam the shell uses:
//
//   - trace: a real span in an in-memory exporter, with the status and the
//     principal, and NO argument values;
//   - guard: the denied body command never runs and the audit ledger carries
//     Decision "deny" — with and without an audit writer;
//   - retry: the actual Next count, bounded, and cancellation-aware;
//   - advice: file-ordered raw-valued specs with stable IDs, applied
//     outermost, idempotent across eval-redefinition, never loaded under
//     cert or outside Bash++, absent from --posix wiring.
package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/lower/shellrt"
	"mvdan.cc/sh/v3/lower/shellrt/shellexec"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/policy/audit"
)

// runDecorated runs script through a runner wired exactly as wireExec wires
// the agentic shell, in the given language variant.
//
// The skills store is pointed at a private directory unless the test names
// one: a completed decorated call attests into the store (attest.go), and a
// test must never append to the developer's own ledger.
func runDecorated(t *testing.T, ctx context.Context, lang syntax.LangVariant, script string, env map[string]string) (error, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	environ := os.Environ()
	if _, ok := env["BASHY_SKILLS_DIR"]; !ok {
		env = maps.Clone(env)
		if env == nil {
			env = map[string]string{}
		}
		env["BASHY_SKILLS_DIR"] = t.TempDir()
	}
	for k, v := range env {
		t.Setenv(k, v)
		environ = append(environ, k+"="+v)
	}
	prog, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader(script), "decorators_test.sh")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := interp.New(interp.Lang(lang), interp.Env(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range wireExec(nil, false, environ, nil, out, errOut, false) {
		if err := opt(runner); err != nil {
			t.Fatal(err)
		}
	}
	return runner.Run(ctx, prog), out, errOut
}

// recordSpans installs an in-memory exporter as the GLOBAL provider — the one
// the trace decorator must emit through — for the duration of the test.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func callSpans(sr *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s.Name() == "call "+name {
			out = append(out, s)
		}
	}
	return out
}

func spanAttr(s sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String(), true
		}
	}
	return "", false
}

func TestNativeTraceDecoratorSpan(t *testing.T) {
	sr := recordSpans(t)
	script := `@trace()
function greet() {
	echo "hi $1"
}
greet "s3cretvalue"
`
	err, out, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{
		"BASHY_PRINCIPAL": "dhnt:agent/test",
	})
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !strings.Contains(out.String(), "hi s3cretvalue") {
		t.Fatalf("body did not run: %q", out.String())
	}
	spans := callSpans(sr, "greet")
	if len(spans) != 1 {
		t.Fatalf("want 1 'call greet' span, got %d", len(spans))
	}
	s := spans[0]
	for want, value := range map[string]string{
		"call.name":       "greet",
		"call.status":     "0",
		"call.argc":       "1",
		"agent.principal": "dhnt:agent/test",
	} {
		if got, ok := spanAttr(s, want); !ok || got != value {
			t.Errorf("attr %s = %q (present=%v), want %q", want, got, ok, value)
		}
	}
	if _, ok := spanAttr(s, "call.advised"); ok {
		t.Error("source decorator must not carry call.advised")
	}
	// The argument VALUE must never reach the span — only the count does.
	for _, kv := range s.Attributes() {
		if strings.Contains(kv.Value.String(), "s3cretvalue") {
			t.Errorf("argument value leaked into span attribute %s", kv.Key)
		}
	}
}

func TestNativeTraceDecoratorFailureStatus(t *testing.T) {
	sr := recordSpans(t)
	script := `@trace()
function boom() { return 7; }
boom
`
	err, _, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if status, ok := exitStatusOf(err); !ok || status != 7 {
		t.Fatalf("want exit 7, got %v", err)
	}
	spans := callSpans(sr, "boom")
	if len(spans) != 1 {
		t.Fatalf("want 1 span, got %d", len(spans))
	}
	if got, _ := spanAttr(spans[0], "call.status"); got != "7" {
		t.Fatalf("call.status = %q, want 7", got)
	}
}

// guardDeniesTouch runs a guarded function whose body touches marker, and
// asserts the deny: non-zero exit and no marker file — the body command never
// executed.
func guardDeniesTouch(t *testing.T, env map[string]string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "marker")
	if env == nil {
		env = map[string]string{}
	}
	env["GUARD_MARKER"] = marker
	script := `@guard(effects: "read")
function fetchit() {
	touch "$GUARD_MARKER"
}
fetchit
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, env)
	if status, ok := exitStatusOf(err); !ok || status == 0 {
		t.Fatalf("want denied non-zero exit, got %v (stderr %q)", err, errOut.String())
	}
	if strings.Contains(errOut.String(), "EDECO") {
		t.Fatalf("guard itself failed instead of denying the command: %q", errOut.String())
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("denied command ran: marker exists (stat err %v)", statErr)
	}
}

func TestNativeGuardDecoratorDeniesAndAudits(t *testing.T) {
	auditLog := filepath.Join(t.TempDir(), "audit.jsonl")
	guardDeniesTouch(t, map[string]string{"BASHY_AUDIT": auditLog})

	f, err := os.Open(auditLog)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	denied := false
	for dec.More() {
		var rec audit.Record
		if err := dec.Decode(&rec); err != nil {
			t.Fatal(err)
		}
		if rec.Binary == "touch" {
			if rec.Decision != "deny" {
				t.Fatalf("touch decision = %q, want deny", rec.Decision)
			}
			denied = true
		}
	}
	if !denied {
		t.Fatal("no audit record for the denied touch")
	}
}

func TestNativeGuardDeniesWithoutAuditWriter(t *testing.T) {
	// Caps are enforced even with auditing off (nil writer).
	guardDeniesTouch(t, map[string]string{"BASHY_AUDIT": "0"})
}

func TestNativeGuardAllowsWithinCap(t *testing.T) {
	script := `@guard(effects: "read,write")
function fine() {
	echo ok
}
fine
`
	err, out, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatalf("allowed run failed: %v", err)
	}
	if !strings.Contains(out.String(), "ok") {
		t.Fatalf("body did not run: %q", out.String())
	}
}

func TestNativeRetryDecoratorAttempts(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	script := `@retry(n: 3, backoff: "1ms")
function flaky() {
	echo x >> "$RETRY_COUNT"
	return 1
}
flaky
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"RETRY_COUNT": count})
	if status, ok := exitStatusOf(err); !ok || status != 1 {
		t.Fatalf("want exit 1 after exhausted retries, got %v (stderr %q)", err, errOut.String())
	}
	data, readErr := os.ReadFile(count)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.Count(string(data), "x"); got != 3 {
		t.Fatalf("body ran %d times, want 3", got)
	}
}

func TestNativeRetryDecoratorStopsOnSuccess(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	script := `@retry(n: 5, backoff: "1ms")
function steady() {
	echo x >> "$RETRY_COUNT"
}
steady
`
	err, _, _ := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"RETRY_COUNT": count})
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	data, readErr := os.ReadFile(count)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.Count(string(data), "x"); got != 1 {
		t.Fatalf("body ran %d times, want 1", got)
	}
}

func TestNativeRetryDecoratorCancellation(t *testing.T) {
	count := filepath.Join(t.TempDir(), "count")
	script := `@retry(n: 5, backoff: "30s")
function flaky() {
	echo x >> "$RETRY_COUNT"
	return 1
}
flaky
`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, _, _ = func() (error, *bytes.Buffer, *bytes.Buffer) {
		return runDecorated(t, ctx, syntax.LangBashPP, script, map[string]string{"RETRY_COUNT": count})
	}()
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("cancellation did not stop the backoff wait: %v", elapsed)
	}
	data, readErr := os.ReadFile(count)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.Count(string(data), "x"); got != 1 {
		t.Fatalf("body ran %d times after cancellation, want 1", got)
	}
}

func writeAdviceRules(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "advice.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const orderedRules = `{"schema":"bashy-advice-v1","rules":[
  {"id":"t1","name":"work*","decorator":"trace"},
  {"id":"g1","name":"work*","decorator":"guard","args":{"effects":"read,net"}}
]}`

func TestAdviceCallbackSpecsRawAndOrdered(t *testing.T) {
	path := writeAdviceRules(t, orderedRules)
	warn := new(bytes.Buffer)
	cb := newAdviceCallback([]string{"BASHY_ADVICE=" + path}, warn, false)

	specs := cb("worker", "job.sh", false)
	if len(specs) != 2 {
		t.Fatalf("want 2 specs, got %d (warn %q)", len(specs), warn.String())
	}
	// File order — the deterministic outermost-first application order.
	if specs[0].ID != "t1" || specs[0].Name != "trace" || specs[1].ID != "g1" || specs[1].Name != "guard" {
		t.Fatalf("unexpected order/ids: %+v", specs)
	}
	// RAW native value — advice.Value.String() would quote it, and ParseCap
	// rejects a quoted "read,net".
	if len(specs[1].Args) != 1 || specs[1].Args[0].Name != "effects" || specs[1].Args[0].Value != "read,net" {
		t.Fatalf("guard args not raw: %+v", specs[1].Args)
	}
	// Stable across calls (re-registration keeps the same IDs, which is what
	// the engine's dedupe keys on).
	again := cb("worker", "job.sh", false)
	if len(again) != 2 || again[0].ID != specs[0].ID || again[1].ID != specs[1].ID {
		t.Fatalf("ids not stable: %+v vs %+v", specs, again)
	}
	// No selector match, no specs.
	if got := cb("other", "job.sh", false); got != nil {
		t.Fatalf("unmatched name advised %+v", got)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestAdviceAppliedOutermostAndIdempotent(t *testing.T) {
	sr := recordSpans(t)
	path := writeAdviceRules(t, `{"schema":"bashy-advice-v1","rules":[
  {"id":"t1","name":"work","decorator":"trace"}]}`)
	// The source decorator plus the advised one; eval re-registers, and the
	// stable rule id must not stack a duplicate.
	script := `@trace()
function work() { echo one; }
eval '@trace()
function work() { echo two; }'
work
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{"BASHY_ADVICE": path})
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "two") {
		t.Fatalf("redefinition did not take: %q", out.String())
	}
	spans := callSpans(sr, "work")
	if len(spans) != 2 {
		t.Fatalf("want exactly 2 'call work' spans (1 advised + 1 source, no duplicates), got %d", len(spans))
	}
	var advised, source sdktrace.ReadOnlySpan
	for _, s := range spans {
		if id, ok := spanAttr(s, "call.advised"); ok {
			if id != "t1" {
				t.Fatalf("call.advised = %q, want t1", id)
			}
			advised = s
		} else {
			source = s
		}
	}
	if advised == nil || source == nil {
		t.Fatal("expected one advised and one source span")
	}
	// Outermost: the advised span is the PARENT of the author's span — advice
	// never interposes between an author's decorator and the body.
	if source.Parent().SpanID() != advised.SpanContext().SpanID() {
		t.Fatal("advised trace is not outermost")
	}
}

func TestAdviceAdvisedGuardDenies(t *testing.T) {
	// End to end through registration-time advice: the rule's raw effects
	// value must reach ParseCap unquoted, and the deny must land in the audit
	// ledger.
	path := writeAdviceRules(t, `{"schema":"bashy-advice-v1","rules":[
  {"id":"g1","name":"fetchit","decorator":"guard","args":{"effects":"read"}}]}`)
	auditLog := filepath.Join(t.TempDir(), "audit.jsonl")
	marker := filepath.Join(t.TempDir(), "marker")
	script := `function fetchit() {
	touch "$GUARD_MARKER"
}
fetchit
`
	err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, map[string]string{
		"BASHY_ADVICE": path,
		"BASHY_AUDIT":  auditLog,
		"GUARD_MARKER": marker,
	})
	if status, ok := exitStatusOf(err); !ok || status == 0 {
		t.Fatalf("want denied non-zero exit, got %v (stderr %q)", err, errOut.String())
	}
	if strings.Contains(errOut.String(), "EDECO") {
		t.Fatalf("advised guard failed instead of denying (quoted value?): %q", errOut.String())
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatal("denied command ran under advised guard")
	}
	data, readErr := os.ReadFile(auditLog)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"deny"`) {
		t.Fatal("no deny decision in the audit ledger")
	}
}

func TestAdviceCertNeverOpensFile(t *testing.T) {
	// Under cert the configured file — even an invalid one — is never opened:
	// no warning, no rules, and the file stays untouched.
	garbage := writeAdviceRules(t, `{not json`)
	warn := new(bytes.Buffer)
	cb := newAdviceCallback([]string{"VSC_PROFILE=cert", "BASHY_ADVICE=" + garbage}, warn, false)
	if got := cb("work", "job.sh", false); got != nil {
		t.Fatalf("cert advised %+v", got)
	}
	if warn.Len() != 0 {
		t.Fatalf("cert opened/parsed the rules file: %q", warn.String())
	}
	// A path that cannot be opened at all is equally invisible under cert.
	cb = newAdviceCallback([]string{"VSC_PROFILE=cert", "BASHY_ADVICE=" + filepath.Join(t.TempDir(), "missing.json")}, warn, false)
	if got := cb("work", "job.sh", false); got != nil || warn.Len() != 0 {
		t.Fatalf("cert touched a missing rules file: %+v %q", got, warn.String())
	}
}

func TestAdviceBrokenFileWarnsOnceAndRefusesCalls(t *testing.T) {
	garbage := writeAdviceRules(t, `{not json`)
	warn := new(bytes.Buffer)
	cb := newAdviceCallback([]string{"BASHY_ADVICE=" + garbage}, warn, false)
	if got := cb("work", "job.sh", false); len(got) != 1 || got[0].Name != "__bashy_invalid_advice" {
		t.Fatalf("broken file failed open: %+v", got)
	}
	if !strings.Contains(warn.String(), "advice") {
		t.Fatalf("a configured policy failed silently: %q", warn.String())
	}
	before := warn.Len()
	_ = cb("more", "job.sh", false)
	if warn.Len() != before {
		t.Fatalf("warned more than once: %q", warn.String())
	}
}

func TestAdviceNotLoadedOutsideBashPP(t *testing.T) {
	// Plain Bash: the engine never consults advice, so a garbage rules file
	// is never opened — no warning, and the script runs.
	garbage := writeAdviceRules(t, `{not json`)
	script := "f() { echo plain; }\nf\n"
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBash, script, map[string]string{"BASHY_ADVICE": garbage})
	if err != nil {
		t.Fatalf("plain bash run failed: %v", err)
	}
	if !strings.Contains(out.String(), "plain") {
		t.Fatalf("function did not run: %q", out.String())
	}
	if strings.Contains(errOut.String(), "advice") {
		t.Fatalf("advice loaded outside Bash++: %q", errOut.String())
	}
}

func TestPosixWireExecCarriesNoDecoratorWiring(t *testing.T) {
	// The posix branch returns before the decorator/advice options: a posix
	// shell with a garbage BASHY_ADVICE registers and runs functions with no
	// advice load and no warning.
	garbage := writeAdviceRules(t, `{not json`)
	t.Setenv("BASHY_ADVICE", garbage)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	prog, err := syntax.NewParser().Parse(strings.NewReader("f() { echo posix; }\nf\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	runner, err := interp.New(interp.Env(nil), interp.Params("-o", "posix"))
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range wireExec(nil, true, os.Environ(), nil, out, errOut, false) {
		if err := opt(runner); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Run(context.Background(), prog); err != nil {
		t.Fatalf("posix run failed: %v", err)
	}
	if !strings.Contains(out.String(), "posix") {
		t.Fatalf("function did not run: %q", out.String())
	}
	if strings.Contains(errOut.String(), "advice") {
		t.Fatalf("advice active under --posix: %q", errOut.String())
	}
}

func TestRetryPolicyBounds(t *testing.T) {
	if _, _, err := retryPolicy([]interp.DecoratorArg{{Name: "n", Value: "0"}}); err == nil {
		t.Error("n: 0 accepted")
	}
	if _, _, err := retryPolicy([]interp.DecoratorArg{{Name: "n", Value: "101"}}); err == nil {
		t.Error("n over the attempt limit accepted")
	}
	if _, _, err := retryPolicy([]interp.DecoratorArg{{Name: "backoff", Value: "-1s"}}); err == nil {
		t.Error("negative backoff accepted")
	}
	if _, _, err := retryPolicy([]interp.DecoratorArg{{Name: "bogus", Value: "1"}}); err == nil {
		t.Error("unknown argument accepted")
	}
	// Defaults: autoretry's attempt count, escalating backoff.
	attempts, fixed, err := retryPolicy(nil)
	if err != nil || attempts != 3 || fixed >= 0 {
		t.Errorf("defaults = (%d, %v, %v)", attempts, fixed, err)
	}
	// Positional form matches the documented signature retry(c, n, backoff).
	attempts, fixed, err = retryPolicy([]interp.DecoratorArg{{Value: "2"}, {Value: "5ms"}})
	if err != nil || attempts != 2 || fixed != 5*time.Millisecond {
		t.Errorf("positional = (%d, %v, %v)", attempts, fixed, err)
	}
}

func TestRetryRefusesAdvisedApplication(t *testing.T) {
	// Defense in depth: the loader already refuses advised retry; the native
	// itself refuses too, for any other AdviceFunc an embedder installs.
	c := &interp.Call{Name: "f", Advised: "rogue-rule"}
	if err := adaptInterpreterDecorator(retryDecorator)(context.Background(), c, nil); err == nil {
		t.Fatal("advised retry accepted")
	}
}

func TestAdviceInvalidPolicyFailsClosed(t *testing.T) {
	for _, policy := range []string{writeAdviceRules(t, `{broken`), filepath.Join(t.TempDir(), "missing.json")} {
		err, out, diagnostic := runDecorated(t, context.Background(), syntax.LangBashPP, "func work() { echo escaped }\nwork()\n", map[string]string{"BASHY_ADVICE": policy, "VSC_PROFILE": ""})
		if err == nil || out.Len() != 0 || !strings.Contains(diagnostic.String(), "advised calls refused") {
			t.Fatalf("invalid policy escaped: err=%v out=%q stderr=%q", err, out, diagnostic)
		}
	}
}

func TestRetryRejectsDuplicateArguments(t *testing.T) {
	for _, args := range [][]interp.DecoratorArg{
		{{Name: "n", Value: "2"}, {Name: "n", Value: "3"}},
		{{Value: "2"}, {Name: "n", Value: "3"}},
		{{Name: "backoff", Value: "0s"}, {Name: "backoff", Value: "1s"}},
	} {
		if _, _, err := retryPolicy(args); err == nil {
			t.Fatalf("accepted duplicate: %v", args)
		}
	}
}

func TestNativeCompiledDecorators(t *testing.T) {
	sr := recordSpans(t)
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	writer, err := audit.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	handler := auditHandler(writer, audit.Actor{}, "test")(func(context.Context, []string) error { ran++; return nil })
	var out, diagnostic bytes.Buffer
	p, err := shellrt.NewProgram(shellrt.WithStdio(nil, &out, &diagnostic), shellrt.WithShellFactory(shellexec.New(shellexec.RunnerOptions(interp.ExecHandler(handler)))))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Session.Close()
	call := &shellrt.Call{Name: "work"}
	rungs := []shellrt.Decorator{
		{Name: "trace"},
		{Name: "guard", Args: func(*shellrt.Program) []shellrt.DecoratorArg {
			return []shellrt.DecoratorArg{{Name: "effects", Value: "read"}}
		}},
		{Name: "retry", Args: func(*shellrt.Program) []shellrt.DecoratorArg {
			return []shellrt.DecoratorArg{{Name: "n", Value: "2"}, {Name: "backoff", Value: "0s"}}
		}},
	}
	attempts := 0
	if !p.Decorate(call, rungs, func(region *shellrt.Program) error { attempts++; region.ShellRegion("touch forbidden"); return nil }) {
		t.Fatalf("chain failed: %s", &diagnostic)
	}
	if attempts != 2 || ran != 0 || call.Status == 0 {
		t.Fatalf("attempts=%d dispatched=%d status=%d stderr=%s", attempts, ran, call.Status, &diagnostic)
	}
	spans := callSpans(sr, "work")
	if len(spans) != 1 {
		t.Fatalf("trace spans=%d", len(spans))
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"decision":"deny"`) != 2 {
		t.Fatalf("missing denial records: %s", data)
	}
	// The guard context must not leak into the caller's next shell region.
	p.ShellRegion("touch allowed")
	if ran != 1 {
		t.Fatalf("caller remained guarded: dispatches=%d stderr=%s", ran, &diagnostic)
	}
}
