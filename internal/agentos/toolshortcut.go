// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// `bashy <tool>` — run a registered agent CLI the way its operator would run
// it by hand, in this repo, plus yolo, as a steerable session.
//
// `bashy codex` (or agy, claude, muse, opencode, ...) opens that tool's own
// TUI in the foreground under the same steerable-session machinery as
// `bashy chat -i`: each launch gets its own session id, the tool name plus a
// short form of its instance UUID (codex-7f3a9c1e), so no later session
// inherits its inbox. `bashy chat steer codex "..."` reaches it from another
// terminal while it is the only live codex session, and the
// capped host-inbox relay stays on (BASHY_CHAT_INBOX=off opts out, as usual).
//
// The operator decision of record (2026-11-19): the launch follows the
// VENDOR's own defaults — cwd is $PWD, the model is whatever the tool's own
// config names unless --model selects a registry binding, the tool keeps its
// normal home/sign-in — plus permission bypass ON by default (--yolo
// semantics; --no-yolo keeps the vendor's own approval prompts). There is
// deliberately no second launcher here: after argument handling this
// delegates to the `bashy chat -i` path (runChatCmd), which owns the PTY,
// the control socket, the room card, and the inbox relay.
//
// Shape:
//   bashy <tool> [--model MODEL] [--yolo|--no-yolo] [PROMPT...]
//   bashy <tool> -- ARGS...          raw passthrough to the vendor CLI
//   bashy <tool> --help              resolved model, binding, steer id
//
// A TUI needs a terminal: without one stdin is refused with guidance instead
// of starting a session that exits at once. A second launch of the same tool
// is simply a second session with its own id.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/qiangli/yoke/pkg/agentlaunch"
	"github.com/qiangli/yoke/pkg/chat"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
)

// toolShortcutYoloDefault is the operator decision: permission bypass ON by
// default, --no-yolo to keep the vendor's prompts.
const toolShortcutYoloDefault = true

// hasSteerLaunch reports whether the tool declares an interactive steerable
// launch (steer_exec). The predicate is the fleet's own SteerArgvPrefix with
// no model bound: the ok return is false exactly when there is no steer
// template, and rendering with an empty model drops the orphaned model flag.
func hasSteerLaunch(t fleet.Tool) bool {
	_, ok := t.SteerArgvPrefix("")
	return ok
}

// steerToolNames lists every registered tool with a steer launch, sorted.
// Pure over the catalog so tests can point it at a scratch ring.
func steerToolNames(cat *fleet.Catalog) []string {
	if cat == nil {
		return nil
	}
	tools, _ := cat.Tools(false)
	var out []string
	for _, t := range tools {
		if hasSteerLaunch(t) {
			out = append(out, t.Name)
		}
	}
	sort.Strings(out)
	return out
}

// lookupSteerTool resolves a front-door word to its registered tool when it
// names one with a steer launch. Aliases resolve to their canonical tool.
func lookupSteerTool(name string) (fleet.Tool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return fleet.Tool{}, false
	}
	t, ok := fleet.New().Tool(name)
	if !ok || !hasSteerLaunch(t) {
		return fleet.Tool{}, false
	}
	return t, true
}

// toolShortcutSet caches the steer-tool names for isFrontDoorInvocation,
// which runs on every direct invocation. A CLI process is one dispatch, so
// the cache cannot go stale within a process; tests reset it around scratch
// rings (see resetToolShortcutCache).
var toolShortcutSetCache struct {
	mu   sync.Mutex
	set  map[string]bool
	done bool
}

func toolShortcutNameSet() map[string]bool {
	toolShortcutSetCache.mu.Lock()
	defer toolShortcutSetCache.mu.Unlock()
	if !toolShortcutSetCache.done {
		set := map[string]bool{}
		for _, n := range steerToolNames(fleet.New()) {
			set[n] = true
		}
		toolShortcutSetCache.set = set
		toolShortcutSetCache.done = true
	}
	return toolShortcutSetCache.set
}

func resetToolShortcutCache() {
	toolShortcutSetCache.mu.Lock()
	defer toolShortcutSetCache.mu.Unlock()
	toolShortcutSetCache.set = nil
	toolShortcutSetCache.done = false
}

// isToolShortcutName reports whether name opens a steerable tool session.
// Checked last in isFrontDoorInvocation so every static surface keeps
// precedence; `bashy ycode` never reaches here (its own front-door case).
func isToolShortcutName(name string) bool {
	if name == "" || name[0] == '-' || name[0] == '+' ||
		strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) {
		return false
	}
	return toolShortcutNameSet()[name]
}

// toolShortcutAgentName is the chat agent addressed by the shortcut: the
// bare tool (the vendor's own default model) or a tool:model binding when
// --model selects one.
func toolShortcutAgentName(toolName, model string) string {
	toolName = strings.TrimSpace(toolName)
	if m := strings.TrimSpace(model); m != "" {
		return toolName + ":" + m
	}
	return toolName
}

// toolShortcutSessionID is the base of the shortcut's session id: the tool
// name (or tool:model), sanitized exactly the way chat derives it. The live
// id is this plus a per-launch suffix, so `bashy chat steer <base>` reaches
// the session while it is the only one live on that base.
func toolShortcutSessionID(agent string) string {
	return room.AgentClaimID(agent)
}

// toolShortcutOpts are the parsed `bashy <tool>` arguments.
type toolShortcutOpts struct {
	model       string
	yolo        bool
	noYolo      bool // explicit --no-yolo: outranks containment and the env (agentlaunch DenyUnsafe)
	yoloSet     bool
	help        bool
	explain     bool
	dryRun      bool
	passthrough []string // set when -- was present; everything after it
	prompt      []string
}

func toolShortcutUsage(tool string) string {
	return fmt.Sprintf(`Usage:
  bashy %[1]s [--model MODEL] [--yolo|--no-yolo] [PROMPT...]
  bashy %[1]s -- ARGS...
  bashy %[1]s --help

Open %[1]s's own TUI in this repo as a steerable session (like running %[1]s
by hand, plus yolo): the vendor's own default model and config, permission
bypass on. Each launch gets its own session id, %[1]s-<8 hex> (shown in the
launch banner and by bashy chat sessions). While one %[1]s session is live,
the tool name reaches it: bashy chat steer %[1]s "...".

  --model MODEL   run a registry tool:model binding instead of the vendor default
  --yolo          keep permission bypass on (the default)
  --no-yolo       keep the vendor's own approval prompts
  -- ARGS...      raw passthrough to the vendor CLI (no session, no governance)
  --help          show the resolved model, binding and steer id
  --explain, --dry-run
                  print the resolved launch without starting it
`, tool)
}

// parseToolShortcutArgs splits `bashy <tool>` arguments. The first -- ends
// option parsing; everything after it is a raw vendor passthrough.
func parseToolShortcutArgs(args []string) (toolShortcutOpts, error) {
	opts := toolShortcutOpts{yolo: toolShortcutYoloDefault}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			opts.passthrough = append([]string(nil), args[i+1:]...)
			break
		}
		if m, ok := strings.CutPrefix(a, "--model="); ok {
			if strings.TrimSpace(m) == "" {
				return opts, fmt.Errorf("--model requires a value")
			}
			opts.model = m
			continue
		}
		switch a {
		case "--model":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--model requires a value")
			}
			i++
			if strings.TrimSpace(args[i]) == "" {
				return opts, fmt.Errorf("--model requires a value")
			}
			opts.model = args[i]
		case "--yolo", "--no-yolo":
			if opts.yoloSet {
				return opts, fmt.Errorf("--yolo and --no-yolo are mutually exclusive")
			}
			opts.yoloSet = true
			opts.yolo = args[i] == "--yolo"
			opts.noYolo = !opts.yolo
		case "--help", "-h":
			opts.help = true
		case "--explain":
			opts.explain = true
		case "--dry-run":
			opts.dryRun = true
		default:
			if strings.HasPrefix(a, "-") {
				return opts, fmt.Errorf("unknown flag %q", a)
			}
			opts.prompt = append(opts.prompt, a)
		}
	}
	return opts, nil
}

// checkToolShortcutTTY refuses a session launch without a real terminal,
// with the guidance an agent hitting this from a `!` shell needs, instead of
// starting a TUI that exits at once. Pure over the stdin file so tests can
// drive it with pipes.
func checkToolShortcutTTY(tool string, stdin *os.File) error {
	if stdin != nil && term.IsTerminal(int(stdin.Fd())) {
		return nil
	}
	return fmt.Errorf("bashy %s needs a terminal: stdin is not a TTY, so the tool's TUI would exit at once\n"+
		"  run it where you sit (a terminal), or with the vendor CLI directly: %s\n"+
		"  non-interactive work goes through: bashy chat --agent %s -m \"...\"",
		tool, tool, tool)
}

// toolShortcutHelp renders --help: usage plus the resolved model, binding,
// session id and steer commands.
func toolShortcutHelp(w io.Writer, t fleet.Tool, opts toolShortcutOpts) {
	agent := toolShortcutAgentName(t.Name, opts.model)
	model := "(vendor default)"
	if m := strings.TrimSpace(opts.model); m != "" {
		model = m
	}
	id := toolShortcutSessionID(agent)
	fmt.Fprint(w, toolShortcutUsage(t.Name))
	fmt.Fprintf(w, `Resolved:
  tool:     %s
  binding:  %s
  model:    %s
  session:  %s-<8 hex>, new per launch
  vendor:   %s
Steer it: bashy chat steer %s "..."   (watch: bashy chat attach %s)
With several live, use the full id: bashy chat sessions
`, t.Name, agent, model, id, t.Binary(), id, id)
}

// toolShortcutExplain resolves the steer launch exactly as the session would
// run it (dry-run: no side effects) and prints the argv, for --explain and
// --dry-run.
func toolShortcutExplain(w io.Writer, agent string, yolo, noYolo bool) error {
	l, err := agentlaunch.Resolve(agent, agentlaunch.Options{
		Steer: true, Attended: true, AllowUnsafe: yolo, DenyUnsafe: noYolo, DryRun: true,
	})
	if err != nil {
		return err
	}
	// A vendor-default launch (no model selected) carries no binding; show
	// the agent addressed, which is what `chat steer` needs.
	binding := l.Binding()
	if strings.TrimSpace(binding) == "" {
		binding = agent
	}
	id := toolShortcutSessionID(agent)
	argv := append([]string{l.Tool}, l.Args...)
	fmt.Fprintf(w, "tool: %s\nmodel: %s\nbinding: %s\nsession: %s-<8 hex>, new per launch\nargv: %s\n",
		l.ToolName, modelOrDefault(l.Model), binding, id, shellJoin(argv))
	fmt.Fprintf(w, "Steer it: bashy chat steer %s \"...\"   (watch: bashy chat attach %s)\n", id, id)
	return nil
}

func modelOrDefault(m string) string {
	if strings.TrimSpace(m) == "" {
		return "(vendor default)"
	}
	return m
}

// runToolPassthrough execs the vendor binary with the caller's stdio and
// environment, propagating signals and the exit status — `bashy <tool> --`
// is the vendor CLI, not a session.
func runToolPassthrough(bin string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "bashy: %s: %v\n", bin, err)
		return 127
	}
	stop := forwardSignals(cmd.Process)
	err := cmd.Wait()
	stop()
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() >= 0 {
		return exit.ExitCode()
	}
	return 1
}

// dispatchToolShortcut runs `bashy <tool> ...`. toolName is the canonical
// registry name (aliases resolve before this is called).
func dispatchToolShortcut(toolName string, args []string, stdin *os.File, stdout, stderr io.Writer) int {
	t, ok := lookupSteerTool(toolName)
	if !ok {
		fmt.Fprintf(stderr, "bashy %s: no registered steerable tool\n", toolName)
		return 1
	}
	opts, err := parseToolShortcutArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "bashy %s: %v\n\n%s", t.Name, err, toolShortcutUsage(t.Name))
		return 2
	}
	if opts.help {
		toolShortcutHelp(stdout, t, opts)
		return 0
	}
	agent := toolShortcutAgentName(t.Name, opts.model)
	if opts.explain || opts.dryRun {
		if err := toolShortcutExplain(stdout, agent, opts.yolo, opts.noYolo); err != nil {
			fmt.Fprintf(stderr, "bashy %s: %v\n", t.Name, err)
			return 1
		}
		return 0
	}
	if opts.passthrough != nil {
		// Raw passthrough to the vendor CLI: the binary on PATH, exactly
		// what the operator would run by hand. Only when nothing on PATH
		// (or the user bins) answers do we fall back to the session's own
		// resolved binary (the managed pin, computed without side
		// effects) — a managed-only tool must still answer `-- --version`.
		bin := chat.ResolveToolBinary(t.Binary())
		if bin == "" {
			if l, err := agentlaunch.Resolve(agent, agentlaunch.Options{
				Steer: true, Attended: true, AllowUnsafe: opts.yolo, DryRun: true,
			}); err == nil && strings.TrimSpace(l.Tool) != "" {
				bin = l.Tool
			} else {
				bin = t.Binary()
			}
		}
		return runToolPassthrough(bin, opts.passthrough, stdin, stdout, stderr)
	}
	if err := checkToolShortcutTTY(t.Name, stdin); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// The yolo default is the operator decision; --yolo is passed through
	// explicitly so the intent is visible in process listings. Note yoke
	// policy applies on top: codex launches default to full access fleet-wide
	// (agentlaunch.FinalizeArgs codexDefaultYolo), so --no-yolo alone does not
	// restore codex's prompts — same as `bashy chat -i --agent codex`.
	chatArgs := []string{"-i", "--agent", agent}
	switch {
	case opts.yolo:
		chatArgs = append(chatArgs, "--yolo")
	case opts.noYolo:
		// The command-line flag is the most specific setting: it must beat
		// containment and $BASHY_ALLOW_UNSAFE_AGENT_LAUNCH, so pass it on
		// explicitly rather than merely omitting --yolo.
		chatArgs = append(chatArgs, "--no-yolo")
	}
	// The session is always interactive (-i above): positional words are its
	// opening instruction, delivered after the TUI settles. The -- separator
	// keeps a leading-dash prompt from parsing as a chat flag.
	if len(opts.prompt) > 0 {
		chatArgs = append(chatArgs, "--")
		chatArgs = append(chatArgs, opts.prompt...)
	}
	return runChatCmd(chatArgs, stdout, stderr)
}
