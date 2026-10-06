package agentos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	yokemcp "github.com/qiangli/yoke/mcp"
)

func mcpSessionClient(t *testing.T, limit int) (context.Context, *mcpsdk.ClientSession, *mcpShells, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	srv := yokemcp.NewServerWithOptions("test", "1", yokemcp.Options{Policy: &yokemcp.Policy{Audit: func(yokemcp.Record) {}}})
	shells := registerMCPShells(ctx, srv, limit)
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		cancel()
		ss.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); ss.Close(); cancel() })
	return ctx, cs, shells, cancel
}
func mcpSessionCall(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession, name string, in any) *mcpsdk.CallToolResult {
	t.Helper()
	r, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: in})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func mcpSessionDecode[T any](t *testing.T, r *mcpsdk.CallToolResult) T {
	t.Helper()
	if r.IsError {
		t.Fatalf("tool error: %+v", r.Content)
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func mcpSessionOpen(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession, in shellOpenInput) string {
	t.Helper()
	id := mcpSessionDecode[shellOpenOutput](t, mcpSessionCall(t, ctx, cs, "shell_open", in)).SessionID
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) {
		t.Fatalf("invalid handle %q", id)
	}
	return id
}
func mcpSessionExec(t *testing.T, ctx context.Context, cs *mcpsdk.ClientSession, id, script string) shellExecOutput {
	t.Helper()
	return mcpSessionDecode[shellExecOutput](t, mcpSessionCall(t, ctx, cs, "shell_exec", shellExecInput{SessionID: id, Script: script}))
}

func TestMCPSessionPersistence(t *testing.T) {
	ctx, cs, _, _ := mcpSessionClient(t, defaultMCPMaxOutput)
	id := mcpSessionOpen(t, ctx, cs, shellOpenInput{Dir: t.TempDir(), Env: map[string]string{"M4_ENV": "initial"}})
	if out := mcpSessionExec(t, ctx, cs, id, "cd /tmp"); out.ExitCode != 0 {
		t.Fatalf("cd: %+v", out)
	}
	want, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	out := mcpSessionExec(t, ctx, cs, id, "pwd")
	// Bash preserves the logical /tmp spelling on macOS; compare its realpath.
	printed, pathErr := filepath.EvalSymlinks(strings.TrimSuffix(out.Stdout.(string), "\n"))
	if pathErr != nil || printed != want {
		t.Fatalf("pwd: %+v, want %q", out, want)
	}
	actual, err := filepath.EvalSymlinks(out.Cwd)
	if err != nil || actual != want {
		t.Fatalf("cwd %q: %v", out.Cwd, err)
	}
	mcpSessionExec(t, ctx, cs, id, "X=1; export M4_ENV=changed; f() { echo function; }")
	out = mcpSessionExec(t, ctx, cs, id, "echo $X; echo $M4_ENV; f")
	if out.Stdout != "1\nchanged\nfunction\n" {
		t.Fatalf("state: %+v", out)
	}
	out = mcpSessionDecode[shellExecOutput](t, mcpSessionCall(t, ctx, cs, "shell_exec", shellExecInput{SessionID: id, Script: "read value; echo \"$value\"; echo problem >&2; false", Stdin: "input\n"}))
	if out.Stdout != "input\n" || out.Stderr != "problem\n" || out.ExitCode != 1 {
		t.Fatalf("stdio/status: %+v", out)
	}
	if out = mcpSessionExec(t, ctx, cs, id, "echo $X"); out.Stdout != "1\n" {
		t.Fatalf("after nonzero: %+v", out)
	}
}

func TestMCPSessionIsolationAndClose(t *testing.T) {
	ctx, cs, shells, _ := mcpSessionClient(t, defaultMCPMaxOutput)
	first := mcpSessionOpen(t, ctx, cs, shellOpenInput{})
	second := mcpSessionOpen(t, ctx, cs, shellOpenInput{})
	if first == second {
		t.Fatal("duplicate handles")
	}
	mcpSessionExec(t, ctx, cs, first, "M4_PRIVATE=secret")
	if out := mcpSessionExec(t, ctx, cs, second, "echo \"${M4_PRIVATE-}\""); out.Stdout != "\n" {
		t.Fatalf("state leaked: %+v", out)
	}
	shells.mu.Lock()
	dir := shells.sessions[first].dir
	shells.mu.Unlock()
	closed := mcpSessionDecode[shellCloseOutput](t, mcpSessionCall(t, ctx, cs, "shell_close", shellCloseInput{first}))
	if !closed.Closed {
		t.Fatal("not closed")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("session dir survived: %v", err)
	}
	for _, id := range []string{first, "unknown"} {
		for _, name := range []string{"shell_exec", "shell_close"} {
			in := map[string]any{"session_id": id}
			if name == "shell_exec" {
				in["script"] = "echo wrong"
			}
			r := mcpSessionCall(t, ctx, cs, name, in)
			if !r.IsError || len(r.Content) != 1 {
				t.Fatalf("expected missing session: %+v", r)
			}
			if text, ok := r.Content[0].(*mcpsdk.TextContent); !ok || text.Text != "no such session: "+id {
				t.Fatalf("unexpected error: %+v", r.Content)
			}
		}
	}
}

func TestMCPSessionBoundedOutput(t *testing.T) {
	ctx, cs, _, _ := mcpSessionClient(t, defaultMCPMaxOutput)
	id := mcpSessionOpen(t, ctx, cs, shellOpenInput{})
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			script := "printf '%1048576s' x"
			if stream == "stderr" {
				script += " >&2"
			}
			out := mcpSessionExec(t, ctx, cs, id, script)
			value := out.Stdout
			if stream == "stderr" {
				value = out.Stderr
			}
			b, _ := json.Marshal(value)
			var file shellOutputFile
			if err := json.Unmarshal(b, &file); err != nil {
				t.Fatal(err)
			}
			if file.Bytes != 1048576 || len(file.Preview) != 2048 {
				t.Fatalf("bad output metadata: bytes=%d preview=%d", file.Bytes, len(file.Preview))
			}
			data, err := os.ReadFile(file.Path)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) != 1048576 || string(data[:2048]) != file.Preview || data[len(data)-1] != 'x' {
				t.Fatal("incorrect spill content")
			}
		})
	}
}

func TestMCPSessionCancellationCleanup(t *testing.T) {
	ctx, cs, shells, cancel := mcpSessionClient(t, 8)
	id := mcpSessionOpen(t, ctx, cs, shellOpenInput{})
	mcpSessionExec(t, ctx, cs, id, "echo 123456789")
	shells.mu.Lock()
	dir := shells.sessions[id].dir
	shells.mu.Unlock()
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cancel did not remove session directory")
}

func TestMCPMaxOutputArgs(t *testing.T) {
	for _, args := range [][]string{{"serve", "--max-output", "8"}, {"serve", "--max-output=8"}} {
		rest, n, err := mcpOutputArgs(args)
		if err != nil || n != 8 || strings.Join(rest, " ") != "serve" {
			t.Fatalf("%v: %v %d %v", args, rest, n, err)
		}
	}
	for _, value := range []string{"", "-1", "no", "99999999999999999999999"} {
		if _, _, err := mcpOutputArgs([]string{"serve", "--max-output=" + value}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	ctx, cs, _, _ := mcpSessionClient(t, 8)
	id := mcpSessionOpen(t, ctx, cs, shellOpenInput{})
	if out := mcpSessionExec(t, ctx, cs, id, "printf 12345678"); out.Stdout != "12345678" {
		t.Fatalf("boundary: %+v", out)
	}
	if out := mcpSessionExec(t, ctx, cs, id, "printf 123456789"); out.Stdout == "123456789" {
		t.Fatal("limit not enforced")
	}
}
