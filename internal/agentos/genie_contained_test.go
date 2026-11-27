// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const containedGenieFake = `#!/bin/sh
set -eu
case "$1 $2" in
"llm session")
 case "$3" in
 open)
  printf '%s\n' "$@" > "$TRACE/open"
  shift 9
  label=$2
  case "$SCENARIO" in
   door) echo "door is down" >&2; exit 1;;
   malformed) echo oops; exit 0;;
  esac
  printf '{"label":"%s","socket":"/session.sock","token":"run-key","expires_at":"2030-01-01T00:00:00Z","models":["seat"]}\n' "$label"
  ;;
 close)
  echo "$4" >> "$TRACE/close"
  test "$SCENARIO" != close
  ;;
 forward)
  test "$4" = --socket && test "$5" = /session.sock
  echo http://127.0.0.1:31415/v1
  exec sleep 60
  ;;
 esac;;
"contain --net")
 printf '%s\n' "$@" > "$TRACE/contain"
 test "$OPENAI_API_KEY" = run-key
 case "$SCENARIO" in
  backend) exit 125;;
  signal) exec sleep 60;;
  failed) exit 23;;
 esac
 echo answer;;
"run --target")
 test "$3" = chat
 test "$GENIE_MODEL_ID" = seat && test "$GENIE_EXTERNAL_MODEL" = seat || exit 30
 test "$GENIE_EXTERNAL_CONTEXT" = 32768 && test "$OPENAI_API_KEY" = run-key || exit 31
 test "$OPENAI_BASE_URL" = http://127.0.0.1:31415/v1 || exit 32
 test "$GENIE_YCODE_SESSION" = label || exit 33
 if test "$GENIE_MODE" = chat; then
  test "$(cat)" = 'one task' || exit 34
  echo DONE
 else
  test "$5 $6 $7" = 'show label --json' || exit 35
  echo '{"messages":[{"role":"user","content":[{"type":"text","text":"task"}]},{"role":"assistant","content":[{"type":"text","text":"substantive answer"}]},{"role":"assistant","content":[{"type":"text","text":"DONE"}]}]}'
 fi;;
*) exit 99;;
esac
`

func setupContainedGenie(t *testing.T, scenario string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fakes")
	}
	dir := t.TempDir()
	self := filepath.Join(dir, "fake")
	if err := os.WriteFile(self, []byte(containedGenieFake), 0700); err != nil {
		t.Fatal(err)
	}
	bar := filepath.Join(dir, "genie.bar")
	if err := os.WriteFile(bar, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENIE_BAR", bar)
	t.Setenv("BASHY_HOME", dir)
	t.Setenv("BASHY_SELF", self)
	t.Setenv("TRACE", dir)
	t.Setenv("SCENARIO", scenario)
	return dir
}

func TestGenieContainedComposition(t *testing.T) {
	for _, tc := range []struct {
		scenario, selector string
		code               int
		refusal            string
	}{
		{"success", "seat", 0, ""}, {"band", "L4", 0, ""}, {"door", "seat", 1, "bashy llm up"}, {"scope", "other", 1, "select a served binding"}, {"malformed", "seat", 1, "session door version"}, {"backend", "seat", 125, "enforcing containment backend"}, {"failed", "seat", 23, "exit status 23"}, {"close", "seat", 1, "bashy llm session close"},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			dir := setupContainedGenie(t, tc.scenario)
			var out, diag bytes.Buffer
			task := "literal ; $workspace"
			code, err := runContainedGenie(context.Background(), tc.selector, dir, task, &out, &diag)
			if code != tc.code || (err == nil) != (tc.refusal == "") || (err != nil && !strings.Contains(err.Error(), tc.refusal)) {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, &diag)
			}
			read := func(file string) string {
				b, e := os.ReadFile(filepath.Join(dir, file))
				if e != nil {
					t.Fatal(e)
				}
				return string(b)
			}
			label := strings.TrimSuffix(read("close"), "\n")
			flag := "--model"
			if tc.selector == "L4" {
				flag = "--band"
			}
			want := []string{"llm", "session", "open", flag, tc.selector, "--ttl", "30m", "--max-requests", "200", "--label", label, "--json"}
			if got := strings.Split(strings.TrimSuffix(read("open"), "\n"), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("session open %q", got)
			}
			record := read(filepath.Join("genie", "runs", label+".json"))
			var rec containedGenieRecord
			if json.Unmarshal([]byte(record), &rec) != nil || strings.Contains(record, "run-key") || rec.Dir != dir || rec.Label != label {
				t.Fatal(record)
			}
			if code == 0 {
				want = []string{"contain", "--net", "deny", "--dir", dir, "--socket", "/session.sock", "--ro", os.Getenv("GENIE_BAR"), "--env", "OPENAI_API_KEY", "--", os.Getenv("BASHY_SELF"), "genie", "--session-launcher", "/session.sock", os.Getenv("GENIE_BAR"), "seat", label, task}
				if got := strings.Split(strings.TrimSuffix(read("contain"), "\n"), "\n"); !reflect.DeepEqual(got, want) {
					t.Fatalf("contain %q", got)
				}
				if rec.Binding != "seat" || (flag == "--band" && rec.Band != "L4") || rec.Exit != 0 || rec.MaxRequests != 200 || rec.ExpiresAt == "" || rec.Sandbox == "" || !reflect.DeepEqual(rec.Models, []string{"seat"}) || out.String() != "answer\n" {
					t.Fatalf("%s output %q", record, &out)
				}
			} else if tc.scenario != "close" && rec.Exit != code {
				t.Fatal(record)
			}
			if tc.scenario == "scope" || tc.scenario == "door" || tc.scenario == "malformed" {
				if _, e := os.Stat(filepath.Join(dir, "contain")); !os.IsNotExist(e) {
					t.Fatal("unexpected contain invocation")
				}
			}
		})
	}
}

func TestGenieContainedLauncher(t *testing.T) {
	setupContainedGenie(t, "launch")
	t.Setenv("OPENAI_API_KEY", "run-key")
	var code int
	out := captureStdout(t, func() {
		code = launchContainedGenie([]string{"/session.sock", "genie.bar", "seat", "label", "one task"})
	})
	if code != 0 || out != "substantive answer\n" {
		t.Fatalf("code=%d output=%q", code, out)
	}
}

func TestGenieContainedSignal(t *testing.T) {
	if os.Getenv("GENIE_TEST_SIGNAL_CHILD") == "1" {
		os.Exit(dispatchGenie([]string{"--contained", "task"}))
	}
	dir := setupContainedGenie(t, "signal")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGenieContainedSignal$")
	cmd.Env = append(os.Environ(), "GENIE_TEST_SIGNAL_CHILD=1")
	var diag bytes.Buffer
	cmd.Stderr = &diag
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		if _, err := os.Stat(filepath.Join(dir, "contain")); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		cmd.Wait()
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil || cmd.ProcessState.ExitCode() != 130 {
		t.Fatalf("interrupt: %v %s", err, &diag)
	}
	data, err := os.ReadFile(filepath.Join(dir, "close"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := os.ReadFile(filepath.Join(dir, "genie", "runs", strings.TrimSpace(string(data))+".json"))
	if err != nil || !strings.Contains(string(record), `"exit":130`) {
		t.Fatalf("record=%s err=%v", record, err)
	}
}
