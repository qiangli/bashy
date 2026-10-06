# M8 blockers — Sprint 336, Story 1580 (4c1cb5a64bde)

M8 is **not delivered**. The commit preserves two bounded prerequisite tests;
production wiring and documentation are unchanged. No sibling module was edited,
no branch was switched, and nothing was pushed.

## Native front-door dispatch is missing

The requested assumption that `mcpRunScript` dispatches verbs in-process is false
in this checkout. It runs the interpreter in-process, but `PreambleFor(false)`
generates functions such as `sprint() { command <bashySelfPath> sprint "$@"; }`.
`WireSessionExec(false)` has registry and registered-ring handlers, but no
front-door handler. The final interpreter handler launches an executable.

Three investigated integration routes:

1. Built the requested catalog/Options adapter and invoked `sprint --help` and
   allowed `weave --help` using the existing in-memory MCP client. Both calls
   exceeded its 10-second deadline: self re-entry selected the test executable.
   Removed this draft rather than ship an adapter that violates “never exec the
   bashy binary.” Build and vet on the draft passed; these calls did not.
2. Examined the request-local session interpreter and reproduced its fallback
   with a terminal ExecHandler interceptor. The committed
   `TestMCPVerbInterpreterRequiresSelfExec` proves that `sprint`, `weave`, `dag`,
   `commands`, and `kb` all reach `[bashySelfPath(), VERB, --help]` after existing
   handlers. The interceptor returns 126 and never calls the external executor.
3. Examined reusing `Dispatch`/`dispatch` inside that interpreter. They consume
   `os.Args`, `os.Stdin`, `os.Stdout`, `os.Stderr`, and process cwd; the public
   entry point finishes with shutdown and `os.Exit`. `dispatchExit` relies on
   the process-global `frontDoorObserving` flag. Temporarily replacing these
   globals would race concurrent MCP requests and cannot isolate cwd or streams.
   A mutex local to MCP does not protect unrelated users of those globals.

Required prerequisite: refactor the front door into a request-local dispatcher
with argv, context, streams, cwd and returned status, then install it in the
session interpreter before external execution. The current dispatcher and
session wiring live in `internal/agentos/agentos.go`, outside the authorized
four-file scope. Some verb implementations also use process-global I/O/cwd;
those need an audit before promising generic in-process dispatch. There is no
verified complete dispatcher patch here; duplicating a few Cobra factories
would not implement the required visible catalog.

## Registered refresh cannot preserve static verbs

`yoke/mcp/direct.go:refreshRegistered` removes every `state.names` entry and
re-adds the entire `Options.Registered()` result. Caching the verb adapters only
avoids rebuilding the Go slice; it does not avoid tool re-registration. Returning
only ring entries on later callbacks instead deletes the verbs. Registering them
again via `DeclareSyntheticEffects` only changes policy metadata, not ownership
of the tools. `mcp.go` Options wiring alone cannot change this behavior.

The committed `TestMCPVerbRegisteredSnapshotIsNotStatic` exercises both replacement
and omission with an in-memory client. This is evidence of the prerequisite,
not an acceptance test claiming M8 works.

The following yoke patch adds an explicit `RegisteredCommand.Static` contract.
A retained static adapter and its synthetic effects stay installed; omission
removes it, and a non-static ring entry with the same name replaces it. Set
`Static: true` on future verb adapters only. Ring adapters retain their default
`false`, so changes to script/exec bodies with unchanged descriptors still refresh.
This patch was **verified with a Go build overlay**, without writing to yoke.
The accompanying test delta checks retention, ring takeover, and removal.

Apply the first diff from the yoke root; apply the second from the bashy root.
These patches solve only the static-refresh prerequisite, not native dispatch.

```diff
--- a/mcp/direct.go
+++ b/mcp/direct.go
@@ -27,6 +27,7 @@
 	mu       sync.Mutex
 	policy   *Policy
 	names    []string
+	static   map[string]bool
 	reserved map[string]bool
 }
 
@@ -53,6 +54,9 @@
 // RegisteredCommand adapts a caller-owned command to a direct MCP tool.
 // Schema parameters named stdin or dir take precedence over transport inputs.
 type RegisteredCommand struct {
+	// Static retains this adapter across refreshes while the snapshot keeps
+	// its name marked Static. Omission removes it; a non-static entry replaces it.
+	Static                bool
 	Name, Synopsis, Usage string
 	Effects, OS           []string
 	Schema                *tool.ArgSchema
@@ -183,6 +187,7 @@
 		description *mcpsdk.Tool
 	}
 	var ready []prepared
+	var retained []string
 	seen := map[string]bool{}
 	for _, command := range commands {
 		if !directToolName.MatchString(command.Name) {
@@ -194,6 +199,10 @@
 		seen[command.Name] = true
 		if command.Run == nil {
 			return fmt.Errorf("registered command %s has no runner", command.Name)
+		}
+		if command.Static && state.static[command.Name] {
+			retained = append(retained, command.Name)
+			continue
 		}
 		usage, _, _ := strings.Cut(command.Usage, "\n")
 		doc := tool.DocumentAsMCP(command.Name, command.Synopsis+" — "+usage)
@@ -228,11 +237,23 @@
 		ready = append(ready, prepared{command, description})
 	}
 	// Validate the full snapshot before removing any previously registered tools.
-	srv.RemoveTools(state.names...)
+	var removed []string
 	for _, name := range state.names {
+		if !slices.Contains(retained, name) {
+			removed = append(removed, name)
+		}
+	}
+	if len(removed) > 0 {
+		srv.RemoveTools(removed...)
+	}
+	for _, name := range removed {
 		state.policy.synthetic.Delete(name)
 	}
-	state.names = nil
+	state.names = retained
+	state.static = make(map[string]bool, len(retained))
+	for _, name := range retained {
+		state.static[name] = true
+	}
 	for _, item := range ready {
 		command := item.command
 		state.policy.synthetic.Store(command.Name, command.Effects)
@@ -282,6 +303,7 @@
 			return &mcpsdk.CallToolResult{IsError: exit != 0}, RunToolOutput{Stdout: stdout, Stderr: stderr, ExitCode: exit}, nil
 		})
 		state.names = append(state.names, command.Name)
+		state.static[command.Name] = command.Static
 	}
 	return nil
 }
```

```diff
--- a/internal/agentos/mcp_verbs_test.go
+++ b/internal/agentos/mcp_verbs_test.go
@@ -60,7 +60,7 @@
 		t.Fatal("sprint absent from atlas")
 	}
 	command := yokemcp.RegisteredCommand{
-		Name: "sprint", Synopsis: "static candidate", Effects: entry.Effects, OS: entry.OS,
+		Static: true, Name: "sprint", Synopsis: "static candidate", Effects: entry.Effects, OS: entry.OS,
 		Run: func(context.Context, []string, string, string) (string, string, int) { return "initial", "", 0 },
 	}
 	commands := []yokemcp.RegisteredCommand{command}
@@ -72,8 +72,16 @@
 		t.Fatal(err)
 	}
 	out := mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "sprint", map[string]any{}))
+	if out.Stdout != "initial" {
+		t.Fatalf("unexpected refresh result: %+v", out)
+	}
+	commands[0].Static = false
+	if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
+		t.Fatal(err)
+	}
+	out = mcpSessionDecode[yokemcp.RunToolOutput](t, mcpSessionCall(t, ctx, cs, "sprint", map[string]any{}))
 	if out.Stdout != "refreshed" {
-		t.Fatalf("unexpected refresh result: %+v", out)
+		t.Fatalf("ring did not replace static adapter: %+v", out)
 	}
 	commands = nil
 	if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
```

## Validation

Final unmodified-dependency gate:

```sh
go build ./internal/... && go vet ./internal/agentos/ && go test ./internal/agentos/ -run 'MCP'
```

Build: passed. Vet: passed. MCP tests: **22 top-level passed, 0 failed, 0 skipped;
33 passed including subtests, 0 failed, 0 skipped**. Counts were collected from
`go test -json ./internal/agentos/ -run MCP` on the same final test sources.

The yoke patch and test delta were supplied through `go test -overlay` using
workspace-local scratch copies of `mcp/direct.go` and `mcp_verbs_test.go`.
Overlay MCP tests: **22 top-level passed, 0 failed, 0 skipped; 33 passed including
subtests, 0 failed, 0 skipped**. The original sibling sources were untouched.

An initial `go doc` probe reported a missing nested ollama module; the actual
build, vet and test gate succeeded, so that diagnostic is not a gate blocker.
`make test-bash` was not run.
