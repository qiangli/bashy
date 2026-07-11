package session

import (
	"bytes"
	"context"
	"encoding/gob"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/bashy/internal/cli"
)

// TestRunSessionCommand exercises the warm-session runner directly: stdout
// capture, exit-status passthrough, and the in-process coreutils userland.
func TestRunSessionCommand(t *testing.T) {
	cases := []struct {
		name    string
		command string
		wantOut string
		wantErr int
	}{
		{"echo", "echo hi", "hi\n", 0},
		{"exit-code", "echo x; exit 4", "x\n", 4},
		{"coreutils-printf", "printf '%s-%s' a b", "a-b", 0},
		{"pipe", "printf 'l1\\nl2\\n' | { read first; read second; echo \"$second\"; }", "l2\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			exit := cli.RunSessionCommand(context.Background(), cli.SessionIO{
				Command: tc.command,
				Dir:     t.TempDir(),
				Env:     os.Environ(),
				Stdin:   strings.NewReader(""),
				Stdout:  &out,
				Stderr:  &errb,
			})
			if exit != tc.wantErr {
				t.Fatalf("exit = %d, want %d (stderr=%q)", exit, tc.wantErr, errb.String())
			}
			if out.String() != tc.wantOut {
				t.Fatalf("stdout = %q, want %q", out.String(), tc.wantOut)
			}
		})
	}
}

// TestServeRoundTrip starts a warm session and drives one request over the
// socket, asserting the response carries the command output and exit code.
func TestServeRoundTrip(t *testing.T) {
	// Keep the socket path short — macOS caps sun_path at 104 bytes, well
	// under a typical t.TempDir() path, so use the shorter os.TempDir().
	sock := filepath.Join(os.TempDir(), "bst-roundtrip.sock")
	_ = os.Remove(sock)
	t.Cleanup(func() { _ = os.Remove(sock) })

	go func() { _ = Serve(sock) }()

	// Wait for the listener to come up.
	var conn net.Conn
	var err error
	for i := 0; i < 100; i++ {
		conn, err = net.Dial("unix", sock)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("session never came up: %v", err)
	}
	defer conn.Close()

	if err := gob.NewEncoder(conn).Encode(Request{Command: "echo served; exit 7", Dir: os.TempDir(), Env: os.Environ()}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := gob.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Stdout) != "served\n" {
		t.Fatalf("stdout = %q, want %q", resp.Stdout, "served\n")
	}
	if resp.Exit != 7 {
		t.Fatalf("exit = %d, want 7", resp.Exit)
	}
}
