package agentos

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qiangli/yoke/pkg/service"
)

// This control uses an actual prebuilt product, not a fake middleware terminal.
// The native acceptance gate must supply the candidate with BASHY_STATUS_E2E_BIN.
func TestServiceStatusRealCLIContext(t *testing.T) {
	bin := os.Getenv("BASHY_STATUS_E2E_BIN")
	if bin == "" {
		t.Skip("requires exact prebuilt product via BASHY_STATUS_E2E_BIN")
	}
	root := t.TempDir()
	sock := shortSocket(t)
	listener, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var failed atomic.Bool
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			func() {
				defer c.Close()
				var req ctlRequest
				if json.NewDecoder(c).Decode(&req) != nil {
					return
				}
				state := service.Active
				reason := ""
				if failed.Load() {
					state = service.Failed
					reason = "fixture failure"
				}
				_ = json.NewEncoder(c).Encode(ctlResponse{OK: true, PID: 1234, Units: []service.Status{{Unit: "s562-telemetry-fixture", Enabled: true, State: state, Reason: reason}}})
			}()
		}
	}()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(root, "telemetry", "spans.jsonl")
	env := append(os.Environ(),
		"HOME="+home, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"),
		"BASHY_HOME="+filepath.Join(home, ".bashy"), "BASHY_SERVICES_DIR="+filepath.Join(home, "services"), "BASHY_SERVICES_PATH=",
		"BASHY_SUPERVISORD_SOCKET="+sock, "BASHY_AGENTIC=0", "BASHY_OTEL_SPOOL="+spool, "OTEL_TRACES_EXPORTER=file", "OTEL_TRACES_SAMPLER=always_on")
	run := func(wantSuccess bool, args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if (err == nil) != wantSuccess {
			t.Fatalf("%v: unexpected exit %v; output: %s", args, err, out)
		}
		if strings.Contains(string(out), "emitting without suppression") || strings.Contains(string(out), "cannot persist status state") {
			t.Fatalf("isolated status writer fell back: %s", out)
		}
	}
	records := func() []map[string]any {
		t.Helper()
		f, err := os.Open(spool)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var result []map[string]any
		s := bufio.NewScanner(f)
		for s.Scan() {
			var rec map[string]any
			if err := json.Unmarshal(s.Bytes(), &rec); err != nil {
				t.Fatal(err)
			}
			result = append(result, rec)
		}
		if err := s.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	count := func(recs []map[string]any, msg string) int {
		n := 0
		for _, r := range recs {
			if r["_msg"] == msg {
				n++
			}
		}
		return n
	}
	for i := 0; i < 12; i++ {
		run(true, "service", "status", "s562-telemetry-fixture", "--json")
	}
	recs := records()
	if got := count(recs, "exec service"); got != 0 {
		t.Fatalf("actual CLI lost typed producer context: %d generic polls", got)
	}
	if got := count(recs, "service.status"); got != 1 {
		t.Fatalf("identical subprocess polls emitted %d statuses, want first observation only", got)
	}
	failed.Store(true)
	run(true, "service", "status", "s562-telemetry-fixture", "--json")
	run(true, "service", "status", "s562-telemetry-fixture", "--json")
	recs = records()
	if got := count(recs, "service.status"); got != 2 {
		t.Fatalf("state transition/repeat emitted %d statuses, want 2", got)
	}
	foundFailure := false
	for _, r := range recs {
		if r["status_state"] == "error" && r["status_prior_state"] == "healthy" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Fatalf("reported failure transition missing: %v", recs)
	}
	run(false, "service", "status", "s562-absent-fixture", "--json")
	recs = records()
	if got := count(recs, "service.status_query_failed"); got != 1 {
		t.Fatalf("query failure missing: %v", recs)
	}
	if got := count(recs, "exec service"); got != 0 {
		t.Fatalf("typed success/query failure leaked %d generic spans", got)
	}
	// Cobra rejects this before the typed producer: its failure must stay visible.
	run(false, "service", "status", "--s562-invalid-flag")
	if got := count(records(), "exec service"); got != 1 {
		t.Fatalf("unhandled status-shaped failure emitted %d generic spans, want 1", got)
	}
}
