package agentos

import (
	"errors"
	"reflect"
	"runtime"
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
		{"stale running machine is restarted once", "darwin", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine inspect", false}, {"machine stop", false}, {"machine start", false}, {"info", false}}, ""},
		{"unreachable after restart", "darwin", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine inspect", false}, {"machine stop", true}, {"machine start", true}, {"info", true}}, "bashy podman machine init"},
		// S525.A5w: a fresh host has no machine at all — create it, then boot it.
		{"no machine is initialized and started", "windows", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine inspect", true}, {"machine init", false}, {"machine start", false}, {"info", false}}, ""},
		{"no machine and init fails stays closed", "windows", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine inspect", true}, {"machine init", true}}, "bashy podman machine init"},
		{"initialized machine still unreachable stays closed", "darwin", []step{{"info", true}, {"machine start", true}, {"info", true}, {"machine inspect", true}, {"machine init", false}, {"machine start", true}, {"info", true}}, "bashy podman machine init"},
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

// S525.A5w: the oci/sandbox/docker/podman front door ensures the machine for
// engine subcommands only — never for `machine …` or the probes the ensure
// itself runs, and never when a host engine is selected.
func TestPodmanNeedsMachine(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	cases := []struct {
		name string
		goos string
		args []string
		env  func(string) string
		want bool
	}{
		{"run on windows", "windows", []string{"run", "--rm", "alpine", "true"}, env(), true},
		{"pull on darwin", "darwin", []string{"pull", "alpine"}, env(), true},
		{"ps after a global value flag", "windows", []string{"--log-level", "debug", "ps"}, env(), true},
		{"linux runs natively", "linux", []string{"run", "alpine"}, env(), false},
		{"machine init is not ensured", "windows", []string{"machine", "init"}, env(), false},
		{"machine start is not ensured", "windows", []string{"machine", "start"}, env(), false},
		{"info is the probe", "windows", []string{"info"}, env(), false},
		{"version", "windows", []string{"version"}, env(), false},
		{"help", "windows", []string{"run", "--help"}, env(), false},
		{"bare", "windows", nil, env(), false},
		{"inside the ensure", "windows", []string{"run", "alpine"}, env(podmanEnsuringEnv, "1"), false},
		{"host engine", "darwin", []string{"run", "alpine"}, env("BASHY_PODMAN_SYSTEM", "1"), false},
		{"explicit CONTAINER_HOST", "darwin", []string{"run", "alpine"}, env("CONTAINER_HOST", "unix:///x.sock"), false},
		{"other connection", "windows", []string{"-c", "other", "run", "alpine"}, env(), false},
		{"run -c is cpu shares", "windows", []string{"run", "-c", "512", "alpine"}, env(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := podmanNeedsMachine(tc.goos, tc.args, tc.env); got != tc.want {
				t.Fatalf("podmanNeedsMachine(%s, %q) = %v, want %v", tc.goos, tc.args, got, tc.want)
			}
		})
	}
}

// The engine alias routes `bashy oci run` through the ensure seam, and a
// machine that cannot be made reachable fails closed with 125.
func TestEnsureEngineReadyOciRouting(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("linux runs podman natively; no machine to ensure")
	}
	t.Setenv(podmanEnsuringEnv, "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("BASHY_PODMAN_SYSTEM", "")
	t.Setenv("CONTAINER_CONNECTION", "")
	saved := ensurePodmanMachineHook
	defer func() { ensurePodmanMachineHook = saved }()
	calls := 0
	ensurePodmanMachineHook = func(string) error { calls++; return nil }
	for _, alias := range []string{"oci", "sandbox", "docker", "podman"} {
		if code := ensureEngineReady(engineAlias(alias), []string{"run", "--rm", "alpine", "true"}); code != 0 {
			t.Fatalf("%s run: code %d", alias, code)
		}
	}
	if calls != 4 {
		t.Fatalf("ensure calls = %d, want 4", calls)
	}
	if code := ensureEngineReady(engineAlias("oci"), []string{"machine", "init"}); code != 0 || calls != 4 {
		t.Fatalf("machine init must not ensure: code %d calls %d", code, calls)
	}
	if code := ensureEngineReady("ollama", []string{"run", "x"}); code != 0 || calls != 4 {
		t.Fatalf("ollama must not ensure podman: code %d calls %d", code, calls)
	}
	ensurePodmanMachineHook = func(string) error { return errPodmanUnreachable }
	if code := ensureEngineReady(engineAlias("oci"), []string{"run", "alpine"}); code != 125 {
		t.Fatalf("unreachable machine: code %d, want 125", code)
	}
}
