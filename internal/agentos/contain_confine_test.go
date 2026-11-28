package agentos

// Sprint: #738; Story: #823; Story-ID: 380fd660ae7e
//
// Per-backend unit tests of `bashy contain --dir`: the generated Seatbelt
// profile (macOS backend) and the bashy podman spec (Linux backend) are pure
// functions, so both are checked on every host; no container runs here.

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func confineFixture(t *testing.T) *containConfine {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "work")
	bundle := filepath.Join(root, "genie")
	for _, d := range []string{dir, bundle} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// a short path: sun_path is 104 bytes on macOS
	sockDir, err := os.MkdirTemp("/tmp", "cf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix socket: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return &containConfine{Dir: dir, Sockets: []string{sock}, RO: []string{bundle}, Env: []string{"GENIE_MODEL_ID"}}
}

func TestContainConfinePrepareValidates(t *testing.T) {
	c := confineFixture(t)
	if err := c.prepare(); err != nil {
		t.Fatalf("valid spec refused: %v", err)
	}
	if real, _ := filepath.EvalSymlinks(c.Sockets[0]); real != c.Sockets[0] {
		t.Errorf("socket not resolved: %q", c.Sockets[0])
	}
	bad := []struct {
		name string
		mut  func(*containConfine)
		want string
	}{
		{"missing dir", func(c *containConfine) { c.Dir = filepath.Join(c.Dir, "nope") }, "--dir"},
		{"dir is a file", func(c *containConfine) {
			f := filepath.Join(c.Dir, "f")
			os.WriteFile(f, nil, 0o644)
			c.Dir = f
		}, "not a directory"},
		{"root", func(c *containConfine) { c.Dir = "/" }, "whole file system"},
		{"relative ro", func(c *containConfine) { c.RO = []string{"genie"} }, "absolute"},
		{"socket not a socket", func(c *containConfine) { c.Sockets = []string{c.RO[0]} }, "not a unix socket"},
		{"env assignment", func(c *containConfine) { c.Env = []string{"A=1"} }, "NAMES"},
		{"env HOME", func(c *containConfine) { c.Env = []string{"HOME"} }, "private temp"},
		{"quote in path", func(c *containConfine) {
			d := filepath.Join(c.Dir, `a"b`)
			os.Mkdir(d, 0o755)
			c.Dir = d
		}, "may not contain"},
	}
	for _, tc := range bad {
		c := confineFixture(t)
		tc.mut(c)
		if err := c.prepare(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestContainConfinedEnvIsScrubbed(t *testing.T) {
	environ := []string{"PATH=/bin", "LANG=C", "TERM=xterm", "HOME=/Users/x", "TMPDIR=/var/t",
		"OPENAI_API_KEY=sk-secret", "AWS_SECRET_ACCESS_KEY=x", "GENIE_MODEL_ID=m", "LC_ALL=C"}
	got := confinedEnv(environ, []string{"GENIE_MODEL_ID"}, "/priv")
	want := []string{"PATH=/bin", "LANG=C", "TERM=xterm", "GENIE_MODEL_ID=m", "LC_ALL=C", "TMPDIR=/priv", "HOME=/priv"}
	if !slices.Equal(got, want) {
		t.Fatalf("confinedEnv = %q, want %q", got, want)
	}
}

// The macOS backend's profile.
func TestSeatbeltConfinedProfile(t *testing.T) {
	c := &containConfine{Dir: "/Users/u/runs/r1", Sockets: []string{"/tmp/cf/s.sock"},
		RO: []string{"/Users/u/.local/share/genie"}, Loopback: []int{41234}}
	p := seatbeltConfinedProfile(c, seatbeltHost{
		Tmp:    "/private/var/folders/x/T/bashy-confine-1",
		Deny:   []string{"/Users/u", "/Volumes/fake-home", "/private/var/folders/x/T"},
		Self:   []string{"/Users/u/.local/bin/bashy"},
		System: []string{"/Applications/Xcode.app/Contents/Developer"},
	})
	lines := strings.Split(p, "\n")
	idx := func(rule string) int {
		t.Helper()
		i := slices.Index(lines, rule)
		if i < 0 {
			t.Fatalf("profile missing %q:\n%s", rule, p)
		}
		return i
	}
	after := func(rule string, floor int, what string) {
		t.Helper()
		if idx(rule) < floor {
			t.Errorf("%s precedes %s (last match wins)", rule, what)
		}
	}
	// ipc: default-deny Mach lookups, a short allow-list without the keychain
	denyMach := idx("(deny mach-lookup)")
	after(`(allow mach-lookup (global-name "com.apple.system.opendirectoryd.libinfo"))`, denyMach, "the mach deny")
	for _, l := range lines {
		if strings.HasPrefix(l, "(allow mach-lookup") && (strings.Contains(l, "Security") || strings.Contains(l, "secd") || strings.Contains(l, "trustd") || strings.Contains(l, "cfprefsd")) {
			t.Errorf("keychain or preferences service allowed: %s", l)
		}
	}
	// network: deny, then the named socket and the one loopback port only
	denyNet := idx("(deny network*)")
	after(`(allow network-outbound (remote unix-socket (path-literal "/tmp/cf/s.sock")))`, denyNet, "the network deny")
	after(`(allow network-outbound (remote ip "localhost:41234"))`, denyNet, "the network deny")
	after(`(allow network-bind network-inbound (local ip "localhost:41234"))`, denyNet, "the network deny")
	if strings.Contains(p, `localhost:*`) || strings.Contains(p, `(remote unix-socket))`) || strings.Contains(p, "(remote tcp") || strings.Contains(p, `"*:`) {
		t.Errorf("profile opens more than the listed sockets and port:\n%s", p)
	}
	noLoop := seatbeltConfinedProfile(&containConfine{Dir: c.Dir}, seatbeltHost{Tmp: "/t"})
	if strings.Contains(noLoop, "ip \"") {
		t.Errorf("no --loopback must mean no loopback at all:\n%s", noLoop)
	}
	// writes: deny, then DIR and the private temp only
	denyW := idx("(deny file-write*)")
	after(`(allow file-write* (subpath "/Users/u/runs/r1") (subpath "/private/var/folders/x/T/bashy-confine-1"))`, denyW, "the write deny")
	for _, l := range lines {
		if strings.HasPrefix(l, "(allow file-write*") && strings.Contains(l, "genie") {
			t.Errorf("an --ro path is writable: %s", l)
		}
	}
	// reads: default deny, system roots, keychains denied again, then the
	// homes and temp root denied, then DIR, temp, ro, socket, bashy
	denyR := idx("(deny file-read*)")
	if denyR < denyW {
		t.Error("read deny precedes the write rules")
	}
	usr := idx(`(allow file-read* (subpath "/usr"))`)
	after(`(allow file-read* (literal "/"))`, denyR, "the read deny")
	idx(`(allow file-read* (subpath "/Applications/Xcode.app/Contents/Developer"))`)
	if usr < denyR {
		t.Error("system read allowance precedes the default read deny")
	}
	after(`(deny file-read* (subpath "/Library/Keychains"))`, idx(`(allow file-read* (subpath "/Library"))`), "the /Library allowance")
	denyHome := idx(`(deny file-read* (subpath "/Users/u"))`)
	after(`(deny file-read* (subpath "/Volumes/fake-home"))`, usr, "the system reads")
	after(`(deny file-read* (subpath "/private/var/folders/x/T"))`, usr, "the system reads")
	for _, rule := range []string{
		`(allow file-read* (subpath "/Users/u/runs/r1"))`,
		`(allow file-read* (subpath "/Users/u/.local/share/genie"))`,
		`(allow file-read* (subpath "/private/var/folders/x/T/bashy-confine-1"))`,
		`(allow file-read* (literal "/Users/u/.local/bin/bashy"))`,
		`(allow file-read* (literal "/tmp/cf/s.sock"))`,
		`(allow file-read-metadata (literal "/Users/u"))`,
		`(allow file-read-metadata (literal "/Users/u/runs"))`,
	} {
		after(rule, denyHome, "the home deny")
	}
	// nothing outside the named roots is readable: no broad allowance
	for _, l := range lines {
		if strings.HasPrefix(l, "(allow file-read* (subpath \"/Users\")") || strings.HasPrefix(l, "(allow file-read* (subpath \"/\")") ||
			strings.HasPrefix(l, "(allow file-read* (subpath \"/Volumes") || strings.HasPrefix(l, "(allow file-read* (subpath \"/private/tmp") {
			t.Errorf("broad read allowance: %s", l)
		}
	}
}

func TestContainConfineDenyRoots(t *testing.T) {
	got := containConfineDenyRoots("/Users/acct", "/Users/other", "/private/var/folders/x/T")
	if want := []string{"/Users/acct", "/Users/other", "/private/var/folders/x/T"}; !slices.Equal(got, want) {
		t.Errorf("deny roots = %q, want %q", got, want)
	}
	if got := containConfineDenyRoots("/Users/acct", "/Users/acct/", ""); !slices.Equal(got, []string{"/Users/acct"}) {
		t.Errorf("same home twice = %q", got)
	}
	if got := containConfineDenyRoots("/Users/acct", "/", ""); !slices.Equal(got, []string{"/Users/acct"}) {
		t.Errorf("HOME=/ must not deny the whole tree: %q", got)
	}
}

// The Linux backend's container spec.
func TestContainerConfinedArgs(t *testing.T) {
	c := &containConfine{Dir: "/home/u/runs/r1", Sockets: []string{"/run/user/1/door.sock"},
		RO: []string{"/home/u/.local/share/genie"}, Env: []string{"GENIE_MODEL_ID"}, Loopback: []int{41234}}
	environ := []string{"PATH=/home/u/bin:/usr/bin", "OPENAI_API_KEY=sk-secret", "GENIE_MODEL_ID=m", "LANG=C.UTF-8", "HOME=/home/u"}
	args := containerConfinedArgs(c, "localhost/bashy:test", nil, environ, []string{"/home/u/.local/share/genie/run", "-x"})
	line := strings.Join(args, " ")
	for _, want := range []string{
		"podman run --rm -i --network=none --unsetenv-all",
		"--read-only --tmpfs /tmp:rw,nosuid,nodev,mode=1777",
		"-v /home/u/runs/r1:/home/u/runs/r1:rw -w /home/u/runs/r1",
		"-v /home/u/.local/share/genie:/home/u/.local/share/genie:ro",
		"-v /run/user/1/door.sock:/run/user/1/door.sock:ro",
		"localhost/bashy:test -c '/home/u/.local/share/genie/run' '-x'",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("container spec missing %q:\n%s", want, line)
		}
	}
	for _, leak := range []string{"OPENAI_API_KEY", "sk-secret", "/home/u/bin", "HOME=/home/u", "--network=host", "--privileged", "41234"} {
		if strings.Contains(line, leak) {
			t.Errorf("container spec carries %q:\n%s", leak, line)
		}
	}
	// the complete environment, nothing else
	var env, mounts []string
	for i, a := range args {
		switch a {
		case "-e":
			env = append(env, args[i+1])
		case "-v":
			mounts = append(mounts, args[i+1])
		}
	}
	wantEnv := []string{"PATH=/", "OTEL_TRACES_EXPORTER=none", "BASHY_TELEMETRY_QUIET=1", "BASHY_AGENTIC=1", "BASHY_HINTS=off",
		"GENIE_MODEL_ID=m", "LANG=C.UTF-8", "TMPDIR=/tmp", "HOME=/tmp"}
	if !slices.Equal(env, wantEnv) {
		t.Errorf("environment = %q, want exactly %q", env, wantEnv)
	}
	withEnv := containerConfinedEnv(nil, nil, []string{"go"})
	if !slices.Contains(withEnv, "BASHY_BIN_CACHE=/opt/bashy/bin") || len(withEnv) != 10 {
		t.Errorf("--with image environment = %q", withEnv)
	}
	if len(mounts) != 3 {
		t.Errorf("mounts = %q, want DIR, the --ro path and the socket only", mounts)
	}
	// a bare command resolves inside the image
	if got := containerConfinedCommand(c, []string{"/usr/bin/python3", "t.py"}); got != "'python3' 't.py'" {
		t.Errorf("host-only argv[0] kept its host path: %s", got)
	}
}

func TestContainConfineBackendSelection(t *testing.T) {
	t.Setenv("BASHY_CONTAIN_PROVIDER", "")
	b, err := confineBackend("")
	switch runtime.GOOS {
	case "darwin":
		if nativeContainSupported() == nil && (err != nil || b != confineSeatbelt) {
			t.Errorf("darwin default backend = %q, %v; want seatbelt", b, err)
		}
		if b, err := confineBackend("image"); err != nil || b != confineImage {
			t.Errorf("darwin image backend = %q, %v", b, err)
		}
	case "linux":
		if err != nil || b != confineImage {
			t.Errorf("linux default backend = %q, %v; want image", b, err)
		}
		if _, err := confineBackend("native"); err == nil {
			t.Error("linux native provider accepted for --dir (it confines the network only)")
		}
	default:
		if err == nil || !strings.Contains(err.Error(), runtime.GOOS) {
			t.Errorf("%s must refuse --dir with a named reason, got %q, %v", runtime.GOOS, b, err)
		}
	}
	if _, err := confineBackend("custom"); err == nil {
		t.Error("custom provider accepted for --dir")
	}
}

func TestDispatchContainConfineFlags(t *testing.T) {
	c := confineFixture(t)
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"--net", "model", "--dir", c.Dir, "--", "true"}, 2},
		{[]string{"--net", "deny", "--ro", c.RO[0], "--", "true"}, 2},
		{[]string{"--net", "deny", "--loopback", "4000", "--", "true"}, 2},
		{[]string{"--net", "deny", "--dir", filepath.Join(c.Dir, "missing"), "--", "true"}, containUnsupportedStatus},
		// an empty or missing --dir never falls back to network-only mode
		{[]string{"--net", "deny", "--dir=", "--", "true"}, 2},
		{[]string{"--net", "deny", "--dir", "", "--", "true"}, 2},
		{[]string{"--net", "deny", "--", "true", "--dir"}, -1}, // after --: the command's own argument
		{[]string{"--net", "deny", "--dir"}, 2},
		{[]string{"--net", "deny", "--dir", c.Dir, "--loopback", "0", "--", "true"}, 2},
		{[]string{"--net", "deny", "--dir", c.Dir, "--loopback=http", "--", "true"}, 2},
		{[]string{"--net", "deny", "--dir", c.Dir, "--socket=", "--", "true"}, 2},
	} {
		if tc.want < 0 {
			continue
		}
		if st := dispatchContain(tc.args); st != tc.want {
			t.Errorf("%q: status %d, want %d", tc.args, st, tc.want)
		}
	}
	var cc containConfine
	if err := cc.add("--loopback", "41234"); err != nil || !slices.Equal(cc.Loopback, []int{41234}) {
		t.Errorf("--loopback 41234: %v %v", cc.Loopback, err)
	}
	if err := cc.add("--dir", ""); err == nil || !cc.active() {
		t.Error("an empty --dir must be refused and still count as asked for")
	}
}

// BASHY_CONTAIN_BASHY replaces the image's bashy in a confined run.
func TestContainerConfinedInject(t *testing.T) {
	c := &containConfine{Dir: "/home/u/r"}
	args := containerConfinedArgs(c, "localhost/bashy:test", nil, []string{"LANG=C"}, []string{"bashy", "genie"})
	got := strings.Join(containerConfinedInject(args, "/opt/cand/bashy"), " ")
	for _, want := range []string{
		"--user ",
		"--entrypoint /.bashy/bashy -v /opt/cand/bashy:/.bashy/bashy:ro -e PATH=/.bashy:/ localhost/bashy:test -c ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("injected spec missing %q:\n%s", want, got)
		}
	}
}
