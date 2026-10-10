// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/coreutils/tool"
	yokemcp "github.com/qiangli/yoke/mcp"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

// mcpUsage is the `bashy mcp` front-door usage, printed to stderr with
// exit 2 when no subcommand (or --help) is given.
const mcpUsage = `usage: bashy mcp serve [--transport stdio|http] [--listen ADDR] [--allow EFFECTS]
                       [--tools default|all|NAME,...] [--max-output BYTES]
       bashy mcp tools [--tools default|all|NAME,...] [--json]

Serve bashy commands to agents over the Model Context Protocol.

  serve --transport stdio   run the MCP server over stdio (default)
  serve --transport http    loopback HTTP at /mcp (default 127.0.0.1:0)
  tools                     print the tools a profile would serve, without serving
                            (--json: the bashy-mcp-tools-v1 envelope)
  --tools default           registered tools plus available core commands
  --tools all               all canonical registry commands and visible verbs for this OS
  --allow EFFECTS           grant destroy,spend,cred,priv (comma-separated)
`

// mcpToolsSchemaVersion is the envelope `bashy mcp tools --json` emits: the
// resolved tool profile an MCP client would see from `bashy mcp serve`,
// computed without starting a server.
const mcpToolsSchemaVersion = "bashy-mcp-tools-v1"

type mcpToolsEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	Profile       string         `json:"profile"`
	Transports    []string       `json:"transports"`
	AllTools      bool           `json:"all_tools"`
	Tools         []string       `json:"tools"`
	Registered    []string       `json:"registered"`
	Definitions   []*mcpsdk.Tool `json:"definitions"`
}

// dispatchMCP is the `bashy mcp` front door: it runs the yoke MCP server
// over stdio or loopback HTTP. A clean shutdown returns 0.
func dispatchMCP(args []string) int {
	args, maxOutput, err := mcpOutputArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 2
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	if args[0] == "tools" {
		return dispatchMCPTools(args[1:])
	}
	if args[0] != "serve" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	transport := "stdio"
	allow := ""
	profile, listen := "default", ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, value, hasValue := strings.Cut(a, "=")
		if !hasValue {
			switch a {
			case "--transport", "--allow", "--tools", "--listen":
				name, value, hasValue = a, "", false
				if i+1 < len(rest) {
					i++
					value, hasValue = rest[i], true
				}
			}
		}
		switch name {
		case "--transport":
			if !hasValue || value == "" {
				fmt.Fprint(os.Stderr, mcpUsage)
				return 2
			}
			transport = value
		case "--allow":
			if !hasValue {
				fmt.Fprint(os.Stderr, mcpUsage)
				return 2
			}
			allow = value
		case "--tools", "--listen":
			if !hasValue || value == "" {
				fmt.Fprint(os.Stderr, mcpUsage)
				return 2
			}
			if name == "--tools" {
				profile = value
			} else {
				listen = value
			}
		case "--help", "-h":
			fmt.Fprint(os.Stderr, mcpUsage)
			return 2
		default:
			fmt.Fprint(os.Stderr, mcpUsage)
			return 2
		}
	}
	if transport != "stdio" && transport != "http" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	opts, err := mcpOptions(allow, profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// cli.BashyVersion is the exported source of what `bashy --version`
	// prints (internal/cli/version.go); "dev" when unstamped.
	version := cli.BashyVersion()
	if version == "" {
		version = "dev"
	}
	if transport == "http" {
		srv, err := yokemcp.BuildServer("bashy", version, opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy mcp:", err)
			return 2
		}
		if err := runMCPHTTP(ctx, srv, opts, maxOutput, listen); err != nil {
			fmt.Fprintln(os.Stderr, "bashy mcp:", err)
			return 2
		}
		return 0
	}
	srv, err := yokemcp.BuildServer("bashy", version, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 2
	}
	if err := runMCPStdio(ctx, srv, opts, maxOutput); err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 1
	}
	return 0
}

// dispatchMCPTools answers `bashy mcp tools [--tools PROFILE] [--json]`:
// the same option resolution `serve` performs, reported instead of served.
func dispatchMCPTools(args []string) int {
	profile, asJSON := "default", false
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		switch name {
		case "--json":
			asJSON = true
		case "--tools":
			if !hasValue && i+1 < len(args) {
				i++
				value, hasValue = args[i], true
			}
			if !hasValue || value == "" {
				fmt.Fprint(os.Stderr, mcpUsage)
				return 2
			}
			profile = value
		default:
			fmt.Fprint(os.Stderr, mcpUsage)
			return 2
		}
	}
	opts, err := mcpOptions("", profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp:", err)
		return 2
	}
	env := mcpToolsEnvelope{SchemaVersion: mcpToolsSchemaVersion, Profile: profile, Transports: []string{"stdio", "http"},
		AllTools: opts.AllTools, Tools: append([]string{}, opts.Tools...), Registered: []string{}}
	slices.Sort(env.Tools)
	if opts.Registered != nil {
		for _, command := range opts.Registered() {
			env.Registered = append(env.Registered, command.Name)
		}
		slices.Sort(env.Registered)
	}
	if asJSON {
		env.Definitions, err = mcpToolDefinitions(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy mcp:", err)
			return 1
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(env); err != nil {
			fmt.Fprintln(os.Stderr, "bashy mcp:", err)
			return 1
		}
		return 0
	}
	if env.AllTools {
		fmt.Println("tools: all canonical registry commands and visible verbs for this OS")
	}
	for _, name := range env.Tools {
		fmt.Println(name)
	}
	for _, name := range env.Registered {
		fmt.Println(name + "\t(registered)")
	}
	return 0
}

func mcpOptions(allow, profile string) (yokemcp.Options, error) {
	grants, err := yokemcp.ParseAllow(allow)
	if err != nil {
		return yokemcp.Options{}, err
	}
	opts := yokemcp.Options{Policy: &yokemcp.Policy{Allow: grants, Audit: mcpAudit()},
		Registered: mcpRegisteredCommands, RunScript: mcpRunScript}
	switch profile {
	case "default":
		_, commands, verbs := commandsCatalog()
		for _, name := range append(commands, verbs...) {
			if !isCoreCommand(name) || tool.Lookup(name) == nil {
				continue
			}
			if entry, ok := atlas.Lookup(name); ok && entry.AliasOf != "" {
				continue
			}
			opts.Tools = append(opts.Tools, name)
		}
	case "all":
		opts.AllTools = true
	default:
		opts.Tools = strings.Split(profile, ",")
		for i, name := range opts.Tools {
			opts.Tools[i] = strings.TrimSpace(name)
			if opts.Tools[i] == "" {
				return opts, fmt.Errorf("empty tool name in --tools")
			}
		}
	}
	if profile != "default" {
		verbs := mcpVerbCommands()
		var selected []yokemcp.RegisteredCommand
		for _, verb := range verbs {
			if profile == "all" || slices.Contains(opts.Tools, verb.Name) {
				selected = append(selected, verb)
				opts.Tools = slices.DeleteFunc(opts.Tools, func(name string) bool { return name == verb.Name })
			}
		}
		opts.Registered = func() []yokemcp.RegisteredCommand {
			ring := mcpRegisteredCommands()
			out := append([]yokemcp.RegisteredCommand(nil), ring...)
			for _, verb := range selected {
				if !slices.ContainsFunc(ring, func(command yokemcp.RegisteredCommand) bool { return command.Name == verb.Name }) {
					out = append(out, verb)
				}
			}
			return out
		}
	}
	return opts, nil
}

// Keep the transport's single call record in the same hash-chained audit as
// shell dispatch. Action identifies the MCP tool; Binary identifies its command.
func mcpAudit() func(yokemcp.Record) {
	writer := newAuditWriter()
	actor, host := auditActor(), auditHost()
	return func(r yokemcp.Record) {
		if writer == nil {
			data, _ := json.Marshal(r)
			_, _ = os.Stderr.Write(append(data, '\n'))
			return
		}
		decision := "deny"
		if r.Allowed {
			decision = "allow"
		}
		_, _ = writer.Append(audit.Record{Time: r.Time, Actor: actor, Host: host,
			Action: "mcp:" + r.Tool, Binary: r.Command, Argv: []string{r.Command},
			Effects: r.Effects, Decision: decision, Exit: r.ExitCode, DurationMs: r.Duration.Milliseconds()})
	}
}

func runMCPStdio(ctx context.Context, srv *mcpsdk.Server, opts yokemcp.Options, maxOutput int) error {
	ctx, cancel := context.WithCancel(ctx)
	sessions := registerMCPShells(ctx, srv, maxOutput)
	watcher := watchMCPRegistered(ctx, srv, opts, 2*time.Second)
	defer func() { cancel(); <-sessions.done; <-watcher }()
	err := srv.Run(ctx, &mcpsdk.StdioTransport{})
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || (err != nil && strings.Contains(err.Error(), "server is closing")) {
		return nil
	}
	return err
}

// runMCPHTTP serves the SAME prebuilt server as stdio — shell sessions and
// the registered-ring watcher attach to the server actually being served —
// over loopback-only stateless Streamable HTTP, until ctx is cancelled.
func runMCPHTTP(ctx context.Context, srv *mcpsdk.Server, opts yokemcp.Options, maxOutput int, listen string) error {
	ctx, cancel := context.WithCancel(ctx)
	sessions := registerMCPShells(ctx, srv, maxOutput)
	watcher := watchMCPRegistered(ctx, srv, opts, 2*time.Second)
	defer func() { cancel(); <-sessions.done; <-watcher }()
	addr, shutdown, err := yokemcp.ServeHTTPServerWithShutdown(ctx, srv, listen)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "listening on http://%s/mcp\n", addr)
	<-ctx.Done()
	return shutdown()
}
