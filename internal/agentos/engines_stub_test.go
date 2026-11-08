// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeanOllamaFailsClosed(t *testing.T) {
	msg := engineNotFoundMessage("ollama")
	for _, want := range []string{"bashy_engines", "embed_runner", "embedded signed runner"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Ollama error %q lacks %q", msg, want)
		}
	}
	if got := provisionEngine("ollama"); got != "" {
		t.Errorf("lean build provisioned Ollama: %q", got)
	}
	if probe := probeOllamaEngine(); probe.Binary != "" || probe.Provisionable {
		t.Errorf("lean build reports Ollama available: %+v", probe)
	}
}

func TestResolveEngineBinaryUnknown(t *testing.T) {
	if got := resolveEngineBinary("definitely-not-an-engine-xyz"); got != "" {
		t.Errorf("unknown engine should resolve to empty, got %q", got)
	}
}

// runStubOllamaDispatch drives the default build's REAL `bashy ollama`
// dispatch in-process and returns the exit status plus both streams.
// dispatchExit panics a private frontDoorExit while the front door is
// observing, which keeps the test in-process — no engine, no network.
func runStubOllamaDispatch(t *testing.T, argv []string) (code int, stdout, stderr string) {
	t.Helper()
	oldArgs := os.Args
	os.Args = argv
	t.Cleanup(func() { os.Args = oldArgs })
	frontDoorObserving.Store(true)
	defer frontDoorObserving.Store(false)

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := make(chan string), make(chan string)
	go func() { b, _ := io.ReadAll(outR); outDone <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errDone <- string(b) }()

	func() {
		defer func() {
			os.Stdout, os.Stderr = oldOut, oldErr
			_ = outW.Close()
			_ = errW.Close()
			v := recover()
			if v == nil {
				t.Error("dispatchEngine returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatchEngine("ollama")
	}()

	stdout, stderr = <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return code, stdout, stderr
}

// TestStubOllamaHelpExitsZero pins the story contract: on the default build
// `bashy ollama --help` (and -h, and --help with extra flags) prints usage
// — stating that local inference needs the engines build — and exits 0
// instead of the engine refusal.
func TestStubOllamaHelpExitsZero(t *testing.T) {
	for _, argv := range [][]string{
		{"bashy", "ollama", "--help"},
		{"bashy", "ollama", "-h"},
		{"bashy", "ollama", "--help", "--format", "classic"},
	} {
		code, stdout, stderr := runStubOllamaDispatch(t, argv)
		if code != 0 {
			t.Fatalf("%v: exit = %d, want 0; stderr:\n%s", argv, code, stderr)
		}
		if stderr != "" {
			t.Errorf("%v: help wrote to stderr:\n%s", argv, stderr)
		}
		for _, want := range []string{"usage: bashy ollama", "engines build"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: help output lacks %q:\n%s", argv, want, stdout)
			}
		}
	}
}

// TestStubOllamaNonHelpStillRefuses pins the other half: non-help stub
// invocations keep the refusal text and exit 1.
func TestStubOllamaNonHelpStillRefuses(t *testing.T) {
	for _, argv := range [][]string{
		{"bashy", "ollama", "list"},
		{"bashy", "ollama", "run", "llama3"},
	} {
		code, stdout, stderr := runStubOllamaDispatch(t, argv)
		if code != 1 {
			t.Fatalf("%v: exit = %d, want 1; stdout:\n%s\nstderr:\n%s", argv, code, stdout, stderr)
		}
		if !strings.Contains(stderr, "local inference requires the engines build") {
			t.Errorf("%v: refusal text changed; stderr:\n%s", argv, stderr)
		}
		if stdout != "" {
			t.Errorf("%v: refusal wrote to stdout:\n%s", argv, stdout)
		}
	}
}

func TestLeanOllamaIgnoresExternalBinary(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	path := filepath.Join(cache, "ollama")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o755); err != nil {
		t.Fatal(err)
	}
	if probe := probeOllamaEngine(); probe.Binary != "" || probe.Provisionable {
		t.Errorf("lean build selected external Ollama: %+v", probe)
	}
}
