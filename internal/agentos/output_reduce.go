// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/reduce"
	"github.com/qiangli/yoke/pkg/secrets"
	"golang.org/x/term"
	"mvdan.cc/sh/v3/interp"
)

// optionalBool distinguishes an absent --reduce from --reduce=false.
type optionalBool struct {
	set   bool
	value bool
}

func (v *optionalBool) String() string   { return strconv.FormatBool(v.value) }
func (v *optionalBool) IsBoolFlag() bool { return true }
func (v *optionalBool) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	v.set, v.value = true, b
	return nil
}

var (
	noElideFlag bool
	reduceFlag  optionalBool
)

func init() {
	flag.BoolVar(&noElideFlag, "no-elide", false, "bashy: keep complete agent-facing command output")
	flag.BoolVar(&noElideFlag, "full", false, "bashy: alias for --no-elide")
	flag.Var(&reduceFlag, "reduce", "bashy: reduce agent-facing command output (true or false)")
}

// outputReductionEnabled implements the Stage 1 explicit-opt-in precedence.
// VSC_PROFILE is the harness's declared certification mode and is deliberately
// stronger than every output request; --posix is intentionally absent because
// POSIX language semantics and the model-facing output sink are orthogonal.
func outputReductionEnabled(env []string) bool {
	if strings.EqualFold(strings.TrimSpace(outputEnv(env, "VSC_PROFILE")), "cert") {
		return false
	}
	if noElideFlag {
		return false
	}
	reduceEnv := strings.ToLower(strings.TrimSpace(outputEnv(env, "BASHY_OUTPUT_REDUCE")))
	switch reduceEnv {
	case "off", "0", "false", "no":
		return false
	}
	if reduceFlag.set {
		return reduceFlag.value
	}
	switch reduceEnv {
	case "on", "1", "true", "yes":
		return true
	default:
		if strings.HasPrefix(reduceEnv, "profile:") && len(strings.TrimSpace(strings.TrimPrefix(reduceEnv, "profile:"))) > 0 {
			return true
		}
	}
	return false
}

func outputEnv(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if name, value, ok := strings.Cut(env[i], "="); ok && name == key {
			return value
		}
	}
	return ""
}

func agentModeForEnv(env []string) bool {
	switch outputEnv(env, "BASHY_AGENTIC") {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

// shellOutputStore uses the existing Bashy command artifact tree. The reducer
// contributes one content-addressed subdirectory; it does not create another
// state root or consult the secrets renderer.
func shellOutputStore() (*reduce.Store, error) {
	return shellOutputStoreForEnv(os.Environ())
}

func shellOutputStoreForEnv(env []string) (*reduce.Store, error) {
	root := strings.TrimSpace(outputEnv(env, "BASHY_HOME"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("out: resolve Bashy session artifacts: %w", err)
		}
		if home == "" {
			return nil, errors.New("out: resolve Bashy session artifacts: empty home directory")
		}
		root = filepath.Join(home, ".bashy")
	}
	return reduce.NewStore(filepath.Join(root, "exec", "output")), nil
}

func shellOutputRedactor(env []string) *secrets.Redactor {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		if name, value, ok := strings.Cut(entry, "="); ok {
			values[name] = value
		}
	}
	r := secrets.NewRedactor()
	_ = r.SetShapeMode(secrets.ShapeMask)
	for name := range secrets.VaultEnvNames() {
		if value, ok := values[name]; ok {
			// Short values cannot be masked safely; Register refuses them without
			// including the value in its error. Shape masking still applies.
			_ = r.Register(name, value)
		}
	}
	return r
}

type shellOutputReducer struct {
	store    *reduce.Store
	redactor *secrets.Redactor
	home     string
	stage1   bool
	outDst   io.Writer
	errDst   io.Writer

	mu      sync.Mutex
	flushMu sync.Mutex
	out     bytes.Buffer
	err     bytes.Buffer
	active  int
	failed  bool
	verdict bool
	argv    []string
}

type shellCaptureWriter struct {
	r   *shellOutputReducer
	err bool
}

func (w shellCaptureWriter) Write(p []byte) (int, error) {
	w.r.mu.Lock()
	if w.r.active > 0 {
		if w.err {
			_, _ = w.r.err.Write(p)
		} else {
			_, _ = w.r.out.Write(p)
		}
		w.r.mu.Unlock()
		return len(p), nil
	}
	w.r.mu.Unlock()
	// Builtins write straight to the runner's terminal sink, without passing
	// through ExecHandler middleware. They are still model-visible output, so
	// Stage 0 must canonicalize them too. Commands do pass through the capture
	// above, which lets us canonicalize a complete stream before Stage 1 makes
	// any redaction or spill decision.
	p = w.r.prepare(p)
	if w.err {
		return w.r.errDst.Write(p)
	}
	return w.r.outDst.Write(p)
}

func newShellOutputReducer(out, errOut io.Writer, env []string) (*shellOutputReducer, error) {
	store, err := shellOutputStoreForEnv(env)
	if err != nil {
		return nil, err
	}
	home := displayHome(outputEnv(env, "HOME"), runtime.GOOS)
	if outputParentIsBashy {
		home = "" // data for the parent bashy's program, not for a model
	}
	return &shellOutputReducer{
		store: store, redactor: shellOutputRedactor(env),
		home: home, stage1: outputReductionEnabled(env),
		outDst: out, errDst: errOut,
	}, nil
}

// outputParentEnv carries the pid of the bashy that started a process.
// Stage 0 canonicalization is a display transform for a MODEL reading bashy's
// output; a bashy whose direct parent is a bashy writes data for a program — a
// `$("$BASHY" ...)` capture, a pipeline, a dag body's command — and rewriting
// HOME there corrupts every captured path. The parent canonicalizes whatever
// of it reaches a model. Matching the parent pid (not mere presence) keeps
// Stage 0 on for the harness's shell in bashy → agent CLI → bashy: the
// variable is inherited through the agent, but the agent is the parent.
const outputParentEnv = "BASHY_OUTPUT_PARENT"

// outputParentIsBashy is decided from the inherited value before the AgentOS
// shell exports its own pid. Utility and plain shell aliases also import
// agentos, but must leave their environment alone.
var outputParentIsBashy bool

func init() {
	outputParentIsBashy = os.Getenv(outputParentEnv) == strconv.Itoa(os.Getppid())
}

// MarkOutputParent identifies an AgentOS shell process to its child bashy
// processes. Call it only after selecting the AgentOS route, never from package
// init: the same executable serves plain shell and coreutils applet aliases.
func MarkOutputParent() {
	_ = os.Setenv(outputParentEnv, strconv.Itoa(os.Getpid()))
}

// displayHome is the directory Stage 0 canonicalizes to the `$HOME` token.
//
// On Windows it is empty, so no canonicalization happens there. The token is
// only honest where it round-trips: on Unix `$HOME/bin` re-expands to the
// path it replaced. On Windows the shell hands scripts HOME in the MSYS form
// (/c/Users/x) while the process environment carries the native one
// (C:\Users\x), so canonicalizing native output yields `$HOME\s213` — a
// spelling that expands to `/c/Users/xs213`, i.e. to nothing. Measured
// 2026-09-19 (todo 1ec1081071d7): `echo $PATH` printed
// `C:\Windows\System32;C:\Windows;$HOME\s213`, and a PATH pasted back from
// that display cannot find anything under the profile directory. Better no
// reduction than a token that leaks a broken path into executable lookup.
func displayHome(home, goos string) string {
	if goos == "windows" {
		return ""
	}
	return home
}

func (r *shellOutputReducer) stdout() io.Writer { return shellCaptureWriter{r: r} }
func (r *shellOutputReducer) stderr() io.Writer { return shellCaptureWriter{r: r, err: true} }

// terminalSink reports whether w is the process's own terminal — an *os.File
// on a tty (a pty on unix, the console handle on Windows).
//
// Stage 0 exists for MODEL-visible output: a harness reads bashy through a pipe.
// A terminal sink is a human (or the bashy app's own xterm), and the bytes a
// foreground child writes there must go to the terminal itself: wrapping the
// sink in shellCaptureWriter turns it into a plain io.Writer, os/exec then hands
// every external command an os.Pipe, and any TUI (codex, claude, vim, less,
// ssh) starts without a terminal — "stdout is not a terminal" (story
// 05c51640). So a tty sink is passed through unwrapped and the child inherits
// the file; capture applies only to a sink that is not a terminal, or to any
// sink when Stage 1 reduction was requested explicitly (outputReductionEnabled),
// which is the operator asking for exactly that capture.
func terminalFile(w io.Writer) *os.File {
	// A copying wrapper over the terminal (bashy dag's run-journal tee) still
	// ends at the terminal: look through it.
	for {
		wrapped, ok := w.(interface{ Unwrap() io.Writer })
		if !ok {
			break
		}
		w = wrapped.Unwrap()
	}
	f, ok := w.(*os.File)
	if ok && term.IsTerminal(int(f.Fd())) {
		return f
	}
	return nil
}

func terminalSink(w io.Writer) bool {
	return terminalFile(w) != nil
}

// wrapSinks composes the reducer's writers over the configured sinks, leaving a
// terminal sink untouched unless Stage 1 was requested. It reports whether any
// sink is captured, so the caller can skip the middleware entirely when none is.
func (r *shellOutputReducer) wrapSinks(out, errOut io.Writer) (io.Writer, io.Writer, bool) {
	captured := false
	if r.stage1 || !terminalSink(out) {
		out, captured = r.stdout(), true
	} else {
		// Give os/exec the terminal file itself. A journal tee remains a
		// writer to the terminal, but would make child stdout a pipe.
		out = terminalFile(out)
	}
	if r.stage1 || !terminalSink(errOut) {
		errOut, captured = r.stderr(), true
	} else {
		errOut = terminalFile(errOut)
	}
	return out, errOut, captured
}

func (r *shellOutputReducer) middleware(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, argv []string) error {
		// Background commands may outlive this script. They must not hold a
		// shared capture window that buffers later foreground writes until
		// the background handler returns; $! makes that loss deterministic.
		if interp.HandlerCtx(ctx).Async() {
			return next(ctx, argv)
		}
		r.begin(argv)
		err := next(ctx, argv)
		status := 0
		if err != nil {
			status = 1
			var exit interp.ExitStatus
			if errors.As(err, &exit) {
				status = int(exit)
			}
		}
		flushErr := r.finish(status)
		if err != nil {
			return err
		}
		return flushErr
	}
}

func (r *shellOutputReducer) begin(argv []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	argv = outputShapeArgv(argv)
	shape := atlas.ResolveOutputShape(argv)
	if r.active == 0 {
		r.failed = false
		r.verdict = shape == atlas.ShapeVerdict
		r.argv = append(r.argv[:0], argv...)
	} else {
		r.verdict = r.verdict && shape == atlas.ShapeVerdict
		if !equalArgv(r.argv, argv) {
			r.argv = []string{"pipeline"}
		}
	}
	r.active++
}

// outputShapeArgv presents the atlas with command names rather than resolved
// paths. Agent-mode provisioner shims execute `bashy go test`; unwrap that one
// transparent hop as well, otherwise every measured argv[0]+argv[1] verdict
// would be misclassified as a result by the very shell mode that enables the
// reducer.
func outputShapeArgv(argv []string) []string {
	if len(argv) == 0 {
		return argv
	}
	out := append([]string(nil), argv...)
	out[0] = outputCommandBase(out[0])
	if (out[0] == "bashy" || out[0] == "bashy.real") && len(out) > 1 {
		out = out[1:]
		out[0] = outputCommandBase(out[0])
	}
	return out
}

func outputCommandBase(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	return strings.TrimSuffix(filepath.Base(name), ".exe")
}

func equalArgv(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *shellOutputReducer) finish(status int) error {
	// Pipeline components may finish concurrently and share the terminal stderr
	// sink. Serialize snapshot + reduction so no bytes are lost or emitted twice.
	r.flushMu.Lock()
	defer r.flushMu.Unlock()

	r.mu.Lock()
	if status != 0 {
		r.failed = true
	}
	r.active--
	if r.active > 0 {
		r.mu.Unlock()
		return nil
	}
	out := append([]byte(nil), r.out.Bytes()...)
	errOut := append([]byte(nil), r.err.Bytes()...)
	argv := append([]string(nil), r.argv...)
	failed := r.failed
	verdict := r.verdict
	r.out.Reset()
	r.err.Reset()
	r.argv = r.argv[:0]
	r.mu.Unlock()

	if failed {
		// Until a failure-line classifier exists, the only honest implementation
		// of "failure is never compressed below its evidence" is to keep it all,
		// except for the closed Stage 0.1 telemetry registry.
		if err := r.emit(r.outDst, argv, out, false, true); err != nil {
			return err
		}
		return r.emit(r.errDst, argv, errOut, false, true)
	}
	if err := r.emit(r.outDst, argv, out, verdict, !r.stage1); err != nil {
		return err
	}
	return r.emit(r.errDst, argv, errOut, verdict, !r.stage1)
}

func (r *shellOutputReducer) emit(dst io.Writer, argv []string, full []byte, verdict, telemetryOnly bool) error {
	if len(full) == 0 {
		return nil
	}
	body := r.prepare(full)
	if verdict && !telemetryOnly {
		digest, err := r.store.Put(body)
		if err != nil {
			// Spill-before-emit means a failed spill may never produce a lossy
			// view. Preserve the complete redacted stream and report the failure.
			_, writeErr := dst.Write(body)
			if writeErr != nil {
				return writeErr
			}
			return err
		}
		_, err = fmt.Fprintln(dst, verdictMarker(argv, body, digest, shortestOutputHandle(r.store, digest)))
		return err
	}

	res, err := reduce.Reduce(r.store, body, reduce.Config{TelemetryHintsOnly: telemetryOnly})
	if err != nil {
		_, writeErr := dst.Write(body)
		if writeErr != nil {
			return writeErr
		}
		return err
	}
	_, err = io.WriteString(dst, res.Text)
	return err
}

// prepare is the shared Stage 0 → Stage 0.1 → Stage 1 boundary. Canonicalize
// before redaction and before a reducer can compare or persist output. Stage 1
// remains explicit opt-in; the redaction safety gate and the closed telemetry
// deduper remain active without enabling arbitrary head elision.
func (r *shellOutputReducer) prepare(full []byte) []byte {
	body := reduce.CanonicalizeHome(full, r.home)
	return r.redactor.Redact(body)
}

func verdictMarker(argv []string, body []byte, digest, handle string) string {
	name := "command"
	if len(argv) > 0 {
		name = argv[0]
		if len(argv) > 1 {
			name += " " + argv[1]
		}
	}
	hexsum := strings.TrimPrefix(digest, "sha256:")
	short := hexsum
	if len(short) > 4 {
		short = short[:4]
	}
	return fmt.Sprintf("[bashy: %s succeeded · %d lines / %d B elided · sha256:%s… · full: bashy out %s · keep: BASHY_OUTPUT_REDUCE=off]",
		name, outputLineCount(body), len(body), short, handle)
}

func outputLineCount(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := bytes.Count(b, []byte{'\n'})
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// shortestOutputHandle mirrors the store's collision rule for the A2 verdict
// marker. The A1 reducer exposes its chosen handle in Result; A2 deliberately
// spills without retaining any head bytes, so it selects the same safe prefix
// from the public store root.
func shortestOutputHandle(store *reduce.Store, digest string) string {
	hexsum := strings.TrimPrefix(digest, "sha256:")
	n := min(reduce.MinHandleLen, len(hexsum))
	entries, err := os.ReadDir(store.Root())
	if err != nil {
		return hexsum[:n]
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Name()) == 64 && entry.Name() != hexsum {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for n < len(hexsum) {
		clash := false
		for _, name := range names {
			if strings.HasPrefix(name, hexsum[:n]) {
				clash = true
				break
			}
		}
		if !clash {
			break
		}
		n++
	}
	return hexsum[:n]
}

// outSchemaVersion is the envelope `bashy out --json HANDLE` (one recovered
// artifact with its digest, size and content) and `bashy out --list [--json]`
// (the artifacts the spill store holds) emit. Plain `bashy out HANDLE` stays
// raw bytes so it composes with a pipe.
const outSchemaVersion = "bashy-out-v1"

type outArtifact struct {
	Digest   string `json:"digest"`
	Handle   string `json:"handle,omitempty"`
	Bytes    int64  `json:"bytes"`
	Lines    int    `json:"lines,omitempty"`
	Modified string `json:"modified,omitempty"`
	Content  string `json:"content,omitempty"`
}

type outEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	Root          string         `json:"root"`
	Handle        string         `json:"handle,omitempty"`
	Digest        string         `json:"digest,omitempty"`
	Bytes         int64          `json:"bytes,omitempty"`
	Lines         int            `json:"lines,omitempty"`
	Content       string         `json:"content,omitempty"`
	Artifacts     *[]outArtifact `json:"artifacts,omitempty"`
}

func dispatchOut(args []string) int {
	var list, asJSON bool
	rest := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--list":
			list = true
		case "--json":
			asJSON = true
		default:
			rest = append(rest, a)
		}
	}
	if list || asJSON {
		return dispatchOutEnvelope(os.Stdout, rest, list, asJSON)
	}
	cmd := reduce.NewOutCmd(shellOutputStore)
	cmd.SetArgs(args)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy out:", err)
		return 1
	}
	return 0
}

// dispatchOutEnvelope serves the structured forms of `bashy out`.
func dispatchOutEnvelope(w io.Writer, args []string, list, asJSON bool) int {
	store, err := shellOutputStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy out:", err)
		return 1
	}
	env := outEnvelope{SchemaVersion: outSchemaVersion, Root: store.Root()}
	switch {
	case list && len(args) != 0:
		fmt.Fprintln(os.Stderr, "bashy out: --list takes no handle")
		return 2
	case list:
		artifacts := []outArtifact{}
		env.Artifacts = &artifacts
		entries, err := os.ReadDir(store.Root())
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "bashy out:", err)
			return 1
		}
		for _, entry := range entries {
			name := entry.Name()
			if len(name) != 64 || entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			artifacts = append(artifacts, outArtifact{Digest: "sha256:" + name, Handle: shortestOutputHandle(store, name),
				Bytes: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339)})
		}
		if !asJSON {
			for _, a := range artifacts {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", a.Handle, a.Bytes, a.Modified, a.Digest)
			}
			return 0
		}
	case len(args) != 1:
		fmt.Fprintln(os.Stderr, "bashy out: --json takes exactly one handle")
		return 2
	default:
		content, digest, err := store.Get(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy out:", err)
			return 1
		}
		env.Handle, env.Digest, env.Bytes, env.Lines, env.Content = args[0], digest, int64(len(content)), outputLineCount(content), string(content)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		fmt.Fprintln(os.Stderr, "bashy out:", err)
		return 1
	}
	return 0
}

// dispatchFull runs one argv through Bashy's own shell with reduction disabled,
// preserving the in-process userland instead of accidentally selecting a
// different host command from PATH.
func dispatchFull(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: bashy full -- command [args...]")
		return 2
	}
	cmdArgs := append([]string{"-c", `command "$@"`, "bashy full"}, args...)
	cmd := exec.Command(bashySelfPath(), cmdArgs...)
	cmd.Env = setEnv(os.Environ(), "BASHY_OUTPUT_REDUCE", "off")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "bashy full:", err)
		return 127
	}
	_ = cmd.Wait()
	status, _ := procStatus(cmd.ProcessState)
	return status
}
