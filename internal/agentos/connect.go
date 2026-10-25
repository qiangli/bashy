package agentos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/yoke/external/meshagent"
)

const connectHelp = `Usage: bashy connect <host> [--native] [-- <cmd> [args...]]

Attach your terminal to a remote host through outpost SSH.
Outpost resolves the host, chooses the user, and handles authentication,
elevation, LAN-direct access and tunnel fallback. The shell runs remotely.

Examples:
  bashy connect elephant
  bashy connect dev@elephant
  bashy connect dev@192.168.1.5:2222

Current outpost compatibility: only its interactive shell is available.
TTY commands, login-shell selection and automatic WSL Ubuntu selection
require an outpost SSH extension. --native and commands after -- are
recognized but refused until that extension is available.

Options:
  -h, --help  Show this help
  --native    Stay in Windows instead of WSL (not yet available)
`

func dispatchConnect(args []string) int {
	return runConnect(context.Background(), args, meshagent.Exec)
}

func runConnect(ctx context.Context, args []string, transport func(context.Context, ...string) error) int {
	opts, err := parseConnectArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy connect:", err)
		return 2
	}
	if opts.help {
		fmt.Fprint(os.Stdout, connectHelp)
		return 0
	}
	// Do not silently run an agent without a PTY or promise WSL behavior the
	// current outpost CLI cannot provide. Keep the transport boundary honest.
	if opts.native || len(opts.command) != 0 {
		fmt.Fprintln(os.Stderr, "bashy connect: TTY commands and WSL/native selection require an outpost SSH extension; only the interactive shell is currently available")
		return 2
	}
	// -- also protects hosts named like outpost subcommands (e.g. list).
	if err := transport(ctx, "ssh", "--", opts.host); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, "bashy connect:", err)
		}
		return childExitStatus(err)
	}
	return 0
}

type connectOptions struct {
	host    string
	native  bool
	command []string
	help    bool
}

// parseConnectArgs stops interpreting options at --. Remote arguments must
// never become local flags (in particular --help and --native).
func parseConnectArgs(args []string) (connectOptions, error) {
	var opts connectOptions
	for i, arg := range args {
		switch arg {
		case "--":
			if opts.host == "" {
				return opts, fmt.Errorf("expected a host before --")
			}
			if i+1 == len(args) || args[i+1] == "" {
				return opts, fmt.Errorf("expected a command after --")
			}
			opts.command = append([]string(nil), args[i+1:]...)
			return opts, nil
		case "--native":
			opts.native = true
		case "--help", "-h":
			opts.help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown option %q", arg)
			}
			if opts.host != "" {
				return opts, fmt.Errorf("put the remote command after --")
			}
			if strings.TrimSpace(arg) == "" || strings.ContainsAny(arg, "\x00\r\n") {
				return opts, fmt.Errorf("invalid host %q", arg)
			}
			opts.host = arg
		}
	}
	if opts.host == "" && !opts.help {
		return opts, fmt.Errorf("expected a host")
	}
	return opts, nil
}
