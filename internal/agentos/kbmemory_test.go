// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/ref"
)

func kbMemoryTarget(path string) string {
	return (ref.Locator{Scheme: "db", Target: "sqlite:file:" + path}).String()
}

// runKBMemory runs `kb memory ARGS` through the same tree the runtime
// dispatch executes and returns stdout plus the exit code it would report.
func runKBMemory(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := schemaKBCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(append([]string{"memory"}, args...))
	if err := cmd.Execute(); err != nil {
		return stdout.String(), kbExitCode(err)
	}
	return stdout.String(), 0
}

func TestKBMemorySchemaRegistered(t *testing.T) {
	schemas := cobraSchemas("kb")
	memory, ok := schemas["kb memory"]
	if !ok {
		t.Fatalf("kb memory missing from the kb schema tree: %v", schemas)
	}
	for _, name := range []string{"if-revision", "request-id", "session", "json", "file"} {
		if !slices.ContainsFunc(memory.Flags, func(f tool.ArgFlag) bool { return f.Name == name }) {
			t.Fatalf("kb memory schema lacks --%s: %+v", name, memory.Flags)
		}
	}
	if _, ok := schemas["kb recall"]; !ok {
		t.Fatal("mounting memory dropped kb recall from the schema tree")
	}
}

func TestKBMemoryRefusesInheritedScope(t *testing.T) {
	for _, flag := range []string{"--dir", "--repo", "--user", "--base-dir"} {
		t.Run(strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			root := isolateKBCommandTest(t)
			t.Chdir(root)
			path := filepath.Join(root, "mem.db")
			args := []string{"select", kbMemoryTarget(path), "--json", flag}
			if flag == "--dir" || flag == "--base-dir" {
				args = append(args, root)
			}
			out, code := runKBMemory(t, args...)
			if code != 2 || !strings.Contains(out, `"invalid_input"`) || !strings.Contains(out, "a memory TARGET is named explicitly") {
				t.Fatalf("%s: exit %d %s, want invalid_input refusal", flag, code, out)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s refusal touched %s: %v", flag, path, err)
			}
		})
	}
}

// TestKBMemoryExplicitInitNoLegacyStore: nothing exists until init names a
// target, the lifecycle runs end to end through the mounted command, and kb's
// own single-store hook never runs — no OKF store is opened or created.
func TestKBMemoryExplicitInitNoLegacyStore(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	path := filepath.Join(root, "mem.db")
	target := kbMemoryTarget(path)

	if out, code := runKBMemory(t, "select", target, "--json"); code == 0 || !strings.Contains(out, "bashy-kb-memory-error-v1") {
		t.Fatalf("select before init: exit %d %s", code, out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("select created %s: %v", path, err)
	}
	if out, code := runKBMemory(t, "init", target, "--json"); code != 0 || !strings.Contains(out, `"schema_version":"bashy-kb-memory-result-v1"`) {
		t.Fatalf("init: exit %d %s", code, out)
	}
	out, code := runKBMemory(t, "add", target, "--body", "remember this", "--json")
	var env struct {
		Result struct {
			Record struct {
				ID       string `json:"id"`
				Revision int64  `json:"revision"`
			} `json:"record"`
		} `json:"result"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || env.Result.Record.ID == "" {
		t.Fatalf("add: exit %d %s", code, out)
	}
	if out, code := runKBMemory(t, "inspect", target, env.Result.Record.ID, "--json"); code != 0 || !strings.Contains(out, "remember this") {
		t.Fatalf("inspect: exit %d %s", code, out)
	}
	// A stale revision is the contract's conflict exit, preserved by dispatch.
	if out, code := runKBMemory(t, "update", target, env.Result.Record.ID, "--if-revision", "99", "--body", "x", "--json"); code != 1 || !strings.Contains(out, `"conflict"`) {
		t.Fatalf("stale update: exit %d %s", code, out)
	}
	if out, code := runKBMemory(t, "bogus", target, "--json"); code != 2 {
		t.Fatalf("unknown verb: exit %d %s", code, out)
	}

	entries, err := os.ReadDir(filepath.Join(root, "host-kb"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("kb memory touched the host kb store: %v %v", entries, err)
	}
	for _, legacy := range []string{".agents", "home"} {
		if _, err := os.Stat(filepath.Join(root, legacy)); !os.IsNotExist(err) {
			t.Fatalf("kb memory created legacy store %s: %v", legacy, err)
		}
	}
}

// TestKBMemoryCLIAndMCPExitParity: the kb_memory MCP tool is registered when
// selected and reports the same exit codes as the CLI dispatch.
func TestKBMemoryCLIAndMCPExitParity(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	ringDir(t)
	opts, err := mcpOptions("", "kb_memory")
	if err != nil {
		t.Fatal(err)
	}
	opts.Policy.Audit = func(yokemcp.Record) {}
	ctx, client, _ := mcpFrontClient(t, opts)
	listed, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(listed.Tools, func(item *mcpsdk.Tool) bool { return item.Name == "kb_memory" }) {
		t.Fatal("kb_memory not registered for --tools kb_memory")
	}
	target := kbMemoryTarget(filepath.Join(root, "absent.db"))
	for _, verb := range []string{"select", "bogus"} {
		_, cli := runKBMemory(t, verb, target, "--json")
		out, _ := dbMCPResult(t, mcpSessionCall(t, ctx, client, "kb_memory", map[string]any{"verb": verb, "target": target, "json": true}).StructuredContent)
		if cli == 0 || out.ExitCode != cli {
			t.Fatalf("%s: cli exit %d, mcp exit %d (%s)", verb, cli, out.ExitCode, out.Stdout)
		}
	}
}

func TestKBMemoryExitCodeKeepsContract(t *testing.T) {
	if got := kbExitCode(&kbmemory.ExitError{Code: 124}); got != 124 {
		t.Fatalf("timeout exit %d", got)
	}
	if got := kbExitCode(errors.New("other kb failure")); got != 1 {
		t.Fatalf("plain kb error exit %d", got)
	}
}
