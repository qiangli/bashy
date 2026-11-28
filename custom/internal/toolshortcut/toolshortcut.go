// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package toolshortcut

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/bash/pathconv"
	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/pkg/agentlaunch"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/room"
	"golang.org/x/term"
)

const toolShortcutYoloDefault = true

type toolShortcutOpts struct {
	model       string
	yolo        bool
	noYolo      bool
	yoloSet     bool
	help        bool
	explain     bool
	dryRun      bool
	passthrough []string
	prompt      []string
}

func hasSteerLaunch(t fleet.Tool) bool {
	_, ok := t.SteerArgvPrefix("")
	return ok
}

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

func toolShortcutAgentName(toolName, model string) string {
	toolName = strings.TrimSpace(toolName)
	if m := strings.TrimSpace(model); m != "" {
		return toolName + ":" + m
	}
	return toolName
}

func toolShortcutSessionID(agent string) string {
	return room.AgentClaimID(agent)
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

func checkToolShortcutTTY(tool string, stdin *os.File) error {
	if stdin != nil && term.IsTerminal(int(stdin.Fd())) {
		return nil
	}
	return fmt.Errorf("bashy %s needs a terminal: stdin is not a TTY, so the tool's TUI would exit at once\n"+
		"  run it where you sit (a terminal), or with the vendor CLI directly: %s\n"+
		"  non-interactive work goes through: bashy chat --agent %s -m \"...\"",
		tool, tool, tool)
}

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

func modelOrDefault(m string) string {
	if strings.TrimSpace(m) == "" {
		return "(vendor default)"
	}
	return m
}

func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a == "" {
			parts[i] = "''"
			continue
		}
		if strings.IndexAny(a, " \t\n'\"\\$`!&|;<>()*?[]{}#~") >= 0 {
			parts[i] = "'" + strings.ReplaceAll(a, "'", "'\\''") + "'"
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}

func toolShortcutExplain(w io.Writer, agent string, yolo, noYolo bool) error {
	l, err := agentlaunch.Resolve(agent, agentlaunch.Options{
		Steer: true, Attended: true, AllowUnsafe: yolo, DenyUnsafe: noYolo, DryRun: true,
	})
	if err != nil {
		return err
	}
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

func resolveToolBinary(tool string) string {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return ""
	}
	if strings.ContainsRune(tool, filepath.Separator) || strings.ContainsRune(tool, '/') {
		if runtime.GOOS == "windows" {
			return pathconv.NativePath(tool)
		}
		return tool
	}
	if p, err := exec.LookPath(tool); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	if p := lookInPathList(tool, os.Getenv("PATH"), runtime.GOOS == "windows", os.Getenv("PATHEXT")); p != "" {
		return p
	}
	return lookInUserBins(tool)
}

func userBinDirs() []string {
	var dirs []string
	add := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" {
			return
		}
		for _, have := range dirs {
			if have == d {
				return
			}
		}
		dirs = append(dirs, d)
	}
	add(os.Getenv("DHNT_BIN_DIR"))
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dirs
	}
	add(filepath.Join(home, ".local", "bin"))
	add(filepath.Join(home, "bin"))
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		add(gobin)
	} else if gopath := os.Getenv("GOPATH"); gopath != "" {
		add(filepath.Join(gopath, "bin"))
	} else {
		add(filepath.Join(home, "go", "bin"))
	}
	return dirs
}

func executableFile(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}

func lookInUserBins(tool string) string {
	exts := pathExts(tool, runtime.GOOS == "windows", os.Getenv("PATHEXT"))
	for _, dir := range userBinDirs() {
		for _, ext := range exts {
			p := filepath.Join(dir, tool+ext)
			if executableFile(p) {
				return p
			}
		}
	}
	return ""
}

func lookInPathList(tool, pathValue string, windows bool, pathext string) string {
	exts := pathExts(tool, windows, pathext)
	for _, dir := range pathconv.SplitPathList(pathValue, windows) {
		if dir == "" {
			continue
		}
		if windows {
			dir = pathconv.NativePath(dir)
		}
		for _, ext := range exts {
			p := filepath.Join(dir, tool+ext)
			if executableFile(p) {
				return p
			}
		}
	}
	return ""
}

func pathExts(tool string, windows bool, pathext string) []string {
	if !windows {
		return []string{""}
	}
	var exts []string
	for _, e := range filepath.SplitList(pathext) {
		exts = append(exts, strings.ToLower(e))
	}
	if len(exts) == 0 {
		exts = []string{".exe", ".bat", ".cmd"}
	}
	if filepath.Ext(tool) != "" {
		exts = append([]string{""}, exts...)
	}
	return exts
}

func runExecPassthrough(bin string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "bashy: %s: %v\n", bin, err)
		return 127
	}
	stop := adapter.ForwardSignals(cmd.Process)
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

// RunToolShortcut runs the steerable agent CLI adapter using BASHY_INVOKED_AS
// (defaulting to defaultTool) and os.Args[1:].
func RunToolShortcut(defaultTool string, args []string) int {
	return DispatchToolShortcut(adapter.InvokedAs(defaultTool), args, os.Stdin, os.Stdout, os.Stderr)
}

// DispatchToolShortcut implements `bashy <tool> ...` with identical UX to
// internal/agentos/toolshortcut.go, crossing the process boundary to
// `bashy chat` when launching an interactive steerable session.
func DispatchToolShortcut(toolName string, args []string, stdin *os.File, stdout, stderr io.Writer) int {
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
		bin := resolveToolBinary(t.Binary())
		if bin == "" {
			if l, err := agentlaunch.Resolve(agent, agentlaunch.Options{
				Steer: true, Attended: true, AllowUnsafe: opts.yolo, DryRun: true,
			}); err == nil && strings.TrimSpace(l.Tool) != "" {
				bin = l.Tool
			} else {
				bin = t.Binary()
			}
		}
		return runExecPassthrough(bin, opts.passthrough, stdin, stdout, stderr)
	}
	if err := checkToolShortcutTTY(t.Name, stdin); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	chatArgs := []string{"chat", "-i", "--agent", agent}
	switch {
	case opts.yolo:
		chatArgs = append(chatArgs, "--yolo")
	case opts.noYolo:
		chatArgs = append(chatArgs, "--no-yolo")
	}
	if len(opts.prompt) > 0 {
		chatArgs = append(chatArgs, "--")
		chatArgs = append(chatArgs, opts.prompt...)
	}
	self := adapter.BashySelf()
	if self == "" {
		self = "bashy"
	}
	return runExecPassthrough(self, chatArgs, stdin, stdout, stderr)
}
