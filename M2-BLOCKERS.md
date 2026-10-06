# M2/M3/M5 delivery blockers — Sprint 336, Story 1572

The bashy changes are committed, but the full goal and full build gate remain
blocked. No sibling files were edited, no remote was followed, and no branch
was switched or pushed.

## 1. HTTP cannot retain the caller's server

All APIs explicitly listed in the assignment exist. The additional API needed
for HTTP parity is absent: a transport accepting a prebuilt MCP server (or a
setup callback / returned server handle).

Three integration approaches were checked:

1. `BuildServer` returns the server needed by `registerMCPShells` and
   `NotifyToolsChanged`, but `ServeHTTP` constructs a different private server.
2. `ServeHTTPWithShutdown` also constructs its server privately; its extra
   return is a shutdown function, not a server handle or configuration hook.
3. `Options.Registered` can add argv/stdout/stderr command adapters but cannot
   access that private server. It cannot attach the shell tools' structured
   session/output schemas or issue `NotifyToolsChanged` on the HTTP server.
   `Options` has no other setup hook.

Implemented behavior: stdio includes session tools and a two-second registered
ring watcher; HTTP has the configured policy, direct profile, typed registered
snapshot and in-process script tool. HTTP ring changes require restart and
HTTP does not expose shell session tools. HTTP validates an immutable registered
snapshot through `BuildServer` before entering yoke's legacy panicking helper.

The patch below was verified in a temporary copy of `../yoke/mcp` **inside this
workspace**, without changing the sibling. `go test ./.mcp-yoke-probe -count=1`
passed 35 top-level tests plus 24 subtests (59 total; zero failed/skipped).
The added HTTP test proves that a caller-attached synthetic tool and a later
`NotifyToolsChanged` update reach the same live HTTP server.

After yoke accepts the patch, bashy can build one server, attach
`registerMCPShells` and `watchMCPRegistered`, then call
`ServeHTTPServerWithShutdown`, retaining shutdown/session/watcher cleanup on
SIGINT/SIGTERM. That replaces the snapshot workaround in `startMCPHTTP`.

## 2. Full build blocked by an unpopulated sibling module

The requested `go build ./...` exits 1 before compilation:

```
github.com/ollama/ollama (replaced by ../yoke/external/ollama/src):
reading ../yoke/external/ollama/src/go.mod: no such file or directory
```

Three probes establish the boundary:

1. Exact requested `go build ./...` fails on the missing replacement module.
2. `go list -e -json ./...` fails on the same missing `go.mod` during package
   discovery, before any changed package is compiled.
3. Narrower `go build ./cmd/bashy ./internal/agentos/` also fails on that module.

`go.mod:435` points to the provided sibling's unpopulated Ollama source directory.
The conductor must populate the pinned dependency through its workspace setup;
there is no verified source patch for absent dependency contents. Fetching or
editing sibling modules is explicitly outside this worker's authorization.

Independent requested checks succeed: `go test ./internal/agentos/ -run MCP`
passes 14 top-level tests plus 6 subtests (20 total; zero failed/skipped), and
`go vet ./internal/agentos/` exits 0 with zero diagnostics. The build/test/vet
chain was attempted exactly; since build fails, test and vet were also run
individually. `make test-bash` was not run, as instructed.

## Verified yoke patch (not applied to the sibling)

```diff
--- a/mcp/transport.go
+++ b/mcp/transport.go
@@ -36,6 +36,16 @@
 // seconds before active connections are closed). It reports serving or
 // shutdown errors; normal cancellation returns nil.
 func ServeHTTPWithShutdown(ctx context.Context, name, version string, opts Options, listen string) (addr string, shutdown func() error, err error) {
+	server, err := BuildServer(name, version, opts)
+	if err != nil {
+		return "", nil, err
+	}
+	return ServeHTTPServerWithShutdown(ctx, server, listen)
+}
+
+// ServeHTTPServerWithShutdown serves a prebuilt server, preserving caller-owned
+// tools, middleware and registered-command refreshes on the actual HTTP server.
+func ServeHTTPServerWithShutdown(ctx context.Context, server *mcpsdk.Server, listen string) (addr string, shutdown func() error, err error) {
 	listen, err = loopbackListenAddress(ctx, listen)
 	if err != nil {
 		return "", nil, err
@@ -44,7 +54,6 @@
 	if err != nil {
 		return "", nil, err
 	}
-	server := NewServerWithOptions(name, version, opts)
 	mux := http.NewServeMux()
 	mux.Handle("/mcp", mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{Stateless: true}))
 	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
--- /dev/null
+++ b/mcp/http_prebuilt_test.go
@@ -0,0 +1,53 @@
+package mcp
+
+import (
+	"context"
+	"testing"
+	"time"
+
+	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
+)
+
+func TestHTTPPrebuiltToolsAndRefresh(t *testing.T) {
+	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
+	defer cancel()
+	opts := Options{Policy: &Policy{Audit: func(Record) {}}}
+	srv, err := BuildServer("test", "1", opts)
+	if err != nil {
+		t.Fatal(err)
+	}
+	DeclareSyntheticEffects(srv, "shell_probe", []string{"exec"})
+	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shell_probe"}, func(context.Context, *mcpsdk.CallToolRequest, struct{}) (*mcpsdk.CallToolResult, RunToolOutput, error) {
+		return nil, RunToolOutput{Stdout: "attached"}, nil
+	})
+	addr, shutdown, err := ServeHTTPServerWithShutdown(ctx, srv, "127.0.0.1:0")
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer shutdown()
+	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: "http://" + addr + "/mcp"}, nil)
+	if err != nil {
+		t.Fatal(err)
+	}
+	defer cs.Close()
+	result, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "shell_probe", Arguments: map[string]any{}})
+	if err != nil || result.IsError {
+		t.Fatalf("attached tool: %v %+v", err, result)
+	}
+	opts.Registered = func() []RegisteredCommand {
+		return []RegisteredCommand{{Name: "refreshed", Effects: []string{"pure"}, Run: func(context.Context, []string, string, string) (string, string, int) { return "", "", 0 }}}
+	}
+	if err := NotifyToolsChanged(srv, opts); err != nil {
+		t.Fatal(err)
+	}
+	list, err := cs.ListTools(ctx, nil)
+	if err != nil {
+		t.Fatal(err)
+	}
+	for _, item := range list.Tools {
+		if item.Name == "refreshed" {
+			return
+		}
+	}
+	t.Fatal("HTTP did not expose refreshed tool")
+}
```
