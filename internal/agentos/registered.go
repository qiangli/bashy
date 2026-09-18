package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/external/registry"
	"github.com/qiangli/yoke/pkg/assetring"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
)

// The registered-command ring (`bashy commands add`), as bashy consumes it.
//
// A registered command is the operator's own rung in the dispatch ladder —
// builtin → applet → front-door verb → REGISTERED → PATH — and it takes that
// rung in every mode, `--posix` included: resolution is shared, the agentic
// chrome (advisor, learn, audit, reduction) is not. The one exclusion is a
// certification run (VSC_PROFILE=cert), which must measure the host and never
// the operator's ring.
//
// Three rules, each enforced here rather than documented:
//
//   - A registered name may shadow a PATH program (that is the point) but
//     never a command bashy ships. `add` refuses the collision through
//     reservedCommandName, and a ring entry a NEWER bashy has since claimed
//     is reported as shadowed and skipped at dispatch — never silently won.
//   - The rung is innermost, after the coreutils applet handler, so every
//     observing/advising middleware sees argv[0] = the REGISTERED name, and a
//     record can never pre-empt an applet.
//   - The hot path never pays for an empty ring: the index loads lazily on
//     the first builtin/applet miss, is fingerprinted by the ring dirs'
//     mtimes, and re-checks at most once per registeredRecheck.

const registeredRecheck = 2 * time.Second

type registeredIndex struct {
	byName   map[string]fleet.Command // canonical name AND every alias
	shadowed map[string]string        // record name → holder that now owns it
	names    []string                 // visible canonical names, sorted
	hidden   []string                 // hidden canonical names, sorted
	fp       string
	checked  time.Time
}

var (
	registeredMu  sync.Mutex
	registeredIdx *registeredIndex
)

// registeredCatalog is the fleet catalog with bashy's holes filled: the
// command surface (for the collision filter) and the script syntax probe.
func registeredCatalog() *fleet.Catalog {
	return fleet.New(fleet.WithReservedNames(reservedCommandName), fleet.WithCommandProbe(scriptSyntaxProbe))
}

// registeredFingerprint is the ring dirs' mtimes: a write into any of them
// (add/set/rm rename a file into place) bumps the directory's mtime.
func registeredFingerprint(cat *fleet.Catalog) string {
	var b strings.Builder
	for _, d := range cat.CommandDirs() {
		fi, err := os.Stat(d)
		if err != nil {
			b.WriteString(d + ":-;")
			continue
		}
		fmt.Fprintf(&b, "%s:%d;", d, fi.ModTime().UnixNano())
	}
	return b.String()
}

func loadRegistered(cat *fleet.Catalog, fp string) *registeredIndex {
	idx := &registeredIndex{byName: map[string]fleet.Command{}, shadowed: cat.CommandShadows(), fp: fp, checked: time.Now()}
	cmds, _ := cat.Commands()
	for _, r := range cmds {
		if _, taken := idx.shadowed[r.Name]; taken {
			continue // reported by `commands list`/`verify`, never dispatched
		}
		if r.Mode() == "" {
			continue // a broken record is reported by the CRUD verbs, not run
		}
		for _, n := range r.Names() {
			idx.byName[n] = r
		}
		if r.Hidden {
			idx.hidden = append(idx.hidden, r.Name)
		} else {
			idx.names = append(idx.names, r.Name)
		}
	}
	sort.Strings(idx.names)
	sort.Strings(idx.hidden)
	return idx
}

// registeredIndexFor returns the cached index, reloading when the ring
// changed. fresh=true forces the fingerprint check (the listing/CRUD paths
// and every miss on the dispatch path); a hit never stats.
func registeredIndexFor(fresh bool) *registeredIndex {
	registeredMu.Lock()
	defer registeredMu.Unlock()
	if registeredIdx != nil && (!fresh || time.Since(registeredIdx.checked) < registeredRecheck) {
		return registeredIdx
	}
	cat := registeredCatalog()
	fp := registeredFingerprint(cat)
	if registeredIdx != nil && registeredIdx.fp == fp {
		registeredIdx.checked = time.Now()
		return registeredIdx
	}
	registeredIdx = loadRegistered(cat, fp)
	return registeredIdx
}

// resetRegisteredIndex drops the cache (tests; and the CRUD verbs after a write).
func resetRegisteredIndex() {
	registeredMu.Lock()
	registeredIdx = nil
	registeredMu.Unlock()
}

// registeredLookup resolves a registered command by name or alias. A hit
// costs a map read; a miss re-checks the ring at most once per
// registeredRecheck, so `commands add x` followed by `x` in the same shell
// resolves within that window.
func registeredLookup(name string) (fleet.Command, bool) {
	if name == "" || certProfile() {
		return fleet.Command{}, false
	}
	if r, ok := registeredIndexFor(false).byName[name]; ok {
		return r, true
	}
	r, ok := registeredIndexFor(true).byName[name]
	return r, ok
}

// registeredNames returns the visible registered command names, sorted.
func registeredNames() []string {
	if certProfile() {
		return nil
	}
	return append([]string(nil), registeredIndexFor(true).names...)
}

// registeredHiddenNames returns the hidden registered command names, sorted.
func registeredHiddenNames() []string {
	if certProfile() {
		return nil
	}
	return append([]string(nil), registeredIndexFor(true).hidden...)
}

// registeredShadowed reports the ring entries this bashy now ships a command
// for: record name → holder.
func registeredShadowed() map[string]string {
	out := map[string]string{}
	for k, v := range registeredIndexFor(true).shadowed {
		out[k] = v
	}
	return out
}

// certProfile reports a POSIX certification run (VSC_PROFILE=cert): the one
// mode that must measure the host and never the operator's ring — the same
// override output reduction honours.
func certProfile() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("VSC_PROFILE")), "cert")
}

// dispatchOnlyNames are front-door words the dispatch switch owns without an
// atlas or shim entry.
var dispatchOnlyNames = []string{"help", "serve", "steward", "conductor", "jobs", "fg", "bg", "kill", "login", "mirror"}

// reservedCommandName is the collision filter: does name already resolve to
// something bashy SHIPS, and what. It reads the static surfaces only —
// never the catalog that includes the ring itself — so the ring cannot
// reserve names from itself. The holder's class is what the refusal prints.
func reservedCommandName(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if interp.IsBuiltin(name) {
		return "bash builtin", true
	}
	if t := tool.Lookup(name); t != nil {
		if e, ok := atlas.Lookup(name); ok {
			return atlas.OriginLabel(e.Origin) + " applet", true
		}
		return "coreutils applet", true
	}
	for _, list := range [][]string{alwaysShimVerbs, directFrontDoorVerbs, agentModeShimVerbs, hiddenFrontDoorVerbs, curatedHiddenVerbs, {"docker", "sandbox"}, dispatchOnlyNames} {
		if slices.Contains(list, name) {
			return "yoke command (bashy " + name + ")", true
		}
	}
	if slices.Contains(registry.Names(), name) {
		return "bin-managed external (bashy " + name + ")", true
	}
	if e, ok := atlas.Lookup(name); ok {
		if e.AliasOf != "" {
			return "alias of " + e.AliasOf, true
		}
		return atlas.OriginLabel(e.Origin), true
	}
	if isEmbeddedSkillName(name) {
		return "embedded skill", true
	}
	if slices.Contains(fleet.ReservedCommandWords(), name) {
		return "a word bashy commands keeps for itself", true
	}
	return "", false
}

// registeredArgv resolves a record to the argv that runs it: a download
// record is provisioned first (cache-first; the record's digest is the trust
// root), an exec record splices the user args, a script body re-enters bashy.
func registeredArgv(ctx context.Context, rec fleet.Command, args []string) ([]string, error) {
	bin := ""
	if rec.Mode() == atlas.RegisteredDownload {
		p, err := rec.Ensure(ctx)
		if err != nil {
			return nil, err
		}
		bin = p
	}
	argv := rec.Argv(bashySelfPath(), bin, args)
	if len(argv) == 0 {
		return nil, fmt.Errorf("registered command %q has no runnable implementation", rec.Name)
	}
	return argv, nil
}

// registeredResolver is the ring's answer to the shell's OWN introspection —
// `type`, `command -v`, `command -V` — wired through interp.CommandResolver
// on both wireExec branches. Dispatch alone is not enough: a registered name
// RAN while `command -v NAME` said not found, so a script that probes before
// calling (the idiom every portable script uses) concluded the command was
// absent. The resolver reports the name exactly where the exec rung sits —
// after the shell's own names, before PATH — and stands down with
// registeredLookup under VSC_PROFILE=cert.
//
// Path is deliberately empty for every mode, so `command -v NAME` prints the
// bare NAME (the builtin/function shape): an exec record's argv[0] may carry
// baked arguments (`git log --oneline`) that `$(command -v gl)` would lose,
// and a download record's binary may not be provisioned yet. `type -t` says
// `file` — of bash's closed vocabulary, the one a script switching on it
// expects for something the shell hands off rather than runs itself.
func registeredResolver(name string) (interp.ResolvedCommand, bool) {
	rec, ok := registeredLookup(name)
	if !ok {
		return interp.ResolvedCommand{}, false
	}
	desc := fmt.Sprintf("%s is a bashy registered command (%s)", name, rec.Mode())
	if name != rec.Name {
		desc = fmt.Sprintf("%s is a bashy registered command (%s, alias of %s)", name, rec.Mode(), rec.Name)
	}
	return interp.ResolvedCommand{Desc: desc}, true
}

// registeredHandler is the innermost ExecHandler rung. It sits AFTER the
// coreutils applet handler on both wireExec branches, so an applet always
// wins and every middleware outside it has already seen the registered
// name. A miss falls through to the interpreter's PATH exec unchanged.
func registeredHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return next(ctx, args)
			}
			rec, ok := registeredLookup(args[0])
			if !ok {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			argv, err := registeredArgv(ctx, rec, args[1:])
			if err != nil {
				fmt.Fprintf(hc.Stderr, "bashy: %s: %v\n", args[0], err)
				return interp.ExitStatus(126)
			}
			if len(rec.Env) == 0 && rec.Cwd == "" {
				// The interpreter's own exec runs argv[0] (now an absolute path,
				// a PATH name, or bashy itself) with the handler's dir/env/stdio.
				return next(ctx, argv)
			}
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
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
	}
}

// handlerEnv flattens the interpreter environment to KEY=VALUE, the shape a
// spawned process observes (set variables only).
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

// runRegisteredFrontDoor runs `bashy NAME args…`: the same argv the rung
// builds, as a direct child with inherited stdio and status passthrough —
// the shape every bin-managed external's front door has.
func runRegisteredFrontDoor(rec fleet.Command, args []string) int {
	argv, err := registeredArgv(context.Background(), rec, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", rec.Name, err)
		return 126
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), rec.Env...)
	if rec.Cwd != "" {
		cmd.Dir = rec.Cwd
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", rec.Name, err)
		return 126
	}
	return 0
}

// scriptSyntaxProbe parses a script body without running it (`bashy -n -c
// BODY NAME`), for `commands verify`.
func scriptSyntaxProbe(rec fleet.Command) (string, bool) {
	if rec.Mode() != atlas.RegisteredScript {
		return "", true
	}
	argv := []string{bashySelfPath()}
	if rec.Dialect == fleet.DialectBash {
		argv = append(argv, "--no-bashpp")
	}
	argv = append(argv, "-n", "-c", rec.Script, rec.Name)
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return msg, false
	}
	return "", true
}

// registeredCommands returns every dispatchable record (fresh read), for
// the listing and diagnostic paths.
func registeredCommands() []fleet.Command {
	if certProfile() {
		return nil
	}
	idx := registeredIndexFor(true)
	seen := map[string]bool{}
	out := make([]fleet.Command, 0, len(idx.names)+len(idx.hidden))
	for _, n := range append(append([]string(nil), idx.names...), idx.hidden...) {
		if r, ok := idx.byName[n]; ok && !seen[r.Name] {
			seen[r.Name] = true
			out = append(out, r)
		}
	}
	return out
}

// registeredStatus is the doctor row for one record, network-free.
func registeredStatus(r fleet.Command) (status, detail string) {
	switch r.Mode() {
	case atlas.RegisteredExec:
		if p, ok := r.Executable(); ok {
			return "ok", "exec: " + p
		}
		return "warn", "exec: " + r.Exec[0] + " is not on PATH"
	case atlas.RegisteredDownload:
		st, detail := externalToolStatus(r.Name)
		return st, "download: " + detail
	case atlas.RegisteredScript:
		return "ok", "script (" + r.Dialect + ")"
	}
	return "warn", "no runnable implementation"
}

// registeredFeatureFields adds the ring-only fields to a one-command report:
// how the record runs, which ring it came from, and where it lives on disk.
func registeredFeatureFields(out map[string]any, name string) {
	rec, ok := registeredLookup(name)
	if !ok {
		return
	}
	out["resolver"] = "bashy-registered"
	out["mode"] = rec.Mode()
	out["ring"] = rec.Ring.String()
	if rec.Ring == assetring.RingLocal {
		if p, err := registeredCatalog().MaterializeCommand(rec.Name); err == nil {
			out["path"] = p
		}
	}
}
