package agentos

// Sprint: #290; Story: #1042; Story-ID: 530c3e2e68ac
//
// @contain(image: ...) — a pinned third-party image as ONE contained call.
//
//	@contain(image: "NAME@sha256:<digest>", workdir: "/testbed", out: DIR,
//	         env: "A,B", ro: "HOST:CTR,..." [, provider: "custom"])
//	function f() { ...; }
//
// A decorator only: it adds no command. The container runs through bashy's
// existing podman verb, in the call's own frame (its redirections apply).
// Running somebody else's image is a privileged operation (supply chain,
// exec), so it is containment, not a convenience: the image is named by
// digest only (a tag is refused; resolve one with `bashy podman pull` and
// `bashy podman image inspect --format '{{.Digest}}'`), its own entrypoint
// and shell are never trusted (bashy is mounted read-only and IS the
// entrypoint), the call gets one container for its whole life, nothing of the
// host is mounted but the read-only mounts asked for and one output
// directory, and there is no network. provider: "custom" hands the prepared
// podman command line to the BASHY_CONTAIN_CUSTOM wrapper (its words, then
// the command line), for a setup this decorator does not provide. Anything
// that cannot be enforced fails closed (125).

import (
	"crypto/rand"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/policy/audit"
)

// Inside the container: bashy is mounted at /.bashy/bashy, results at /out.
const (
	containInBashy = "/.bashy/bashy"
	containInOut   = "/out"
)

var containDigestRef = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)

// containImageSpec is one contained image call.
type containImageSpec struct {
	Image    string   // NAME@sha256:<digest>
	Workdir  string   // inside the container ("" = the image's)
	Out      string   // host directory mounted at /out ("" = none)
	Env      []string // variable NAMES passed through
	RO       []string // HOST:CTR read-only mounts
	Provider string   // "" | builtin | image | custom
	Argv     []string // run by the injected bashy: bashy's own arguments
	// Environ is the caller's exported environment (the decorated call's,
	// not bashy's process environment); nil = the process environment.
	Environ map[string]string
}

func (s *containImageSpec) lookup(name string) (string, bool) {
	if s.Environ == nil {
		return os.LookupEnv(name)
	}
	v, ok := s.Environ[name]
	return v, ok
}

func (s *containImageSpec) getenv(name string) string {
	v, _ := s.lookup(name)
	return strings.TrimSpace(v)
}

func (s *containImageSpec) validate() error {
	if !containDigestRef.MatchString(s.Image) {
		return fmt.Errorf("image %q is not pinned by digest (NAME@sha256:<64 hex>); resolve a tag once with `bashy podman pull %s` and `bashy podman image inspect --format '{{.Digest}}' %s`", s.Image, s.Image, s.Image)
	}
	switch s.Provider {
	case "", containBuiltin, containImageP:
	case containCustom:
		if s.getenv("BASHY_CONTAIN_CUSTOM") == "" {
			return errors.New("provider custom needs BASHY_CONTAIN_CUSTOM (the wrapper command)")
		}
	default:
		return fmt.Errorf("provider %q does not run images (builtin or custom)", s.Provider)
	}
	if s.Workdir != "" && !strings.HasPrefix(s.Workdir, "/") {
		return fmt.Errorf("workdir %q must be an absolute path inside the container", s.Workdir)
	}
	for _, m := range s.RO {
		host, ctr, ok := strings.Cut(m, ":")
		if !ok || !filepath.IsAbs(host) || !strings.HasPrefix(ctr, "/") || strings.Contains(ctr, ":") {
			return fmt.Errorf("ro: %q: want HOST:CTR, both absolute (always read-only)", m)
		}
		if ctr == "/.bashy" || strings.HasPrefix(ctr, "/.bashy/") || ctr == containInOut {
			return fmt.Errorf("ro: %q: %s is reserved", m, ctr)
		}
	}
	for _, n := range s.Env {
		if n == "" || strings.ContainsAny(n, "= \t\n") {
			return fmt.Errorf("env: takes variable NAMES, got %q", n)
		}
	}
	if len(s.Argv) == 0 {
		return errors.New("no command")
	}
	return nil
}

// containInjectedBashy is the static linux bashy mounted as the entrypoint:
// BASHY_CONTAIN_BASHY, else this bashy when it is a static linux binary of
// the image's architecture.
func containInjectedBashy(path, arch string) (string, error) {
	if path == "" {
		if runtime.GOOS != "linux" {
			return "", errors.New("the image needs a static linux bashy: set BASHY_CONTAIN_BASHY (e.g. `env make build-bashy-scratch` output)")
		}
		path = bashySelfPath()
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
	}
	f, err := elf.Open(path)
	if err != nil {
		return "", fmt.Errorf("injected bashy %s: not a linux executable: %v", path, err)
	}
	defer f.Close()
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			return "", fmt.Errorf("injected bashy %s is dynamically linked; the image may not have its loader (build it with CGO_ENABLED=0, or set BASHY_CONTAIN_BASHY)", path)
		}
	}
	want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[arch]
	if want != 0 && f.Machine != want {
		return "", fmt.Errorf("injected bashy %s is %v, the image is %s", path, f.Machine, arch)
	}
	return path, nil
}

// containPodman runs `bashy podman ARGS` and returns its trimmed stdout.
func containPodman(args ...string) (string, error) {
	cmd := exec.Command(bashySelfPath(), append([]string{"podman"}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("podman %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// containEnsureImage pulls a digest reference when it is not present (the
// content is fixed by the digest) and reports the image's architecture.
func containEnsureImage(ref string) (string, error) {
	if exec.Command(bashySelfPath(), "podman", "image", "exists", ref).Run() != nil {
		fmt.Fprintf(os.Stderr, "contain: pulling %s\n", ref)
		if _, err := containPodman("pull", "-q", ref); err != nil {
			return "", err
		}
	}
	return containPodman("image", "inspect", "--format", "{{.Architecture}}", ref)
}

// containRunArgs is the `podman run ...` command line for one contained
// image call (without bashy's own path in front).
func containRunArgs(s *containImageSpec, name, bashyPath string) []string {
	args := []string{"podman", "run", "--rm", "-i", "--name", name, "--network=none",
		"--entrypoint", containInBashy,
		"-v", bashyPath + ":" + containInBashy + ":ro",
		"-e", "BASHY_CONTAINED=image", "-e", "BASHY_HINTS=off", "-e", "BASHY_AGENTIC=1"}
	if s.Out != "" {
		args = append(args, "-v", s.Out+":"+containInOut)
	}
	for _, m := range s.RO {
		args = append(args, "-v", m+":ro")
	}
	if s.Workdir != "" {
		args = append(args, "-w", s.Workdir)
	}
	for _, n := range s.Env {
		if v, ok := s.lookup(n); ok {
			args = append(args, "-e", n+"="+v)
		}
	}
	return append(append(args, s.Image), s.Argv...)
}

// runContainedImage runs one contained image call: it validates the spec,
// ensures the pinned image, the injected bashy and the output directory, then
// hands the command line (bashy's path first; the custom wrapper's words
// before it under provider custom) to run, and cleans up after it.
func runContainedImage(s *containImageSpec, stderr io.Writer, run func(argv []string) int) int {
	fail := func(status int, format string, a ...any) int {
		fmt.Fprintf(stderr, "contain: "+format+"\n", a...)
		return status
	}
	if err := s.validate(); err != nil {
		return fail(2, "%v", err)
	}
	arch, err := containEnsureImage(s.Image)
	if err != nil {
		return fail(containUnsupportedStatus, "%v", err)
	}
	bashyPath, err := containInjectedBashy(s.getenv("BASHY_CONTAIN_BASHY"), arch)
	if err != nil {
		return fail(containUnsupportedStatus, "%v", err)
	}
	if s.Out != "" {
		if s.Out, err = filepath.Abs(s.Out); err == nil {
			err = os.MkdirAll(s.Out, 0o755)
		}
		if err != nil {
			return fail(containUnsupportedStatus, "out: %v", err)
		}
	}
	name := "bashy-contain-" + randomHex(6)
	argv := append([]string{bashySelfPath()}, containRunArgs(s, name, bashyPath)...)
	if s.Provider == containCustom {
		argv = append(strings.Fields(s.getenv("BASHY_CONTAIN_CUSTOM")), argv...)
	}
	start := time.Now()
	status := run(argv)
	// --rm removes it on a normal exit; this covers a killed podman client.
	_ = exec.Command(bashySelfPath(), "podman", "rm", "-f", "-i", name).Run()
	containRecord(s, status, start)
	return status
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// containLedgerPath is the per-host record of contained image calls.
func containLedgerPath() string {
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".bashy")
		}
	}
	return filepath.Join(home, "contain", "ledger.jsonl")
}

// containRecord appends the call to the contain ledger and, when BASHY_AUDIT
// is on, to the hash-chained audit log. Best-effort: a record that cannot be
// written never changes the call's status.
func containRecord(s *containImageSpec, status int, start time.Time) {
	rec := map[string]any{
		"time": start.UTC().Format(time.RFC3339Nano), "image": s.Image, "provider": s.Provider,
		"workdir": s.Workdir, "out": s.Out, "ro": s.RO, "env": s.Env,
		"exit": status, "duration_ms": time.Since(start).Milliseconds(),
	}
	if line, err := json.Marshal(rec); err == nil {
		path := containLedgerPath()
		if os.MkdirAll(filepath.Dir(path), 0o700) == nil {
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
	}
	if w := newAuditWriter(); w != nil {
		argv, masked := audit.Redact([]string{"@contain", "image:", s.Image})
		cwd, _ := os.Getwd()
		_, _ = w.Append(audit.Record{
			Time: start.UTC().Format(time.RFC3339Nano), Actor: auditActor(), Argv: argv,
			Binary: "contain", Cwd: cwd, Effects: []string{"exec"}, Host: auditHost(),
			Decision: "allow", Exit: status, DurationMs: time.Since(start).Milliseconds(), Redactions: masked,
		})
	}
}
