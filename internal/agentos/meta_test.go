// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/webconsole"
)

const metaHelperEnv = "BASHY_TEST_META_DISPATCH"
const metaHumanEnv = "BASHY_TEST_META_HUMAN"

// TestMetaDispatchHelper is a real process boundary because dispatchMeta exits
// after answering, exactly as the installed front door does.
func TestMetaDispatchHelper(t *testing.T) {
	verb := os.Getenv(metaHelperEnv)
	if verb == "" {
		return
	}
	args := []string{"bashy", verb, "meta", "--json"}
	if os.Getenv(metaHumanEnv) != "" {
		args = args[:3]
	}
	dispatchMeta(args)
	// A non-surface must get its operand back rather than being consumed.
	_, _ = os.Stdout.WriteString("PASSTHROUGH")
}

func runMetaHelper(t *testing.T, verb string, env ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMetaDispatchHelper$")
	cmd.Env = append(os.Environ(), append([]string{metaHelperEnv + "=" + verb}, env...)...)
	out, err := cmd.Output()
	return string(out), err
}

func TestEveryWebSurfaceAnswersExecutableMetaJSON(t *testing.T) {
	for _, verb := range atlas.WebSurfaceNames() {
		out, err := runMetaHelper(t, verb)
		if err != nil {
			t.Fatalf("%s meta: %v", verb, err)
		}
		var got webconsole.AppMeta
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%s meta JSON %q: %v", verb, out, err)
		}
		if got.SchemaVersion != webconsole.MetaSchema || got.Name != verb || got.Auth != webconsole.AuthSystem {
			t.Fatalf("%s meta = %+v", verb, got)
		}
		if len(got.Start) > 0 && got.Start[0] != "bashy" {
			t.Fatalf("%s start is not complete argv: %#v", verb, got.Start)
		}
	}
}

func TestNonSurfaceMetaOperandIsNotIntercepted(t *testing.T) {
	out, err := runMetaHelper(t, "grep")
	if err != nil || !strings.HasPrefix(out, "PASSTHROUGH") {
		t.Fatalf("grep meta dispatch = %q, %v", out, err)
	}
}

func TestHumanMetaStartHintUsesCompleteArgvOnce(t *testing.T) {
	out, err := runMetaHelper(t, "meet", metaHumanEnv+"=1")
	if err != nil || !strings.Contains(out, "start") || !strings.Contains(out, "bashy meet serve\n") {
		t.Fatalf("meet human meta = %q, %v", out, err)
	}
	if strings.Contains(out, "bashy bashy") {
		t.Fatalf("meet human meta duplicated launcher: %q", out)
	}
}

func TestMBMetaIsReadOnlyAndAppendsNothing(t *testing.T) {
	board := t.TempDir()
	out, err := runMetaHelper(t, "mb", "BASHY_MB_DIR="+board, "BASHY_ROOM_DIR="+board)
	if err != nil || !strings.Contains(out, `"schema_version"`) {
		t.Fatalf("mb meta = %q, %v", out, err)
	}
	entries, err := os.ReadDir(board)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("mb meta wrote board state: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(board, "posts.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("mb meta created posts.jsonl: %v", err)
	}
}
