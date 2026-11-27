package agentos

// Sprint: #738; Story: #823; Story-ID: 380fd660ae7e
//
// File confinement on `bashy contain` (Seam B of the Sprint 738 plan):
//
//	bashy contain --net deny --dir DIR [--socket /abs.sock]… [--ro /abs/path]… [--env NAME]… -- CMD [ARGS…]
//
// With --dir, DIR and a fresh private temp (TMPDIR, and HOME) are the only
// writable places and DIR is the working directory; the user's home is
// unreadable except each --ro path, DIR and bashy itself; each --socket is
// reachable; nothing else on the network is. The environment is scrubbed to
// a fixed allow-list plus the --env names. One spec (containConfine), two
// backends: a generated Seatbelt profile on macOS (seatbeltConfinedProfile)
// and bashy's own image through bashy podman elsewhere (containerConfinedArgs:
// DIR, the --ro paths and the sockets bind-mounted, network none). A host
// that has neither fails closed (125) with a named reason; nothing runs.
// Without --dir the verb is unchanged.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// containConfine is one file-confined run.
type containConfine struct {
	Dir     string   // the one writable working directory
	Sockets []string // unix sockets the child may connect to
	RO      []string // host paths readable (never writable) inside
	Env     []string // variable NAMES passed through besides the allow-list
}

// containConfineEnvAllow is what survives the scrub (TMPDIR and HOME are
// always set to the private temp, never inherited).
var containConfineEnvAllow = []string{"PATH", "LANG", "LC_ALL", "TERM"}

func (c *containConfine) active() bool { return c.Dir != "" }

// add records one flag; a repeated --dir keeps the last.
func (c *containConfine) add(flag, v string) {
	switch flag {
	case "--dir":
		c.Dir = v
	case "--socket":
		c.Sockets = append(c.Sockets, v)
	case "--ro":
		c.RO = append(c.RO, v)
	case "--env":
		c.Env = append(c.Env, v)
	}
}

// prepare validates the spec and resolves every path to its real absolute
// form (Seatbelt matches resolved paths; podman needs host paths).
func (c *containConfine) prepare() error {
	resolve := func(flag, p string) (string, os.FileInfo, error) {
		if !filepath.IsAbs(p) && flag != "--dir" {
			return "", nil, fmt.Errorf("%s %q: must be an absolute path", flag, p)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", nil, fmt.Errorf("%s %q: %v", flag, p, err)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", nil, fmt.Errorf("%s %q: %v", flag, p, err)
		}
		if err := sbplSafePath(real); err != nil {
			return "", nil, fmt.Errorf("%s %q: %v", flag, p, err)
		}
		fi, err := os.Stat(real)
		if err != nil {
			return "", nil, fmt.Errorf("%s %q: %v", flag, p, err)
		}
		return real, fi, nil
	}
	dir, fi, err := resolve("--dir", c.Dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("--dir %q: not a directory", c.Dir)
	}
	if dir == "/" {
		return errors.New("--dir /: the whole file system is not a confinement")
	}
	c.Dir = dir
	for i, s := range c.Sockets {
		real, fi, err := resolve("--socket", s)
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("--socket %q: not a unix socket", s)
		}
		c.Sockets[i] = real
	}
	for i, p := range c.RO {
		real, _, err := resolve("--ro", p)
		if err != nil {
			return err
		}
		c.RO[i] = real
	}
	for _, n := range c.Env {
		if n == "" || strings.ContainsAny(n, "= \t\n") {
			return fmt.Errorf("--env takes variable NAMES, got %q", n)
		}
		if n == "HOME" || n == "TMPDIR" {
			return fmt.Errorf("--env %s: always the private temp inside the confinement", n)
		}
	}
	return nil
}

// sbplSafePath refuses what cannot be written into a profile or a mount
// spec unambiguously.
func sbplSafePath(p string) error {
	if strings.ContainsAny(p, "\"\\:\x00\n\r") {
		return errors.New(`a confined path may not contain '"', '\', ':' or a control character`)
	}
	return nil
}

// confinedEnv is the scrubbed environment: the allow-list and the --env
// names from environ, then TMPDIR and HOME at tmp.
func confinedEnv(environ []string, names []string, tmp string) []string {
	keep := append(slices.Clone(containConfineEnvAllow), names...)
	var out []string
	for _, kv := range environ {
		k, _, ok := strings.Cut(kv, "=")
		if ok && slices.Contains(keep, k) {
			out = append(out, kv)
		}
	}
	return append(out, "TMPDIR="+tmp, "HOME="+tmp)
}

// sbplQuote is a Seatbelt string literal (prepare refused '"' and '\').
func sbplQuote(p string) string { return `"` + p + `"` }

// pathAncestors are the directories above p ("/a/b/c" → "/a", "/a/b").
func pathAncestors(p string) []string {
	var out []string
	for d := filepath.Dir(p); d != "/" && d != "." && d != p; d = filepath.Dir(d) {
		out = append(out, d)
		p = d
	}
	slices.Reverse(out)
	return out
}

// seatbeltConfinedProfile is the macOS backend's profile for c: writes only
// to c.Dir, tmp and the terminal/null devices; reads of home and of the
// user temp root denied except c.Dir, tmp, the --ro paths and bashy (self);
// no network but the --socket paths and loopback (for an in-sandbox
// forwarder; Seatbelt has no network namespace, so this loopback is the
// host's — see the story record). Rules are last-match-wins.
func seatbeltConfinedProfile(c *containConfine, tmp, home, userTemp string, self []string) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	line("(version 1)")
	line("(allow default)")
	line("; network: nothing but the listed unix sockets and loopback")
	line("(deny network*)")
	line(`(allow network-bind network-inbound (local ip "localhost:*"))`)
	line(`(allow network-outbound (remote ip "localhost:*"))`)
	line("(allow network-bind network-inbound (local unix-socket (subpath %s)) (local unix-socket (subpath %s)))", sbplQuote(c.Dir), sbplQuote(tmp))
	line("(allow network-outbound (remote unix-socket (subpath %s)) (remote unix-socket (subpath %s)))", sbplQuote(c.Dir), sbplQuote(tmp))
	for _, s := range c.Sockets {
		line("(allow network-outbound (remote unix-socket (path-literal %s)))", sbplQuote(s))
	}
	line("; writes: DIR, the private temp, terminal and null devices")
	line("(deny file-write*)")
	line("(allow file-write* (subpath %s) (subpath %s))", sbplQuote(c.Dir), sbplQuote(tmp))
	line(`(allow file-write* (literal "/dev/null") (literal "/dev/zero") (literal "/dev/tty") (literal "/dev/dtracehelper") (regex #"^/dev/fd/") (regex #"^/dev/ttys[0-9]+$"))`)
	line("; reads: not home, not the user temp root")
	var denied []string
	for _, d := range []string{home, userTemp} {
		if d != "" && d != "/" {
			denied = append(denied, d)
			line("(deny file-read* (subpath %s))", sbplQuote(d))
		}
	}
	allowed := append([]string{c.Dir, tmp}, c.RO...)
	allowed = append(allowed, c.Sockets...)
	allowed = append(allowed, self...)
	var meta []string
	for _, p := range allowed {
		for _, a := range pathAncestors(p) {
			for _, d := range denied {
				if (a == d || strings.HasPrefix(a, d+"/")) && !slices.Contains(meta, a) {
					meta = append(meta, a)
				}
			}
		}
	}
	for _, a := range meta {
		line("(allow file-read-metadata (literal %s))", sbplQuote(a))
	}
	for _, p := range append([]string{c.Dir, tmp}, c.RO...) {
		line("(allow file-read* (subpath %s))", sbplQuote(p))
	}
	for _, p := range append(slices.Clone(c.Sockets), self...) {
		line("(allow file-read* (literal %s))", sbplQuote(p))
	}
	return strings.TrimRight(b.String(), "\n")
}

// containConfineSelf is bashy's own executable (and the .real pair behind
// the unix launcher), resolved, for the profile's read allowance.
func containConfineSelf() []string {
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if _, err := os.Stat(p); err == nil && sbplSafePath(p) == nil && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	self := bashySelfPath()
	add(self)
	add(self + ".real")
	return out
}

// Inside the image: DIR, the --ro paths and the sockets keep their host
// paths; the private temp is a tmpfs at /tmp.
const containConfineInTmp = "/tmp"

// containerConfinedArgs is the Linux backend's `bashy podman` command line
// for c: bashy's image, no network, only DIR (rw), the --ro paths and the
// sockets bind-mounted at their own paths, a private tmpfs as TMPDIR/HOME,
// the scrubbed environment, DIR as the working directory.
func containerConfinedArgs(c *containConfine, image string, environ []string, argv []string) []string {
	opts := []string{"--userns=keep-id", "--read-only", "--tmpfs", containConfineInTmp + ":rw,nosuid,nodev,mode=1777",
		"-e", "BASHY_CONTAINED=confine",
		"-v", c.Dir + ":" + c.Dir + ":rw", "-w", c.Dir}
	for _, p := range c.RO {
		opts = append(opts, "-v", p+":"+p+":ro")
	}
	for _, s := range c.Sockets {
		opts = append(opts, "-v", s+":"+s)
	}
	for _, kv := range confinedEnv(environ, c.Env, containConfineInTmp) {
		if k, _, _ := strings.Cut(kv, "="); k == "PATH" {
			continue // the image's own PATH: host entries do not exist inside
		}
		opts = append(opts, "-e", kv)
	}
	return podmanRun(opts, image, "-c", containerConfinedCommand(c, argv))
}

// containerConfinedCommand is containerCommand, except an absolute argv[0]
// under a mounted path keeps its path (it exists inside).
func containerConfinedCommand(c *containConfine, argv []string) string {
	if filepath.IsAbs(argv[0]) && !isBashyExecutable(argv[0]) {
		for _, m := range append([]string{c.Dir}, c.RO...) {
			if argv[0] == m || strings.HasPrefix(argv[0], m+"/") {
				words := make([]string, 0, len(argv))
				for _, a := range argv {
					words = append(words, shellQuote(a))
				}
				return strings.Join(words, " ")
			}
		}
	}
	return containerCommand(argv)
}

// Confinement backends.
const (
	confineSeatbelt = "seatbelt"
	confineImage    = "image"
)

// runConfined runs argv under c with the backend confineBackend picks for
// provider; anything it cannot enforce fails closed (125).
func runConfined(c *containConfine, provider string, argv []string) int {
	backend, err := confineBackend(provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v; nothing ran\n", err)
		return containUnsupportedStatus
	}
	if err := c.prepare(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v; nothing ran\n", err)
		return containUnsupportedStatus
	}
	if backend == confineSeatbelt {
		return runSeatbeltConfined(c, argv)
	}
	return runContainerConfined(c, argv)
}

func runContainerConfined(c *containConfine, argv []string) int {
	self := bashySelfPath()
	image, with := containImage(argv)
	if err := ensureContainImage(self, image, with); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v; nothing ran\n", err)
		return containUnsupportedStatus
	}
	return runContainChild(self, containerConfinedArgs(c, image, os.Environ(), argv), "", nil)
}

// runContainChild runs name args with the terminal, in dir (if set) with env
// (nil = inherited), mapping a start failure to 125.
func runContainChild(name string, args []string, dir string, env []string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Dir, cmd.Env = dir, env
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	return 0
}
