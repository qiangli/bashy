// Package session implements the warm `bashy serve` process: a persistent,
// local, explicitly-started listener that runs `bashy -c "…"` commands inside
// one already-initialized process, so an agent that fires thousands of shell
// invocations pays the process- and package-init tax once instead of per call.
//
// Discipline (see docs/bashy-startup-performance.md):
//   - No auto-spawn. The server only runs when the user starts `bashy serve`.
//   - No stranding. It listens on a local unix socket (not a network port),
//     removes the socket on exit, and a dead/absent session NEVER breaks a
//     call — the client silently falls through to normal in-process execution.
//     That preserves the readily-available invariant: `bashy <cmd>` works the
//     same with or without a live session; the session only makes it faster.
package session

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/qiangli/bashy/internal/cli"
	"golang.org/x/term"
)

// Request is one command to run in the warm session.
type Request struct {
	Command string   // the -c command string
	Dir     string   // caller working directory
	Env     []string // caller environment (KEY=VALUE)
	Stdin   []byte   // caller stdin (slurped; empty when stdin is a tty)
}

// Response is the result of running a Request.
type Response struct {
	Stdout []byte
	Stderr []byte
	Exit   int
}

// DefaultSocket returns the per-user default session socket path.
func DefaultSocket() string {
	if s := os.Getenv("BASHY_SESSION"); s != "" {
		return s
	}
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "bashy", "session.sock")
}

// Serve starts the warm session listener on socket (default DefaultSocket()).
// It blocks until interrupted, then removes the socket. It refuses to start if
// a live session is already listening on that path.
func Serve(socket string) error {
	if socket == "" {
		socket = DefaultSocket()
	}
	if dialable(socket) {
		return fmt.Errorf("a bashy session is already listening on %s", socket)
	}
	// Clear any stale socket left by a crashed server.
	_ = os.Remove(socket)
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return fmt.Errorf("session dir: %w", err)
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return fmt.Errorf("listen %s: %w", socket, err)
	}
	defer func() {
		ln.Close()
		_ = os.Remove(socket)
	}()

	// Cleanup on signal so the socket does not strand the next `serve`.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		ln.Close()
		_ = os.Remove(socket)
		os.Exit(0)
	}()

	fmt.Fprintf(os.Stderr, "bashy serve: warm session listening on %s\n", socket)
	fmt.Fprintf(os.Stderr, "  point clients at it with:  export BASHY_SESSION=%s\n", socket)
	fmt.Fprintln(os.Stderr, "  stop with Ctrl-C (the socket is removed on exit)")

	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed (signal path) — normal shutdown.
			return nil
		}
		go handle(conn)
	}
}

// handle runs one request on the warm runner and writes the response back.
func handle(conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := gob.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	var out, errb bytes.Buffer
	exit := cli.RunSessionCommand(context.Background(), cli.SessionIO{
		Command: req.Command,
		Dir:     req.Dir,
		Env:     req.Env,
		Stdin:   bytes.NewReader(req.Stdin),
		Stdout:  &out,
		Stderr:  &errb,
	})
	_ = gob.NewEncoder(conn).Encode(Response{
		Stdout: out.Bytes(),
		Stderr: errb.Bytes(),
		Exit:   exit,
	})
}

// Route forwards a `bashy -c "…"` invocation to a live warm session when one is
// available, writes its output to the process stdio, and returns the exit code
// with handled=true. If no session is configured, the invocation is not the
// simple `-c` form, or the session cannot be reached, it returns handled=false
// so the caller falls through to normal in-process execution (never stranding a
// command on a dead session).
func Route() (exit int, handled bool) {
	socket := os.Getenv("BASHY_SESSION")
	if socket == "" {
		return 0, false
	}
	command, ok := dashCCommand(os.Args)
	if !ok {
		return 0, false
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return 0, false // dead session -> fall through
	}
	defer conn.Close()

	// Only slurp stdin when it is piped/redirected; a `-c` command reading the
	// controlling tty is rare and would block a buffered protocol. (Streaming
	// stdin is a v2 refinement.)
	var stdin []byte
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		stdin, _ = io.ReadAll(os.Stdin)
	}

	cwd, _ := os.Getwd()
	req := Request{Command: command, Dir: cwd, Env: os.Environ(), Stdin: stdin}
	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		return 0, false
	}
	var resp Response
	if err := gob.NewDecoder(conn).Decode(&resp); err != nil {
		// Half-run request: safer to report failure than to silently re-run
		// locally (the command may have already had effects in the session).
		fmt.Fprintln(os.Stderr, "bashy: session error:", err)
		return 1, true
	}
	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	return resp.Exit, true
}

// dashCCommand extracts the command string from a simple `bashy -c "STRING"`
// invocation. It returns ok=false for any other shape (extra flags, script
// files, interactive) so only the common agent hot path is routed; everything
// else falls through to the full cold shell.
func dashCCommand(argv []string) (string, bool) {
	// argv[0] is the program; the first operand must be exactly "-c" and be
	// followed by the command string, with nothing after it.
	if len(argv) != 3 || argv[1] != "-c" {
		return "", false
	}
	return argv[2], true
}

// dialable reports whether a session socket currently accepts connections.
func dialable(socket string) bool {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
