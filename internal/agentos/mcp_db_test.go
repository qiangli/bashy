package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
)

func dbMCPResult(t *testing.T, result any) (yokemcp.RunToolOutput, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var out yokemcp.RunToolOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if out.Stdout != "" {
		if err := json.Unmarshal([]byte(out.Stdout), &doc); err != nil {
			t.Fatalf("invalid db envelope %q: %v", out.Stdout, err)
		}
	}
	return out, doc
}

func TestMCPDbLifecycleBridge(t *testing.T) {
	ringDir(t)
	writeRecord(t, fleet.Command{Name: "mcp-existing", Synopsis: "existing", Script: "true", Effects: []string{"pure"}})
	opts := mcpTestOptions(t)
	opts.Policy.Allow["destroy"] = true
	ctx, client, _ := mcpFrontClient(t, opts)
	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var schema map[string]any
	for _, item := range listed.Tools {
		names = append(names, item.Name)
		if item.Name == "db_lifecycle" {
			raw, _ := json.Marshal(item.InputSchema)
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"db_lifecycle", "mcp-existing", "run_tool"} {
		if !slices.Contains(names, name) {
			t.Fatalf("missing %s from %v", name, names)
		}
	}
	props, _ := schema["properties"].(map[string]any)
	for _, name := range []string{"verb", "engine", "mode", "path", "yes"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("missing declared db flag %s: %v", name, schema)
		}
	}
	path := filepath.Join(t.TempDir(), "lifecycle.db")
	call := func(verb string) (yokemcp.RunToolOutput, map[string]any) {
		t.Helper()
		in := map[string]any{"verb": verb, "engine": "sqlite", "mode": "persistent", "path": path}
		return dbMCPResult(t, mcpSessionCall(t, ctx, client, "db_lifecycle", in).StructuredContent)
	}
	missing, doc := call("show")
	if missing.ExitCode != 1 || doc["schema_version"] != "bashy-db-error-v1" || doc["error"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("missing show: %+v %v", missing, doc)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing show created %s: %v", path, err)
	}
	created, doc := call("create")
	if created.ExitCode != 0 || doc["schema_version"] != "bashy-db-result-v1" {
		t.Fatalf("create: %+v %v", created, doc)
	}
	generic, doc := dbMCPResult(t, mcpSessionCall(t, ctx, client, "run_tool", yokemcp.RunToolInput{
		Name: "db", Args: []string{"check", "--engine", "sqlite", "--mode", "persistent", "--path", path},
	}).StructuredContent)
	if generic.ExitCode != 0 || doc["schema_version"] != "bashy-db-result-v1" {
		t.Fatalf("generic check: %+v %v", generic, doc)
	}
}

func TestMCPDbLifecycleSelectionAndPolicy(t *testing.T) {
	ringDir(t)
	for _, profile := range []string{"default", "all", "db_lifecycle", "db"} {
		opts, err := mcpOptions("", profile)
		if err != nil || !slices.ContainsFunc(opts.Registered(), func(command yokemcp.RegisteredCommand) bool {
			return command.Name == "db_lifecycle"
		}) {
			t.Fatalf("profile %s omits typed db: %v", profile, err)
		}
	}
	echo, err := mcpOptions("", "echo")
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(echo.Registered(), func(command yokemcp.RegisteredCommand) bool {
		return command.Name == "db_lifecycle"
	}) {
		t.Fatal("unrelated restricted profile exposed db_lifecycle")
	}
	echo.Policy.Audit = func(yokemcp.Record) {}
	echoCtx, echoClient, _ := mcpFrontClient(t, echo)
	echoTools, err := echoClient.ListTools(echoCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range echoTools.Tools {
		if item.Name == "db_lifecycle" {
			t.Fatal("echo-only discovery exposed db_lifecycle")
		}
	}
	opts, err := mcpOptions("", "db_lifecycle")
	if err != nil || len(opts.Tools) != 0 {
		t.Fatalf("explicit selection: %+v %v", opts.Tools, err)
	}
	opts.Policy.Audit = func(yokemcp.Record) {}
	ctx, client, _ := mcpFrontClient(t, opts)
	result := mcpSessionCall(t, ctx, client, "db_lifecycle", map[string]any{"verb": "list"})
	out, _ := dbMCPResult(t, result.StructuredContent)
	if !result.IsError || out.ExitCode != 126 {
		t.Fatalf("destroy policy: %+v %+v", result, out)
	}
}
