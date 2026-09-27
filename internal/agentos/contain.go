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
		const usage = `contain takes net: "deny" [, provider: builtin|native|image|custom], or image: "NAME@sha256:..." [, workdir:, out:, env: "A,B", ro: "HOST:CTR,...", provider: builtin|custom]`
		net, provider := "", ""
		img := &containImageSpec{}
		list := func(v string) []string {
			var out []string
			for _, f := range strings.Split(v, ",") {
				if f = strings.TrimSpace(f); f != "" {
					out = append(out, f)
				}
			}
			return out
		}
		for i, a := range args {
			switch {
			case (a.Name == "net" || a.Name == "" && i == 0) && net == "":
				net = a.Value
			case a.Name == "provider" && provider == "":
				provider = a.Value
			case a.Name == "image" && img.Image == "":
				img.Image = a.Value
			case a.Name == "workdir" && img.Workdir == "":
				img.Workdir = a.Value
			case a.Name == "out" && img.Out == "":
				img.Out = a.Value
			case a.Name == "env" && img.Env == nil:
				img.Env = list(a.Value)
			case a.Name == "ro" && img.RO == nil:
				img.RO = list(a.Value)
			default:
				return errors.New(usage)
			}
		}
		if img.Image != "" {
			if net != "" {
				return errors.New("contain: an image call has no network; a setup that needs one is a provider: custom")
			}
			// validated by runContainedImage, against the call's environment
			img.Provider = provider
			c.Status = runContainedCall(ctx, c, img, stderr)
			return nil
		}
		if img.Workdir != "" || img.Out != "" || img.Env != nil || img.RO != nil {
			return errors.New(usage)
		}
		if net != "deny" {
			return errors.New(usage)
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

// runContainedCall runs the decorated function's whole call inside the image:
// the script's own functions (never bashy's verb shims) travel as source, the
// call's own @contain line stays behind (it is what put the call there), and
// the arguments pass as "$@". Only what the declaration names crosses — no
// variables but env:, no host files but ro: and out:.
func runContainedCall(ctx context.Context, c *nativeDecoratorCall, img *containImageSpec, stderr io.Writer) int {
	dump, err := os.CreateTemp("", "bashy-contain-fns-")
	if err != nil {
		return containUnsupportedStatus
	}
	dump.Close()
	defer os.Remove(dump.Name())
	// The call's functions and its exported environment, from its own frame.
	envFile := dump.Name() + ".env"
	defer os.Remove(envFile)
	if st := c.Run(ctx, `declare -f > "$__bashy_contain_fns" && env -0 > "$__bashy_contain_env"`,
		map[string]string{"__bashy_contain_fns": dump.Name(), "__bashy_contain_env": envFile}); st != 0 {
		return containUnsupportedStatus
	}
	data, err := os.ReadFile(dump.Name())
	if err != nil {
		return containUnsupportedStatus
	}
	envData, err := os.ReadFile(envFile)
	if err != nil {
		return containUnsupportedStatus
	}
	img.Environ = map[string]string{}
	for _, kv := range strings.Split(string(envData), "\x00") {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			img.Environ[k] = v
		}
	}
	// bashy is mounted at /.bashy/bashy: a bare `bashy` inside the call finds it.
	script := "PATH=/.bashy:$PATH\n" + containScriptFunctions(string(data), c.Name) + "\n" + shellQuote(c.Name) + ` "$@"` + "\n"
	const srcSlot = "\x00src\x00"
	img.Argv = []string{"-c", srcSlot, c.Name}
	return runContainedImage(img, stderr, func(argv []string) int {
		// Run in the call's frame, so the function's redirections and its
		// arguments ("$@") apply; the script travels in a variable.
		words := make([]string, 0, len(argv)+1)
		for _, a := range argv {
			if a == srcSlot {
				words = append(words, `"$__bashy_contain_src"`)
				continue
			}
			words = append(words, shellQuote(a))
		}
		words = append(words, `"$@"`)
		return c.Run(ctx, strings.Join(words, " "), map[string]string{"__bashy_contain_src": script})
	})
}

// containScriptFunctions keeps the script's own functions from `declare -f`
// output: bashy's verb shims (a body that only runs the host's bashy) stay
// behind, and so do the decorator lines of the contained function itself.
func containScriptFunctions(dump, self string) string {
	var out, pending []string
	lines := strings.Split(dump, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "@") {
			pending = append(pending, line)
			continue
		}
		name, ok := strings.CutSuffix(strings.TrimRight(line, " "), " ()")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			pending = nil
			continue
		}
		block := []string{line}
		for i++; i < len(lines); i++ {
			block = append(block, lines[i])
			if lines[i] == "}" {
				break
			}
		}
		body := strings.TrimSpace(strings.Join(block[1:], "\n"))
		shim := strings.HasPrefix(body, "{") && strings.Count(body, "\n") == 2 &&
			strings.Contains(body, "command '") && strings.Contains(body, "bashy") && strings.HasSuffix(strings.TrimSpace(strings.TrimSuffix(body, "}")), `"$@"`)
		if !shim {
			if name != self {
				out = append(out, pending...)
			}
			out = append(out, block...)
		}
		pending = nil
	}
	return strings.Join(out, "\n")
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
