// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/policy/audit"
	"io"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/pathconv"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMCPUsageNoArgs(t *testing.T) {
	if got := dispatchMCP(nil); got != 2 {
		t.Fatalf("dispatchMCP(nil) = %d, want 2", got)
	}
	if got := dispatchMCP([]string{"--help"}); got != 2 {
		t.Fatalf("dispatchMCP(--help) = %d, want 2", got)
	}
}

// captureMCPStderr runs fn with os.Stderr replaced by a pipe and returns
// everything fn wrote to it.
func captureMCPStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old; r.Close(); w.Close() }()
	fn()
	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestMCPInvalidOptions(t *testing.T) {
	ringDir(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--allow", "network"}, `invalid allowed effect "network"`},
		{[]string{"--tools", "bogus-name"}, "unknown command: bogus-name"},
		{[]string{"--transport", "http", "--listen", "0.0.0.0:0"}, "refusing non-loopback bind"},
		{[]string{"--transport=http", "--tools=bogus-name"}, "unknown command: bogus-name"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out := captureMCPStderr(t, func() {
				if got := dispatchMCP(append([]string{"serve"}, tc.args...)); got != 2 {
					t.Fatalf("exit = %d", got)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Fatalf("stderr = %q, want %q", out, tc.want)
			}
		})
	}
}

func mcpFrontClient(t *testing.T, opts yokemcp.Options) (context.Context, *mcpsdk.ClientSession, *mcpsdk.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	srv, err := yokemcp.BuildServer("bashy", "test", opts)
	if err != nil {
		t.Fatal(err)
	}
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return ctx, cs, srv
}

func mcpTestOptions(t *testing.T) yokemcp.Options {
	t.Helper()
	opts, err := mcpOptions("", "default")
	if err != nil {
		t.Fatal(err)
	}
	opts.Policy.Audit = func(yokemcp.Record) {}
	return opts
}

func TestMCPProfilesAndSyntheticNames(t *testing.T) {
	ringDir(t)
	opts := mcpTestOptions(t)
	if len(opts.Tools) == 0 {
		t.Fatal("empty default profile")
	}
	for _, name := range opts.Tools {
		if !isCoreCommand(name) || tool.Lookup(name) == nil {
			t.Fatalf("noncore default: %s", name)
		}
	}
	for _, row := range coreRows {
		for _, name := range row.Commands {
			if tool.Lookup(name) != nil && !slices.Contains(opts.Tools, name) {
				t.Errorf("missing core %s", name)
			}
		}
	}
	all, err := mcpOptions("priv,cred", "all")
	if err != nil || !all.AllTools || !all.Policy.Allow["priv"] || !all.Policy.Allow["cred"] {
		t.Fatalf("all: %+v %v", all, err)
	}
	ctx, cs, srv := mcpFrontClient(t, opts)
	registerMCPShells(ctx, srv, defaultMCPMaxOutput)
	for _, name := range []string{"shell_open", "shell_exec", "shell_close"} {
		if tool.Lookup(name) != nil {
			t.Errorf("synthetic tool leaked into registry: %s", name)
		}
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Tools {
		if item.Name == "cat" {
			t.Fatal("noncore cat exposed by default")
		}
	}
}

func TestMCPScriptTool(t *testing.T) {
	ringDir(t)
	ctx, cs, _ := mcpFrontClient(t, mcpTestOptions(t))
	out := mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "bashy", map[string]any{"script": "printf %s hi"}))
	if out.Stdout != "hi" || out.Stderr != "" || out.ExitCode != 0 {
		t.Fatalf("script: %+v", out)
	}
	dir := t.TempDir()
	out = mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "bashy", map[string]any{
		"script": `read -r value; printf '%s\n' "$value"; pwd; printf problem >&2`, "stdin": "input bytes\n", "dir": dir,
	}))
	lines := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	if len(lines) != 2 || lines[0] != "input bytes" || out.Stderr != "problem" {
		t.Fatalf("stdio: %+v", out)
	}
	actual, _ := filepath.EvalSymlinks(interp.ShellPathToOS(dir, lines[1]))
	want, _ := filepath.EvalSymlinks(dir)
	if actual != want {
		t.Fatalf("dir = %s, want %s; output: %+v", actual, want, out)
	}
	result := mcpSessionCall(t, ctx, cs, "bashy", map[string]any{"script": "exit 7"})
	data, _ := json.Marshal(result.StructuredContent)
	if !result.IsError || !strings.Contains(string(data), `"exit_code":7`) {
		t.Fatalf("status: %s", data)
	}
}

// A portable external fixture, explicitly selected by the registered exec
// record. Script-tool tests never launch a bashy executable.
func TestMCPRegisteredHelperProcess(t *testing.T) {
	if os.Getenv("MCP_FIXTURE_HELPER") != "1" {
		return
	}
	at := slices.Index(os.Args, "--")
	input, _ := io.ReadAll(os.Stdin)
	dir, _ := os.Getwd()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": os.Args[at+1:], "stdin": string(input), "dir": dir})
	fmt.Fprint(os.Stderr, "fixture stderr")
	os.Exit(0)
}

func TestMCPRegisteredTypedTool(t *testing.T) {
	ringDir(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	rec := fleet.Command{Name: "mcp-fixture", Synopsis: "fixture", Effects: []string{"exec"},
		Exec: []string{exe, "-test.run=^TestMCPRegisteredHelperProcess$", "--", "{args}"}, Env: []string{"MCP_FIXTURE_HELPER=1"},
		Args: &fleet.CommandSchema{
			Positionals: []fleet.CommandParameter{{Name: "message", Type: "string", Required: true}},
			Flags:       []fleet.CommandFlag{{Name: "count", Shorthand: "n", Type: "int", Default: "2", Enum: []string{"2", "3"}}},
		},
	}
	if runtime.GOOS == "windows" {
		// Registered commands with Env bypass the interpreter's ordinary exec
		// rung. Their native child must still receive usable Windows temp paths.
		// The helper re-execs TestMain, whose MkdirTemp catches a broken path
		// before it can produce the required argv/stdin response.
		temp := pathconv.FromOS(t.TempDir())
		rec.Env = append(rec.Env, "TEMP="+temp, "TMP="+temp)
	}
	writeRecord(t, rec)
	ctx, cs, _ := mcpFrontClient(t, mcpTestOptions(t))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	for _, item := range list.Tools {
		if item.Name == rec.Name {
			raw, _ := json.Marshal(item.InputSchema)
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
		}
	}
	if schema == nil {
		t.Fatal("registered tool missing")
	}
	props := schema["properties"].(map[string]any)
	if props["count"].(map[string]any)["type"] != "integer" || props["message"].(map[string]any)["type"] != "string" {
		t.Fatalf("schema: %v", schema)
	}
	dir := t.TempDir()
	message := `hello; $(printf injected) 'world'`
	out := mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, rec.Name, map[string]any{"message": message, "count": 3, "stdin": "payload", "dir": dir}))
	var got struct {
		Args       []string
		Stdin, Dir string
	}
	if err := json.Unmarshal([]byte(out.Stdout), &got); err != nil {
		t.Fatalf("output: %+v: %v", out, err)
	}
	if !reflect.DeepEqual(got.Args, []string{"--count=3", message}) || got.Stdin != "payload" || out.Stderr != "fixture stderr" {
		t.Fatalf("argv/stdio: %+v %+v", got, out)
	}
	want, _ := filepath.EvalSymlinks(dir)
	actual, _ := filepath.EvalSymlinks(interp.ShellPathToOS(dir, got.Dir))
	if actual != want {
		t.Fatalf("cwd %q, want %q", actual, want)
	}
}

func TestMCPAuditPolicy(t *testing.T) {
	ringDir(t)
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("BASHY_AUDIT", path)
	writeRecord(t, fleet.Command{Name: "mcp-denied", Script: "exit 99", Effects: []string{"destroy"}})
	opts, err := mcpOptions("", "default")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cs, _ := mcpFrontClient(t, opts)
	denied := mcpSessionCall(t, ctx, cs, "mcp-denied", map[string]any{})
	if !denied.IsError {
		t.Fatal("destroy permitted without grant")
	}
	mcpSessionCall(t, ctx, cs, "bashy", map[string]any{"script": "printf %s hi"})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one record/call: %s", data)
	}
	var deniedRecord, allowedRecord audit.Record
	if err := json.Unmarshal([]byte(lines[0]), &deniedRecord); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &allowedRecord); err != nil {
		t.Fatal(err)
	}
	if deniedRecord.Action != "mcp:mcp-denied" || deniedRecord.Binary != "mcp-denied" || deniedRecord.Decision != "deny" || deniedRecord.Exit != 126 || !slices.Contains(deniedRecord.Effects, "destroy") {
		t.Fatalf("denial: %+v", deniedRecord)
	}
	if allowedRecord.Action != "mcp:bashy" || allowedRecord.Decision != "allow" || allowedRecord.Exit != 0 || allowedRecord.PrevHash != deniedRecord.Hash {
		t.Fatalf("allow: %+v", allowedRecord)
	}
}

func TestMCPHTTP(t *testing.T) {
	ringDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts := mcpTestOptions(t)
	srv, err := yokemcp.BuildServer("test", "1", opts)
	if err != nil {
		t.Fatal(err)
	}
	sessions := registerMCPShells(ctx, srv, 65536)
	defer func() { <-sessions.done }()
	addr, shutdown, err := yokemcp.ServeHTTPServerWithShutdown(ctx, srv, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()
	if _, _, err := yokemcp.ServeHTTPServerWithShutdown(ctx, srv, "0.0.0.0:0"); err == nil {
		t.Fatal("non-loopback bind accepted")
	}
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "http-test", Version: "1"}, nil).Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found, shells := false, false
	for _, item := range list.Tools {
		found = found || item.Name == "bashy"
		shells = shells || item.Name == "shell_exec"
	}
	if !found || !shells {
		t.Fatalf("HTTP tools/list missing bashy or shell_exec: found=%v shells=%v", found, shells)
	}
	out := mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "bashy", map[string]any{"script": "printf %s hi"}))
	if out.Stdout != "hi" {
		t.Fatalf("HTTP call: %+v", out)
	}
}

func TestMCPRegisteredWatcher(t *testing.T) {
	dir := ringDir(t)
	opts := mcpTestOptions(t)
	ctx, cs, srv := mcpFrontClient(t, opts)
	watchCtx, cancel := context.WithCancel(ctx)
	done := watchMCPRegistered(watchCtx, srv, opts, 10*time.Millisecond)
	defer func() { cancel(); <-done }()
	writeRecord(t, fleet.Command{Name: "mcp-changing", Synopsis: "first", Script: "true", Effects: []string{"pure"}})
	waitDescription := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			list, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			for _, item := range list.Tools {
				if item.Name == "mcp-changing" {
					got = item.Description
				}
			}
			if (want == "" && got == "") || (want != "" && strings.Contains(got, want)) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("watcher never exposed description %q", want)
	}
	waitDescription("first")
	// In-place edit: neither replace the file nor clear the registered index.
	path := filepath.Join(dir, "mcp-changing.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "first", "second"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	waitDescription("second")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitDescription("")
}
