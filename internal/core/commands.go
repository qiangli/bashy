// Package core is the small one-file Bashy route used to prove the base/core
// import boundary. It deliberately has no AgentOS, ycode, or Genie imports.
// The bashy_core build tag is a non-shipping architecture probe until the
// optional front door has migrated to separately versioned registrations.
package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/qiangli/bashy/internal/extensions"
	"github.com/qiangli/coreutils/shell"
	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/pkg/fleet"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
)

func certProfile() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("VSC_PROFILE")), "cert")
}

// reservedName preserves the base collision boundary. The full AgentOS
// profile has a larger optional verb set; this profile is not released until
// those verbs are represented by versioned records in the shared catalog.
func reservedName(name string) (string, bool) {
	if interp.IsBuiltin(name) {
		return "bash builtin", true
	}
	if tool.Lookup(name) != nil {
		return "coreutils applet", true
	}
	if name == "commands" || name == "command" || name == "help" {
		return "bashy core command", true
	}
	for _, word := range fleet.ReservedCommandWords() {
		if name == word {
			return "a word bashy commands keeps for itself", true
		}
	}
	return "", false
}

func catalog() *fleet.Catalog {
	return fleet.New(fleet.WithReservedNames(reservedName), fleet.WithCommandProbe(scriptSyntaxProbe))
}

func scriptSyntaxProbe(rec fleet.Command) (string, bool) {
	if rec.Mode() != "script" {
		return "", true
	}
	self, err := os.Executable()
	if err != nil {
		return err.Error(), false
	}
	argv := []string{"-n", "-c", rec.Script, rec.Name}
	if rec.Dialect == fleet.DialectBash {
		argv = append([]string{"--no-bashpp"}, argv...)
	}
	out, err := exec.Command(self, argv...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return msg, false
	}
	return "", true
}

func lookup(name string) (fleet.Command, bool) {
	if certProfile() {
		return fleet.Command{}, false
	}
	if _, reserved := reservedName(name); reserved {
		return fleet.Command{}, false
	}
	return catalog().Command(name)
}

func argv(ctx context.Context, rec fleet.Command, args []string) ([]string, error) {
	bin := ""
	if rec.Mode() == "download" {
		var err error
		bin, err = rec.Ensure(ctx)
		if err != nil {
			return nil, err
		}
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	out := rec.Argv(self, bin, args)
	if len(out) == 0 {
		return nil, fmt.Errorf("registered command %q has no runnable implementation", rec.Name)
	}
	return out, nil
}

func registeredHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return next(ctx, args)
		}
		rec, ok := lookup(args[0])
		if !ok {
			return next(ctx, args)
		}
		resolved, err := argv(ctx, rec, args[1:])
		if err != nil {
			fmt.Fprintf(interp.HandlerCtx(ctx).Stderr, "bashy: %s: %v\n", args[0], err)
			return interp.ExitStatus(126)
		}
		if len(rec.Env) != 0 || rec.Cwd != "" {
			hc := interp.HandlerCtx(ctx)
			cmd := exec.CommandContext(ctx, resolved[0], resolved[1:]...)
			cmd.Dir = hc.Dir
			if rec.Cwd != "" {
				cmd.Dir = rec.Cwd
			}
			cmd.Env = append(handlerEnv(hc.Env), rec.Env...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = hc.Stdin, hc.Stdout, hc.Stderr
			if err := cmd.Run(); err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					return interp.ExitStatus(uint8(ee.ExitCode()))
				}
				fmt.Fprintf(hc.Stderr, "bashy: %s: %v\n", args[0], err)
				return interp.ExitStatus(126)
			}
			return nil
		}
		return next(ctx, resolved)
	}
}

func handlerEnv(env expand.Environ) []string {
	if env == nil {
		return nil
	}
	var out []string
	env.Each(func(name string, vr expand.Variable) bool {
		if vr.IsSet() {
			out = append(out, name+"="+vr.String())
		}
		return true
	})
	return out
}

func registeredResolver(name string) (interp.ResolvedCommand, bool) {
	rec, ok := lookup(name)
	if !ok {
		return interp.ResolvedCommand{}, false
	}
	desc := fmt.Sprintf("%s is a bashy registered command (%s)", name, rec.Mode())
	if name != rec.Name {
		desc = fmt.Sprintf("%s is a bashy registered command (%s, alias of %s)", name, rec.Mode(), rec.Name)
	}
	return interp.ResolvedCommand{Desc: desc, Schema: registeredSchema(rec.Args)}, true
}

func registeredSchema(args *fleet.CommandSchema) *interp.CommandSchema {
	if args == nil {
		return nil
	}
	schema := &interp.CommandSchema{}
	for _, p := range args.Positionals {
		schema.Positionals = append(schema.Positionals, interp.CommandParameter{
			Name: p.Name, Type: p.Type, Required: p.Required,
			Default: p.Default, Enum: slices.Clone(p.Enum),
		})
	}
	for _, f := range args.Flags {
		schema.Flags = append(schema.Flags, interp.CommandFlag{
			Name: f.Name, Shorthand: f.Shorthand, Type: f.Type,
			Required: f.Required, Default: f.Default, Enum: slices.Clone(f.Enum),
		})
	}
	return schema
}

// WireExec keeps Coreutils before the registered ring and PATH, matching the
// full profile's stable shell resolution order. In cert mode the ring is off.

func WireExec(opts []interp.RunnerOption, _ bool, _ []string, in io.Reader, out, errout io.Writer) []interp.RunnerOption {
	opts = append(opts, interp.StdIO(in, out, errout))
	opts = append(opts, interp.CommandResolver(registeredResolver))
	opts = append(opts, interp.ServedInProcess(func(name string) bool { return tool.Lookup(name) != nil }))
	return append(opts, interp.ExecHandlers(shell.Handler(), registeredHandler))
}

// Dispatch handles only the core's registered-command CRUD. Other arguments
// remain shell arguments for cli.Main; optional front-door verbs are absent.
func Dispatch() {
	if len(os.Args) < 2 {
		return
	}
	if os.Args[1] == "commands" || os.Args[1] == "command" {
		if len(os.Args) > 2 && (os.Args[2] == "language" || os.Args[2] == "toolchain") {
			if certProfile() {
				fmt.Fprintln(os.Stderr, "bashy commands: extension records are unavailable in cert profile")
				os.Exit(2)
			}
			if err := extensions.Run(extensions.NewStore(extensions.BuiltinReserved), os.Args[2], os.Args[3:], os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "bashy commands: %v\n", err)
				os.Exit(2)
			}
			os.Exit(0)
		}
		cmd := fleet.NewCommandsCmd(fleet.WithReservedNames(reservedName), fleet.WithCommandProbe(scriptSyntaxProbe))
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "bashy commands: %v\n", err)
			os.Exit(fleet.ExitCode(err))
		}
		os.Exit(0)
	}
	if rec, ok := lookup(os.Args[1]); ok {
		resolved, err := argv(context.Background(), rec, os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", os.Args[1], err)
			os.Exit(126)
		}
		cmd := exec.Command(resolved[0], resolved[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), rec.Env...)
		cmd.Dir = rec.Cwd
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Exit(ee.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", os.Args[1], err)
			os.Exit(126)
		}
		os.Exit(0)
	}
}
