package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bashsharp/bashsharp/cli"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
)

// mcpConnectSchemaVersion is the versioned JSON envelope emitted by
// `bashy mcp connect URL [--account REF] --json`.
const mcpConnectSchemaVersion = "bashy-mcp-connect-v1"

type mcpConnectEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	OK            bool           `json:"ok"`
	URL           string         `json:"url,omitempty"`
	Satellite     string         `json:"satellite,omitempty"`
	Host          string         `json:"host,omitempty"`
	App           string         `json:"app,omitempty"`
	Account       string         `json:"account,omitempty"`
	Scopes        []string       `json:"scopes,omitempty"`
	ExpiresAt     string         `json:"expires_at,omitempty"`
	Tools         []string       `json:"tools,omitempty"`
	Definitions   []*mcpsdk.Tool `json:"definitions,omitempty"`
	Command       []string       `json:"command,omitempty"`
	Code          string         `json:"code,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type mcpConnectOptions struct {
	targetURL string
	account   string
	url       string
	token     string
	asJSON    bool
	help      bool
}

type mcpConnectDeps struct {
	stdio      mcpsdk.Transport
	stdout     io.Writer
	stderr     io.Writer
	httpClient *http.Client
	now        func() time.Time
	ttl        string
}

func defaultMCPConnectDeps() mcpConnectDeps {
	return mcpConnectDeps{
		stdio:      &mcpsdk.StdioTransport{},
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		now:        time.Now,
		ttl:        fleet.DefaultProxyExchangeTTL,
	}
}

func dispatchMCPConnect(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runMCPConnect(ctx, args, defaultMCPConnectDeps())
}

// secretScrubber ensures neither the account token nor any exchanged/refreshed
// satellite token ever appears in stdout, stderr, or JSON envelopes.
type secretScrubber struct {
	mu      sync.Mutex
	secrets []string
}

func (s *secretScrubber) add(val string) {
	val = strings.TrimSpace(val)
	if val == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.secrets {
		if existing == val {
			return
		}
	}
	s.secrets = append(s.secrets, val)
}

func (s *secretScrubber) scrub(text string) string {
	if text == "" {
		return ""
	}
	s.mu.Lock()
	list := append([]string(nil), s.secrets...)
	s.mu.Unlock()
	for _, secret := range list {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

func (s *secretScrubber) scrubErr(err error) error {
	if err == nil {
		return nil
	}
	msg := s.scrub(err.Error())
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

type mcpConnectCodedError struct {
	code string
	msg  string
}

func (e *mcpConnectCodedError) Error() string { return e.msg }

func parseMCPConnectArgs(args []string, scrub *secretScrubber) (mcpConnectOptions, error) {
	var opts mcpConnectOptions
	// Pre-scan --token values into the scrubber before any error can format args.
	for i := 0; i < len(args); i++ {
		if args[i] == "--token" && i+1 < len(args) {
			scrub.add(args[i+1])
		} else if v, ok := strings.CutPrefix(args[i], "--token="); ok {
			scrub.add(v)
		}
		if args[i] == "--json" || args[i] == "--json=true" {
			opts.asJSON = true
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasEq := strings.Cut(arg, "=")
		switch name {
		case "--help", "-h":
			if hasEq {
				return opts, fmt.Errorf("unexpected value for %s", name)
			}
			opts.help = true
		case "--json":
			if !hasEq || val == "true" {
				opts.asJSON = true
			} else if val == "false" {
				opts.asJSON = false
			} else {
				return opts, fmt.Errorf("--json expects a boolean")
			}
		case "--account":
			if !hasEq {
				if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
					return opts, fmt.Errorf("--account needs an account alias or id")
				}
				i++
				val = args[i]
			}
			if strings.TrimSpace(val) == "" {
				return opts, fmt.Errorf("--account needs an account alias or id")
			}
			opts.account = strings.TrimSpace(val)
		case "--url":
			if !hasEq {
				if i+1 >= len(args) || args[i+1] == "" {
					return opts, fmt.Errorf("--url needs a URL")
				}
				i++
				val = args[i]
			}
			if strings.TrimSpace(val) == "" {
				return opts, fmt.Errorf("--url needs a URL")
			}
			opts.url = strings.TrimSpace(val)
		case "--token":
			if !hasEq {
				if i+1 >= len(args) || args[i+1] == "" {
					return opts, fmt.Errorf("--token needs a value")
				}
				i++
				val = args[i]
			}
			if strings.TrimSpace(val) == "" {
				return opts, fmt.Errorf("--token needs a value")
			}
			opts.token = strings.TrimSpace(val)
			scrub.add(opts.token)
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown option %q", scrub.scrub(arg))
			}
			if opts.targetURL != "" {
				return opts, fmt.Errorf("expected a single site MCP URL")
			}
			opts.targetURL = arg
		}
	}
	if !opts.help && strings.TrimSpace(opts.targetURL) == "" {
		return opts, fmt.Errorf("missing site MCP URL (want https://SATELLITE/matrix/h/HOST/app/NAME/mcp)")
	}
	return opts, nil
}

func runMCPConnect(ctx context.Context, args []string, deps mcpConnectDeps) int {
	if deps.stdout == nil {
		deps.stdout = os.Stdout
	}
	if deps.stderr == nil {
		deps.stderr = os.Stderr
	}
	if deps.stdio == nil {
		deps.stdio = &mcpsdk.StdioTransport{}
	}
	if deps.httpClient == nil {
		deps.httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if deps.now == nil {
		deps.now = time.Now
	}
	if strings.TrimSpace(deps.ttl) == "" {
		deps.ttl = fleet.DefaultProxyExchangeTTL
	}

	scrub := &secretScrubber{}
	scrub.add(os.Getenv("BASHY_FLEET_TOKEN"))
	scrub.add(os.Getenv("BASHY_API_KEY"))

	opts, err := parseMCPConnectArgs(args, scrub)
	if err != nil {
		return failMCPConnect(deps, scrub, opts, fleet.SatelliteAppEndpoint{}, 2, "usage_error", err.Error())
	}
	if opts.help {
		fmt.Fprint(deps.stderr, mcpUsage)
		return 2
	}

	ep, err := fleet.ParseSatelliteAppURL(opts.targetURL)
	if err != nil || !strings.HasSuffix(ep.URL, "/mcp") {
		msg := fmt.Sprintf("invalid site MCP URL %q (want https://SATELLITE/matrix/h/HOST/app/NAME/mcp)", opts.targetURL)
		if err != nil {
			msg = err.Error()
		}
		return failMCPConnect(deps, scrub, opts, ep, 2, "invalid_url", msg)
	}

	sel, _, accountToken, err := fleet.ResolveAccountSelection(ctx, opts.account, opts.url, opts.token)
	scrub.add(accountToken)
	if err != nil {
		code := fleet.AccountErrorCode(err)
		if code == "" {
			code = "account_error"
		}
		switch code {
		case "credential_unavailable", "account_unresolved":
			return failMCPConnect(deps, scrub, opts, ep, 1, "no_account_token", fmt.Sprintf("no account token: %s", err.Error()))
		default:
			return failMCPConnect(deps, scrub, opts, ep, 2, code, err.Error())
		}
	}
	if strings.TrimSpace(accountToken) == "" {
		return failMCPConnect(deps, scrub, opts, ep, 1, "no_account_token",
			"no account token: pass --account REF, set $BASHY_FLEET_TOKEN, or sign in with 'bashy login'")
	}
	if sel != nil && sel.Category == fleet.AccountSatellite {
		if selOrigin := fleet.URLOrigin(sel.Endpoint); selOrigin != "" && selOrigin != fleet.URLOrigin(ep.Satellite) {
			return failMCPConnect(deps, scrub, opts, ep, 2, "account_conflict",
				fmt.Sprintf("satellite account %s is bound to %s, not %s", sel.Ref, sel.Endpoint, ep.Satellite))
		}
	}

	exchangeClient := *deps.httpClient
	exchangeClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	tokenSource := &fleet.SatelliteTokenSource{
		Client: fleet.SatelliteClient{
			BaseURL: ep.Satellite,
			Token:   accountToken,
			HTTP:    &exchangeClient,
		},
		Scopes: []string{"host:proxy"},
		TTL:    deps.ttl,
		Now:    deps.now,
	}

	initialExchange, err := tokenSource.Token(ctx)
	if err != nil {
		code, msg := classifyExchangeError(ep.Satellite, err, scrub)
		return failMCPConnect(deps, scrub, opts, ep, 1, code, msg)
	}
	scrub.add(initialExchange.Token)

	baseTransport := deps.httpClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	rt := &mcpConnectRoundTripper{
		base:   baseTransport,
		tokens: tokenSource,
		ep:     ep,
		scrub:  scrub,
	}
	authedHTTP := &http.Client{
		Transport:     rt,
		Timeout:       deps.httpClient.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	version := cli.BashyVersion()
	if version == "" {
		version = "dev"
	}
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	connectRemote := func(cctx context.Context) (*mcpsdk.ClientSession, error) {
		client := mcpsdk.NewClient(
			&mcpsdk.Implementation{Name: "bashy-mcp-connect", Version: version},
			&mcpsdk.ClientOptions{
				Logger:       discardLogger,
				Capabilities: &mcpsdk.ClientCapabilities{},
			},
		)
		transport := &mcpsdk.StreamableClientTransport{
			Endpoint:             ep.URL,
			HTTPClient:           authedHTTP,
			DisableStandaloneSSE: true,
			MaxRetries:           -1,
		}
		cs, cerr := client.Connect(cctx, transport, nil)
		if cerr != nil {
			return nil, rt.wrapError(cerr)
		}
		return cs, nil
	}

	remoteSession, err := connectRemote(ctx)
	if err != nil {
		code, msg := extractConnectFailure(err, scrub)
		return failMCPConnect(deps, scrub, opts, ep, 1, code, msg)
	}

	if opts.asJSON {
		page, err := remoteSession.ListTools(ctx, &mcpsdk.ListToolsParams{})
		_ = remoteSession.Close()
		if err != nil {
			code, msg := extractConnectFailure(rt.wrapError(err), scrub)
			return failMCPConnect(deps, scrub, opts, ep, 1, code, msg)
		}
		defs := page.Tools
		if defs == nil {
			defs = []*mcpsdk.Tool{}
		}
		names := make([]string, 0, len(defs))
		for _, t := range defs {
			if t != nil {
				names = append(names, t.Name)
			}
		}
		scopes := initialExchange.Scopes
		if len(scopes) == 0 {
			scopes = []string{"host:proxy"}
		}
		var expStr string
		if !initialExchange.ExpiresAt.IsZero() {
			expStr = initialExchange.ExpiresAt.UTC().Format(time.RFC3339)
		}
		cmdArgv := []string{"bashy", "mcp", "connect", ep.URL}
		if opts.account != "" {
			cmdArgv = append(cmdArgv, "--account", opts.account)
		}
		env := mcpConnectEnvelope{
			SchemaVersion: mcpConnectSchemaVersion,
			OK:            true,
			URL:           ep.URL,
			Satellite:     ep.Satellite,
			Host:          ep.Host,
			App:           ep.App,
			Account:       opts.account,
			Scopes:        scopes,
			ExpiresAt:     expStr,
			Tools:         names,
			Definitions:   defs,
			Command:       cmdArgv,
		}
		enc := json.NewEncoder(deps.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(env); err != nil {
			fmt.Fprintln(deps.stderr, "bashy mcp connect:", scrub.scrub(err.Error()))
			return 1
		}
		return 0
	}

	proxy := &mcpRemoteProxy{
		session:   remoteSession,
		reconnect: connectRemote,
		rt:        rt,
		scrub:     scrub,
	}
	defer proxy.close()

	srv := proxy.newServer(version, discardLogger)
	if err := srv.Run(ctx, deps.stdio); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "server is closing") {
			return 0
		}
		fmt.Fprintln(deps.stderr, "bashy mcp connect:", scrub.scrub(err.Error()))
		return 1
	}
	return 0
}

func failMCPConnect(deps mcpConnectDeps, scrub *secretScrubber, opts mcpConnectOptions, ep fleet.SatelliteAppEndpoint, exit int, code, msg string) int {
	safeMsg := scrub.scrub(msg)
	if opts.asJSON {
		env := mcpConnectEnvelope{
			SchemaVersion: mcpConnectSchemaVersion,
			OK:            false,
			URL:           ep.URL,
			Satellite:     ep.Satellite,
			Host:          ep.Host,
			App:           ep.App,
			Account:       opts.account,
			Code:          code,
			Error:         safeMsg,
		}
		if env.URL == "" && opts.targetURL != "" {
			env.URL = scrub.scrub(opts.targetURL)
		}
		enc := json.NewEncoder(deps.stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(env)
	}
	fmt.Fprintln(deps.stderr, "bashy mcp connect:", safeMsg)
	return exit
}

func classifyExchangeError(satellite string, err error, scrub *secretScrubber) (string, string) {
	var pe *fleet.ProvisionError
	if errors.As(err, &pe) {
		detail := scrub.scrub(pe.Message)
		if pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden {
			return "exchange_refused", fmt.Sprintf("token exchange refused by %s (HTTP %d): %s", satellite, pe.Status, detail)
		}
		return "exchange_failed", fmt.Sprintf("token exchange failed on %s (HTTP %d): %s", satellite, pe.Status, detail)
	}
	return "exchange_failed", fmt.Sprintf("token exchange failed on %s: %s", satellite, scrub.scrub(err.Error()))
}

func extractConnectFailure(err error, scrub *secretScrubber) (string, string) {
	var ce *mcpConnectCodedError
	if errors.As(err, &ce) {
		return ce.code, scrub.scrub(ce.msg)
	}
	return "connect_failed", scrub.scrub(err.Error())
}

type mcpConnectRoundTripper struct {
	base   http.RoundTripper
	tokens *fleet.SatelliteTokenSource
	ep     fleet.SatelliteAppEndpoint
	scrub  *secretScrubber

	mu      sync.Mutex
	lastErr *mcpConnectCodedError
}

func (rt *mcpConnectRoundTripper) setLastErr(code, msg string) *mcpConnectCodedError {
	ce := &mcpConnectCodedError{code: code, msg: rt.scrub.scrub(msg)}
	rt.mu.Lock()
	rt.lastErr = ce
	rt.mu.Unlock()
	return ce
}

func (rt *mcpConnectRoundTripper) takeLastErr() *mcpConnectCodedError {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	ce := rt.lastErr
	rt.lastErr = nil
	return ce
}

func (rt *mcpConnectRoundTripper) wrapError(err error) error {
	if err == nil {
		return nil
	}
	if ce := rt.takeLastErr(); ce != nil {
		return ce
	}
	var ce *mcpConnectCodedError
	if errors.As(err, &ce) {
		return &mcpConnectCodedError{code: ce.code, msg: rt.scrub.scrub(ce.msg)}
	}
	return rt.scrub.scrubErr(err)
}

func (rt *mcpConnectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if fleet.URLOrigin(req.URL.String()) != fleet.URLOrigin(rt.ep.Satellite) {
		return nil, rt.setLastErr("origin_mismatch", fmt.Sprintf("refusing to send satellite token to %s", req.URL.Host))
	}
	ex, err := rt.tokens.Token(req.Context())
	if err != nil {
		code, msg := classifyExchangeError(rt.ep.Satellite, err, rt.scrub)
		return nil, rt.setLastErr(code, msg)
	}
	rt.scrub.add(ex.Token)

	resp, err := rt.doWithBearer(req, ex.Token)
	if err != nil {
		return nil, rt.scrub.scrubErr(err)
	}

	// If the satellite rejected a cached token with 401 (e.g. clock skew or
	// server-side revocation before our refresh window), invalidate once and
	// exchange a fresh token when the request body is replayable.
	if resp.StatusCode == http.StatusUnauthorized && (req.Body == nil || req.GetBody != nil) {
		rt.tokens.Invalidate(ex.Token)
		if ex2, err2 := rt.tokens.Token(req.Context()); err2 == nil && ex2.Token != ex.Token {
			rt.scrub.add(ex2.Token)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
			_ = resp.Body.Close()
			if req.GetBody != nil {
				bodyCopy, berr := req.GetBody()
				if berr != nil {
					return nil, rt.scrub.scrubErr(berr)
				}
				req2 := req.Clone(req.Context())
				req2.Body = bodyCopy
				resp, err = rt.doWithBearer(req2, ex2.Token)
				if err != nil {
					return nil, rt.scrub.scrubErr(err)
				}
			} else {
				resp, err = rt.doWithBearer(req, ex2.Token)
				if err != nil {
					return nil, rt.scrub.scrubErr(err)
				}
			}
		}
	}

	switch resp.StatusCode {
	case http.StatusForbidden:
		rt.setLastErr("not_shared", fmt.Sprintf("app %s on host %s is not shared with you (HTTP 403)", rt.ep.App, rt.ep.Host))
	case http.StatusUnauthorized:
		rt.setLastErr("unauthorized", fmt.Sprintf("satellite %s rejected the exchanged token for app %s on host %s (HTTP 401)", rt.ep.Satellite, rt.ep.App, rt.ep.Host))
	}
	return resp, nil
}

func (rt *mcpConnectRoundTripper) doWithBearer(req *http.Request, bearer string) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+bearer)
	return rt.base.RoundTrip(clone)
}

type mcpRemoteProxy struct {
	mu        sync.Mutex
	session   *mcpsdk.ClientSession
	reconnect func(context.Context) (*mcpsdk.ClientSession, error)
	rt        *mcpConnectRoundTripper
	scrub     *secretScrubber
}

func (p *mcpRemoteProxy) close() {
	p.mu.Lock()
	cs := p.session
	p.session = nil
	p.mu.Unlock()
	if cs != nil {
		_ = cs.Close()
	}
}

func (p *mcpRemoteProxy) currentSession() *mcpsdk.ClientSession {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.session
}

func (p *mcpRemoteProxy) withSession(ctx context.Context, fn func(*mcpsdk.ClientSession) (mcpsdk.Result, error)) (mcpsdk.Result, error) {
	cs := p.currentSession()
	if cs == nil {
		return nil, errors.New("remote MCP session is closed")
	}
	res, err := fn(cs)
	if err != nil && ctx.Err() == nil && (errors.Is(err, mcpsdk.ErrSessionMissing) || errors.Is(err, mcpsdk.ErrConnectionClosed)) {
		p.mu.Lock()
		if p.session == cs && p.reconnect != nil {
			if fresh, rerr := p.reconnect(ctx); rerr == nil {
				_ = cs.Close()
				p.session = fresh
				cs = fresh
			}
		} else if p.session != nil {
			cs = p.session
		}
		p.mu.Unlock()
		if cs != nil {
			res, err = fn(cs)
		}
	}
	if err != nil {
		return nil, p.rt.wrapError(err)
	}
	return res, nil
}

func (p *mcpRemoteProxy) newServer(version string, logger *slog.Logger) *mcpsdk.Server {
	impl := &mcpsdk.Implementation{Name: "bashy-mcp-connect", Version: version}
	caps := &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{}}
	var instructions string
	if cs := p.currentSession(); cs != nil {
		if initRes := cs.InitializeResult(); initRes != nil {
			if initRes.ServerInfo != nil && initRes.ServerInfo.Name != "" {
				cp := *initRes.ServerInfo
				impl = &cp
			}
			instructions = initRes.Instructions
			if initRes.Capabilities != nil {
				if initRes.Capabilities.Prompts != nil {
					caps.Prompts = &mcpsdk.PromptCapabilities{}
				}
				if initRes.Capabilities.Resources != nil {
					caps.Resources = &mcpsdk.ResourceCapabilities{}
				}
			}
		}
	}
	srv := mcpsdk.NewServer(impl, &mcpsdk.ServerOptions{
		Capabilities: caps,
		Instructions: instructions,
		Logger:       logger,
	})
	srv.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			switch method {
			case "tools/list":
				var params *mcpsdk.ListToolsParams
				if r, ok := req.(*mcpsdk.ListToolsRequest); ok && r != nil {
					params = r.Params
				}
				if params == nil {
					params = &mcpsdk.ListToolsParams{}
				}
				return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
					out, err := cs.ListTools(ctx, params)
					if err != nil {
						return nil, err
					}
					if out.Tools == nil {
						out.Tools = []*mcpsdk.Tool{}
					}
					return out, nil
				})
			case "tools/call":
				r, ok := req.(*mcpsdk.CallToolRequest)
				if !ok || r == nil || r.Params == nil {
					return next(ctx, method, req)
				}
				var arguments any
				if len(r.Params.Arguments) > 0 {
					arguments = r.Params.Arguments
				}
				callParams := &mcpsdk.CallToolParams{
					Meta:      r.Params.Meta,
					Name:      r.Params.Name,
					Arguments: arguments,
				}
				return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
					out, err := cs.CallTool(ctx, callParams)
					if err != nil {
						return nil, err
					}
					if out != nil && out.Content == nil {
						out.Content = []mcpsdk.Content{}
					}
					return out, nil
				})
			case "prompts/list":
				if r, ok := req.(*mcpsdk.ListPromptsRequest); ok && r != nil {
					return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
						return cs.ListPrompts(ctx, r.Params)
					})
				}
			case "prompts/get":
				if r, ok := req.(*mcpsdk.GetPromptRequest); ok && r != nil {
					return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
						return cs.GetPrompt(ctx, r.Params)
					})
				}
			case "resources/list":
				if r, ok := req.(*mcpsdk.ListResourcesRequest); ok && r != nil {
					return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
						return cs.ListResources(ctx, r.Params)
					})
				}
			case "resources/templates/list":
				if r, ok := req.(*mcpsdk.ListResourceTemplatesRequest); ok && r != nil {
					return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
						return cs.ListResourceTemplates(ctx, r.Params)
					})
				}
			case "resources/read":
				if r, ok := req.(*mcpsdk.ReadResourceRequest); ok && r != nil {
					return p.withSession(ctx, func(cs *mcpsdk.ClientSession) (mcpsdk.Result, error) {
						return cs.ReadResource(ctx, r.Params)
					})
				}
			}
			return next(ctx, method, req)
		}
	})
	return srv
}
