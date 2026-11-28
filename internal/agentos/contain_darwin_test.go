//go:build darwin

package agentos

// Sprint: #421; Story: #1906; Story-ID: e853e9f1e2d9

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSeatbeltProfileAllowsOnlyListedLoopbackPorts(t *testing.T) {
	if got := seatbeltProfile(nil); got != seatbeltDenyNetwork {
		t.Fatalf("no ports must leave the deny-all profile unchanged:\n%s", got)
	}
	got := seatbeltProfile([]int{24556, 51234})
	for _, want := range []string{
		"(deny network*)",
		`(allow network-outbound (remote ip "localhost:24556"))`,
		`(allow network-outbound (remote ip "localhost:51234"))`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("profile missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"*:`) || strings.Contains(got, "remote tcp") || strings.Contains(got, "api.z.ai") {
		t.Fatalf("profile opens more than loopback ports:\n%s", got)
	}
}

func TestContainNativeReachesOnlyAllowedModelEndpoint(t *testing.T) {
	if nativeContainSupported() != nil {
		t.Skip("no Seatbelt on this host")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl(1)")
	}
	hit := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, name) }))
	}
	model, stranger := hit("model"), hit("stranger")
	defer model.Close()
	defer stranger.Close()

	old := loadContainEgress
	defer func() { loadContainEgress = old }()
	loadContainEgress = func() containEgress {
		return containEgress{Remote: []string{strings.TrimPrefix(model.URL, "http://")}}
	}

	curl := func(args string) int {
		return runNativeContained([]string{"sh", "-c", "curl -sf -m 5 -o /dev/null " + args}, true)
	}
	viaProxy := `--noproxy '' -x "$http_proxy" `
	if code := curl(viaProxy + model.URL); code != 0 {
		t.Fatalf("allowed model endpoint through the proxy: exit %d, want 0", code)
	}
	if code := curl("--noproxy '*' " + model.URL); code == 0 {
		t.Fatalf("direct connection from the contained tree succeeded")
	}
	if code := curl(viaProxy + stranger.URL); code == 0 {
		t.Fatalf("an endpoint outside the allowlist was reachable through the proxy")
	}
}

func TestContainNativeDenyStaysOffline(t *testing.T) {
	if nativeContainSupported() != nil {
		t.Skip("no Seatbelt on this host")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl(1)")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "x") }))
	defer srv.Close()
	old := loadContainEgress
	defer func() { loadContainEgress = old }()
	loadContainEgress = func() containEgress {
		return containEgress{Remote: []string{strings.TrimPrefix(srv.URL, "http://")}}
	}
	if code := runNativeContained([]string{"sh", "-c", `test -z "$HTTPS_PROXY$http_proxy" && ! curl -sf -m 5 -o /dev/null ` + srv.URL}, false); code != 0 {
		t.Fatalf("deny must have no proxy variables and no reachable endpoint: exit %d", code)
	}
}

// Sprint: #738; Story: #823; Story-ID: 380fd660ae7e
//
// The generated profile, enforced: Seatbelt (not a container) on this host.
func TestContainConfinedSeatbeltEnforces(t *testing.T) {
	if nativeContainSupported() != nil {
		t.Skip("no Seatbelt on this host")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	dir := filepath.Join(home, "runs", "r1")
	bundle := filepath.Join(home, "bundle")
	for _, d := range []string{dir, bundle} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(home, "canary"), []byte("secret"), 0o600)
	os.WriteFile(filepath.Join(bundle, "tool"), []byte("bundle"), 0o644)
	t.Setenv("HOME", home)
	t.Setenv("CONFINE_SECRET", "sk-leak")

	sockDir, err := os.MkdirTemp("/tmp", "cf")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	serve := func(name string) string {
		p := filepath.Join(sockDir, name)
		ln, err := net.Listen("unix", p)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Write([]byte("door\n"))
				c.Close()
			}
		}()
		return p
	}
	door, other := serve("door.sock"), serve("other.sock")
	// a file outside every allowed root and not under any home
	outside := filepath.Join(sockDir, "outside")
	os.WriteFile(outside, []byte("secret"), 0o644)
	acct, err := containAccountHome()
	if err != nil {
		t.Fatal(err)
	}
	// loopback: the listed port is reachable, another host listener is not
	tcp := func() (net.Listener, int) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		return ln, ln.Addr().(*net.TCPAddr).Port
	}
	_, listed := tcp()
	_, unlisted := tcp()
	free := func() int { ln, p := tcp(); ln.Close(); return p }
	bindListed, bindUnlisted := free(), free()
	keychainOutside := exec.Command("security", "list-keychains").Run() == nil

	script := `set -u
fail() { echo "FAIL: $*"; exit 1; }
[ "$(pwd -P)" = "$WANT_DIR" ] || fail "cwd $(pwd -P)"
echo ok > out || fail "write DIR"
echo ok > "$TMPDIR/t" || fail "write TMPDIR"
[ "$HOME" = "$TMPDIR" ] || fail "HOME=$HOME"
[ -z "${CONFINE_SECRET:-}" ] || fail "secret leaked"
cat "$WANT_BUNDLE/tool" >/dev/null || fail "read --ro"
cat "$WANT_HOME/canary" 2>/dev/null && fail "canary readable"
ls "$WANT_HOME" 2>/dev/null && fail "home listable"
echo x 2>/dev/null > "$WANT_HOME/x" && fail "home writable"
echo x 2>/dev/null > "$WANT_BUNDLE/x" && fail "--ro writable"
echo x 2>/dev/null > /tmp/bashy-confine-escape && fail "/tmp writable"
cat "$WANT_OUTSIDE" 2>/dev/null && fail "file outside the allowed roots readable"
ls "$WANT_ACCT" 2>/dev/null && fail "account home listable with HOME pointing elsewhere"
ls /Library/Keychains 2>/dev/null && fail "keychains listable"
ls /Volumes 2>/dev/null && fail "/Volumes listable"
cat /etc/hosts >/dev/null || fail "system config unreadable"
if [ -n "${WANT_KEYCHAIN:-}" ]; then
  security list-keychains >/dev/null 2>&1 && fail "keychain services reachable"
fi
if command -v nc >/dev/null; then
  nc -z -G 2 127.0.0.1 "$WANT_LISTED" 2>/dev/null || fail "listed loopback port unreachable"
  nc -z -G 2 127.0.0.1 "$WANT_UNLISTED" 2>/dev/null && fail "unlisted loopback port reachable"
fi
if command -v python3 >/dev/null; then
  bind() { python3 -c 'import socket,sys; s=socket.socket(); s.bind(("127.0.0.1", int(sys.argv[1]))); s.listen(1)' "$1" 2>/dev/null; }
  bind "$WANT_BIND_LISTED" || fail "listed loopback port not bindable"
  bind "$WANT_BIND_UNLISTED" && fail "unlisted loopback port bindable"
fi
if command -v nc >/dev/null; then
  [ "$(nc -U "$WANT_DOOR" </dev/null)" = door ] || fail "door socket unreachable"
  [ "$(nc -U "$WANT_OTHER" </dev/null 2>/dev/null)" = door ] && fail "other socket reachable"
fi
exit 0`
	c := &containConfine{Dir: dir, Sockets: []string{door}, RO: []string{bundle},
		Loopback: []int{listed, bindListed},
		Env: []string{"WANT_DIR", "WANT_HOME", "WANT_BUNDLE", "WANT_DOOR", "WANT_OTHER", "WANT_OUTSIDE", "WANT_ACCT",
			"WANT_KEYCHAIN", "WANT_LISTED", "WANT_UNLISTED", "WANT_BIND_LISTED", "WANT_BIND_UNLISTED"}}
	t.Setenv("WANT_OUTSIDE", outside)
	t.Setenv("WANT_ACCT", acct)
	if keychainOutside {
		t.Setenv("WANT_KEYCHAIN", "1")
	}
	t.Setenv("WANT_LISTED", fmt.Sprint(listed))
	t.Setenv("WANT_UNLISTED", fmt.Sprint(unlisted))
	t.Setenv("WANT_BIND_LISTED", fmt.Sprint(bindListed))
	t.Setenv("WANT_BIND_UNLISTED", fmt.Sprint(bindUnlisted))
	t.Setenv("WANT_DIR", dir)
	t.Setenv("WANT_HOME", home)
	t.Setenv("WANT_BUNDLE", bundle)
	t.Setenv("WANT_DOOR", door)
	t.Setenv("WANT_OTHER", other)
	if st := runConfined(c, "native", []string{"/bin/sh", "-c", script}); st != 0 {
		t.Fatalf("confined run: exit %d", st)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "out")); string(b) != "ok\n" {
		t.Fatalf("DIR/out = %q", b)
	}
	if _, err := os.Stat("/tmp/bashy-confine-escape"); err == nil {
		os.Remove("/tmp/bashy-confine-escape")
		t.Fatal("wrote outside DIR")
	}
}

// The home to deny comes from the OS user record; without one nothing runs.
func TestContainConfinedSeatbeltNeedsAccountHome(t *testing.T) {
	if nativeContainSupported() != nil {
		t.Skip("no Seatbelt on this host")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	old := containAccountHome
	defer func() { containAccountHome = old }()
	containAccountHome = func() (string, error) { return "", fmt.Errorf("no user record") }
	marker := filepath.Join(dir, "ran")
	c := &containConfine{Dir: dir, dirSet: true}
	if st := runConfined(c, "native", []string{"/usr/bin/touch", marker}); st != containUnsupportedStatus {
		t.Fatalf("status %d, want 125", st)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the command ran without a resolvable account home")
	}
	// both the user-record home and a different $HOME are denied
	containAccountHome = func() (string, error) { return "/Users/acct-from-record", nil }
	t.Setenv("HOME", dir)
	host, err := seatbeltConfinedHost()
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(host.Tmp)
	for _, want := range []string{"/Users/acct-from-record", dir} {
		if !slices.Contains(host.Deny, want) {
			t.Errorf("deny roots %q miss %s", host.Deny, want)
		}
	}
	if h, err := old(); err != nil || !filepath.IsAbs(h) {
		t.Fatalf("account home from the user record: %q, %v", h, err)
	}
}
