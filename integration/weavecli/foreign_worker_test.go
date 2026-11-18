package weavecli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/qiangli/coreutils/pkg/weavecli"
	"mvdan.cc/sh/v3/polyglot"
)

// Sprint 221 B4 review correction (Story-ID af2e56930f08): a foreign-worker
// error reaches NewEnvelopeError in two shapes — used directly, and wrapped
// by a Go caller with fmt.Errorf("…: %w", err). errors.As finds the detail
// through the wrapper, and the first cut then replaced Message with the
// worker's bare message, dropping the wrapper's context that the pre-B4
// EmitError (Message = err.Error()) always carried. These tests drive REAL
// sh foreign errors (built by sh's own worker decoder, not a stand-in type)
// through NewEnvelopeError in both shapes and pin the invariant: Message is
// err.Error() in full, Code/Help/Cause are additive.
//
// The worker is this test binary re-executed (fakeWorkerEnv): it answers the
// load request with a failing frame whose "error" value is the plan Source,
// so Module.Call returns exactly what sh builds from that frame. No Python
// or Rust toolchain is needed and nothing leaves the process tree.

const fakeWorkerEnv = "WEAVECLI_TEST_FAKE_WORKER"

func TestMain(m *testing.M) {
	if os.Getenv(fakeWorkerEnv) == "1" {
		os.Exit(runFakeWorker())
	}
	os.Exit(m.Run())
}

// runFakeWorker speaks just enough of sh's worker protocol to fail the load:
// read the load request, echo its Source back as the error frame. The
// protocol channel is fd 3 on Unix and stdout on Windows, exactly as
// polyglot.(*Module).ensure wires it.
func runFakeWorker() int {
	line, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
	if err != nil {
		return 3
	}
	var load struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(line, &load); err != nil {
		return 3
	}
	out := os.Stdout
	if runtime.GOOS != "windows" {
		out = os.NewFile(3, "protocol")
		if out == nil {
			return 3
		}
	}
	fmt.Fprintf(out, `{"id":0,"ok":false,"error":%s}`+"\n", load.Source)
	return 0
}

// foreignError returns the error sh builds for a worker frame whose "error"
// value is frame, exactly as Module.Call would hand it to a weave caller.
func foreignError(t *testing.T, frame string) error {
	t.Helper()
	t.Setenv(fakeWorkerEnv, "1")
	module := polyglot.Start(polyglot.Plan{Language: "python", Source: frame}, polyglot.Python{Command: os.Args[0]})
	defer module.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := module.Call(ctx, "fail")
	if err == nil {
		t.Fatal("fake worker did not fail the call")
	}
	if strings.Contains(err.Error(), "runtime unavailable") || strings.Contains(err.Error(), "invalid worker response") {
		t.Fatalf("fake worker did not speak the protocol: %v", err)
	}
	if _, ok := polyglot.ForeignErrorDetail(err); !ok {
		t.Fatalf("test premise broken: %q (%T) carries no foreign detail", err, err)
	}
	return err
}

func TestNewEnvelopeError_ForeignDirectAndWrapped(t *testing.T) {
	const wrap = "island call"
	cases := []struct {
		name       string
		frame      string
		code       int
		wantCode   string // Code for both the direct and the wrapped error
		wantHelp   string
		wantCause  *EnvelopeError
		wantDirect string // err.Error() of the direct error, sh's rendering
	}{
		{
			name:       "structured python exception",
			frame:      `{"code":"ValueError","message":"boom","help":"Traceback (most recent call last):\n  raise ValueError('boom')"}`,
			code:       ExitGenericFail,
			wantCode:   "ValueError",
			wantHelp:   "Traceback (most recent call last):\n  raise ValueError('boom')",
			wantDirect: "ValueError: boom",
		},
		{
			name:       "structured rust result",
			frame:      `{"code":"RUST-ECALL","message":"missing field ` + "`x`" + `"}`,
			code:       ExitGenericFail,
			wantCode:   "RUST-ECALL",
			wantDirect: "RUST-ECALL: missing field `x`",
		},
		{
			name:       "legacy plain string",
			frame:      `"plain boom"`,
			code:       ExitPrecondFail,
			wantCode:   "precondition_failed",
			wantDirect: "plain boom",
		},
		{
			name:       "structured code without message",
			frame:      `{"code":"KeyError"}`,
			code:       ExitGenericFail,
			wantCode:   "KeyError",
			wantDirect: "KeyError",
		},
		{
			name:       "nested cause",
			frame:      `{"code":"RuntimeError","message":"outer","cause":{"code":"ValueError","message":"inner","help":"inner trace"}}`,
			code:       ExitGenericFail,
			wantCode:   "RuntimeError",
			wantCause:  &EnvelopeError{Code: "ValueError", Message: "inner", Help: "inner trace"},
			wantDirect: "RuntimeError: outer: ValueError: inner",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			direct := foreignError(t, tc.frame)
			if direct.Error() != tc.wantDirect {
				t.Fatalf("test premise: sh renders the frame as %q, want %q", direct.Error(), tc.wantDirect)
			}
			wrapped := fmt.Errorf("%s: %w", wrap, direct)

			for _, shape := range []struct {
				name string
				err  error
			}{{"direct", direct}, {"wrapped", wrapped}} {
				got := NewEnvelopeError(tc.code, shape.err)
				want := &EnvelopeError{Code: tc.wantCode, Message: shape.err.Error(), Help: tc.wantHelp, Cause: tc.wantCause}
				if !bytes.Equal(mustJSON(t, got), mustJSON(t, want)) {
					t.Errorf("%s: got  %s\n         want %s", shape.name, mustJSON(t, got), mustJSON(t, want))
				}
				// The pre-B4 invariant, stated on its own: the envelope's
				// message is the rendered error, never less.
				if got.Message != shape.err.Error() {
					t.Errorf("%s: message %q lost text from %q", shape.name, got.Message, shape.err.Error())
				}
			}

			// The wrapped message carries the wrapper context, then sh's own
			// rendering; the direct message is that rendering alone.
			wrappedEnv := NewEnvelopeError(tc.code, wrapped)
			if want := wrap + ": " + tc.wantDirect; wrappedEnv.Message != want {
				t.Errorf("wrapped message = %q, want %q", wrappedEnv.Message, want)
			}
			if directEnv := NewEnvelopeError(tc.code, direct); directEnv.Message != tc.wantDirect {
				t.Errorf("direct message = %q, want %q", directEnv.Message, tc.wantDirect)
			}
		})
	}
}

// TestEmitError_WrappedForeignKeepsContext: the JSON envelope a weave verb
// emits for a wrapped foreign error contains the wrapper context in
// error.message and the worker's code/help alongside — what a pre-B4 reader
// got, plus the structured detail.
func TestEmitError_WrappedForeignKeepsContext(t *testing.T) {
	direct := foreignError(t, `{"code":"ValueError","message":"boom","help":"trace"}`)
	wrapped := fmt.Errorf("weave island: %w", direct)

	var buf bytes.Buffer
	if code := EmitError(&buf, OutputJSON, "island", ExitGenericFail, wrapped); code != ExitGenericFail {
		t.Fatalf("exit code = %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	want := &EnvelopeError{Code: "ValueError", Message: "weave island: ValueError: boom", Help: "trace"}
	if env.Error == nil || !bytes.Equal(mustJSON(t, env.Error), mustJSON(t, want)) {
		t.Fatalf("envelope error = %s, want %s", mustJSON(t, env.Error), mustJSON(t, want))
	}

	// Plain mode prints the same text the JSON message carries.
	buf.Reset()
	EmitError(&buf, OutputPlain, "island", ExitGenericFail, wrapped)
	if buf.String() != "island: weave island: ValueError: boom\n" {
		t.Fatalf("plain = %q", buf.String())
	}

	// And a wrapped ORDINARY error through the same path is still the
	// legacy pair, so wrapping alone never invents structure.
	plain := fmt.Errorf("weave island: %w", errors.New("no worker"))
	if got := NewEnvelopeError(ExitGenericFail, plain); got.Code != "generic_failure" || got.Message != "weave island: no worker" || got.Help != "" || got.Cause != nil {
		t.Fatalf("wrapped ordinary error = %#v", got)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
