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
		const usage = `contain takes net: "deny" [, provider: builtin|native|image|custom], or image: "NAME@sha256:..." [, net: "deny"|"door", sticky:, workdir:, out:, env: "A,B", ro: "HOST:CTR,..."]`
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
			case a.Name == "sticky" && img.Sticky == "":
				img.Sticky = a.Value
			case a.Name == "env" && img.Env == nil:
				img.Env = list(a.Value)
			case a.Name == "ro" && img.RO == nil:
				img.RO = list(a.Value)
			default:
				return errors.New(usage)
			}
		}
		if img.Image != "" {
			if provider != "" {
				return errors.New("contain: provider does not apply to image: (the image is the provider)")
			}
			img.Net, img.Argv = net, []string{"true"}
			if err := img.validate(); err != nil {
				c.Status = 2
				fmt.Fprintf(stderr, "%s: contain: %v\n", c.Name, err)
				return nil
			}
			c.Status = runContainedCall(ctx, c, img)
			return nil
		}
		if img.Workdir != "" || img.Out != "" || img.Sticky != "" || img.Env != nil || img.RO != nil {
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
func runContainedCall(ctx context.Context, c *nativeDecoratorCall, img *containImageSpec) int {
	dump, err := os.CreateTemp("", "bashy-contain-fns-")
	if err != nil {
		return containUnsupportedStatus
	}
	dump.Close()
	defer os.Remove(dump.Name())
	if st := c.Run(ctx, `declare -f > "$__bashy_contain_fns"`, map[string]string{"__bashy_contain_fns": dump.Name()}); st != 0 {
		return containUnsupportedStatus
	}
	data, err := os.ReadFile(dump.Name())
	if err != nil {
		return containUnsupportedStatus
	}
	script := containScriptFunctions(string(data), c.Name) + "\n" + shellQuote(c.Name) + ` "$@"` + "\n"
	words := []string{shellQuote(bashySelfPath()), "contain", "--image", shellQuote(img.Image), "--net", shellQuote(img.Net)}
	for flag, v := range map[string]string{"--workdir": img.Workdir, "--out": img.Out, "--sticky": img.Sticky} {
		if v != "" {
			words = append(words, flag, shellQuote(v))
		}
	}
	for _, n := range img.Env {
		words = append(words, "--env", shellQuote(n))
	}
	for _, m := range img.RO {
		words = append(words, "--ro", shellQuote(m))
	}
	words = append(words, "--", "bashy", "-c", `"$__bashy_contain_src"`, shellQuote(c.Name), `"$@"`)
	return c.Run(ctx, strings.Join(words, " "), map[string]string{"__bashy_contain_src": script})
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
		fmt.Fprintln(w, "       bashy contain --image NAME@sha256:<digest> [--workdir DIR] [--out HOSTDIR] [--net deny|door]")
		fmt.Fprintln(w, "                     [--sticky KEY] [--env NAME]... [--ro HOST:CTR]... -- command [args...]")
		fmt.Fprintln(w, "       bashy contain pin NAME[:TAG]")
		fmt.Fprintln(w, "  --net deny: run one command with enforced network isolation: native (Linux network")
		fmt.Fprintln(w, "  namespace, macOS Seatbelt) where the OS has it, else bashy's own image with no network.")
		fmt.Fprintln(w, "  providers: builtin (default: native where the OS has it, else bashy's image), native,")
		fmt.Fprintln(w, "  image (bashy's own image everywhere), custom (BASHY_CONTAIN_CUSTOM wrapper command).")
		fmt.Fprintln(w, "  BASHY_CONTAIN_PROVIDER sets the host default; BASHY_CONTAIN_IMAGE overrides the image.")
		fmt.Fprintln(w, "  --image: run the command as ONE contained call in a third-party image pinned by digest")
		fmt.Fprintln(w, "  (`contain pin` resolves a tag once): bashy mounted read-only is the entrypoint, one")
		fmt.Fprintln(w, "  container for the whole call, no host mounts but --ro (read-only) and --out (at /out),")
		fmt.Fprintln(w, "  no network; --net door opens only the host's model door on 127.0.0.1:24556, restricted")
		fmt.Fprintln(w, "  to the sticky binding --sticky KEY (the token never enters the container; linux hosts).")
		fmt.Fprintln(w, "  BASHY_CONTAIN_BASHY names the static linux bashy to inject; BASHY_CONTAIN_DOOR an upstream door.")
	}
	if len(args) > 0 && args[0] == "pin" {
		return containPin(args[1:])
	}
	if len(args) > 0 && args[0] == "--init" {
		return containInit(args[1:])
	}
	net, provider := "", ""
	img := &containImageSpec{}
	value := func(i *int, a, flag string) (string, bool) {
		if a == flag && *i+1 < len(args) {
			*i++
			return args[*i], true
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v, true
		}
		return "", false
	}
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			usage(os.Stdout)
			return 0
		}
		if a == "--" {
			i++
			break
		}
		if v, ok := value(&i, a, "--net"); ok {
			net = v
		} else if v, ok := value(&i, a, "--provider"); ok {
			provider = v
		} else if v, ok := value(&i, a, "--image"); ok {
			img.Image = v
		} else if v, ok := value(&i, a, "--workdir"); ok {
			img.Workdir = v
		} else if v, ok := value(&i, a, "--out"); ok {
			img.Out = v
		} else if v, ok := value(&i, a, "--sticky"); ok {
			img.Sticky = v
		} else if v, ok := value(&i, a, "--env"); ok {
			img.Env = append(img.Env, v)
		} else if v, ok := value(&i, a, "--ro"); ok {
			img.RO = append(img.RO, v)
		} else {
			break
		}
	}
	if img.Image != "" {
		if provider != "" {
			fmt.Fprintln(os.Stderr, "bashy contain: --provider does not apply to --image (the image is the provider)")
			return 2
		}
		img.Net, img.Argv = net, args[i:]
		return runContainedImage(img)
	}
	if img.Workdir != "" || img.Out != "" || img.Sticky != "" || len(img.Env) > 0 || len(img.RO) > 0 {
		fmt.Fprintln(os.Stderr, "bashy contain: --workdir/--out/--sticky/--env/--ro need --image")
		return 2
	}
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
