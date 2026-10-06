// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/qiangli/bashy/internal/cli"
	yokemcp "github.com/qiangli/yoke/mcp"
)

// mcpUsage is the `bashy mcp` front-door usage, printed to stderr with
// exit 2 when no subcommand (or --help) is given.
const mcpUsage = `usage: bashy mcp serve [--transport stdio|http] [--allow EFFECTS]

Serve bashy commands to agents over the Model Context Protocol.

  serve --transport stdio   run the MCP server over stdio (default)
  serve --transport http    not yet supported
`

// dispatchMCP is the `bashy mcp` front door: it runs the yoke MCP server
// over stdio, so any MCP client can launch `bashy mcp serve` as a stdio
// server. A clean shutdown returns 0.
func dispatchMCP(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	if args[0] != "serve" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	transport := "stdio"
	allow := ""
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, value, hasValue := strings.Cut(a, "=")
		if !hasValue {
			switch a {
			case "--transport", "--allow":
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
		case "--help", "-h":
			fmt.Fprint(os.Stderr, mcpUsage)
			return 2
		default:
			fmt.Fprint(os.Stderr, mcpUsage)
			return 2
		}
	}
	if transport == "http" {
		fmt.Fprintln(os.Stderr, "bashy mcp: --transport http is not yet supported")
		return 2
	}
	if transport != "stdio" {
		fmt.Fprint(os.Stderr, mcpUsage)
		return 2
	}
	// The yoke mcp package has no ParseAllow/NewServerWithOptions yet
	// (checked against ../yoke/mcp), so --allow is accepted but not
	// wired: warn and continue with the default server.
	if allow != "" {
		fmt.Fprintln(os.Stderr, "bashy mcp: --allow is not wired yet")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// cli.BashyVersion is the exported source of what `bashy --version`
	// prints (internal/cli/version.go); "dev" when unstamped.
	version := cli.BashyVersion()
	if version == "" {
		version = "dev"
	}
	if err := yokemcp.ServeStdio(ctx, "bashy", version); err != nil {
		fmt.Fprintf(os.Stderr, "bashy mcp: %v\n", err)
		return 1
	}
	return 0
}
