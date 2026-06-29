// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The space-time advisor: a non-intrusive supervisor that, ONLY when a command
// fails, adds a context-aware hint explaining that the failure is determined by
// the agent's ambient resource environment ("space") — filesystem/disk, current
// working directory, CPU/memory, or network/locality — and what to do instead.
//
// The goal is to stop the doomed retry loop an agentic tool (codex, claude,
// aider, …) burns time and tokens on when it cannot see WHY a command failed:
// e.g. an agent reaching `host.local` after the laptop moved off its LAN keeps
// probing IPs and assuming the local uid, all doomed, because the host is simply
// not on this network. Bashy uniquely holds the whole-environment snapshot and
// can say so.
//
// It is wired as the OUTERMOST ExecHandler middleware (mirroring dryrun.go): it
// calls the rest of the chain, observes the resulting exit status, and on a
// non-zero exit runs a small pattern library against an injectable space
// snapshot. It NEVER blocks and ALWAYS returns the underlying error unchanged —
// it only appends one advisory line. Registered only by cmd/bashy (never the
// pure cmd/bash drop-in) and only in non-posix mode; active in agent mode or
// when BASHY_ADVISOR is set.
package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// adviceSchemaVersion versions the agent-mode JSON line (peer to weavecli's
// "loom-v2" envelope; kept distinct because this is a different shape).
const adviceSchemaVersion = "bashy-advice-v1"

// localityVerdict is the coarse network-location signal. It only flavors the
// network hint's wording; the trigger itself uses the concrete per-target
// resolvability probe, which is far more reliable than abstract locality.
type localityVerdict string

const (
	localityHomeLAN localityVerdict = "home-lan"
	localityRemote  localityVerdict = "remote"
	localityUnknown localityVerdict = "unknown"
)

// spaceProbe holds the (injectable) sensors of the ambient environment, so the
// pattern library is unit-testable without touching the real host. Production
// wiring is defaultSpaceProbe(); tests substitute their own funcs.
type spaceProbe struct {
	// resolveHost reports whether host resolves to an address from here.
	resolveHost func(host string) bool
	// diskFor returns free bytes and read-only state for the filesystem
	// backing dir (ok=false when unknown / unsupported platform).
	diskFor func(dir string) (freeBytes uint64, readOnly bool, ok bool)
	// availRAM returns available memory in bytes (ok=false when unknown).
	availRAM func() (uint64, bool)
	// pathExists reports whether name exists relative to dir.
	pathExists func(dir, name string) bool
	// repoRoot returns the VCS top-level at or above dir, if any.
	repoRoot func(dir string) (string, bool)
	// locality is the coarse network-location verdict.
	locality func() localityVerdict
}

// advisor is the configured supervisor instance.
type advisor struct {
	agent bool // agent mode: emit a JSON line instead of human prose
	probe spaceProbe
	mem   *memory // accumulated history (the "time" axis); may be nil in tests
	netfp string  // network fingerprint of the current environment
}

// newAdvisor builds the advisor with production probes and memory.
func newAdvisor() *advisor {
	return &advisor{
		agent: weavecli.IsAgent(),
		probe: defaultSpaceProbe(),
		mem:   newMemory(),
		netfp: networkFingerprint(),
	}
}

// advisorEnabled reports whether the advisor should run: on in agent mode, or
// opt-in for humans via BASHY_ADVISOR (anything but empty/"0"/"false").
func advisorEnabled() bool {
	if weavecli.IsAgent() {
		return true
	}
	switch strings.ToLower(os.Getenv("BASHY_ADVISOR")) {
	case "", "0", "false", "off", "no":
		return false
	}
	return true
}

// hint is one piece of advice for a failed command.
type hint struct {
	dimension string // network | disk | cwd | compute
	retryable bool   // false ⇒ re-running as-is is doomed; change approach
	text      string // the human-readable explanation
	suggest   string // the concrete next action
}

// adviceLine is the agent-mode JSON shape (one line on stderr).
type adviceLine struct {
	Schema    string `json:"schema_version"`
	Kind      string `json:"kind"` // "advice"
	Dimension string `json:"dimension"`
	Command   string `json:"command"`
	Exit      int    `json:"exit"`
	Retryable bool   `json:"retryable"`
	Hint      string `json:"hint"`
	Suggest   string `json:"suggest,omitempty"`
}

// advisorHandler is the post-exec ExecHandler middleware. It runs the rest of
// the chain, and on a non-zero exit consults the pattern library and emits at
// most one advisory line. It always returns the underlying error unchanged.
func advisorHandler(a *advisor) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			err := next(ctx, args)
			if len(args) == 0 {
				return err
			}
			status, ok := exitStatusOf(err)
			if !ok {
				return err // a non-exit error (e.g. interrupt): say nothing
			}
			key := loopKey(args)
			if status == 0 {
				// Success clears the loop counter and records host
				// reachability under the current network fingerprint, so a
				// later failure elsewhere can be diagnosed against this memory.
				if a.mem != nil {
					a.mem.clearFail(key)
					if networkTools[baseName(args[0])] {
						if host := extractNetworkTarget(args); host != "" {
							a.mem.recordSuccess(host, a.netfp)
						}
					}
				}
				return err
			}
			n := 1
			if a.mem != nil {
				n = a.mem.recordFail(key)
			}
			if h := applyLoop(a.advise(handlerDir(ctx), args, status), baseName(args[0]), n); h != nil {
				a.emit(handlerStderr(ctx), args[0], status, h)
			}
			return err
		}
	}
}

// exitStatusOf extracts the integer exit status from an ExecHandler error.
// A non-zero exit is returned as interp.ExitStatus; a signal kill is encoded as
// 128+signal (so an OOM SIGKILL is 137). ok=false for nil or non-exit errors.
func exitStatusOf(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	if st, isExit := err.(interp.ExitStatus); isExit {
		return int(st), true
	}
	return 0, false
}

// advise runs the pattern library in priority order and returns the first
// matching hint, or nil. Each case is deliberately conservative to keep false
// positives low — a wrong hint is harmless (advisory) but erodes trust.
func (a *advisor) advise(dir string, args []string, status int) *hint {
	name := baseName(args[0])

	// 1. CWD — most specific: a relative path argument that doesn't exist here
	//    but does exist at the repo root ⇒ wrong working directory.
	if h := a.adviseCWD(dir, args); h != nil {
		return h
	}
	// 2. Network — a network tool failing to reach a LAN-ish target that does
	//    not resolve from here ⇒ off its network; use the tunnel/public route.
	if h := a.adviseNetwork(name, args); h != nil {
		return h
	}
	// 3. Compute — exit 137 (SIGKILL) on a memory-heavy tool ⇒ likely OOM.
	if h := a.adviseCompute(name, status); h != nil {
		return h
	}
	// 4. Disk — the filesystem backing $PWD is nearly full or read-only.
	if h := a.adviseDisk(dir, name); h != nil {
		return h
	}
	return nil
}

// ---- case 1: wrong working directory ----

func (a *advisor) adviseCWD(dir string, args []string) *hint {
	if a.probe.repoRoot == nil || a.probe.pathExists == nil {
		return nil
	}
	root, ok := a.probe.repoRoot(dir)
	if !ok || root == "" || root == dir {
		return nil
	}
	for _, arg := range args[1:] {
		if arg == "" || strings.HasPrefix(arg, "-") || filepath.IsAbs(arg) {
			continue
		}
		// Only a path-shaped argument that is missing here but present at the
		// repo root — a precise, low-false-positive signal.
		if !looksLikePath(arg) {
			continue
		}
		if a.probe.pathExists(dir, arg) {
			continue
		}
		if a.probe.pathExists(root, arg) {
			return &hint{
				dimension: "cwd",
				retryable: false,
				text: fmt.Sprintf("%q does not exist in the current directory (%s) but is present at the repo root (%s).",
					arg, dir, root),
				suggest: fmt.Sprintf("cd %s — you are likely in the wrong directory; re-running here will keep failing.", root),
			}
		}
	}
	return nil
}

// ---- case 2: host gone remote / LAN-only name ----

var networkTools = map[string]bool{
	"ssh": true, "scp": true, "sftp": true, "mosh": true, "rsync": true,
	"ssh-copy-id": true, "curl": true, "wget": true, "ping": true, "ping6": true,
	"nc": true, "ncat": true, "telnet": true,
}

func (a *advisor) adviseNetwork(name string, args []string) *hint {
	if !networkTools[name] || a.probe.resolveHost == nil {
		return nil
	}
	host := extractNetworkTarget(args)
	if host == "" || !lanish(host) {
		return nil
	}
	if a.probe.resolveHost(host) {
		return nil // it resolves from here — not this problem
	}
	const suggest = "reach it via the tunnel or its public/VPN address; retrying LAN probes (IP scans, mDNS) will keep failing."

	// History upgrade: if we reached this host before from a DIFFERENT network,
	// the cause is concrete (the machine moved), not a guess.
	if a.mem != nil && a.netfp != "" {
		if rec, ok := a.mem.priorSuccess(host); ok && rec.NetFP != "" && rec.NetFP != a.netfp {
			return &hint{
				dimension: "network",
				retryable: false,
				text: fmt.Sprintf("%q was reachable before from a different network but does not resolve here — this machine has moved off its network.",
					host),
				suggest: suggest,
			}
		}
	}

	where := "you may be off its network"
	if a.probe.locality != nil && a.probe.locality() == localityRemote {
		where = "this machine is currently remote (off the LAN)"
	}
	return &hint{
		dimension: "network",
		retryable: false,
		text: fmt.Sprintf("%q looks like a LAN-only address (mDNS/private) and does not resolve from here — %s.",
			host, where),
		suggest: suggest,
	}
}

// ---- case 3: OOM-killed heavy build/test ----

var heavyTools = map[string]bool{
	"go": true, "make": true, "ninja": true, "bazel": true, "cmake": true,
	"cc": true, "gcc": true, "g++": true, "clang": true, "clang++": true,
	"ld": true, "lld": true, "rustc": true, "cargo": true,
	"javac": true, "java": true, "node": true, "npm": true, "pnpm": true,
	"yarn": true, "webpack": true, "tsc": true, "jest": true,
	"pytest": true, "python": true, "python3": true,
}

func (a *advisor) adviseCompute(name string, status int) *hint {
	if status != 137 || !heavyTools[name] { // 137 = 128 + SIGKILL(9)
		return nil
	}
	ram := ""
	if a.probe.availRAM != nil {
		if free, ok := a.probe.availRAM(); ok {
			ram = fmt.Sprintf(" Available RAM was %s.", humanBytes(int64(free)))
		}
	}
	return &hint{
		dimension: "compute",
		retryable: false,
		text: fmt.Sprintf("exit 137 means the process was killed (SIGKILL), typically the OOM killer on a memory-heavy %q.%s",
			name, ram),
		suggest: "reduce parallelism (e.g. a lower -j), batch the work, or free memory; re-running it identically will OOM again.",
	}
}

// ---- case 4: disk full / read-only mount ----

var writerTools = map[string]bool{
	"cp": true, "mv": true, "touch": true, "tee": true, "dd": true,
	"tar": true, "gzip": true, "zip": true, "go": true, "make": true,
	"git": true, "cc": true, "gcc": true, "clang": true, "ld": true,
}

func (a *advisor) adviseDisk(dir, name string) *hint {
	if a.probe.diskFor == nil {
		return nil
	}
	free, readOnly, ok := a.probe.diskFor(dir)
	if !ok {
		return nil
	}
	const lowWater = 64 << 20 // 64 MiB
	switch {
	case readOnly && writerTools[name]:
		return &hint{
			dimension: "disk",
			retryable: false,
			text:      fmt.Sprintf("the filesystem backing the current directory (%s) is mounted read-only.", dir),
			suggest:   "write to a writable location; re-running here will keep failing with EROFS.",
		}
	case free < lowWater:
		return &hint{
			dimension: "disk",
			retryable: false,
			text:      fmt.Sprintf("the volume backing the current directory (%s) has only %s free.", dir, humanBytes(int64(free))),
			suggest:   "free space or write elsewhere; the failure may be ENOSPC and will recur as-is.",
		}
	}
	return nil
}

// ---- emission ----

// emit writes at most one advisory line. In agent mode it is a JSON object on
// stderr (so stdout — the command's parsed output — stays clean); for humans it
// is a single prefixed prose line.
func (a *advisor) emit(w io.Writer, cmd string, status int, h *hint) {
	if w == nil {
		return
	}
	if a.agent {
		b, _ := json.Marshal(adviceLine{
			Schema:    adviceSchemaVersion,
			Kind:      "advice",
			Dimension: h.dimension,
			Command:   baseName(cmd),
			Exit:      status,
			Retryable: h.retryable,
			Hint:      h.text,
			Suggest:   h.suggest,
		})
		fmt.Fprintf(w, "%s\n", b)
		return
	}
	if h.suggest != "" {
		fmt.Fprintf(w, "bashy: ⓘ %s %s\n", h.text, h.suggest)
		return
	}
	fmt.Fprintf(w, "bashy: ⓘ %s\n", h.text)
}

// ---- helpers ----

func baseName(cmd string) string {
	if i := strings.LastIndexAny(cmd, `/\`); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

// looksLikePath reports whether arg is shaped like a file path (has a separator
// or an extension), to avoid treating subcommands/flags as missing files.
func looksLikePath(arg string) bool {
	if strings.ContainsAny(arg, `/\`) {
		return true
	}
	return strings.Contains(arg, ".") && !strings.HasPrefix(arg, ".")
}

// lanish reports whether host is a LAN-only name (mDNS .local / private TLDs) or
// an RFC1918 / link-local / unique-local address.
func lanish(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	for _, suf := range []string{".local", ".lan", ".home", ".internal", ".intranet"} {
		if strings.HasSuffix(h, suf) {
			return true
		}
	}
	return false
}

// extractNetworkTarget pulls the destination host out of a network command's
// arguments: a URL, user@host, host:path (scp), or a bare host token. A bare
// token is only accepted when it is an IP or a LAN-ish name — a plain filename
// like "file.txt" is not a host, so an scp source argument is skipped in favor
// of the real host:path destination. The structured (URL/@/colon) forms return
// the host regardless; adviseNetwork's lanish() filter decides relevance.
func extractNetworkTarget(args []string) string {
	for _, arg := range args[1:] {
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		// URL (scheme://host/…) — check first, it also contains ':' and '/'.
		if strings.Contains(arg, "://") {
			if u, err := url.Parse(arg); err == nil && u.Hostname() != "" {
				return u.Hostname()
			}
			continue
		}
		// user@host[:path]
		if at := strings.LastIndex(arg, "@"); at >= 0 {
			h := arg[at+1:]
			if c := strings.Index(h, ":"); c >= 0 {
				h = h[:c]
			}
			if h != "" && !strings.ContainsAny(h, `/\`) {
				return h
			}
			continue
		}
		// host:path or host:port (scp) — the part before the colon must look
		// like a host (no path separator), which excludes "/a/b:c".
		if c := strings.Index(arg, ":"); c > 0 {
			if left := arg[:c]; !strings.ContainsAny(left, `/\`) {
				return left
			}
			continue
		}
		// A bare token (no scheme/@/colon): a host only if it is an IP or a
		// LAN-ish name — a plain filename like "file.txt" is skipped.
		if !strings.ContainsAny(arg, `/\`) && (net.ParseIP(arg) != nil || lanish(arg)) {
			return arg
		}
	}
	return ""
}

// ---- default (production) probes ----

func defaultSpaceProbe() spaceProbe {
	return spaceProbe{
		resolveHost: defaultResolveHost,
		diskFor:     probeDisk, // platform files: advisor_unix.go / advisor_other.go
		availRAM:    probeRAM,  // platform files
		pathExists:  defaultPathExists,
		repoRoot:    defaultRepoRoot,
		locality:    func() localityVerdict { return localityUnknown },
	}
}

func defaultResolveHost(host string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	return err == nil && len(addrs) > 0
}

func defaultPathExists(dir, name string) bool {
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, name)
	}
	_, err := os.Lstat(p)
	return err == nil
}

func defaultRepoRoot(dir string) (string, bool) {
	d := dir
	for {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}
