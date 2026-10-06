package agentos

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/bashy/internal/cli"
	yokemcp "github.com/qiangli/yoke/mcp"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

const defaultMCPMaxOutput = 65536

type shellOpenInput struct {
	Dir string            `json:"dir,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}
type shellOpenOutput struct {
	SessionID string `json:"session_id"`
}
type shellExecInput struct {
	SessionID string `json:"session_id"`
	Script    string `json:"script"`
	Stdin     string `json:"stdin,omitempty"`
}
type shellExecOutput struct {
	// Each stream is either inline text or a shellOutputFile.
	Stdout   any    `json:"stdout"`
	Stderr   any    `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Cwd      string `json:"cwd"`
}
type shellOutputFile struct {
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	Preview string `json:"preview"`
}
type shellCloseInput struct {
	SessionID string `json:"session_id"`
}
type shellCloseOutput struct {
	Closed bool `json:"closed"`
}

type mcpShell struct {
	mu     sync.Mutex
	runner *interp.Runner
	ctx    context.Context
	cancel context.CancelFunc
	dir    string
	closed bool
}
type mcpShells struct {
	mu        sync.Mutex
	ctx       context.Context
	sessions  map[string]*mcpShell
	maxOutput int
	done      chan struct{}
}

func registerMCPShells(ctx context.Context, srv *mcpsdk.Server, maxOutput int) *mcpShells {
	for _, name := range []string{"shell_open", "shell_exec", "shell_close"} {
		yokemcp.DeclareSyntheticEffects(srv, name, []string{"exec"})
	}
	m := &mcpShells{ctx: ctx, sessions: make(map[string]*mcpShell), maxOutput: maxOutput, done: make(chan struct{})}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shell_open", Description: "Open an isolated persistent bashy shell."}, m.open)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shell_exec", Description: "Execute a script in a persistent shell. Effect: exec. Large streams return {path, bytes, preview}; files live until shell_close."}, m.exec)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{Name: "shell_close", Description: "Close a shell and remove its output files."}, m.close)
	go func() {
		defer close(m.done)
		<-ctx.Done()
		m.mu.Lock()
		sessions := m.sessions
		m.sessions = make(map[string]*mcpShell)
		m.mu.Unlock()
		for _, s := range sessions {
			s.cancel()
		}
		for _, s := range sessions {
			s.dispose()
		}
	}()
	return m
}

func (s *mcpShell) dispose() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	// Reset releases interpreter-owned directory state; the runner is discarded.
	s.runner.Reset()
	_ = os.RemoveAll(s.dir)
}

func (m *mcpShells) open(ctx context.Context, _ *mcpsdk.CallToolRequest, in shellOpenInput) (*mcpsdk.CallToolResult, shellOpenOutput, error) {
	var out shellOpenOutput
	if err := ctx.Err(); err != nil {
		return nil, out, err
	}
	env := make([]string, 0, len(os.Environ())+len(in.Env))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := in.Env[key]; !ok {
			env = append(env, entry)
		}
	}
	for k, v := range in.Env {
		if k == "" || strings.ContainsAny(k, "=\x00") || strings.ContainsRune(v, 0) {
			return nil, out, fmt.Errorf("invalid environment key or value")
		}
		env = append(env, k+"="+v)
	}
	r, err := cli.NewSessionRunnerWithConfig(cli.SessionIO{Dir: in.Dir, Env: env, Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard}, cli.SessionConfig{WireExec: WireSessionExec(false), Preamble: func() string { return PreambleFor(false) }, ExecProcessGroups: true})
	if err != nil {
		return nil, out, err
	}
	dir, err := os.MkdirTemp("", "bashy-mcp-")
	if err != nil {
		return nil, out, err
	}
	lifetime, cancel := context.WithCancel(m.ctx)
	s := &mcpShell{runner: r, ctx: lifetime, cancel: cancel, dir: dir}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err = m.ctx.Err(); err != nil {
		s.dispose()
		return nil, out, err
	}
	for {
		var id [8]byte
		if _, err = rand.Read(id[:]); err != nil {
			s.dispose()
			return nil, out, err
		}
		out.SessionID = hex.EncodeToString(id[:])
		if m.sessions[out.SessionID] == nil {
			break
		}
	}
	m.sessions[out.SessionID] = s
	return nil, out, nil
}

func noMCPSession(id string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{IsError: true, Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "no such session: " + id}}}
}

func (m *mcpShells) exec(ctx context.Context, _ *mcpsdk.CallToolRequest, in shellExecInput) (*mcpsdk.CallToolResult, shellExecOutput, error) {
	m.mu.Lock()
	s := m.sessions[in.SessionID]
	m.mu.Unlock()
	out := shellExecOutput{Stdout: "", Stderr: ""}
	if s == nil {
		return noMCPSession(in.SessionID), out, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return noMCPSession(in.SessionID), out, nil
	}
	runCtx, cancel := context.WithCancel(s.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	defer cancel()
	stdout := &mcpOutput{dir: s.dir, limit: m.maxOutput}
	stderr := &mcpOutput{dir: s.dir, limit: m.maxOutput}
	defer stdout.close()
	defer stderr.close()
	if err := interp.StdIO(strings.NewReader(in.Stdin), stdout, stderr)(s.runner); err != nil {
		return nil, out, err
	}
	_ = interp.WithBashSource([]byte(in.Script))(s.runner)
	node, err := syntax.NewParser(syntax.Variant(s.runner.LangVariant())).Parse(strings.NewReader(in.Script), "mcp")
	if err != nil {
		out.ExitCode = 2
		fmt.Fprintln(stderr, err)
	} else {
		err = s.runner.Run(runCtx, node)
		var status interp.ExitStatus
		if errors.As(err, &status) {
			out.ExitCode = int(status)
		} else if err != nil {
			out.ExitCode = 1
			fmt.Fprintln(stderr, err)
		}
	}
	out.Cwd = s.runner.Dir
	if out.Stdout, err = stdout.result(); err != nil {
		return nil, out, err
	}
	if out.Stderr, err = stderr.result(); err != nil {
		return nil, out, err
	}
	return nil, out, nil
}

func (m *mcpShells) close(_ context.Context, _ *mcpsdk.CallToolRequest, in shellCloseInput) (*mcpsdk.CallToolResult, shellCloseOutput, error) {
	m.mu.Lock()
	s := m.sessions[in.SessionID]
	delete(m.sessions, in.SessionID)
	m.mu.Unlock()
	if s == nil {
		return noMCPSession(in.SessionID), shellCloseOutput{}, nil
	}
	s.dispose()
	return nil, shellCloseOutput{Closed: true}, nil
}

// Spill while writing, rather than buffering arbitrarily large streams in RAM.
// The mutex also covers concurrent writers in shell pipelines.
type mcpOutput struct {
	mu      sync.Mutex
	dir     string
	limit   int
	buffer  bytes.Buffer
	preview []byte
	file    *os.File
	size    int64
	err     error
}

func (w *mcpOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	if n := 2048 - len(w.preview); n > 0 {
		w.preview = append(w.preview, p[:min(n, len(p))]...)
	}
	if w.file == nil && w.size+int64(len(p)) > int64(w.limit) {
		w.file, w.err = os.CreateTemp(w.dir, "output-")
		if w.err != nil {
			return 0, w.err
		}
		_, w.err = w.file.Write(w.buffer.Bytes())
		w.buffer.Reset()
		if w.err != nil {
			return 0, w.err
		}
	}
	var n int
	if w.file != nil {
		n, w.err = w.file.Write(p)
	} else {
		n, w.err = w.buffer.Write(p)
	}
	w.size += int64(n)
	return n, w.err
}
func (w *mcpOutput) result() (any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return nil, w.err
	}
	if w.file == nil {
		return w.buffer.String(), nil
	}
	return shellOutputFile{Path: w.file.Name(), Bytes: w.size, Preview: string(w.preview)}, nil
}
func (w *mcpOutput) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		_ = w.file.Close()
	}
}

// Keep CLI integration small: consume only this feature's flag, leaving all
// existing transport/allow parsing in dispatchMCP.
func mcpOutputArgs(args []string) ([]string, int, error) {
	maxOutput := defaultMCPMaxOutput
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		name, value, eq := strings.Cut(args[i], "=")
		if name != "--max-output" {
			rest = append(rest, args[i])
			continue
		}
		if !eq && i+1 < len(args) {
			i++
			value = args[i]
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return nil, 0, fmt.Errorf("--max-output requires a nonnegative byte count")
		}
		maxOutput = n
	}
	return rest, maxOutput, nil
}

func serveMCPShells(ctx context.Context, name, version string, maxOutput int) error {
	ctx, cancel := context.WithCancel(ctx)
	srv := yokemcp.NewServerWithOptions(name, version, yokemcp.Options{})
	sessions := registerMCPShells(ctx, srv, maxOutput)
	defer func() { cancel(); <-sessions.done }()
	err := srv.Run(ctx, &mcpsdk.StdioTransport{})
	if errors.Is(err, io.EOF) || (err != nil && strings.Contains(err.Error(), "server is closing")) {
		return nil
	}
	return err
}
