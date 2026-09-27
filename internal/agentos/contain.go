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
// Two backends (contain_container.go): native kernel primitives where they
// exist, else bashy's own image run with no network and only the working
// directory mounted. Whatever cannot be enforced fails closed (exit 125):
// nothing runs uncontained.
// ("sandbox" names the podman engine; this is deliberately a different word.)

const containUnsupportedStatus = 125

type containKey struct{}

// containScope is what a @contain(net: "deny"[, provider: P]) scope carries.
type containScope struct{ provider string }

// containNetFrom reports whether ctx is inside a @contain(net: "deny") scope.
func containNetFrom(ctx context.Context) bool {
	_, ok := ctx.Value(containKey{}).(containScope)
	return ok
}

// containProviderFrom is the scope's provider ("" = the host default).
func containProviderFrom(ctx context.Context) string {
	v, _ := ctx.Value(containKey{}).(containScope)
	return v.provider
}

func withContainNet(ctx context.Context, provider string) context.Context {
	return context.WithValue(ctx, containKey{}, containScope{provider: provider})
}

func containDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		if c.Advised != "" {
			return errors.New("contain is a declaration and never applied by advice")
		}
		net, provider := "", ""
		for i, a := range args {
			switch {
			case (a.Name == "net" || a.Name == "" && i == 0) && net == "":
				net = a.Value
			case a.Name == "provider" && provider == "":
				provider = a.Value
			default:
				return fmt.Errorf("contain takes net: %q and an optional provider: builtin|native|image|custom", "deny")
			}
		}
		if net != "deny" {
			return fmt.Errorf("contain takes net: %q and an optional provider: builtin|native|image|custom", "deny")
		}
		if provider != "" && !validContainProvider(provider) {
			return fmt.Errorf("contain: unknown provider %q (builtin, native, image, custom)", provider)
		}
		if err := containSupportedFor(provider); err != nil {
			c.Status = containUnsupportedStatus
			fmt.Fprintf(stderr, "%s: contain: %v\n", c.Name, err)
			return nil
		}
		c.Next(withContainNet(ctx, provider))
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
			wrapped := []string{bashySelfPath(), "contain", "--net", "deny"}
			if p := containProviderFrom(ctx); p != "" {
				wrapped = append(wrapped, "--provider", p)
			}
			wrapped = append(append(wrapped, "--"), args...)
			return next(ctx, wrapped)
		}
	}
}

func dispatchContain(args []string) int {
	usage := func(w io.Writer) {
		fmt.Fprintln(w, "usage: bashy contain --net deny [--provider builtin|native|image|custom] -- command [args...]")
		fmt.Fprintln(w, "  run one command with enforced network isolation: native (Linux network namespace,")
		fmt.Fprintln(w, "  macOS Seatbelt) where the OS has it, else bashy's own image with no network")
		fmt.Fprintln(w, "  providers: builtin (default: native where the OS has it, else bashy's image), native,")
		fmt.Fprintln(w, "  image (bashy's own image everywhere), custom (BASHY_CONTAIN_CUSTOM wrapper command).")
		fmt.Fprintln(w, "  BASHY_CONTAIN_PROVIDER sets the host default; BASHY_CONTAIN_IMAGE overrides the image.")
	}
	net, provider := "", ""
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
		case a == "--provider" && i+1 < len(args):
			i++
			provider = args[i]
		case strings.HasPrefix(a, "--provider="):
			provider = strings.TrimPrefix(a, "--provider=")
		case a == "--":
			i++
			goto command
		default:
			goto command
		}
	}
command:
	if net != "deny" || i >= len(args) || provider != "" && !validContainProvider(provider) {
		usage(os.Stderr)
		return 2
	}
	if err := containSupportedFor(provider); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	return runContainedWith(provider, args[i:])
}
