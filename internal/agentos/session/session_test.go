package session

import (
	"bytes"
	"context"
	"encoding/gob"
	"net"
	"os"
	"os/exec"
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
	// Keep Serve in an owned child so cleanup can stop its listener and signal
	// handler without terminating the test process.
	if socket := os.Getenv("BASHY_SESSION_ROUNDTRIP_SOCKET"); socket != "" {
		if err := Serve(socket); err != nil {
			t.Fatal(err)
		}
		return
	}
	// A short private directory satisfies both the Unix socket length limit
	// and secureSocketDir ownership checks. Never serve directly from /tmp.
	dir, err := os.MkdirTemp(os.TempDir(), "bst")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")
	cmd := exec.Command(os.Args[0], "-test.run=^TestServeRoundTrip$")
	cmd.Env = append(os.Environ(), "BASHY_SESSION_ROUNDTRIP_SOCKET="+sock)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var serveErr error
	go func() { serveErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })

	// Wait for the listener to come up.
	var conn net.Conn
	for i := 0; i < 100; i++ {
		select {
		case <-done:
			t.Fatalf("session exited before becoming ready: %v\n%s", serveErr, output.String())
		default:
		}
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
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

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
