package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a

import "testing"

func TestPodmanStartsMachine(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"machine", "start"}, true},
		{[]string{"--log-level", "debug", "machine", "start"}, true},
		{[]string{"--log-level=debug", "machine", "start", "podman-machine-default"}, true},
		{[]string{"--remote", "machine", "start"}, true},
		{[]string{"machine", "start", "--no-info"}, true},
		{[]string{"machine", "stop"}, false},
		{[]string{"machine", "init", "--now"}, false},
		{[]string{"run", "--rm", "alpine", "machine", "start"}, false},
		{[]string{"machine"}, false},
		{[]string{"info"}, false},
		{nil, false},
	} {
		if got := podmanStartsMachine(c.args); got != c.want {
			t.Errorf("%q: got %v, want %v", c.args, got, c.want)
		}
	}
}

func TestEngineDaemonDirIsNotTheWorkTree(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	if got := engineDaemonDir(); got == "" || got == work {
		t.Fatalf("engineDaemonDir() = %q: machine daemons would pin the work tree", got)
	}
}
