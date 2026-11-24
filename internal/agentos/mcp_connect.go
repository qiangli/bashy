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
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/bashsharp/bashsharp/cli"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/spf13/pflag"
)

const mcpConnectSchemaVersion = "bashy-mcp-connect-v1"

type mcpConnectEnvelope struct {
	SchemaVersion string   `json:"schema_version"`
	OK            bool     `json:"ok"`
	Tools         []string `json:"tools,omitempty"`
	Code          string   `json:"code,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type mcpConnectDeps struct {
	stdio          mcpsdk.Transport
	stdout, stderr io.Writer
	httpClient     *http.Client
	now            func() time.Time
}

func dispatchMCPConnect(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runMCPConnect(ctx, args, mcpConnectDeps{
		stdio: &mcpsdk.StdioTransport{}, stdout: os.Stdout, stderr: os.Stderr,
	})
}

type connectError struct{ code, message string }

func (e *connectError) Error() string { return e.message }

func runMCPConnect(ctx context.Context, args []string, deps mcpConnectDeps) int {
	flags := pflag.NewFlagSet("mcp connect", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	account := flags.String("account", "", "saved Cloudbox account")
	asJSON := flags.Bool("json", false, "probe the site MCP and print a JSON envelope")
	help := flags.BoolP("help", "h", false, "show usage")

	envelope := mcpConnectEnvelope{SchemaVersion: mcpConnectSchemaVersion}
	var err error
	exit := 1
	switch parseErr := flags.Parse(args); {
	case parseErr != nil || flags.NArg() != 1 || *help || (flags.Changed("account") && strings.TrimSpace(*account) == ""):
		exit = 2
		if *help {
			fmt.Fprint(deps.stderr, mcpUsage)
			return exit
		}
		// A malformed argument can contain credentials.
		*asJSON = *asJSON || slices.Contains(args, "--json")
		err = &connectError{"usage_error", "usage: bashy mcp connect URL [--account REF] [--json]"}
	default:
		var endpoint fleet.SatelliteAppEndpoint
		endpoint, err = fleet.ParseSatelliteAppURL(flags.Arg(0))
		if err != nil {
			exit = 2
			err = &connectError{"invalid_url", err.Error()}
		} else {
			envelope.Tools, err = connectSiteMCP(ctx, endpoint, *account, *asJSON, deps)
		}
	}
	envelope.OK = err == nil
	if err != nil {
		envelope.Code, envelope.Error = "connect_failed", "could not connect to the site MCP"
		var cause *connectError
		if errors.As(err, &cause) {
			envelope.Code, envelope.Error = cause.code, cause.message
		}
		fmt.Fprintln(deps.stderr, "bashy mcp connect:", envelope.Error)
	}
	if *asJSON {
		if err := json.NewEncoder(deps.stdout).Encode(envelope); err != nil {
			fmt.Fprintln(deps.stderr, "bashy mcp connect: could not write JSON output")
			return 1
		}
	}
	if envelope.OK {
		return 0
	}
	return exit
}

func connectSiteMCP(ctx context.Context, endpoint fleet.SatelliteAppEndpoint, account string, probe bool, deps mcpConnectDeps) ([]string, error) {
	selected, base, token, err := fleet.ResolveAccountSelection(ctx, account, "", "")
	if err != nil {
		switch code := fleet.AccountErrorCode(err); code {
		case "account_conflict", "account_unsupported":
			return nil, &connectError{code, "account cannot be used for this satellite"}
		default:
			return nil, &connectError{"no_account_token", "no account token for the selected account"}
		}
	}
	if token == "" {
		return nil, &connectError{"no_account_token", "no account token; select a saved account with --account or sign in"}
	}
	if selected != nil && selected.Category == fleet.AccountSatellite && !strings.EqualFold(fleet.URLOrigin(base), endpoint.Satellite) {
		return nil, &connectError{"account_conflict", "selected account belongs to another satellite"}
	}

	// Both HTTP clients refuse redirects. Only the exchange sees the account bearer.
	source := &fleet.SatelliteTokenSource{
		Client: fleet.SatelliteClient{BaseURL: endpoint.Satellite, Token: token, HTTP: deps.httpClient},
		Now:    deps.now,
	}
	transport := http.DefaultTransport
	if deps.httpClient != nil && deps.httpClient.Transport != nil {
		transport = deps.httpClient.Transport
	}
	httpClient := &http.Client{
		Transport:     &mcpBearerTransport{source, transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	logger := slog.New(slog.DiscardHandler)
	impl := &mcpsdk.Implementation{Name: "bashy-mcp-connect", Version: cli.BashyVersion()}
	client := mcpsdk.NewClient(impl, &mcpsdk.ClientOptions{Logger: logger})
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: endpoint.URL, HTTPClient: httpClient, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	if probe {
		var names []string
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return nil, err
			}
			names = append(names, tool.Name)
		}
		return names, nil
	}

	server := mcpsdk.NewServer(impl, &mcpsdk.ServerOptions{
		Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{}},
		Logger:       logger,
	})
	server.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			switch r := req.(type) {
			case *mcpsdk.ListToolsRequest:
				return session.ListTools(ctx, r.Params)
			case *mcpsdk.CallToolRequest:
				p := r.Params
				if p == nil {
					return next(ctx, method, req)
				}
				params := &mcpsdk.CallToolParams{Name: p.Name, Meta: p.Meta, InputResponses: p.InputResponses, RequestState: p.RequestState}
				if len(p.Arguments) > 0 {
					params.Arguments = p.Arguments
				}
				return session.CallTool(ctx, params)
			default:
				return next(ctx, method, req)
			}
		}
	})
	err = server.Run(ctx, deps.stdio)
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		err = nil
	}
	return nil, err
}

type mcpBearerTransport struct {
	source *fleet.SatelliteTokenSource
	base   http.RoundTripper
}

func (t *mcpBearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.source.Token(req.Context())
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		var refusal *fleet.ProvisionError
		if errors.As(err, &refusal) {
			switch refusal.Status {
			case http.StatusUnauthorized:
				return nil, &connectError{"exchange_refused", "token exchange refused; the account token is invalid or expired"}
			case http.StatusForbidden:
				reason := "check satellite admission, browser sign-in, and account token scopes host:proxy and satellites:delegate"
				switch refusal.Message {
				case "delegation refused":
					reason = "the member must be admitted and must first sign in to this satellite in the browser"
				case "scope escalation refused":
					reason = "the account token lacks host:proxy"
				case "token lacks scope satellites:delegate":
					reason = "the account token lacks satellites:delegate"
				}
				return nil, &connectError{"exchange_refused", "token exchange refused; " + reason}
			}
		}
		return nil, &connectError{"exchange_failed", "could not exchange the account token on the satellite"}
	}
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", "Bearer "+token)
	resp, err := t.base.RoundTrip(out)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, &connectError{"not_shared", "app not shared with you (HTTP 403)"}
	}
	return resp, nil
}
