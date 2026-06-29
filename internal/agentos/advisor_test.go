// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"
)

// healthyProbe is an environment where nothing is wrong: every host resolves,
// disk is ample and writable, every referenced path exists, RAM is unknown, and
// locality is home. The advisor must stay silent against it.
func healthyProbe() spaceProbe {
	return spaceProbe{
		resolveHost: func(string) bool { return true },
		diskFor:     func(string) (uint64, bool, bool) { return 100 << 30, false, true },
		availRAM:    func() (uint64, bool) { return 0, false },
		pathExists:  func(string, string) bool { return true },
		repoRoot:    func(string) (string, bool) { return "/repo", true },
	}
}

func TestAdviseNetworkHostUnresolvable(t *testing.T) {
	p := healthyProbe()
	p.resolveHost = func(string) bool { return false } // .local no longer resolves
	a := &advisor{probe: p}

	h := a.advise("/repo", []string{"ssh", "user@host.local"}, 255)
	if h == nil {
		t.Fatal("expected a network hint, got none")
	}
	if h.dimension != "network" {
		t.Errorf("dimension = %q, want network", h.dimension)
	}
	if h.retryable {
		t.Error("a LAN route that won't resolve must be retryable=false")
	}
	if !strings.Contains(h.text, "host.local") {
		t.Errorf("hint should name the target: %q", h.text)
	}
}

func TestAdviseNetworkResolvesNoHint(t *testing.T) {
	// Same command, but the host resolves from here (home LAN): no hint.
	a := &advisor{probe: healthyProbe()}
	if h := a.advise("/repo", []string{"ssh", "user@host.local"}, 255); h != nil {
		t.Errorf("no hint expected when host resolves, got %q", h.text)
	}
}

func TestAdviseNetworkPublicHostIgnored(t *testing.T) {
	// A public host that fails to resolve is NOT a LAN-locality problem.
	p := healthyProbe()
	p.resolveHost = func(string) bool { return false }
	a := &advisor{probe: p}
	if h := a.advise("/repo", []string{"curl", "https://example.com/x"}, 6); h != nil {
		t.Errorf("public host should not trigger the locality hint, got %q", h.text)
	}
}

func TestAdviseDiskReadOnly(t *testing.T) {
	p := healthyProbe()
	p.diskFor = func(string) (uint64, bool, bool) { return 100 << 30, true, true } // read-only
	a := &advisor{probe: p}
	h := a.advise("/repo", []string{"cp", "a", "b"}, 1)
	if h == nil || h.dimension != "disk" {
		t.Fatalf("expected disk hint, got %+v", h)
	}
	if !strings.Contains(h.text, "read-only") {
		t.Errorf("hint should mention read-only: %q", h.text)
	}
}

func TestAdviseDiskNearlyFull(t *testing.T) {
	p := healthyProbe()
	p.diskFor = func(string) (uint64, bool, bool) { return 1 << 20, false, true } // 1 MiB free
	a := &advisor{probe: p}
	h := a.advise("/repo", []string{"go", "build"}, 1)
	if h == nil || h.dimension != "disk" {
		t.Fatalf("expected disk hint, got %+v", h)
	}
}

func TestAdviseDiskAmpleNoHint(t *testing.T) {
	a := &advisor{probe: healthyProbe()}
	if h := a.advise("/repo", []string{"cp", "a", "b"}, 1); h != nil {
		t.Errorf("ample writable disk should produce no hint, got %q", h.text)
	}
}

func TestAdviseCWDWrongDir(t *testing.T) {
	p := healthyProbe()
	// foo.go is missing in cwd but present at the repo root.
	p.pathExists = func(dir, name string) bool { return dir == "/repo" }
	a := &advisor{probe: p}
	h := a.advise("/repo/sub/dir", []string{"cat", "foo.go"}, 1)
	if h == nil || h.dimension != "cwd" {
		t.Fatalf("expected cwd hint, got %+v", h)
	}
	if h.retryable {
		t.Error("wrong-dir must be retryable=false")
	}
	if !strings.Contains(h.text, "/repo") {
		t.Errorf("hint should point at the repo root: %q", h.text)
	}
}

func TestAdviseCWDFileExistsNoHint(t *testing.T) {
	// The file exists in cwd: nothing to advise.
	a := &advisor{probe: healthyProbe()}
	if h := a.advise("/repo/sub", []string{"cat", "foo.go"}, 1); h != nil {
		t.Errorf("no cwd hint expected when the file exists, got %q", h.text)
	}
}

func TestAdviseComputeOOM(t *testing.T) {
	a := &advisor{probe: healthyProbe()}
	h := a.advise("/repo", []string{"go", "test", "./..."}, 137) // 128 + SIGKILL
	if h == nil || h.dimension != "compute" {
		t.Fatalf("expected compute hint, got %+v", h)
	}
	if h.retryable {
		t.Error("an OOM kill must be retryable=false")
	}
}

func TestAdviseComputeNonHeavyIgnored(t *testing.T) {
	a := &advisor{probe: healthyProbe()}
	if h := a.advise("/repo", []string{"ls", "-l"}, 137); h != nil {
		t.Errorf("137 on a non-heavy tool should not fire the OOM hint, got %q", h.text)
	}
}

func TestAdviseComputeNonKillIgnored(t *testing.T) {
	a := &advisor{probe: healthyProbe()}
	if h := a.advise("/repo", []string{"go", "build"}, 1); h != nil {
		t.Errorf("a plain non-zero exit is not an OOM, got %q", h.text)
	}
}

// TestAdviseHealthyEnvSilent is the false-positive guard: a healthy environment
// across a spread of failing commands must yield no advice.
func TestAdviseHealthyEnvSilent(t *testing.T) {
	a := &advisor{probe: healthyProbe()}
	cases := [][]string{
		{"ssh", "user@host.local"},
		{"cp", "a", "b"},
		{"go", "build"},
		{"cat", "foo.go"},
		{"grep", "-r", "pat", "."},
	}
	for _, args := range cases {
		if h := a.advise("/repo", args, 1); h != nil {
			t.Errorf("advise(%v) on healthy env = %q, want no hint", args, h.text)
		}
	}
}

func TestExitStatusOf(t *testing.T) {
	if s, ok := exitStatusOf(nil); !ok || s != 0 {
		t.Errorf("nil err = (%d,%v), want (0,true)", s, ok)
	}
}

func TestExtractNetworkTarget(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"ssh", "user@host.local"}, "host.local"},
		{[]string{"scp", "-r", "host.lan:/tmp/x", "."}, "host.lan"},
		{[]string{"curl", "-sS", "https://api.internal:8443/v1"}, "api.internal"},
		{[]string{"ssh", "-p", "22", "192.168.1.10"}, "192.168.1.10"},
		{[]string{"cat", "file.txt"}, ""}, // file arg, not a host
	}
	for _, tt := range tests {
		if got := extractNetworkTarget(tt.args); got != tt.want {
			t.Errorf("extractNetworkTarget(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestAdviseComputeReportsRAM(t *testing.T) {
	// When availRAM is known, the OOM hint includes the figure.
	p := healthyProbe()
	p.availRAM = func() (uint64, bool) { return 512 << 20, true } // 512 MiB
	a := &advisor{probe: p}
	h := a.advise("/repo", []string{"go", "test", "./..."}, 137)
	if h == nil || h.dimension != "compute" {
		t.Fatalf("expected compute hint, got %+v", h)
	}
	if !strings.Contains(h.text, "512.0 MB") {
		t.Errorf("hint should report available RAM: %q", h.text)
	}
}

func TestParseMemAvailable(t *testing.T) {
	meminfo := []byte("MemTotal:       16331868 kB\nMemFree:          123456 kB\nMemAvailable:    2097152 kB\nBuffers:           1000 kB\n")
	got, ok := parseMemAvailable(meminfo)
	if !ok {
		t.Fatal("expected MemAvailable to parse")
	}
	if want := uint64(2097152) * 1024; got != want {
		t.Errorf("parseMemAvailable = %d, want %d", got, want)
	}
	if _, ok := parseMemAvailable([]byte("MemTotal: 100 kB\n")); ok {
		t.Error("missing MemAvailable should yield ok=false")
	}
	if _, ok := parseMemAvailable([]byte("MemAvailable: notanumber kB\n")); ok {
		t.Error("malformed MemAvailable should yield ok=false")
	}
}

func TestIsVirtualIface(t *testing.T) {
	virtual := []string{"docker0", "br-abc123", "veth1234", "utun3", "tailscale0", "wg0", "vmnet8"}
	real := []string{"en0", "eth0", "wlan0", "enp3s0"}
	for _, n := range virtual {
		if !isVirtualIface(n) {
			t.Errorf("isVirtualIface(%q) = false, want true", n)
		}
	}
	for _, n := range real {
		if isVirtualIface(n) {
			t.Errorf("isVirtualIface(%q) = true, want false", n)
		}
	}
}

func TestAdvisorEnabledControl(t *testing.T) {
	t.Setenv("DHNT_AGENT", "") // ensure agent mode is not implied by env
	for _, v := range []string{"0", "false", "off", "no"} {
		t.Setenv("BASHY_ADVISOR", v)
		if advisorEnabled() {
			t.Errorf("BASHY_ADVISOR=%q should disable the advisor", v)
		}
	}
	for _, v := range []string{"1", "true", "on", "yes"} {
		t.Setenv("BASHY_ADVISOR", v)
		if !advisorEnabled() {
			t.Errorf("BASHY_ADVISOR=%q should enable the advisor", v)
		}
	}
}

func TestLanish(t *testing.T) {
	yes := []string{"host.local", "nas.lan", "10.0.0.5", "192.168.1.1", "169.254.1.2", "box.internal"}
	no := []string{"example.com", "8.8.8.8", "api.github.com", ""}
	for _, h := range yes {
		if !lanish(h) {
			t.Errorf("lanish(%q) = false, want true", h)
		}
	}
	for _, h := range no {
		if lanish(h) {
			t.Errorf("lanish(%q) = true, want false", h)
		}
	}
}
