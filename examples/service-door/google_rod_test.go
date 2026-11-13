// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package servicedoor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGoogle stands in for the registered `google` command: it logs its argv
// and answers an empty listing, so no Google API is ever called.
const fakeGoogle = `#!/usr/bin/env -S bashy --bashsharp
printf '%s\n' "$*" >> "$GOOGLE_LOG"
echo '{"messages":[],"files":[]}'
`

// googleFixture registers the gws rods with examples/google/register.sh and
// then replaces `google` with fakeGoogle.
func googleFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	log := filepath.Join(f.root, "google.log")
	f.env = append(f.env, "GOOGLE_LOG="+log, "BASHY_BIN="+f.bin)
	register := filepath.Join(moduleRoot(t), "examples", "google", "register.sh")
	cmd := exec.Command(f.bin, register)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("register.sh: %v\n%s", err, out)
	}
	fake := filepath.Join(f.root, "google.bsh")
	if err := os.WriteFile(fake, []byte(fakeGoogle), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(f.bin, "commands", "add", "google", "--force", "--set", "exec.0="+f.bin, "--set", "exec.1=--bashsharp", "--set", "exec.2="+fake, "--set", "effects.0=read")
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fake google: %v\n%s", err, out)
	}
	return f, log
}

func readLog(path string) string { b, _ := os.ReadFile(path); return string(b) }

// Sprint 626 S5: read roles map to gws list/search verbs; email.send is
// selected only with --yes and refuses itself without consent.
func TestGoogleRods(t *testing.T) {
	f, log := googleFixture(t)
	writeReq := func(body string) string {
		p := filepath.Join(f.root, "req.json")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	env, stderr, code := f.runDoor(nil, "email.read", "--provider", "google-email-read", "--json", "newer_than:7d")
	if code != 0 || env.Outcome != "success" || env.Route != "api" || env.Provider != "google-email-read" {
		t.Fatalf("email.read = %+v code %d\n%s", env, code, stderr)
	}
	env, _, code = f.runDoor(nil, "drive.read", "--route", "api", "--json")
	if code != 0 || env.Provider != "google-drive-read" {
		t.Fatalf("drive.read list = %+v code %d", env, code)
	}
	env, _, code = f.runDoor(nil, "drive.read", "--route", "api", "--json", writeReq(`{"query":"report","max":3}`))
	if code != 0 || env.Outcome != "success" {
		t.Fatalf("drive.read search = %+v code %d", env, code)
	}
	want := "gmail-list maxResults:=10 q=newer_than:7d\ndrive-list pageSize:=10\ndrive-search report pageSize:=3\n"
	if got := readLog(log); got != want {
		t.Fatalf("google calls:\n%s\nwant:\n%s", got, want)
	}

	// Without --yes the sender is never selected, even by name.
	for _, extra := range [][]string{nil, {"--provider", "google-email-send"}} {
		args := append([]string{"email.read", "--role", "email.send", "--json"}, extra...)
		args = append(args, "to=a@example.invalid", "subject=s", "body=b")
		env, _, code = f.runDoor(nil, args...)
		if code != 126 || env.Outcome != "unavailable" || len(env.Unavailable) == 0 || !strings.Contains(env.Unavailable[0]["reason"], "--yes") {
			t.Fatalf("send without --yes = %+v code %d", env, code)
		}
	}
	// A direct run without consent refuses before calling google.
	cmd := exec.Command(f.bin, "google-email-send", "to=a@example.invalid", "subject=s", "body=b")
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "refuses without consent") {
		t.Fatalf("direct send = %v\n%s", err, out)
	}
	if got := readLog(log); got != want {
		t.Fatalf("a refused send reached google:\n%s", got)
	}

	env, _, code = f.runDoor(nil, "email.read", "--role", "email.send", "--yes", "--json", writeReq(`{"to":"a@example.invalid","subject":"s","body":"b"}`))
	if code != 0 || env.Outcome != "success" || env.Provider != "google-email-send" {
		t.Fatalf("send with --yes = %+v code %d", env, code)
	}
	if got := readLog(log); !strings.HasSuffix(got, "gmail-send to=a@example.invalid subject=s body=b\n") {
		t.Fatalf("send call:\n%s", got)
	}
}
