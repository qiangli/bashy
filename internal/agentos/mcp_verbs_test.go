// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
)

func mcpVerbOptions(t *testing.T, allow, profile string) yokemcp.Options {
	t.Helper()
	opts, err := mcpOptions(allow, profile)
	if err != nil {
		t.Fatal(err)
	}
	opts.Policy.Audit = func(yokemcp.Record) {}
	return opts
}

func fakeMCPVerb(t *testing.T) *struct {
	Argv       []string
	Stdin, Dir string
	Calls      int
} {
	t.Helper()
	got := &struct {
		Argv       []string
		Stdin, Dir string
		Calls      int
	}{}
	old := mcpVerbExec
	t.Cleanup(func() { mcpVerbExec = old })
	mcpVerbExec = func(_ context.Context, argv []string, stdin, dir string) (string, string, int) {
		got.Argv, got.Stdin, got.Dir = slices.Clone(argv), stdin, dir
		got.Calls++
		return "canned stdout", "canned stderr", 7
	}
	return got
}

func TestMCPVerbCall(t *testing.T) {
	ringDir(t)
	got := fakeMCPVerb(t)
	ctx, cs, _ := mcpFrontClient(t, mcpVerbOptions(t, "", "all"))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var sprint *mcpsdk.Tool
	for _, item := range list.Tools {
		if item.Name == "sprint" {
			sprint = item
		}
	}
	if sprint == nil {
		t.Fatal("sprint absent")
	}
	raw, err := json.Marshal(sprint)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Meta struct {
			Effects []string `json:"bashy.effects"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	entry, _ := atlas.Lookup("sprint")
	if !reflect.DeepEqual(wire.Meta.Effects, entry.Effects) {
		t.Fatalf("effects: %s", raw)
	}
	dir := t.TempDir()
	result := mcpSessionCall(t, ctx, cs, "sprint", map[string]any{"args": []string{"--help"}, "stdin": "input", "dir": dir})
	out := mcpVerbDecode(t, result)
	if !reflect.DeepEqual(got.Argv, []string{bashySelfPath(), "sprint", "--help"}) || got.Stdin != "input" || got.Dir != dir {
		t.Fatalf("dispatch: %+v", got)
	}
	if out.Stdout != "canned stdout" || out.Stderr != "canned stderr" || out.ExitCode != 7 || !result.IsError {
		t.Fatalf("result: %+v", out)
	}
}

func TestMCPVerbPolicy(t *testing.T) {
	ringDir(t)
	got := fakeMCPVerb(t)
	name := ""
	for _, command := range mcpVerbCommands() {
		if slices.Contains(command.OS, runtime.GOOS) && (slices.Contains(command.Effects, "destroy") || slices.Contains(command.Effects, "spend")) {
			name = command.Name
			break
		}
	}
	if name == "" {
		t.Fatal("no privileged verb fixture")
	}
	ctx, cs, _ := mcpFrontClient(t, mcpVerbOptions(t, "", "all"))
	result := mcpSessionCall(t, ctx, cs, name, map[string]any{"args": []string{"--help"}})
	denied := mcpVerbDecode(t, result)
	if !result.IsError || denied.ExitCode != 126 || got.Calls != 0 {
		t.Fatalf("policy bypass: %+v calls=%d", denied, got.Calls)
	}
	ctx, cs, _ = mcpFrontClient(t, mcpVerbOptions(t, "destroy,spend,cred,priv", "all"))
	out := mcpVerbDecode(t, mcpSessionCall(t, ctx, cs, name, map[string]any{"args": []string{"--help"}}))
	if got.Calls != 1 || out.Stdout != "canned stdout" || out.ExitCode != 7 {
		t.Fatalf("grant: %+v calls=%d", out, got.Calls)
	}
}

func TestMCPVerbCatalog(t *testing.T) {
	ringDir(t)
	opts := mcpVerbOptions(t, "", "all")
	ctx, cs, _ := mcpFrontClient(t, opts)
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, item := range list.Tools {
		if names[item.Name] {
			t.Fatalf("duplicate %s", item.Name)
		}
		names[item.Name] = true
	}
	for _, name := range hiddenVerbsCatalog() {
		// Registry exposure is its own surface; this lane excludes hidden verbs.
		if tool.Lookup(name) == nil && names[name] {
			t.Errorf("hidden verb %s", name)
		}
	}
	for _, command := range mcpVerbCommands() {
		if !command.Static || command.Schema != nil {
			t.Fatalf("adapter contract: %+v", command)
		}
		if !slices.Contains(command.OS, runtime.GOOS) {
			if names[command.Name] {
				t.Errorf("unsupported verb %s", command.Name)
			}
		} else if !names[command.Name] {
			t.Errorf("missing verb %s", command.Name)
		}
	}
	ctx2, cs2, _ := mcpFrontClient(t, mcpVerbOptions(t, "", "all"))
	list2, err := cs2.ListTools(ctx2, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(list)
	second, _ := json.Marshal(list2)
	if !bytes.Equal(first, second) {
		t.Fatal("tools/list differs between servers")
	}
}

func TestMCPVerbProfiles(t *testing.T) {
	ringDir(t)
	for _, profile := range []string{"default", "sprint", "sprint,echo"} {
		t.Run(profile, func(t *testing.T) {
			ctx, cs, _ := mcpFrontClient(t, mcpVerbOptions(t, "", profile))
			list, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range list.Tools {
				found = found || item.Name == "sprint"
				if item.Name == "weave" {
					t.Fatal("unselected verb exposed")
				}
			}
			if found != (profile != "default") {
				t.Fatalf("sprint exposed=%v", found)
			}
		})
	}
}

func TestMCPVerbStaticRefresh(t *testing.T) {
	ringDir(t)
	fakeMCPVerb(t)
	writeRecord(t, fleet.Command{Name: "mcp-changing", Synopsis: "first", Script: "true", Effects: []string{"pure"}})
	opts := mcpVerbOptions(t, "", "all")
	ctx, cs, srv := mcpFrontClient(t, opts)
	snapshot := opts.Registered
	opts.Registered = func() []yokemcp.RegisteredCommand {
		commands := snapshot()
		for i := range commands {
			if commands[i].Name == "sprint" {
				commands[i].Run = func(context.Context, []string, string, string) (string, string, int) { return "replaced", "", 0 }
			}
		}
		return commands
	}
	writeRecord(t, fleet.Command{Name: "mcp-changing", Synopsis: "second", Script: "true", Effects: []string{"pure"}})
	if err := yokemcp.NotifyToolsChanged(srv, opts); err != nil {
		t.Fatal(err)
	}
	out := mcpVerbDecode(t, mcpSessionCall(t, ctx, cs, "sprint", map[string]any{}))
	if out.Stdout != "canned stdout" {
		t.Fatalf("static adapter replaced: %+v", out)
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Tools {
		if item.Name == "mcp-changing" && strings.Contains(item.Description, "second") {
			return
		}
	}
	t.Fatal("ring descriptor not refreshed")
}

// Error results still carry the subprocess status and streams.
func mcpVerbDecode(t *testing.T, result *mcpsdk.CallToolResult) yokemcp.RunToolOutput {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out yokemcp.RunToolOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPVerbOSFilter(t *testing.T) {
	ringDir(t)
	// Lookup exposes the atlas OS slice. Temporarily exclude this OS, restoring
	// the shared atlas before any other test runs (these tests are serial).
	entry, ok := atlas.Lookup("sprint")
	if !ok || len(entry.OS) == 0 {
		t.Fatal("missing sprint atlas OS")
	}
	original := slices.Clone(entry.OS)
	t.Cleanup(func() { copy(entry.OS, original) })
	for i := range entry.OS {
		entry.OS[i] = "unsupported-test-os"
	}
	ctx, cs, _ := mcpFrontClient(t, mcpVerbOptions(t, "", "all"))
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Tools {
		if item.Name == "sprint" {
			t.Fatal("unsupported verb exposed")
		}
	}
}
