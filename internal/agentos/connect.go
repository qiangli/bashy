package agentos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/yoke/external/meshagent"
	"github.com/qiangli/yoke/pkg/accounts"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/qiangli/yoke/pkg/secrets"
)

const connectHelp = `Usage: bashy connect [user@]host[:port] [--native] [-- <cmd> [args...]]
       bashy connect [user@]host[:port] -- <cmd> [args...]
       bashy connect --account <alias|id> [-- <cmd> [args...]]

Attach your terminal to a remote host through outpost SSH.
Outpost resolves the host, chooses the user, and handles authentication,
elevation, LAN-direct access and tunnel fallback. Without a command the
shell runs remotely and interactively; with a command after -- the remote
command runs non-interactively.

Outpost joins the remote argv with spaces like ssh, so quote accordingly.
Agents can set $OUTPOST_SSH_PASSWORD for non-interactive password auth;
never put a password on the command line.

--account selects a host-login account (see 'bashy account'). The target is
built from the account's username and endpoint; its credential is resolved
for this invocation only: secret:<name> is fetched from the secrets store
and handed to the outpost child via $OUTPOST_SSH_PASSWORD (never argv, never
printed); ssh-key:<fingerprint> lets outpost use keys. A host argument that
differs from the account's endpoint or user is a conflict (exit 2). Inspect
the safe metadata with: bashy account resolve <id> --for connect

Examples:
  bashy connect --account owl-login -- uptime
  bashy connect owl
  bashy connect dev@owl -- uptime

Options:
  --account   Connect using a host-login account alias or id
  -h, --help  Show this help
  --native    Stay in Windows instead of WSL (not yet available)
`

func dispatchConnect(args []string) int {
	return runConnectWith(context.Background(), args, defaultConnectDeps())
}

// envTransport runs outpost with extra child-only environment entries.
type envTransport func(ctx context.Context, env []string, args ...string) error

type connectDeps struct {
	store     func() *accounts.Store
	secretGet func(name string) (string, error)
	transport envTransport
}

func defaultConnectDeps() connectDeps {
	return connectDeps{
		store: func() *accounts.Store { return accounts.Open(accounts.DefaultPath()) },
		secretGet: func(name string) (string, error) {
			c, err := (secrets.Config{}).Resolve()
			if err != nil {
				return "", err
			}
			return c.Get(name)
		},
		transport: execOutpost,
	}
}

// execOutpost is meshagent.Exec plus a child-only environment. The secret
// is applied to this one exec.Cmd, never to the bashy process environment.
func execOutpost(ctx context.Context, env []string, args ...string) error {
	if len(env) == 0 {
		return meshagent.Exec(ctx, args...)
	}
	bin, ok := meshagent.Resolve()
	if !ok {
		return meshagent.ErrNotFound
	}
	c := binmgr.Command(ctx, bin, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	c.Env = childEnv(os.Environ(), env)
	return c.Run()
}

func childEnv(base, extra []string) []string {
	drop := map[string]bool{}
	for _, e := range extra {
		if k, _, ok := strings.Cut(e, "="); ok {
			drop[k] = true
		}
	}
	out := make([]string, 0, len(base)+len(extra))
	for _, e := range base {
		if k, _, _ := strings.Cut(e, "="); !drop[k] {
			out = append(out, e)
		}
	}
	return append(out, extra...)
}

func runConnect(ctx context.Context, args []string, transport func(context.Context, ...string) error) int {
	return runConnectWith(ctx, args, connectDeps{transport: func(ctx context.Context, _ []string, a ...string) error {
		return transport(ctx, a...)
	}})
}

func connectFail(code, msg string) int {
	fmt.Fprintf(os.Stderr, "bashy connect: error=%s: %s\n", code, msg)
	return 2
}

// accountTarget resolves the account and credential for one invocation.
// Failures use fixed messages so no credential material can leak.
func accountTarget(opts connectOptions, deps connectDeps) (target string, env []string, code int) {
	if deps.store == nil {
		return "", nil, connectFail("account_unavailable", "no account store")
	}
	st := deps.store()
	acct, err := st.Get(opts.account)
	if err != nil {
		return "", nil, connectFail("account_unresolved", "account not found or ambiguous")
	}
	if acct.Category != accounts.HostLogin {
		return "", nil, connectFail("account_category", fmt.Sprintf("account category is %q, want host-login", acct.Category))
	}
	sel, err := st.Resolve(acct.ID, "connect")
	if err != nil {
		return "", nil, connectFail("account_unresolved", "account is not resolvable for connect")
	}
	endpoint := strings.TrimSpace(sel.Endpoint)
	if endpoint == "" || strings.ContainsAny(endpoint, "\x00\r\n \t") {
		return "", nil, connectFail("account_endpoint", "account has no usable endpoint")
	}
	user := sel.Username
	if u, h, ok := strings.Cut(endpoint, "@"); ok {
		if user != "" && user != u {
			return "", nil, connectFail("account_conflict", "account endpoint user differs from account username")
		}
		user, endpoint = u, h
	}
	if opts.host != "" {
		gu, gh, hasUser := strings.Cut(opts.host, "@")
		if !hasUser {
			gu, gh = "", opts.host
		}
		if gh != endpoint {
			return "", nil, connectFail("account_conflict", "host argument differs from the account endpoint")
		}
		if gu != "" && user != "" && gu != user {
			return "", nil, connectFail("account_conflict", "user in host argument differs from the account user")
		}
		if user == "" {
			user = gu
		}
	}
	target = endpoint
	if user != "" {
		target = user + "@" + endpoint
	}
	ref := sel.CredentialRef
	switch {
	case strings.HasPrefix(ref, "secret:"):
		if deps.secretGet == nil {
			return "", nil, connectFail("credential_unavailable", "no secrets client")
		}
		v, err := deps.secretGet(strings.TrimPrefix(ref, "secret:"))
		if err != nil || v == "" {
			return "", nil, connectFail("credential_unavailable", "credential could not be fetched")
		}
		env = []string{"OUTPOST_SSH_PASSWORD=" + v}
	case strings.HasPrefix(ref, "ssh-key:"):
	case ref == "":
		return "", nil, connectFail("credential_missing", "account has no credential reference")
	default:
		return "", nil, connectFail("credential_unsupported", "credential reference kind is not supported for connect")
	}
	return target, env, 0
}

func runConnectWith(ctx context.Context, args []string, deps connectDeps) int {
	opts, err := parseConnectArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy connect:", err)
		return 2
	}
	if opts.help {
		fmt.Fprint(os.Stdout, connectHelp)
		return 0
	}
	// Do not promise WSL behavior the current outpost CLI cannot provide.
	// Keep the transport boundary honest.
	if opts.native {
		fmt.Fprintln(os.Stderr, "bashy connect: --native (WSL/native selection) requires an outpost SSH extension and is not yet available")
		return 2
	}
	target := opts.host
	var env []string
	if opts.account != "" {
		var code int
		if target, env, code = accountTarget(opts, deps); code != 0 {
			return code
		}
	}
	// -- also protects hosts named like outpost subcommands (e.g. list).
	// The remote command stays as separate argv entries; outpost joins them
	// with spaces like ssh.
	argv := append([]string{"ssh", "--", target}, opts.command...)
	if err := deps.transport(ctx, env, argv...); err != nil {
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
	account string
	native  bool
	command []string
	help    bool
}

// parseConnectArgs stops interpreting options at --. Remote arguments must
// never become local flags (in particular --help and --native).
func parseConnectArgs(args []string) (connectOptions, error) {
	var opts connectOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--account":
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return opts, fmt.Errorf("--account needs an alias or id")
			}
			i++
			opts.account = args[i]
		case strings.HasPrefix(arg, "--account="):
			opts.account = strings.TrimPrefix(arg, "--account=")
			if opts.account == "" {
				return opts, fmt.Errorf("--account needs an alias or id")
			}
		case arg == "--":
			if opts.host == "" && opts.account == "" {
				return opts, fmt.Errorf("expected a host before --")
			}
			if i+1 == len(args) || args[i+1] == "" {
				return opts, fmt.Errorf("expected a command after --")
			}
			opts.command = append([]string(nil), args[i+1:]...)
			return opts, nil
		case arg == "--native":
			opts.native = true
		case arg == "--help" || arg == "-h":
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
	if opts.host == "" && opts.account == "" && !opts.help {
		return opts, fmt.Errorf("expected a host")
	}
	return opts, nil
}
