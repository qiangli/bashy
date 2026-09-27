package agentos

// Sprint: #290; Story: #1042; Story-ID: 530c3e2e68ac
//
// @contain(image: ...) — a pinned third-party image as ONE contained call.
//
//	@contain(image: "NAME@sha256:<digest>", workdir: "/testbed", out: DIR,
//	         net: "deny"|"door", sticky: KEY, env: "A,B", ro: "HOST:CTR,...")
//	function f() { ...; }
//
// A decorator only: it adds no command. The container runs through bashy's
// existing podman verb, in the call's own frame (its redirections apply).
// Running somebody else's image is a privileged operation (supply chain,
// exec), so it is containment, not a convenience: the image is named by
// digest only (a tag is refused; resolve one with `bashy podman pull` and
// `bashy podman image inspect --format '{{.Digest}}'`), its
// own entrypoint and shell are never trusted (bashy is mounted read-only and
// IS the entrypoint), the call gets one container for its whole life (state
// persists across the call's commands, gone after), nothing of the host is
// mounted but the read-only mounts asked for and one output directory, and
// the network is off. `net: door` is the one exception: the container sees
// the host's model door at 127.0.0.1:24556, restricted to ONE sticky binding,
// through a relay to a per-call unix socket whose far end is a filtering
// proxy in this process that holds the owner token — the token never enters
// the container. Anything that cannot be enforced fails closed (125).

import (
	"context"
	"crypto/rand"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/broker/door"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

// Inside the container: bashy's own files live under /.bashy.
const (
	containInBashy   = "/.bashy/bashy"
	containInRun     = "/.bashy/run"
	containInOut     = "/out"
	containDoorSock  = "door.sock"
	containStickyEnv = "BASHY_LLM_STICKY"
)

var containDigestRef = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)

// containImageSpec is one contained image call.
type containImageSpec struct {
	Image   string   // NAME@sha256:<digest>
	Workdir string   // inside the container ("" = the image's)
	Out     string   // host directory mounted at /out ("" = none)
	Net     string   // deny | door
	Sticky  string   // net door: the one sticky binding the call may use
	Env     []string // variable NAMES passed through
	RO      []string // HOST:CTR read-only mounts
	Argv    []string
}

func (s *containImageSpec) validate() error {
	if !containDigestRef.MatchString(s.Image) {
		return fmt.Errorf("image %q is not pinned by digest (NAME@sha256:<64 hex>); resolve a tag once with `bashy podman pull %s` and `bashy podman image inspect --format '{{.Digest}}' %s`", s.Image, s.Image, s.Image)
	}
	switch s.Net {
	case "", "deny":
		s.Net = "deny"
		if s.Sticky != "" {
			return errors.New("sticky: needs net: \"door\"")
		}
	case "door":
		if s.Sticky == "" {
			return errors.New("net: \"door\" needs sticky: KEY — the call may use one sticky binding, never the whole door")
		}
		if strings.ContainsAny(s.Sticky, "/ \t\n") {
			return fmt.Errorf("invalid sticky key %q", s.Sticky)
		}
	default:
		return fmt.Errorf("net %q: an image call takes net deny or door", s.Net)
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
func containInjectedBashy(arch string) (string, error) {
	path := strings.TrimSpace(os.Getenv("BASHY_CONTAIN_BASHY"))
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
		fmt.Fprintf(os.Stderr, "bashy contain: pulling %s\n", ref)
		if _, err := containPodman("pull", "-q", ref); err != nil {
			return "", err
		}
	}
	return containPodman("image", "inspect", "--format", "{{.Architecture}}", ref)
}

// containRunArgs is the podman command line for one contained image call.
func containRunArgs(s *containImageSpec, name, bashyPath, runDir string) []string {
	args := []string{"podman", "run", "--rm", "-i", "--name", name, "--network=none",
		"--entrypoint", containInBashy,
		"-v", bashyPath + ":" + containInBashy + ":ro",
		"-e", "BASHY_CONTAINED=image", "-e", "BASHY_HINTS=off", "-e", "BASHY_AGENTIC=1"}
	if runDir != "" {
		args = append(args, "-v", runDir+":"+containInRun)
	}
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
		if v, ok := os.LookupEnv(n); ok {
			args = append(args, "-e", n+"="+v)
		}
	}
	if s.Net == "door" {
		base := fmt.Sprintf("http://127.0.0.1:%d/sticky/%s", door.DefaultPort, s.Sticky)
		args = append(args,
			"-e", "OPENAI_BASE_URL="+base+"/v1", "-e", "OPENAI_API_KEY=contained",
			"-e", "ANTHROPIC_BASE_URL="+base+"/anthropic", "-e", "ANTHROPIC_API_KEY=contained",
			"-e", "OLLAMA_HOST="+base, "-e", containStickyEnv+"="+s.Sticky)
	}
	args = append(args, s.Image, "contain", "--init")
	if s.Net == "door" {
		args = append(args, "--door-sock", containInRun+"/"+containDoorSock)
	}
	return append(append(args, "--"), s.Argv...)
}

// runContainedImage runs one contained image call: it validates the spec,
// ensures the pinned image, prepares the injected bashy, the output directory
// and (net door) the per-call door proxy in this process, then hands the
// podman command line (`bashy podman run ...`, argv[0] is bashy) to run, and
// cleans up after it.
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
	bashyPath, err := containInjectedBashy(arch)
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
	runDir := ""
	if s.Net == "door" {
		if runtime.GOOS != "linux" {
			return fail(containUnsupportedStatus, "net door needs a linux host (a unix socket cannot cross the podman VM share)")
		}
		runDir, err = os.MkdirTemp("", "bashy-contain-")
		if err != nil {
			return fail(containUnsupportedStatus, "%v", err)
		}
		defer os.RemoveAll(runDir)
		stop, err := serveDoorProxy(filepath.Join(runDir, containDoorSock), s.Sticky)
		if err != nil {
			return fail(containUnsupportedStatus, "net door: %v", err)
		}
		defer stop()
	}
	name := "bashy-contain-" + randomHex(6)
	start := time.Now()
	status := run(append([]string{bashySelfPath()}, containRunArgs(s, name, bashyPath, runDir)...))
	// --rm removes it on a normal exit; this covers a killed podman client.
	_ = exec.Command(bashySelfPath(), "podman", "rm", "-f", "-i", name).Run()
	containRecord(s, status, start)
	return status
}

// runForwardingSignals runs cmd, forwarding SIGINT/SIGTERM to it, and returns
// its exit status (125 when it could not run).
func runForwardingSignals(cmd *exec.Cmd) int {
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case sig := <-sigs:
			_ = cmd.Process.Signal(sig)
		case err := <-done:
			if err == nil {
				return 0
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return exit.ExitCode()
			}
			fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
			return containUnsupportedStatus
		}
	}
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
		"time": start.UTC().Format(time.RFC3339Nano), "image": s.Image, "net": s.Net,
		"sticky": s.Sticky, "workdir": s.Workdir, "out": s.Out, "ro": s.RO, "env": s.Env,
		"argv": s.Argv, "exit": status, "duration_ms": time.Since(start).Milliseconds(),
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
		argv, masked := audit.Redact(append([]string{"bashy", "contain", "--image", s.Image, "--net", s.Net, "--"}, s.Argv...))
		cwd, _ := os.Getwd()
		_, _ = w.Append(audit.Record{
			Time: start.UTC().Format(time.RFC3339Nano), Actor: auditActor(), Argv: argv,
			Binary: "contain", Cwd: cwd, Effects: []string{"exec"}, Host: auditHost(),
			Decision: "allow", Exit: status, DurationMs: time.Since(start).Milliseconds(), Redactions: masked,
		})
	}
}

// ---- net door: the host side ----------------------------------------------

// doorUpstream is where the proxy forwards: BASHY_CONTAIN_DOOR (a base URL; a
// /k/<token> path carries its own credential), else this host's door with the
// owner token as a bearer.
func doorUpstream() (*url.URL, string, error) {
	raw := strings.TrimSpace(os.Getenv("BASHY_CONTAIN_DOOR"))
	bearer := ""
	if raw == "" {
		raw = door.BaseURL()
		t, err := door.Token()
		if err != nil {
			return nil, "", err
		}
		bearer = t
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, "", fmt.Errorf("BASHY_CONTAIN_DOOR %q is not a base URL", raw)
	}
	return u, bearer, nil
}

// doorAllowed reports whether a contained client's request may pass: it must
// be on the call's sticky binding, carry no token of its own, and be an
// inference or read-only model route — never the sticky/session/pool APIs.
func doorAllowed(method, path, sticky string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/sticky/"+sticky+"/")
	if !ok {
		return "", false
	}
	rest = "/" + rest
	switch method {
	case http.MethodPost:
		switch strings.TrimPrefix(rest, "/openai") {
		case "/v1/chat/completions", "/v1/completions", "/v1/embeddings", "/v1/messages", "/anthropic/v1/messages",
			"/api/chat", "/api/generate", "/api/embed", "/api/embeddings", "/api/show":
			return rest, true
		}
	case http.MethodGet:
		switch strings.TrimPrefix(rest, "/openai") {
		case "/v1/models", "/api/tags", "/api/version", "/health":
			return rest, true
		}
	}
	return "", false
}

func newDoorProxy(up *url.URL, bearer, sticky string) http.Handler {
	rp := &httputil.ReverseProxy{
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = up.Scheme, up.Host
			pr.Out.URL.Path = up.Path + "/sticky/" + sticky + pr.In.Context().Value(doorRestKey{}).(string)
			pr.Out.URL.RawPath = ""
			pr.Out.Host = up.Host
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("x-api-key")
			if bearer != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+bearer)
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := doorAllowed(r.Method, r.URL.Path, sticky)
		if !ok {
			http.Error(w, fmt.Sprintf("bashy contain: net door allows only the sticky binding %q (inference and model listing): %s %s refused", sticky, r.Method, r.URL.Path), http.StatusForbidden)
			return
		}
		rp.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), doorRestKey{}, rest)))
	})
}

type doorRestKey struct{}

// serveDoorProxy serves the filtering proxy on a new unix socket; the
// upstream door must answer /health first.
func serveDoorProxy(sock, sticky string) (func(), error) {
	up, bearer, err := doorUpstream()
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, up.String()+"/health", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the model door is not reachable at %s (bashy llm up; or BASHY_CONTAIN_DOOR): %v", up.Host, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the model door at %s answered %s", up.Host, resp.Status)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: newDoorProxy(up, bearer, sticky), ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() { _ = srv.Close() }, nil
}

// ---- inside the container ---------------------------------------------------

// containInit is the injected bashy's entrypoint inside the image: with
// --door-sock it relays 127.0.0.1:<door port> to the per-call socket, then
// runs the command and returns its status.
func containInit(args []string) int {
	sock := ""
	i := 0
	for ; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--door-sock" && i+1 < len(args):
			i++
			sock = args[i]
		case a == "--":
			i++
			goto command
		default:
			goto command
		}
	}
command:
	if i >= len(args) {
		fmt.Fprintln(os.Stderr, "bashy contain --init: no command")
		return 2
	}
	if sock != "" {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", door.DefaultPort))
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy contain --init: door relay: %v\n", err)
			return containUnsupportedStatus
		}
		defer ln.Close()
		go relayToUnix(ln, sock)
	}
	// bashy is mounted at /.bashy/bashy: a bare `bashy` inside the call finds it.
	_ = os.Setenv("PATH", "/.bashy:"+os.Getenv("PATH"))
	argv := args[i:]
	path := argv[0]
	if isBashyExecutable(path) || path == "bashy" {
		path = containInBashy
	} else if p, err := exec.LookPath(path); err == nil {
		path = p
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return runForwardingSignals(cmd)
}

func relayToUnix(ln net.Listener, sock string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			u, err := net.Dial("unix", sock)
			if err != nil {
				return
			}
			defer u.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(u, c); closeWrite(u); done <- struct{}{} }()
			go func() { _, _ = io.Copy(c, u); closeWrite(c); done <- struct{}{} }()
			<-done
			<-done
		}(c)
	}
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}
