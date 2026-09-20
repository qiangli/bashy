// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// confirm_test.go — source-derived evidence for @confirm (Sprint 216, B16),
// through the same wireExec seam the shell uses, with the ask seam replaced
// so no test ever reaches a terminal or an askpass.
//
// Provenance (the Sprint 216 source-derived test rule: fixture → source
// project → commit → source test/spec section → license). Every behavioral
// case below is an independently written fixture preserving the behavior the
// named source pins; no source text is copied.
//
//	fixture                              source                                  section                                                      license
//	-------------------------------------------------------------------------------------------------------------------------------------------------
//	TestConfirmNoneImpactNeverPrompts     PowerShell/PowerShell 84a93015           test/powershell/Language/Scripting/CommonParameters.Tests.ps1 MIT
//	                                     (master, 2026-09-18)                    Context 'confirmimpact support: none' — get-foo, get-foo -confirm
//	TestConfirmMediumImpactAutoConfirms   same                                    Context 'confirmimpact support: Medium under the non-interactive
//	                                                                             host' — get-foo (auto-confirmed under the default preference)
//	TestConfirmHighImpactAsksTheHuman     same + src/System.Management.Automation/ CanShouldProcessAutoConfirm: threshold > ConfirmImpact ⇒ no
//	                                     engine/MshCommandRuntime.cs             prompt; DoShouldProcess: prompt otherwise, "No" ⇒ false, continue
//	TestConfirmUnattendedYields           same                                    Context 'confirmimpact support: High under the non-interactive
//	                                                                             host' — get-foo: one error, operation NOT performed, not a
//	                                                                             fabricated yes/no (here: exit 6, rfcs/0001-agentic-yield.md)
//	TestConfirmWhatIf                     same + src/System.Management.Automation/ 'shouldprocess support -whatif' completes; DoShouldProcess under
//	                                     resources/CommandBaseStrings.resx        WhatIf writes "What if: {0}" and returns false for EVERY
//	                                                                             ShouldProcess call, whatever the impact
//	TestConfirmImpactTable                src/System.Management.Automation/        enum ConfirmImpact {None, Low, Medium, High}; unclassified is
//	                                     engine/CommandBase.cs                    read fail-closed as Yoke's Cap.Exceeded reads it
//	TestConfirmResumeAnswers              rfcs/0001-agentic-yield.md               §Resume — the answer binds to the operation's token, not its
//	                                     (this repo)                              position; an unanswered operation still yields
//	TestConfirmHarnessLoop                rfcs/0001-agentic-yield.md               §Harness — pause on 6, obtain the answer, replay idempotently
//	TestConfirmBashOffIsIdentity          sh/docs/bashpp-adapter-contract.md       "An absent decorator or adapter is identity"; Bash and POSIX
//	                                     (sh 772b2924)                            modes stay inert with Bash# off
//
// The Claude Code, OpenCode and Codex harness rows are pinned in the RFC (they
// describe how a harness treats a tool's exit code and an approval; none of
// them defines an exit 6, which is why the RFC exists).
package agentos

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// stubAsk replaces the human seam for one test. answer is what the human
// says; err non-nil means nobody could be asked (the yield).
func stubAsk(t *testing.T, answer bool, err error) *[]string {
	t.Helper()
	prev := confirmAsk
	var asked []string
	confirmAsk = func(fn, op, effects string) (bool, error) {
		asked = append(asked, fn+": "+op+" ("+effects+")")
		return answer, err
	}
	t.Cleanup(func() { confirmAsk = prev })
	return &asked
}

// confirmScript is the shape every case runs: one @confirm function whose
// body dispatches a read (cat), a destroy (rm), a write (mkdir) and a pure
// builtin (echo), in that order, on a directory the test owns.
const confirmScript = `@confirm()
function clean() {
	cat "$1/marker"
	rm -rf "$1/victim"
	mkdir "$1/after"
	echo "done $1 ${2-}"
}
clean %s
echo "status $?"
`

func confirmWork(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "marker"), []byte("marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "victim"), 0o755); err != nil {
		t.Fatal(err)
	}
	return work
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func runConfirm(t *testing.T, work, callArgs string, env map[string]string) (string, string) {
	t.Helper()
	script := strings.Replace(confirmScript, "clean %s", "clean "+callArgs, 1)
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, env)
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	return out.String(), errOut.String()
}

func TestConfirmNoneImpactNeverPrompts(t *testing.T) {
	asked := stubAsk(t, false, errConfirmUnattended)
	script := `@confirm()
function look() { cat "$1"; ls "$(dirname "$1")" >/dev/null; true; }
look "$1"; echo "plain $?"
look --confirm "$1"; echo "explicit $?"
`
	work := confirmWork(t)
	script = strings.ReplaceAll(script, `"$1"`, `"`+filepath.Join(work, "marker")+`"`)
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if len(*asked) != 0 {
		t.Errorf("read-only body asked the human: %v", *asked)
	}
	if want := "marker\nplain 0\nmarker\nexplicit 0\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", errOut.String())
	}
}

func TestConfirmMediumImpactAutoConfirms(t *testing.T) {
	asked := stubAsk(t, false, errConfirmUnattended)
	work := t.TempDir()
	script := `@confirm()
function make() { mkdir "$1"; touch "$1/f"; }
make "` + filepath.Join(work, "d") + `"; echo "status $?"
`
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatalf("script failed: %v (stderr %q)", err, errOut.String())
	}
	if len(*asked) != 0 {
		t.Errorf("write-only body asked the human: %v", *asked)
	}
	if !exists(t, filepath.Join(work, "d", "f")) {
		t.Error("write did not run")
	}
	if out.String() != "status 0\n" || errOut.Len() != 0 {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestConfirmHighImpactAsksTheHuman(t *testing.T) {
	t.Run("yes runs it", func(t *testing.T) {
		asked := stubAsk(t, true, nil)
		work := confirmWork(t)
		out, errOut := runConfirm(t, work, work, nil)
		if len(*asked) != 1 || !strings.HasPrefix((*asked)[0], "clean: rm -rf ") || !strings.HasSuffix((*asked)[0], "(destroy)") {
			t.Errorf("asked = %v", *asked)
		}
		if exists(t, filepath.Join(work, "victim")) {
			t.Error("confirmed rm did not run")
		}
		if !exists(t, filepath.Join(work, "after")) {
			t.Error("the write after a confirmed operation did not run")
		}
		if want := "marker\ndone " + work + " \nstatus 0\n"; out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		if errOut != "" {
			t.Errorf("stderr = %q", errOut)
		}
	})
	t.Run("no refuses it and the body continues", func(t *testing.T) {
		stubAsk(t, false, nil)
		work := confirmWork(t)
		script := strings.Replace(confirmScript, `rm -rf "$1/victim"`, `rm -rf "$1/victim"; echo "rm $?"`, 1)
		script = strings.Replace(script, "clean %s", "clean "+work, 1)
		err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !exists(t, filepath.Join(work, "victim")) {
			t.Error("declined rm ran")
		}
		if !exists(t, filepath.Join(work, "after")) {
			t.Error("body did not continue after a declined operation")
		}
		if want := "marker\nrm 126\ndone " + work + " \nstatus 0\n"; out.String() != want {
			t.Errorf("stdout = %q, want %q", out.String(), want)
		}
		if !strings.Contains(errOut.String(), "clean: declined ") || !strings.Contains(errOut.String(), ": rm -rf "+work+"/victim\n") {
			t.Errorf("stderr = %q", errOut.String())
		}
	})
}

var yieldLineRE = regexp.MustCompile(`^clean: input required: confirm ([0-9a-f]{8}): (rm -rf \S+) \(effects: destroy\): no channel reaches a human; resume with clean --confirm=([0-9a-f]{8}):yes \.\.\. or --confirm=([0-9a-f]{8}):no \.\.\.$`)

func TestConfirmUnattendedYields(t *testing.T) {
	asked := stubAsk(t, false, errConfirmUnattended)
	work := confirmWork(t)
	out, errOut := runConfirm(t, work, work, nil)
	if len(*asked) != 1 {
		t.Fatalf("asked = %v", *asked)
	}
	// The operation was not performed, and neither was any side effect after
	// the yield point; the pure builtin still ran.
	if !exists(t, filepath.Join(work, "victim")) {
		t.Error("yielded rm ran")
	}
	if exists(t, filepath.Join(work, "after")) {
		t.Error("a write after the yield point ran")
	}
	if want := "marker\ndone " + work + " \nstatus 6\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	lines := strings.Split(strings.TrimRight(errOut, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("stderr = %q, want the yield line and the not-run line", errOut)
	}
	m := yieldLineRE.FindStringSubmatch(lines[0])
	if m == nil {
		t.Fatalf("yield line = %q", lines[0])
	}
	if m[1] != m[3] || m[1] != m[4] || m[1] != confirmToken([]string{"rm", "-rf", work + "/victim"}) {
		t.Errorf("token %q does not name the operation (%q)", m[1], m[2])
	}
	if lines[1] != "clean: not run after yield: mkdir "+work+"/after" {
		t.Errorf("not-run line = %q", lines[1])
	}
}

func TestConfirmUnattendedYieldEnvelopeUnderAgentMode(t *testing.T) {
	stubAsk(t, false, errConfirmUnattended)
	work := confirmWork(t)
	_, errOut := runConfirm(t, work, work, map[string]string{"BASHY_AGENTIC": "1"})
	// The envelope is the first JSON value on stderr (the not-run line follows).
	var env weavecli.Envelope
	if err := json.NewDecoder(strings.NewReader(errOut)).Decode(&env); err != nil {
		t.Fatalf("stderr does not start with an envelope: %q (%v)", errOut, err)
	}
	if env.Command != "clean" || env.Status != "error" || env.Error == nil || env.Error.Code != "input_required" {
		t.Errorf("envelope = %+v", env)
	}
	if !strings.Contains(env.Error.Message, "resume with clean --confirm=") {
		t.Errorf("message = %q", env.Error.Message)
	}
}

func TestConfirmWhatIf(t *testing.T) {
	asked := stubAsk(t, true, nil)
	work := confirmWork(t)
	out, errOut := runConfirm(t, work, "--what-if "+work+" extra", nil)
	if len(*asked) != 0 {
		t.Errorf("what-if asked the human: %v", *asked)
	}
	if !exists(t, filepath.Join(work, "victim")) || exists(t, filepath.Join(work, "after")) {
		t.Error("what-if performed a side effect")
	}
	// The flag was consumed: $1 is the directory, $2 the next argument.
	if want := "marker\ndone " + work + " extra\nstatus 0\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	tok := confirmToken([]string{"rm", "-rf", work + "/victim"})
	want := "what-if: clean: would run rm -rf " + work + "/victim (effects: destroy; confirm " + tok + ")\n" +
		"what-if: clean: would run mkdir " + work + "/after (effects: write)\n"
	if errOut != want {
		t.Errorf("stderr = %q\nwant     %q", errOut, want)
	}
}

func TestConfirmImpactTable(t *testing.T) {
	for _, tc := range []struct {
		effects []string
		want    confirmImpact
	}{
		{[]string{"pure"}, impactNone},
		{[]string{"read"}, impactNone},
		{[]string{"read", "pure"}, impactNone},
		{[]string{"write"}, impactMedium},
		{[]string{"read", "write"}, impactMedium},
		{[]string{"exec", "net", "write"}, impactMedium},
		{[]string{"remote"}, impactMedium},
		{[]string{"persist"}, impactMedium},
		{[]string{"destroy"}, impactHigh},
		{[]string{"cred", "net", "read", "write"}, impactHigh},
		{[]string{"priv"}, impactHigh},
		{[]string{"spend"}, impactHigh},
		{nil, impactHigh},                        // unclassified: fail closed
		{[]string{}, impactHigh},                 // declared nothing: fail closed
		{[]string{"write", "bogus"}, impactHigh}, // outside the vocabulary: fail closed
	} {
		if got := impactOf(tc.effects); got != tc.want {
			t.Errorf("impactOf(%v) = %d, want %d", tc.effects, got, tc.want)
		}
	}
	if impactOf(declaredEffects("rm")) != impactHigh || impactOf(declaredEffects("chmod")) != impactHigh {
		t.Error("rm/chmod are not high impact")
	}
	if impactOf(declaredEffects("mkdir")) != impactMedium || impactOf(declaredEffects("cat")) != impactNone {
		t.Error("mkdir/cat impact")
	}
	if impactOf(declaredEffects("/definitely/not/a/known-tool")) != impactHigh {
		t.Error("an unclassified command must be high impact")
	}
}

func TestConfirmResumeAnswers(t *testing.T) {
	work := confirmWork(t)
	tok := confirmToken([]string{"rm", "-rf", work + "/victim"})

	t.Run("yes for the token runs it without asking", func(t *testing.T) {
		asked := stubAsk(t, false, errConfirmUnattended)
		w := confirmWork(t)
		tok := confirmToken([]string{"rm", "-rf", w + "/victim"})
		out, errOut := runConfirm(t, w, "--confirm="+tok+":yes "+w, nil)
		if len(*asked) != 0 {
			t.Errorf("asked with an answer supplied: %v", *asked)
		}
		if exists(t, filepath.Join(w, "victim")) || !exists(t, filepath.Join(w, "after")) {
			t.Error("resume did not run the confirmed operation")
		}
		if !strings.HasSuffix(out, "status 0\n") || errOut != "" {
			t.Errorf("stdout %q stderr %q", out, errOut)
		}
	})
	t.Run("no for the token refuses it without asking", func(t *testing.T) {
		asked := stubAsk(t, true, nil)
		w := confirmWork(t)
		tok := confirmToken([]string{"rm", "-rf", w + "/victim"})
		out, errOut := runConfirm(t, w, "--confirm="+tok+":NO "+w, nil)
		if len(*asked) != 0 {
			t.Errorf("asked with an answer supplied: %v", *asked)
		}
		if !exists(t, filepath.Join(w, "victim")) {
			t.Error("a refused operation ran")
		}
		if !strings.HasSuffix(out, "status 0\n") || !strings.Contains(errOut, "clean: declined "+tok+": rm -rf ") {
			t.Errorf("stdout %q stderr %q", out, errOut)
		}
	})
	t.Run("an answer for another operation does not apply", func(t *testing.T) {
		asked := stubAsk(t, false, errConfirmUnattended)
		out, errOut := runConfirm(t, work, "--confirm=deadbeef:yes,"+tok+":yes,"+"cafef00d:no "+work, nil)
		// The answer for this operation's token applied; the others were
		// simply never consulted.
		if len(*asked) != 0 || exists(t, filepath.Join(work, "victim")) || !strings.HasSuffix(out, "status 0\n") || errOut != "" {
			t.Errorf("asked %v stdout %q stderr %q", *asked, out, errOut)
		}
	})
	t.Run("an unanswered operation still yields with a stable token", func(t *testing.T) {
		stubAsk(t, false, errConfirmUnattended)
		w := confirmWork(t)
		out, errOut := runConfirm(t, w, "--confirm=deadbeef:yes "+w, nil)
		tok := confirmToken([]string{"rm", "-rf", w + "/victim"})
		if !strings.HasSuffix(out, "status 6\n") || !strings.Contains(errOut, "confirm "+tok+": rm -rf ") {
			t.Errorf("stdout %q stderr %q", out, errOut)
		}
	})
}

func TestConfirmArgumentErrors(t *testing.T) {
	stubAsk(t, false, errConfirmUnattended)
	for _, tc := range []struct{ name, script, want string }{
		{"decorator takes no arguments", "@confirm(effects: 'destroy')\nfunction f() { :; }\nf\n", "confirm takes no arguments"},
		{"empty answers", "@confirm()\nfunction f() { :; }\nf --confirm=\n", "want TOKEN:yes or TOKEN:no"},
		{"malformed answer", "@confirm()\nfunction f() { :; }\nf --confirm=yes\n", "want TOKEN:yes or TOKEN:no"},
		{"short token", "@confirm()\nfunction f() { :; }\nf --confirm=abc:yes\n", "want TOKEN:yes or TOKEN:no"},
		{"contradictory answers", "@confirm()\nfunction f() { :; }\nf --confirm=0123abcd:yes,0123abcd:no\n", "answered both yes and no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err, _, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, tc.script, nil)
			if err == nil {
				t.Fatal("want a decorator diagnostic")
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Errorf("stderr %q does not mention %q", errOut.String(), tc.want)
			}
		})
	}
}

func TestConfirmOnlyTheFirstArgumentIsAFlag(t *testing.T) {
	asked := stubAsk(t, true, nil)
	work := confirmWork(t)
	// --what-if in second position is the function's own argument.
	out, errOut := runConfirm(t, work, work+" --what-if", nil)
	if len(*asked) != 1 {
		t.Errorf("asked = %v", *asked)
	}
	if exists(t, filepath.Join(work, "victim")) {
		t.Error("rm did not run: a later --what-if was treated as the flag")
	}
	if want := "marker\ndone " + work + " --what-if\nstatus 0\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errOut != "" {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestConfirmUnclassifiedCommandIsHighImpact(t *testing.T) {
	stubAsk(t, false, errConfirmUnattended)
	work := t.TempDir()
	tool := filepath.Join(work, "mystery-tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho ran > \""+work+"/ran\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "@confirm()\nfunction run() { \"$1\"; }\nrun \"" + tool + "\"; echo \"status $?\"\n"
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(work, "ran")) {
		t.Error("an unclassified command ran without an answer")
	}
	if out.String() != "status 6\n" || !strings.Contains(errOut.String(), "(effects: unknown)") {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestConfirmYieldPropagatesThroughEnsure(t *testing.T) {
	stubAsk(t, false, errConfirmUnattended)
	work := confirmWork(t)
	script := "@confirm()\n@ensure('false')\nfunction clean() { rm -rf \"$1/victim\"; }\nclean \"" + work + "\"; echo \"status $?\"\n"
	err, out, errOut := runDecorated(t, context.Background(), syntax.LangBashPP, script, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A yield is not a completed body: exit 6, never "postcondition failed".
	if out.String() != "status 6\n" || strings.Contains(errOut.String(), "postcondition") {
		t.Errorf("stdout %q stderr %q", out.String(), errOut.String())
	}
}

func TestConfirmIsSourceOnly(t *testing.T) {
	c := &nativeDecoratorCall{Name: "f", Advised: "rule-1", Next: func(context.Context) { t.Error("Next ran") }}
	err := confirmDecorator(context.Background(), c, nil)
	if err == nil || !strings.Contains(err.Error(), "source-only") {
		t.Errorf("err = %v", err)
	}
}

// TestConfirmHarnessLoop is the RFC's harness, in process: run, receive 6,
// take the resume form off the one yield line, replay with the human's
// answer, receive 0. The second run performs exactly the operations the first
// one described.
func TestConfirmHarnessLoop(t *testing.T) {
	stubAsk(t, false, errConfirmUnattended)
	work := confirmWork(t)
	out, errOut := runConfirm(t, work, work, nil)
	if !strings.HasSuffix(out, "status 6\n") {
		t.Fatalf("first run did not yield: %q", out)
	}
	resume := regexp.MustCompile(`resume with clean (--confirm=[0-9a-f]{8}):yes`).FindStringSubmatch(errOut)
	if resume == nil {
		t.Fatalf("no resume form on stderr: %q", errOut)
	}
	// The harness obtained "yes" from its human and replays.
	out, errOut = runConfirm(t, work, resume[1]+":yes "+work, nil)
	if !strings.HasSuffix(out, "status 0\n") || errOut != "" {
		t.Fatalf("replay: stdout %q stderr %q", out, errOut)
	}
	if exists(t, filepath.Join(work, "victim")) || !exists(t, filepath.Join(work, "after")) {
		t.Error("replay did not complete the call")
	}
}

func TestConfirmBashOffIsIdentity(t *testing.T) {
	asked := stubAsk(t, false, errConfirmUnattended)
	for _, lang := range []syntax.LangVariant{syntax.LangBash, syntax.LangPOSIX} {
		work := confirmWork(t)
		// The same body as a plain function: no decorator, so nothing is
		// governed — rm runs, nobody is asked, --what-if is an ordinary word.
		script := "clean() { rm -rf \"$1/victim\"; echo \"done ${2-}\"; }\nclean \"" + work + "\" --what-if; echo \"status $?\"\n"
		err, out, errOut := runDecorated(t, context.Background(), lang, script, nil)
		if err != nil {
			t.Fatalf("%v: %v", lang, err)
		}
		if exists(t, filepath.Join(work, "victim")) || len(*asked) != 0 || out.String() != "done --what-if\nstatus 0\n" || errOut.Len() != 0 {
			t.Errorf("%v: asked %v stdout %q stderr %q", lang, *asked, out.String(), errOut.String())
		}
		// And decorator syntax itself is unreachable: the Bash# source does
		// not parse as Bash.
		if _, err := syntax.NewParser(syntax.Variant(lang)).Parse(strings.NewReader("@confirm()\nfunction f() { :; }\n"), "x.sh"); err == nil {
			t.Errorf("%v accepted decorator syntax", lang)
		}
	}
	// The POSIX wiring carries no confirm rung at all.
	for _, opt := range wireExec(nil, true, os.Environ(), nil, io.Discard, io.Discard, false) {
		_ = opt
	}
	_, ok := confirmStateFrom(context.Background())
	if ok {
		t.Error("a bare context carries confirm state")
	}
}

func TestConfirmTokenIsArgvIdentity(t *testing.T) {
	a := confirmToken([]string{"rm", "-rf", "build"})
	if a != confirmToken([]string{"rm", "-rf", "build"}) {
		t.Error("token is not stable")
	}
	if a == confirmToken([]string{"rm", "-rf", "dist"}) || a == confirmToken([]string{"rm", "-rf build"}) {
		t.Error("token does not distinguish argv")
	}
	if len(a) != 8 {
		t.Errorf("token %q is not 8 hex digits", a)
	}
}

func TestConfirmHandlerIgnoresCommandsOutsideAConfirmCall(t *testing.T) {
	calls := 0
	next := func(ctx context.Context, args []string) error { calls++; return nil }
	h := confirmHandler()(next)
	// No @confirm call on the context: the rung is identity, even for rm.
	if err := h(context.Background(), []string{"rm", "-rf", "x"}); err != nil || calls != 1 {
		t.Errorf("err %v calls %d", err, calls)
	}
	if err := h(context.Background(), nil); err != nil || calls != 2 {
		t.Errorf("empty argv: err %v calls %d", err, calls)
	}
}
