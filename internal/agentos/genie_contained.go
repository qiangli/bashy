// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"

	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/qiangli/yoke/pkg/fleet"
)

type genieSession struct {
	Label     string
	Token     string
	Socket    string
	Models    []string
	ExpiresAt string `json:"expires_at"`
}

type containedGenieRecord struct {
	Label       string   `json:"label"`
	Selector    string   `json:"selector"`
	Binding     string   `json:"binding"`
	Band        string   `json:"band"`
	Models      []string `json:"models"`
	ExpiresAt   string   `json:"expires_at"`
	MaxRequests int      `json:"max_requests"`
	Sandbox     string   `json:"sandbox"`
	Dir         string   `json:"dir"`
	Exit        int      `json:"exit"`
}

func dispatchContainedGenie(args []string) int {
	flags := flag.NewFlagSet("bashy genie --contained", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stdout, genieUsage) }
	model := flags.String("m", "L3", "binding or band L3, L4 or L5")
	dir := flags.String("dir", ".", "writable working directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	task := strings.Join(flags.Args(), " ")
	if strings.TrimSpace(task) == "" || *dir == "" || *model == "" || strings.Contains(*model, ",") {
		fmt.Fprintln(os.Stderr, "bashy genie --contained: supply a task, a working directory and one binding or band")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code, err := runContainedGenie(ctx, *model, *dir, task, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy genie --contained:", err)
	}
	return code
}

func genieCommandStatus(ctx context.Context, err error) int {
	if ctx.Err() != nil {
		return 130
	}
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() > 0 {
		return status.ExitCode()
	}
	return 1
}

func runContainedGenie(ctx context.Context, selector, dir, task string, out, diag io.Writer) (code int, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return 1, err
	}
	bundle, err := genieBundle()
	if err != nil {
		return 1, err
	}
	bundle, err = filepath.Abs(bundle)
	if err != nil {
		return 1, err
	}
	if !isBarBundle(bundle) {
		return 1, errors.New("need a genie .bar bundle; run bashy genie build")
	}
	home, err := genieHome()
	if err != nil {
		return 1, err
	}
	runs := filepath.Join(home, "runs")
	if err = os.MkdirAll(runs, 0700); err != nil {
		return 1, err
	}
	rec := containedGenieRecord{Label: uuid.NewString(), Dir: dir, Selector: selector, MaxRequests: 200, Sandbox: "contain --net deny --dir"}
	// Reserve the host record before the workspace becomes writable by genie.
	record, err := os.OpenFile(filepath.Join(runs, rec.Label+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return 1, err
	}
	scope := "--model"
	switch selector {
	case "L3", "L4", "L5":
		scope = "--band"
		rec.Band = selector
	}
	self := bashySelfPath()
	// Open can issue the token before its reply reaches us. Close by our own
	// label even after cancellation, bad JSON, or a lost reply.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		closeCmd := exec.CommandContext(cleanup, self, "llm", "session", "close", rec.Label)
		closeCmd.Stderr = diag
		if e := closeCmd.Run(); e != nil {
			err = errors.Join(err, fmt.Errorf("revoke the run with bashy llm session close %s: %w", rec.Label, e))
			if code == 0 {
				code = 1
			}
		}
	}()
	defer func() {
		rec.Exit = code
		e := json.NewEncoder(record).Encode(rec)
		e = errors.Join(e, record.Close())
		if e != nil {
			err = errors.Join(err, e)
			if code == 0 {
				code = 1
			}
		}
	}()
	open := exec.CommandContext(ctx, self, "llm", "session", "open", scope, selector, "--ttl", "30m", "--max-requests", "200", "--label", rec.Label, "--json")
	open.Stderr = diag
	response, e := open.Output()
	if e != nil {
		return genieCommandStatus(ctx, e), fmt.Errorf("cannot open model session; check the door with bashy llm up and select a served binding or band with -m: %w", e)
	}
	var session genieSession
	if json.Unmarshal(response, &session) != nil || session.Label != rec.Label || !filepath.IsAbs(session.Socket) || session.Token == "" || session.ExpiresAt == "" {
		return 1, errors.New("invalid llm session open reply; check the session door version")
	}
	rec.Models, rec.ExpiresAt = session.Models, session.ExpiresAt
	if len(session.Models) == 0 || slices.Contains(session.Models, "") || (scope == "--model" && !slices.Contains(session.Models, selector)) {
		return 1, fmt.Errorf("model %q outside session scope; select a served binding or band with -m", selector)
	}
	rec.Binding = selector
	if scope == "--band" {
		rec.Binding = session.Models[0]
	} else if agent, _, model, lookupErr := fleet.New().Binding(rec.Binding); lookupErr == nil {
		band := model.Band
		if agent.IsCascade() && agent.Band > 0 {
			band = agent.Band
		}
		rec.Band = fleet.BandLabel(band)
	}
	port, err := genieLoopbackPort()
	if err != nil {
		return 1, fmt.Errorf("select session loopback port: %w", err)
	}
	// The bundle's adapter is Go source bashy interprets, which asks a Go
	// toolchain what the source imports. The sandbox has no network and no
	// cache to provision one in, so the host's provisioned toolchain is
	// mounted read-only and named explicitly (BASHPP_GO).
	goBin, goRoot, err := genieSandboxGo()
	if err != nil {
		return 1, fmt.Errorf("no Go toolchain to give the sandbox; run `bashy go version` once on this host to provision it: %w", err)
	}
	args := []string{
		"contain", "--net", "deny", "--dir", dir, "--socket", session.Socket,
		"--loopback", port, "--ro", bundle, "--ro", goRoot, "--env", "OPENAI_API_KEY", "--env", "BASHPP_GO", "--",
		self, "genie", "--session-launcher", session.Socket, port, bundle, rec.Binding, rec.Label, task,
	}
	cmd := exec.CommandContext(ctx, self, args...)
	cmd.SysProcAttr = ownProcessGroupAttr()
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process) }
	cmd.Env = setEnv(setEnv(os.Environ(), "OPENAI_API_KEY", session.Token), "BASHPP_GO", goBin)
	cmd.Stdout, cmd.Stderr = out, diag
	if e = cmd.Run(); e != nil {
		code = genieCommandStatus(ctx, e)
		if code == 125 {
			return code, errors.New("no enforcing containment backend; use a host with file confinement enabled")
		}
		return code, e
	}
	return 0, nil
}

// genieSandboxGo is the host's provisioned go binary and its GOROOT, symlinks
// resolved so the read-only mount is the real tree.
var genieSandboxGo = func() (bin, root string, err error) {
	argv, _, err := islandToolResolver("go")
	if err != nil {
		return "", "", err
	}
	if len(argv) != 1 {
		return "", "", fmt.Errorf("%q is not a single go binary", argv)
	}
	if bin, err = filepath.EvalSymlinks(argv[0]); err != nil {
		return "", "", err
	}
	return bin, filepath.Dir(filepath.Dir(bin)), nil
}

func genieLoopbackPort() (string, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	return strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), listener.Close()
}

// Serve the scoped Unix socket on precisely the port permitted by contain.
// If another process took that port, refuse before launching the client.
func genieSessionForward(socket, port string) (string, func(), error) {
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != port {
		return "", nil, errors.New("invalid session loopback port")
	}
	addr := net.JoinHostPort("127.0.0.1", port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return "", nil, err
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme = "http"
			r.Out.URL.Host = "localhost"
			r.Out.Host = "localhost"
		},
	}}
	go server.Serve(ln)
	return "http://" + addr + "/v1", func() {
		server.Close()
		transport.CloseIdleConnections()
	}, nil
}

func launchContainedGenie(args []string) int {
	if len(args) != 6 || os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "genie launcher: missing session parameters")
		return 2
	}
	socket, port, bundle, model, label, task := args[0], args[1], args[2], args[3], args[4], args[5]
	self := bashySelfPath()
	endpoint, stop, err := genieSessionForward(socket, port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start session forwarder:", err)
		return 1
	}
	defer stop()
	env := append(os.Environ(),
		"OPENAI_BASE_URL="+endpoint,
		"GENIE_MODEL_ID="+model, "GENIE_EXTERNAL_MODEL="+model,
		"GENIE_EXTERNAL_CONTEXT="+strconv.Itoa(genieContextFallback),
		"GENIE_YCODE_SESSION="+label, "GENIE_TOOLCHAINS=none",
		// Offline: the mounted toolchain is the only one; nothing is fetched.
		"GOTOOLCHAIN=local", "GOPROXY=off", "GOFLAGS=-mod=mod", "GOWORK=off",
	)
	run := func(mode string, stdin io.Reader, args ...string) ([]byte, error) {
		cmd := exec.Command(self, append([]string{"run", "--target", "chat", bundle}, args...)...)
		cmd.Env = append(slices.Clone(env), "GENIE_MODE="+mode)
		cmd.Stdin, cmd.Stderr = stdin, os.Stderr
		return cmd.Output()
	}
	answer, err := run("chat", strings.NewReader(task))
	if err != nil {
		fmt.Fprintln(os.Stderr, "genie prompt:", err)
		return genieCommandStatus(context.Background(), err)
	}
	reply := strings.TrimSpace(string(answer))
	if reply == "" || reply == "DONE" {
		data, e := run("session", nil, "show", label, "--json")
		if e != nil {
			fmt.Fprintln(os.Stderr, "cannot read genie's session answer:", e)
			return 1
		}
		reply, err = genieTranscriptAnswer(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot read genie's session answer:", err)
			return 1
		}
		if reply == "" {
			fmt.Fprintln(os.Stderr, "genie session has no substantive answer")
			return 1
		}
	}
	fmt.Fprintln(os.Stdout, reply)
	return 0
}

// The final assistant message can be the one-word acknowledgement requested
// by genie's stop rule. Read backwards past it, ignoring user and tool text.
func genieTranscriptAnswer(data []byte) (string, error) {
	var transcript struct {
		Messages []struct {
			Role    string
			Content []struct{ Type, Text string }
		}
	}
	if err := json.Unmarshal(data, &transcript); err != nil {
		return "", err
	}
	for _, m := range slices.Backward(transcript.Messages) {
		if m.Role != "assistant" {
			continue
		}
		var parts []string
		for _, block := range m.Content {
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		if answer := strings.TrimSpace(strings.Join(parts, "\n")); answer != "" && answer != "DONE" {
			return answer, nil
		}
	}
	return "", nil
}
