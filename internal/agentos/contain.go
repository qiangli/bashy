package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/atlas"
	"mvdan.cc/sh/v3/interp"
)

// contain runs child programs with OS-enforced network isolation.
//
//	bashy contain --net deny -- CMD [ARGS...]      (the wrapper)
//	@contain(net: "deny") f() { python -m pytest; } (the decorator; wrapper ≡ decorator)
//
// Under @contain every EXTERNAL child a function starts is re-executed through
// `bashy contain --net deny --`, so the kernel — not the child's good
// behaviour — keeps it off the network. That is what lets an effect cap that
// excludes `net` admit an interpreter (python, pytest, pip) whose atlas
// maximum includes `net`: the containment bounds the effect. In-process
// native tools (fetch, …) are not children and keep their `net` effect.
//
// Containment is network-only; filesystem isolation is a separate rod.
// Unsupported platforms fail closed (exit 125): nothing runs uncontained.
// ("sandbox" names the podman engine; this is deliberately a different word.)

const containUnsupportedStatus = 125

type containKey struct{}

// containNetFrom reports whether ctx is inside a @contain(net: "deny") scope.
func containNetFrom(ctx context.Context) bool {
	v, _ := ctx.Value(containKey{}).(bool)
	return v
}

func withContainNet(ctx context.Context) context.Context {
	return context.WithValue(ctx, containKey{}, true)
}

func containDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		if c.Advised != "" {
			return errors.New("contain is a declaration and never applied by advice")
		}
		if len(args) != 1 || (args[0].Name != "" && args[0].Name != "net") || args[0].Value != "deny" {
			return fmt.Errorf("contain takes exactly one argument, net: %q", "deny")
		}
		if err := containSupported(); err != nil {
			c.Status = containUnsupportedStatus
			fmt.Fprintf(stderr, "%s: contain: %v\n", c.Name, err)
			return nil
		}
		c.Next(withContainNet(ctx))
		return nil
	}
}

var (
	nativeToolsOnce sync.Once
	nativeTools     map[string]bool
)

// isNativeTool reports whether cmd runs in-process as a bashy tool (so it is
// never a contained child).
func isNativeTool(cmd string) bool {
	nativeToolsOnce.Do(func() {
		nativeTools = map[string]bool{}
		for _, name := range atlas.ToolNames() {
			nativeTools[name] = true
		}
	})
	return nativeTools[baseName(cmd)]
}

// containedEffects removes `net` from an external child's effects inside a
// @contain(net: "deny") scope: the network is enforced away, not trusted away.
func containedEffects(ctx context.Context, cmd string, effects []string) []string {
	if !containNetFrom(ctx) || isNativeTool(cmd) || isBashyExecutable(cmd) {
		return effects
	}
	out := make([]string, 0, len(effects))
	for _, e := range effects {
		if e != atlas.EffNet {
			out = append(out, e)
		}
	}
	return out
}

// containHandler is the innermost rung: a command reaching it is about to be
// executed as an external child, so inside a @contain scope it is re-executed
// through `bashy contain --net deny --`.
func containHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || !containNetFrom(ctx) {
				return next(ctx, args)
			}
			wrapped := append([]string{bashySelfPath(), "contain", "--net", "deny", "--"}, args...)
			return next(ctx, wrapped)
		}
	}
}

func dispatchContain(args []string) int {
	usage := func(w io.Writer) {
		fmt.Fprintln(w, "usage: bashy contain --net deny -- command [args...]")
		fmt.Fprintln(w, "  run one command with OS-enforced network isolation (Linux network namespace, macOS Seatbelt)")
	}
	net := ""
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			usage(os.Stdout)
			return 0
		case a == "--net" && i+1 < len(args):
			i++
			net = args[i]
		case strings.HasPrefix(a, "--net="):
			net = strings.TrimPrefix(a, "--net=")
		case a == "--":
			i++
			goto command
		default:
			goto command
		}
	}
command:
	if net != "deny" || i >= len(args) {
		usage(os.Stderr)
		return 2
	}
	if err := containSupported(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	return runContained(args[i:])
}
