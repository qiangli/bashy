// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/execlog"
	"github.com/qiangli/coreutils/pkg/policy/audit"
)

const frontDoorObserveHelper = "BASHY_TEST_FRONT_DOOR_OBSERVE"

func TestFrontDoorObserveHelper(t *testing.T) {
	if os.Getenv(frontDoorObserveHelper) == "" {
		return
	}
	os.Args = []string{os.Args[0], "skill", "show", "conductor"}
	Dispatch()
	t.Fatal("front-door dispatch returned")
}

func TestFrontDoorRecordsAuditAndExecLog(t *testing.T) {
	home := t.TempDir()
	stores := map[string]string{
		"BASHY_HOME":       home,
		"BASHY_FLEET_DIR":  t.TempDir(),
		"BASHY_SKILLS_DIR": t.TempDir(),
		"BASHY_TOOLS_DIR":  t.TempDir(),
		"BASHY_MODELS_DIR": t.TempDir(),
		"BASHY_AGENTS_DIR": t.TempDir(),
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFrontDoorObserveHelper$")
	cmd.Env = frontDoorTestEnv(stores)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("front-door skill show: %v\n%s", err, out)
	}

	auditFile := filepath.Join(home, "audit", "audit.jsonl")
	f, err := os.Open(auditFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var gotAudit audit.Record
	if !bufio.NewScanner(f).Scan() {
		t.Fatal("audit log has no front-door record")
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(f).Decode(&gotAudit); err != nil {
		t.Fatal(err)
	}
	if gotAudit.Binary != "skill" || gotAudit.Exit != 0 || !reflect.DeepEqual(gotAudit.Argv, []string{"skill", "show", "conductor"}) {
		t.Fatalf("audit record = binary %q argv %q exit %d", gotAudit.Binary, gotAudit.Argv, gotAudit.Exit)
	}

	records, _, err := execlog.Read(filepath.Join(home, "exec"), execlog.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Cmd != "skill" || records[0].Exit == nil || *records[0].Exit != 0 {
		t.Fatalf("exec records = %#v", records)
	}
}

func frontDoorTestEnv(stores map[string]string) []string {
	drop := map[string]bool{
		frontDoorObserveHelper: true,
		"BASHY_AUDIT":          true,
		"BASHY_EXECHIST":       true,
		"BASHY_AGENTIC":        true,
	}
	for name := range stores {
		drop[name] = true
	}
	env := make([]string, 0, len(os.Environ())+len(stores)+4)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !drop[name] {
			env = append(env, entry)
		}
	}
	env = append(env, frontDoorObserveHelper+"=1", "BASHY_AUDIT=1", "BASHY_EXECHIST=1", "BASHY_AGENTIC=1")
	for name, dir := range stores {
		env = append(env, name+"="+dir)
	}
	return env
}
