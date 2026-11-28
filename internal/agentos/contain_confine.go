package agentos

// Sprint: #738; Story: #823; Story-ID: 380fd660ae7e
//
// File confinement on `bashy contain` (Seam B of the Sprint 738 plan):
//
//	bashy contain --net deny --dir DIR [--socket /abs.sock]… [--ro /abs/path]… [--env NAME]… [--loopback PORT]… -- CMD [ARGS…]
//
// With --dir, DIR and a fresh private temp (TMPDIR, and HOME) are the only
// writable places and DIR is the working directory; reads are the system
// roots (never the keychains), DIR, each --ro path and bashy itself — the
// account home, $HOME and the user temp root never; each --socket is
// reachable, and loopback only on each --loopback PORT; nothing else on the
// network is. On macOS, Mach lookups are limited to the account database,
// notifications and logging (no keychain, no preferences). The environment is scrubbed to
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
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// containConfine is one file-confined run.
type containConfine struct {
	Dir     string   // the one writable working directory
	Sockets []string // unix sockets the child may connect to
	RO      []string // host paths readable (never writable) inside
	Env     []string // variable NAMES passed through besides the allow-list
	// Loopback ports the child may bind and connect to (the in-sandbox
	// forwarder's). macOS: the only loopback in the profile; Linux: the
	// container's own loopback is private, so no rule is needed.
	Loopback []int
	dirSet   bool // --dir was given (even if its value was refused)
}

// containConfineEnvAllow is what survives the scrub (TMPDIR and HOME are
// always set to the private temp, never inherited).
var containConfineEnvAllow = []string{"PATH", "LANG", "LC_ALL", "TERM"}

func (c *containConfine) active() bool { return c.dirSet }

// containConfineFlag reports whether a is one of the confinement flags.
func containConfineFlag(a string) bool {
	switch a {
	case "--dir", "--socket", "--ro", "--env", "--loopback":
		return true
	}
	return false
}

// add records one flag; a repeated --dir keeps the last. An empty value or
// a bad port is a usage error: it must never silently drop the confinement.
func (c *containConfine) add(flag, v string) error {
	if flag == "--dir" {
		c.dirSet = true
	}
	if v == "" {
		return fmt.Errorf("%s needs a non-empty value", flag)
	}
	switch flag {
	case "--loopback":
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("--loopback %q: not a TCP port (1-65535)", v)
		}
		c.Loopback = append(c.Loopback, port)
	case "--dir":
		c.Dir = v
	case "--socket":
		c.Sockets = append(c.Sockets, v)
	case "--ro":
		c.RO = append(c.RO, v)
	case "--env":
		c.Env = append(c.Env, v)
	}
	return nil
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

// seatbeltHost is what the macOS profile needs from the host besides the
// spec: the private temp, the trees whose reads are denied even inside an
// allowed system root (the account home, $HOME, the user temp root), bashy
// itself, and extra read-only system roots (the selected developer dir).
type seatbeltHost struct {
	Tmp    string
	Deny   []string
	Self   []string
	System []string
}

// seatbeltSystemReads are the read-only roots every process needs: the
// system, the dynamic linker caches, the shared configuration, devices, and
// the usual tool prefixes (/opt holds Homebrew; /usr/local the older one).
var seatbeltSystemReads = []string{"/usr", "/bin", "/sbin", "/System", "/Library", "/opt",
	"/private/etc", "/private/var/db/timezone", "/private/var/db/dyld", "/private/var/select", "/dev",
	"/private/var/db/xcode_select_link"}

// seatbeltSecretReads stay unreadable inside the system roots.
var seatbeltSecretReads = []string{"/Library/Keychains"}

// seatbeltMachAllow are the only Mach services the child may look up: the
// account database, notifications and logging. Everything else — the
// keychain (com.apple.SecurityServer, securityd, secd, trustd), preferences
// (cfprefsd serves the home's) and the rest of launchd — is denied.
var seatbeltMachAllow = []string{
	"com.apple.system.opendirectoryd.libinfo",
	"com.apple.system.opendirectoryd.membership",
	"com.apple.system.notification_center",
	"com.apple.system.logger",
	"com.apple.logd",
	"com.apple.diagnosticd",
}

// containConfineDenyRoots are the read-denied trees: the account home from
// the OS user record, $HOME when it names another place, and the user temp
// root; "/" and duplicates dropped.
func containConfineDenyRoots(accountHome, envHome, userTemp string) []string {
	var out []string
	for _, d := range []string{accountHome, envHome, userTemp} {
		if d == "" {
			continue
		}
		if r, err := filepath.EvalSymlinks(d); err == nil {
			d = r
		}
		d = filepath.Clean(d)
		if d != "/" && filepath.IsAbs(d) && sbplSafePath(d) == nil && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// seatbeltConfinedProfile is the macOS backend's profile for c. Reads are
// denied by default: the system roots are readable (minus the keychains),
// then the home, $HOME and the user temp root are denied again, then DIR,
// the private temp, the --ro paths, the sockets and bashy are allowed.
// Writes go only to DIR, the private temp and the terminal/null devices.
// Network: the listed unix sockets, unix sockets under DIR and the temp, and
// loopback only on the --loopback ports (Seatbelt has no network namespace:
// this loopback is the host's, so no other port of it is open). Mach
// lookups: the short allow-list only. Rules are last-match-wins.
func seatbeltConfinedProfile(c *containConfine, h seatbeltHost) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	tmp := h.Tmp
	line("(version 1)")
	line("(allow default)")
	line("; ipc: no host services but the account database, notifications and logging")
	line("(deny mach-lookup)")
	for _, n := range seatbeltMachAllow {
		line("(allow mach-lookup (global-name %s))", sbplQuote(n))
	}
	line("; network: the listed unix sockets and the listed loopback ports only")
	line("(deny network*)")
	line("(allow network-bind network-inbound (local unix-socket (subpath %s)) (local unix-socket (subpath %s)))", sbplQuote(c.Dir), sbplQuote(tmp))
	line("(allow network-outbound (remote unix-socket (subpath %s)) (remote unix-socket (subpath %s)))", sbplQuote(c.Dir), sbplQuote(tmp))
	for _, s := range c.Sockets {
		line("(allow network-outbound (remote unix-socket (path-literal %s)))", sbplQuote(s))
	}
	for _, p := range c.Loopback {
		line(`(allow network-bind network-inbound (local ip "localhost:%d"))`, p)
		line(`(allow network-outbound (remote ip "localhost:%d"))`, p)
	}
	line("; writes: DIR, the private temp, terminal and null devices")
	line("(deny file-write*)")
	line("(allow file-write* (subpath %s) (subpath %s))", sbplQuote(c.Dir), sbplQuote(tmp))
	line(`(allow file-write* (literal "/dev/null") (literal "/dev/zero") (literal "/dev/tty") (literal "/dev/dtracehelper") (regex #"^/dev/fd/") (regex #"^/dev/ttys[0-9]+$"))`)
	line("; reads: denied by default; the system roots minus the keychains")
	line("(deny file-read*)")
	line(`(allow file-read* (literal "/"))`) // the root directory itself (dyld lists it), not its subtree
	system := append(slices.Clone(seatbeltSystemReads), h.System...)
	for _, p := range system {
		line("(allow file-read* (subpath %s))", sbplQuote(p))
	}
	for _, p := range seatbeltSecretReads {
		line("(deny file-read* (subpath %s))", sbplQuote(p))
	}
	line("; not the home, $HOME or the user temp root, even under a system root")
	for _, d := range h.Deny {
		line("(deny file-read* (subpath %s))", sbplQuote(d))
	}
	line("; DIR, the private temp, the --ro paths, the sockets, bashy")
	trees := append([]string{c.Dir, tmp}, c.RO...)
	files := append(slices.Clone(c.Sockets), h.Self...)
	// the root-level links (/etc /var /tmp) and every ancestor of an allowed
	// path are stat-able so lookups and getcwd work; their contents are not
	meta := []string{"/etc", "/var", "/tmp"}
	for _, p := range slices.Concat(trees, files, system) {
		for _, a := range pathAncestors(p) {
			if !slices.Contains(meta, a) {
				meta = append(meta, a)
			}
		}
	}
	for _, a := range meta {
		line("(allow file-read-metadata (literal %s))", sbplQuote(a))
	}
	for _, p := range trees {
		line("(allow file-read* (subpath %s))", sbplQuote(p))
	}
	for _, p := range files {
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

// containerConfinedImageEnv is the image's own part of the confined run's
// environment, stated here because the run discards the image's ENV
// (--unsetenv-all): bashy's in-image PATH, telemetry off (no network to send
// it to), agent mode without hints, and, for an image built --with
// toolchains, the cache locations its provisioners read.
func containerConfinedImageEnv(with []string) []string {
	env := []string{"PATH=/", "OTEL_TRACES_EXPORTER=none", "BASHY_TELEMETRY_QUIET=1", "BASHY_AGENTIC=1", "BASHY_HINTS=off"}
	if len(normalizedWith(with)) > 0 {
		env = append(env, "BASHY_BIN_CACHE=/opt/bashy/bin", "UV_PYTHON_INSTALL_DIR=/opt/bashy/python", "UV_PYTHON_PREFERENCE=only-managed")
	}
	return env
}

// containerConfinedEnv is the complete environment of the confined image
// run: the image part, then the scrubbed host part (its PATH dropped: host
// entries do not exist inside) with TMPDIR and HOME at the private tmpfs.
func containerConfinedEnv(environ, names, with []string) []string {
	env := containerConfinedImageEnv(with)
	for _, kv := range confinedEnv(environ, names, containConfineInTmp) {
		if k, _, _ := strings.Cut(kv, "="); k != "PATH" {
			env = append(env, kv)
		}
	}
	return env
}

// containerConfinedArgs is the Linux backend's `bashy podman` command line
// for c: bashy's image, no network, only DIR (rw), the --ro paths and the
// sockets (both read-only; a connect works on a read-only bind) mounted at
// their own paths, a private tmpfs as TMPDIR/HOME, exactly
// containerConfinedEnv as the environment (podman's and the image's
// defaults unset), DIR as the working directory. It runs as the calling
// user: the image's default user cannot enter a caller-owned DIR (keep-id
// maps a rootless caller to the same uid; a root caller stays root inside
// the read-only, network-less container). --loopback needs no rule:
// the container's loopback is its own.
func containerConfinedArgs(c *containConfine, image string, with, environ, argv []string) []string {
	args := []string{"podman", "run", "--rm", "-i", "--network=none", "--unsetenv-all",
		"--userns=keep-id", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--read-only", "--tmpfs", containConfineInTmp + ":rw,nosuid,nodev,mode=1777",
		"-v", c.Dir + ":" + c.Dir + ":rw", "-w", c.Dir}
	for _, p := range c.RO {
		args = append(args, "-v", p+":"+p+":ro")
	}
	for _, s := range c.Sockets {
		args = append(args, "-v", s+":"+s+":ro")
	}
	for _, kv := range containerConfinedEnv(environ, c.Env, with) {
		args = append(args, "-e", kv)
	}
	return append(args, image, "-c", containerConfinedCommand(c, argv))
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
	args := containerConfinedArgs(c, image, with, os.Environ(), argv)
	if injected := strings.TrimSpace(os.Getenv("BASHY_CONTAIN_BASHY")); injected != "" {
		path, err := containInjectedBashy(injected, runtime.GOARCH)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy contain: %v; nothing ran\n", err)
			return containUnsupportedStatus
		}
		args = containerConfinedInject(args, path)
	}
	return runContainChild(self, args, "", nil)
}

// containerConfinedInject runs the confined image with the given static
// linux bashy in place of the image's own (BASHY_CONTAIN_BASHY, the same
// override an image call honours): mounted read-only at containInBashy, the
// entrypoint, and first on PATH so `bashy` inside is that binary. A candidate
// build is tested in the released image this way, and anything bashy starts
// inside the sandbox is the caller's version.
func containerConfinedInject(args []string, bashyPath string) []string {
	n := len(args) - 3 // image, "-c", command
	out := append([]string{}, args[:n]...)
	out = append(out, "--entrypoint", containInBashy, "-v", bashyPath+":"+containInBashy+":ro",
		"-e", "PATH="+filepath.Dir(containInBashy)+":/")
	return append(out, args[n:]...)
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
