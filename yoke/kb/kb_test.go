package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/recall"
)

func isolateKB(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASHY_KB_DIR", filepath.Join(root, "host-kb"))
	t.Setenv("BASHY_HOME", filepath.Join(root, "home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(root, "agent-data"))
	if err := os.MkdirAll(filepath.Join(root, "host-kb"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := newKBCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy kb:", nil)
	if code != 0 {
		t.Fatalf("kb --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("kb --help produced empty stdout")
	}
	for _, sub := range []string{"recall", "context", "ask", "memory"} {
		if c, _, err := cmd.Find([]string{sub}); err != nil || c == nil || c.Name() != sub {
			t.Errorf("kb %s is not mounted: %v", sub, err)
		}
	}
	// No allowed extra command packages: kb owns its code-ring reader.
	adapter.AssertStandaloneDeps(t, "kb")
}

func TestKBContextRefusesSingleStoreFlags(t *testing.T) {
	for _, flag := range []string{"--dir", "--repo", "--user", "--base-dir"} {
		t.Run(strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			root := isolateKB(t)
			t.Chdir(root)
			cmd := newKBCmd()
			args := []string{"context", "--for", "task", flag}
			if flag == "--dir" || flag == "--base-dir" {
				args = append(args, root)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "selects ONE kb store") {
				t.Fatalf("%s error = %v, want explicit single-store refusal", flag, err)
			}
		})
	}
}

func TestKBAskMountedWithStepJSON(t *testing.T) {
	root := isolateKB(t)
	t.Chdir(root)
	cmd := newKBCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"ask", "missing topic", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("kb ask: %v; stderr=%s", err, stderr.String())
	}
	var step struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &step); err != nil {
		t.Fatalf("decode kb ask: %v; stdout=%s", err, stdout.String())
	}
	if step.SchemaVersion != "bashy-kb-ask-step-v1" {
		t.Fatalf("kb ask schema_version = %q; stdout=%s", step.SchemaVersion, stdout.String())
	}
}

// Inside a git repo kb injects ITS OWN code-ring reader (no yoke/cmds/graph).
func TestKBContextCodeRingIsOwnReader(t *testing.T) {
	var r recall.Reader = NewCodeRing(t.TempDir())
	if r.Ring() != recall.RingRepo {
		t.Fatalf("code ring = %q, want %q", r.Ring(), recall.RingRepo)
	}
	if f := r.Forms(); len(f) != 1 || f[0] != "code" {
		t.Fatalf("code ring forms = %v", f)
	}
}

func TestKBContextSuppressesSingleStoreScopeHeader(t *testing.T) {
	root := isolateKB(t)
	t.Chdir(root)
	cmd := newKBCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"context", "--for", "missing topic", "--rings", "host", "--forms", "note", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "kb [") {
		t.Fatalf("context leaked kb single-store scope header: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"context_version": 1`) {
		t.Fatalf("context output missing frozen envelope version: %s", stdout.String())
	}
}

func TestIndexFileReaderNoIndexIsEmpty(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	hits, err := indexFileReader{}.Recall(recall.Query{Text: "x"})
	if err != nil || len(hits) != 0 {
		t.Fatalf("hits=%v err=%v, want none", hits, err)
	}
}

func TestKBExitCode(t *testing.T) {
	if got := kbExitCode(os.ErrNotExist); got != 1 {
		t.Fatalf("plain error exit = %d, want 1", got)
	}
	if got := kbExitCode(&kbmemory.ExitError{Code: 4}); got != 4 {
		t.Fatalf("ExitError exit = %d, want 4", got)
	}
}
