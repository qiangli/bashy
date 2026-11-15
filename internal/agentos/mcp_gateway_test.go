package agentos

import (
	"context"
	"net/http"
	"runtime"
	"slices"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/weave"
)

// mcpGatewayTransport signs like outpost and forwards the master's gateway
// headers; an empty value omits that header.
type mcpGatewayTransport struct{ taskID, key string }

func (g mcpGatewayTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	mcpSignedRequest(r, mcpFixtureKey)
	if g.taskID != "" {
		r.Header.Set(mcpGatewayTaskHeader, g.taskID)
	}
	if g.key != "" {
		r.Header.Set(mcpGatewayKeyHeader, g.key)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpOutpostGatewayServer serves what `bashy mcp serve --outpost` serves:
// signed loopback HTTP with the gateway claim hook.
func mcpOutpostGatewayServer(t *testing.T, allow string) (context.Context, func(mcpGatewayTransport) *mcpsdk.ClientSession) {
	t.Helper()
	ringDir(t)
	mcpFixtureConfig(t)
	t.Setenv("BASHY_HOME", t.TempDir())
	opts := mcpVerbOptions(t, allow, "all")
	opts.Policy.Execute = mcpGatewayExecute
	srv, err := yokemcp.BuildServer("bashy", "test", opts)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	addr, stop, err := yokemcp.ServeHTTPServerWithAuth(ctx, srv, "127.0.0.1:0", mcpOutpostAuthenticate)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stop() })
	return ctx, func(tr mcpGatewayTransport) *mcpsdk.ClientSession {
		cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx,
			&mcpsdk.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp", HTTPClient: &http.Client{Transport: tr}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
}

func TestMCPOutpostGatewayHeadersRequired(t *testing.T) {
	got := fakeMCPVerb(t)
	ctx, connect := mcpOutpostGatewayServer(t, "")
	for _, tr := range []mcpGatewayTransport{{}, {taskID: "task"}, {key: "key"}} {
		cs := connect(tr)
		result := mcpSessionCall(t, ctx, cs, "sprint", map[string]any{"args": []string{"--help"}})
		if !result.IsError || got.Calls != 0 {
			t.Fatalf("headers %+v: executed=%d result=%+v", tr, got.Calls, result)
		}
		result = mcpSessionCall(t, ctx, cs, "run_tool", map[string]any{"name": "echo", "args": []string{"hi"}})
		if !result.IsError {
			t.Fatalf("headers %+v: run_tool not refused: %+v", tr, result)
		}
		// Discovery does not execute and needs no gateway identity.
		if _, err := cs.ListTools(ctx, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMCPOutpostGatewayDuplicateKeyExecutesOnce(t *testing.T) {
	got := fakeMCPVerb(t)
	ctx, connect := mcpOutpostGatewayServer(t, "")
	tr := mcpGatewayTransport{taskID: "task-1", key: "key-1"}
	first := mcpVerbDecode(t, mcpSessionCall(t, ctx, connect(tr), "sprint", map[string]any{"args": []string{"--help"}}))
	// A retried sync call (a new satellite connection) with the same key.
	second := mcpVerbDecode(t, mcpSessionCall(t, ctx, connect(tr), "sprint", map[string]any{"args": []string{"--help"}}))
	if got.Calls != 1 {
		t.Fatalf("executed %d times", got.Calls)
	}
	if first != second || first.Stdout != "canned stdout" || first.ExitCode != 7 {
		t.Fatalf("replay differs: %+v vs %+v", first, second)
	}
	task, err := weave.GatewayTaskOnHost("key-1")
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != "task-1" || task.State != "failed" || !task.HasResult || task.Exit != 7 || task.Text != "canned stdoutcanned stderr" || !task.Verified {
		t.Fatalf("host record: %+v", task)
	}
}

// A gated call answers before the claim, so the pull route can still claim
// the key and own the approval.
func TestMCPOutpostGatewayGatedDoesNotClaim(t *testing.T) {
	fakeMCPVerb(t)
	ctx, connect := mcpOutpostGatewayServer(t, "")
	name := mcpPrivilegedVerb(t)
	result := mcpSessionCall(t, ctx, connect(mcpGatewayTransport{taskID: "task-g", key: "key-g"}), name, map[string]any{"args": []string{"--help"}})
	if gated := mcpGatedDecode(t, result); gated.State != yokemcp.GatedState {
		t.Fatalf("not gated: %+v", gated)
	}
	if _, err := weave.GatewayTaskOnHost("key-g"); err == nil {
		t.Fatal("gated call claimed the key")
	}
}

func mcpPrivilegedVerb(t *testing.T) string {
	t.Helper()
	for _, command := range mcpVerbCommands() {
		if slices.Contains(command.OS, runtime.GOOS) && (slices.Contains(command.Effects, "destroy") || slices.Contains(command.Effects, "spend")) {
			return command.Name
		}
	}
	t.Fatal("no privileged verb fixture")
	return ""
}
