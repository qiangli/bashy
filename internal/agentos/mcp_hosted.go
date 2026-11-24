// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/coreutils/tool"
	dbcmd "github.com/qiangli/yoke/cmds/db"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/coopauth"
	"github.com/qiangli/yoke/pkg/kbmemory"
	"github.com/qiangli/yoke/pkg/policy/audit"
	"github.com/qiangli/yoke/pkg/secrets"
)

// The hosted profile (`bashy mcp serve --transport http --hosted`) serves a
// caller who has no host of their own. Nothing of a call outlives it: every
// HTTP request carries a master-signed per-call identity, every executing
// tools/call runs as a child process in a scratch home made for that call and
// removed after it, and there are no shell sessions.
//
// The identity travels in one header:
//
//	X-Bashy-Hosted-Identity: v1.<payload>.<signature>
//
// payload is the unpadded base64url JSON of mcpHostedIdentity; signature is
// the unpadded base64url HMAC-SHA256 of "v1.<payload>" under the runner key.
const (
	mcpHostedHeader        = "X-Bashy-Hosted-Identity"
	mcpHostedTokenVersion  = "v1"
	mcpHostedSchema        = "bashy-hosted-identity-v1"
	mcpHostedMaxTTL        = 5 * time.Minute
	mcpHostedSkew          = 30 * time.Second
	mcpHostedMinKey        = 16
	mcpHostedKeyEnv        = "BASHY_HOSTED_KEY_FILE"
	mcpHostedScratchEnv    = "BASHY_HOSTED_SCRATCH_DIR"
	defaultMCPHostedTimout = 60 * time.Second
	mcpHostedTimeoutExit   = 124
)

// mcpHostedIdentity is the signed per-call payload. Times are Unix seconds.
type mcpHostedIdentity struct {
	Schema         string `json:"v"`
	Caller         string `json:"caller"`
	TaskID         string `json:"task_id"`
	IdempotencyKey string `json:"idempotency_key"`
	SecretsToken   string `json:"secrets_token,omitempty"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
}

// mcpHostedKeyPath is the runner key: $BASHY_HOSTED_KEY_FILE, else
// bashy/hosted-runner.key under the user configuration directory.
func mcpHostedKeyPath() string {
	if path := strings.TrimSpace(os.Getenv(mcpHostedKeyEnv)); path != "" {
		return path
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "bashy", "hosted-runner.key")
}

// Re-read the key on every request, as --outpost does: the master may
// provision or rotate it without restarting the runner. Only a private
// regular file is accepted, and the key is never taken from the command line.
func readMCPHostedKey(path string) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > 4096 || (runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0) {
		return nil, false
	}
	data := make([]byte, st.Size())
	if n, err := f.Read(data); err != nil || int64(n) != st.Size() {
		return nil, false
	}
	key := bytes.TrimSpace(data)
	if len(key) < mcpHostedMinKey {
		return nil, false
	}
	return key, true
}

func mcpHostedSign(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(mcpHostedTokenVersion + "." + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// mcpHostedVerify accepts only a current, short-lived, fully named identity
// signed with the runner key. Every failure is the same refusal.
func mcpHostedVerify(token string, key []byte, now time.Time) (mcpHostedIdentity, bool) {
	var id mcpHostedIdentity
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != mcpHostedTokenVersion || len(key) < mcpHostedMinKey {
		return id, false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(mcpHostedSign(key, parts[1]))) {
		return id, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return id, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&id) != nil || id.Schema != mcpHostedSchema {
		return mcpHostedIdentity{}, false
	}
	for _, field := range []string{id.Caller, id.TaskID, id.IdempotencyKey} {
		if strings.TrimSpace(field) == "" || strings.ContainsAny(field, "\x00\r\n") {
			return mcpHostedIdentity{}, false
		}
	}
	issued, expires := time.Unix(id.IssuedAt, 0), time.Unix(id.ExpiresAt, 0)
	if !now.Before(expires) || issued.After(now.Add(mcpHostedSkew)) || !expires.After(issued) || expires.Sub(issued) > mcpHostedMaxTTL {
		return mcpHostedIdentity{}, false
	}
	return id, true
}

// mcpHostedNonces remembers the idempotency keys that already executed here.
// An entry outlives every identity that could still name its key, so a
// captured request cannot run a second time. The master keeps the durable
// record; this cache only covers the life of an identity on this runner.
type mcpHostedNonces struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (n *mcpHostedNonces) claim(id mcpHostedIdentity, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.seen == nil {
		n.seen = make(map[string]time.Time)
	}
	for key, until := range n.seen {
		if !now.Before(until) {
			delete(n.seen, key)
		}
	}
	if _, used := n.seen[id.IdempotencyKey]; used {
		return false
	}
	n.seen[id.IdempotencyKey] = time.Unix(id.ExpiresAt, 0).Add(mcpHostedMaxTTL + mcpHostedSkew)
	return true
}

// mcpHosted is one hosted runner: the options every per-request server is
// built from, the limits each call runs under and the replay cache.
type mcpHosted struct {
	opts      yokemcp.Options
	version   string
	keyPath   string
	scratch   string
	self      string
	timeout   time.Duration
	maxOutput int
	nonces    mcpHostedNonces
	writer    *audit.Writer
	actor     audit.Actor
	host      string
	now       func() time.Time
	// childEnv is prepended to every call's environment; tests use it to
	// make the test binary stand in for the bashy executable.
	childEnv []string
}

// mcpHostedInProcess are the direct tools that run inside the server process
// and read its own home. A hosted call may not see the runner's state, so the
// profile does not serve them.
func mcpHostedInProcess() []string {
	return []string{dbcmd.MCPCommand().Name, kbmemory.MCPCommand().Name, kbmemory.MCPRetrieveCommand().Name,
		kbmemory.MCPSyncIndexCommand().Name, kbmemory.MCPMaintainCommand().Name}
}

func newMCPHosted(allow, profile, version string, timeout time.Duration, maxOutput int) (*mcpHosted, error) {
	opts, err := mcpOptions(allow, profile)
	if err != nil {
		return nil, err
	}
	registered, inProcess := opts.Registered, mcpHostedInProcess()
	opts.Registered = func() []yokemcp.RegisteredCommand {
		return slices.DeleteFunc(registered(), func(command yokemcp.RegisteredCommand) bool {
			return slices.Contains(inProcess, command.Name)
		})
	}
	keyPath := mcpHostedKeyPath()
	if keyPath == "" {
		return nil, errors.New("--hosted: no runner key path; set " + mcpHostedKeyEnv)
	}
	if keyPath, err = filepath.Abs(keyPath); err != nil {
		return nil, err
	}
	scratch := strings.TrimSpace(os.Getenv(mcpHostedScratchEnv))
	if scratch == "" {
		scratch = os.TempDir()
	}
	if scratch, err = filepath.Abs(scratch); err != nil {
		return nil, err
	}
	return &mcpHosted{opts: opts, version: version, keyPath: keyPath, scratch: scratch, self: bashySelfPath(),
		timeout: timeout, maxOutput: maxOutput, writer: newAuditWriter(), actor: auditActor(), host: auditHost(), now: time.Now}, nil
}

// identity resolves the caller of one HTTP request, failing closed.
func (h *mcpHosted) identity(r *http.Request) (mcpHostedIdentity, bool) {
	if !coopauth.IsLoopbackAddr(r.RemoteAddr) {
		return mcpHostedIdentity{}, false
	}
	values := r.Header.Values(mcpHostedHeader)
	if len(values) != 1 {
		return mcpHostedIdentity{}, false
	}
	key, ok := readMCPHostedKey(h.keyPath)
	if !ok {
		return mcpHostedIdentity{}, false
	}
	return mcpHostedVerify(strings.TrimSpace(values[0]), key, h.now())
}

// ServeHTTP builds one server per request, bound to the verified caller: the
// policy, the audit line and the scratch home of a call cannot belong to any
// other request because no other request can reach this server.
func (h *mcpHosted) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := h.identity(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	opts := h.opts
	policy := yokemcp.Policy{Audit: h.audit(id), Execute: (&mcpHostedCall{runner: h, id: id}).execute}
	if h.opts.Policy != nil {
		policy.Allow = h.opts.Policy.Allow
	}
	opts.Policy = &policy
	srv, err := yokemcp.BuildServer("bashy", h.version, opts)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return srv }, &mcpsdk.StreamableHTTPOptions{Stateless: true}).ServeHTTP(w, r)
}

// mcpHostedAuditLine is the stderr record when the audit log is off.
type mcpHostedAuditLine struct {
	Principal string `json:"principal"`
	TaskID    string `json:"task_id"`
	yokemcp.Record
}

// audit writes the call into the same hash-chained log as every other MCP
// call, with the verified caller as the accountable principal (the field
// BASHY_PRINCIPAL fills for a local shell) and the task as the session.
func (h *mcpHosted) audit(id mcpHostedIdentity) func(yokemcp.Record) {
	return func(r yokemcp.Record) {
		if h.writer == nil {
			data, _ := json.Marshal(mcpHostedAuditLine{Principal: id.Caller, TaskID: id.TaskID, Record: r})
			_, _ = os.Stderr.Write(append(data, '\n'))
			return
		}
		actor := h.actor
		actor.Human, actor.Session = id.Caller, id.TaskID
		decision := "deny"
		if r.Allowed {
			decision = "allow"
		}
		_, _ = h.writer.Append(audit.Record{Time: r.Time, Actor: actor, Host: h.host,
			Action: "mcp:" + r.Tool, Binary: r.Command, Argv: []string{r.Command},
			Effects: r.Effects, Decision: decision, Exit: r.ExitCode, DurationMs: r.Duration.Milliseconds()})
	}
}

// mcpHostedCall is one request's caller. Its scratch tree exists only while a
// tools/call executes.
type mcpHostedCall struct {
	runner *mcpHosted
	id     mcpHostedIdentity
	root   string
}

type mcpHostedCallKey struct{}

// mcpHostedFrom reports the hosted call a runner is executing for, or nil on
// every other profile.
func mcpHostedFrom(ctx context.Context) *mcpHostedCall {
	call, _ := ctx.Value(mcpHostedCallKey{}).(*mcpHostedCall)
	return call
}

// mcpHostedState is the structured refusal of a call that will not execute.
type mcpHostedState struct {
	State  string `json:"state"`
	TaskID string `json:"task_id"`
}

func mcpHostedRefusal(state, taskID, text string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError:           true,
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
		StructuredContent: mcpHostedState{State: state, TaskID: taskID},
	}
}

// execute is the hosted Policy.Execute hook. It runs after the policy, so a
// gated call has already answered {state: "gated"} without spending its
// identity. An executing call claims its idempotency key, gets a scratch home
// and a deadline, and leaves nothing behind.
func (c *mcpHostedCall) execute(ctx context.Context, call *mcpsdk.CallToolRequest, next func(context.Context) (mcpsdk.Result, error)) (mcpsdk.Result, error) {
	h := c.runner
	if !h.nonces.claim(c.id, h.now()) {
		return mcpHostedRefusal("replayed", c.id.TaskID, "refused: this identity already executed; not executed again"), nil
	}
	root, err := os.MkdirTemp(h.scratch, "bashy-hosted-")
	if err != nil {
		return mcpHostedRefusal("refused", c.id.TaskID, "refused: no scratch directory"), nil
	}
	defer os.RemoveAll(root)
	run := &mcpHostedCall{runner: h, id: c.id, root: root}
	for _, dir := range run.dirs() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return mcpHostedRefusal("refused", c.id.TaskID, "refused: no scratch directory"), nil
		}
	}
	ctx, cancel := context.WithTimeout(context.WithValue(ctx, mcpHostedCallKey{}, run), h.timeout)
	defer cancel()
	// run_tool and the direct registry tools execute inside the server process
	// in every other profile. Here they run as a child of the same binary, so
	// the scratch home, the deadline and the output cap hold for them too.
	name := call.Params.Name
	if name != "run_tool" && tool.Lookup(name) == nil {
		return next(ctx)
	}
	var in yokemcp.RunToolInput
	if len(call.Params.Arguments) > 0 {
		if err := json.Unmarshal(call.Params.Arguments, &in); err != nil {
			return mcpHostedOutput("", "invalid arguments: "+err.Error()+"\n", 2), nil
		}
	}
	if name != "run_tool" {
		in.Name = name
	}
	if strings.TrimSpace(in.Name) == "" || strings.HasPrefix(in.Name, "-") {
		return mcpHostedOutput("", "run_tool: name is required\n", 2), nil
	}
	stdout, stderr, exit := run.exec(ctx, append([]string{h.self, in.Name}, in.Args...), in.Stdin, in.Dir, in.Env, false)
	return mcpHostedOutput(stdout, stderr, exit), nil
}

func mcpHostedOutput(stdout, stderr string, exit int) *mcpsdk.CallToolResult {
	out := yokemcp.RunToolOutput{Stdout: stdout, Stderr: stderr, ExitCode: exit}
	data, _ := json.Marshal(out)
	return &mcpsdk.CallToolResult{IsError: exit != 0, Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}, StructuredContent: out}
}

func (c *mcpHostedCall) home() string { return filepath.Join(c.root, "home") }
func (c *mcpHostedCall) work() string { return filepath.Join(c.root, "work") }
func (c *mcpHostedCall) tmp() string  { return filepath.Join(c.root, "tmp") }
func (c *mcpHostedCall) dirs() []string {
	return []string{c.home(), filepath.Join(c.home(), ".bashy"), c.work(), c.tmp(), c.googleRoot()}
}

func (c *mcpHostedCall) googleRoot() string {
	return filepath.Join(c.home(), ".config", "bashy", "google")
}

// dir keeps a call's working directory inside its scratch work tree.
func (c *mcpHostedCall) dir(dir string) (string, error) {
	if dir == "" {
		return c.work(), nil
	}
	if filepath.IsAbs(dir) || !filepath.IsLocal(dir) {
		return "", fmt.Errorf("dir %q is outside the call's scratch directory", dir)
	}
	return filepath.Join(c.work(), dir), nil
}

// mcpHostedPassEnv are the only variables a call inherits from the runner.
// Everything that names a home, a credential or a session is set per call.
var mcpHostedPassEnv = []string{"PATH", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "SystemRoot", "SYSTEMROOT", "ComSpec", "PATHEXT", "windir"}

// env is the whole environment of a call's process: the runner's inherited
// basics, then what the client asked for, then the call's own identity and
// scratch paths, which a client cannot override.
func (c *mcpHostedCall) env(extra map[string]string) ([]string, error) {
	env := slices.Clone(c.runner.childEnv)
	for _, key := range mcpHostedPassEnv {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	keys := make([]string, 0, len(extra))
	for key, value := range extra {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return nil, errors.New("invalid environment key or value")
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		env = append(env, key+"="+extra[key])
	}
	home := c.home()
	own := []string{
		"HOME=" + home,
		"BASHY_HOME=" + filepath.Join(home, ".bashy"),
		"TMPDIR=" + c.tmp(),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"BASHY_PRINCIPAL=" + c.id.Caller,
		"BASHY_SESSION=" + c.id.TaskID,
		"BASHY_HOSTED=1",
		"BASHY_HOSTED_TASK_ID=" + c.id.TaskID,
		"BASHY_HOSTED_IDEMPOTENCY_KEY=" + c.id.IdempotencyKey,
		"BASHY_HOSTED_SCRATCH=" + c.root,
		// The runner's registered commands stay callable, read-only; the
		// call's own command store is the empty one in its scratch home.
		"BASHY_COMMANDS_PATH=" + strings.Join(registeredCatalog().CommandDirs(), string(os.PathListSeparator)),
		"GWS_CONFIG_ROOT=" + c.googleRoot(),
		"BASHY_HOSTED_GOOGLE_NAMES=",
		// Credentials are projected in the runner using the signed token.
		"BASHY_SECRETS_TOKEN=",
	}
	if runtime.GOOS == "windows" {
		own = append(own, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"),
			"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"), "TEMP="+c.tmp(), "TMP="+c.tmp())
	}
	return append(env, own...), nil
}

func (c *mcpHostedCall) runChild(ctx context.Context, cmd *exec.Cmd, vaultAware bool) error {
	token := strings.TrimSpace(c.id.SecretsToken)
	if token == "" {
		return cmd.Run()
	}
	client, err := (secrets.Config{Token: token}).ResolveContext(ctx)
	if err != nil {
		return errors.New("vault credentials: could not configure the caller's vault")
	}
	items, err := client.ListContext(ctx)
	if err != nil {
		return errors.New("vault credentials: could not list the caller's vault")
	}
	bindings, googleFiles := secrets.CredentialBindings(items)
	cmd.Env = append(cmd.Env, "BASHY_HOSTED_GOOGLE_NAMES="+strings.Join(googleFiles, "\n"))
	if vaultAware {
		cmd.Env = append(cmd.Env, "BASHY_CLOUDBOX_URL="+client.BaseURL, "BASHY_SECRETS_TOKEN="+token)
	}
	// Use the same no-cache execution path as secret exec. The runner
	// owns the child's deadline and exit status.
	err = client.RunProjected(ctx, cmd, c.googleRoot(), bindings)
	if err == nil || cmd.ProcessState != nil || cmd.Err != nil {
		return err
	}
	var startErr *os.PathError
	if (errors.As(err, &startErr) && startErr.Op == "fork/exec") || strings.HasPrefix(err.Error(), "no such secret: ") {
		return err
	}
	return errors.New("vault credentials: could not project the caller's credentials")
}

// mcpHostedCapture keeps the first limit bytes of a stream and counts the rest.
type mcpHostedCapture struct {
	mu      sync.Mutex
	limit   int
	buffer  bytes.Buffer
	dropped int64
}

func (w *mcpHostedCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	keep := min(max(w.limit-w.buffer.Len(), 0), len(p))
	w.buffer.Write(p[:keep])
	w.dropped += int64(len(p) - keep)
	return len(p), nil
}

// exec runs one command of the call as a child process and waits for it. The
// deadline kills the child's whole process group and answers exit 124.
func (c *mcpHostedCall) exec(ctx context.Context, argv []string, stdin, dir string, extra map[string]string, vaultAware bool) (string, string, int) {
	h := c.runner
	if c.root == "" || len(argv) == 0 || argv[0] == "" {
		return "", "hosted call has no scratch directory\n", 126
	}
	dir, err := c.dir(dir)
	if err != nil {
		return "", err.Error() + "\n", 2
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "dir: cannot create the working directory\n", 2
	}
	env, err := c.env(extra)
	if err != nil {
		return "", err.Error() + "\n", 2
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env, cmd.Stdin = dir, env, strings.NewReader(stdin)
	stdout, stderr := &mcpHostedCapture{limit: h.maxOutput}, &mcpHostedCapture{limit: h.maxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = 2 * time.Second
	mcpHostedContain(cmd)
	runErr := c.runChild(ctx, cmd, vaultAware)
	exit, note := 0, ""
	var startErr *os.PathError
	switch {
	case ctx.Err() != nil:
		exit = mcpHostedTimeoutExit
		note = fmt.Sprintf("bashy mcp: call stopped after %s\n", h.timeout)
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			note = "bashy mcp: call cancelled\n"
		}
	case cmd.Err != nil || (errors.As(runErr, &startErr) && startErr.Op == "fork/exec"):
		exit, note = 126, "bashy mcp: cannot start the command\n"
	case runErr != nil && cmd.ProcessState == nil:
		exit, note = 1, runErr.Error()+"\n"
	case runErr != nil:
		exit = cmd.ProcessState.ExitCode()
		if exit <= 0 {
			exit = 1
		}
	}
	for _, stream := range []struct {
		name string
		w    *mcpHostedCapture
	}{{"stdout", stdout}, {"stderr", stderr}} {
		if stream.w.dropped > 0 {
			note += fmt.Sprintf("bashy mcp: %s truncated at %d bytes (%d dropped)\n", stream.name, h.maxOutput, stream.w.dropped)
		}
	}
	return stdout.buffer.String(), stderr.buffer.String() + note, exit
}

// runMCPHosted serves the hosted profile on a loopback address until ctx ends.
func runMCPHosted(ctx context.Context, h *mcpHosted, listen string) error {
	if listen == "" {
		listen = "127.0.0.1:0"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return err
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("--hosted: refusing a non-loopback listen address")
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serving := make(chan error, 1)
	go func() { serving <- srv.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "listening on http://%s/mcp\n", listener.Addr())
	select {
	case err = <-serving:
	case <-ctx.Done():
		grace, stop := context.WithTimeout(context.Background(), 5*time.Second)
		err = srv.Shutdown(grace)
		stop()
		if err != nil {
			_ = srv.Close()
		}
		<-serving
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
