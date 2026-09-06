// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/reduce"
	"github.com/qiangli/coreutils/pkg/secrets"
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
	return &shellOutputReducer{
		store: store, redactor: shellOutputRedactor(env),
		home: outputEnv(env, "HOME"), stage1: outputReductionEnabled(env),
		outDst: out, errDst: errOut,
	}, nil
}

func (r *shellOutputReducer) stdout() io.Writer { return shellCaptureWriter{r: r} }
func (r *shellOutputReducer) stderr() io.Writer { return shellCaptureWriter{r: r, err: true} }

func (r *shellOutputReducer) middleware(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, argv []string) error {
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
		// of "failure is never compressed below its evidence" is to keep it all.
		// It still passes through the same redaction gate as successful output.
		if _, err := r.outDst.Write(r.prepare(out)); err != nil {
			return err
		}
		_, err := r.errDst.Write(r.prepare(errOut))
		return err
	}
	if err := r.emit(r.outDst, argv, out, verdict); err != nil {
		return err
	}
	return r.emit(r.errDst, argv, errOut, verdict)
}

func (r *shellOutputReducer) emit(dst io.Writer, argv []string, full []byte, verdict bool) error {
	if len(full) == 0 {
		return nil
	}
	body := r.prepare(full)
	if !r.stage1 {
		_, err := dst.Write(body)
		return err
	}
	if verdict {
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

	res, err := reduce.Reduce(r.store, body, reduce.Config{})
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

// prepare is the shared Stage 0 → Stage 1 boundary.  Stage 0 is deliberately
// always active in bashy: canonicalize before redaction and before a reducer
// can persist output.  Stage 1 remains explicit opt-in, so ordinary output
// receives no other transformation.
func (r *shellOutputReducer) prepare(full []byte) []byte {
	body := reduce.CanonicalizeHome(full, r.home)
	if r.stage1 {
		return r.redactor.Redact(body)
	}
	return body
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

func dispatchOut(args []string) int {
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
