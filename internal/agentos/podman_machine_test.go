package agentos

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Sprint 320: a ~~~dockerfile/~~~k8s fence (and @contain's image provider)
// must find bashy's podman machine reachable without a manual start — a fresh
// ssh session on Windows finds it stopped, and macOS can leave a "running"
// machine whose socket refuses the handshake until stop + start.
func TestEnsurePodmanMachine(t *testing.T) {
	type step struct {
		args string
		fail bool
	}
	cases := []struct {
		name    string
		goos    string
		steps   []step
		wantErr string
	}{
		{"linux needs no VM", "linux", nil, ""},
		{"already reachable", "windows", []step{{"info", false}}, ""},
		{"stopped machine is started", "windows", []step{{"info", true}, {"machine start", false}, {"info", false}}, ""},
		{"stale running machine is restarted once", "darwin", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine stop", false}, {"machine start", false}, {"info", false}}, ""},
		{"unreachable after restart", "darwin", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine stop", true}, {"machine start", true}, {"info", true}}, "bashy podman machine init"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			i := 0
			run := func(args ...string) error {
				joined := strings.Join(args, " ")
				got = append(got, joined)
				if i >= len(tc.steps) {
					t.Fatalf("unexpected podman %s", joined)
				}
				s := tc.steps[i]
				i++
				if s.args != joined {
					t.Fatalf("call %d = podman %s, want podman %s", i, joined, s.args)
				}
				if s.fail {
					return errors.New("exit 125")
				}
				return nil
			}
			err := ensurePodmanMachineWith(tc.goos, run)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
			}
			var want []string
			for _, s := range tc.steps {
				want = append(want, s.args)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("calls = %q, want %q", got, want)
			}
		})
	}
}
