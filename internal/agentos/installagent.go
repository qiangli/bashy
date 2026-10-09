// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/chat"
)

// install-agent wires a coding agent to use bashy as its shell. Each agent
// has a different (verified) selection surface — see
// docs/agent-adoption/matrix.md for the per-agent verification status:
//
//	claude    CLAUDE_CODE_SHELL via settings.json "env" (unix; E2E verified)
//	opencode  "shell": "<path>" in opencode.json (STRING form; E2E verified)
//	aider     $SHELL environment variable (pexpect -i -c PTY shape verified)
//	gemini    PATH shim dir (~/.bashy/shims) — bare-name `bash -c` spawn
//	copilot   PATH shim dir (~/.bashy/shims)
//	codex     no override surface on macOS (spawns /bin/zsh by absolute
//	          path, verified v0.142.5) — upstream work tracked
//
// The written value defaults to the running binary itself (bashy IS a bash);
// override with --shell.

type agentInstaller struct {
	name string
	// install writes the agent's config to use shellPath; returns a
	// human summary of what was done (or guidance when nothing can be
	// written).
	install func(shellPath string, project bool) (string, error)
	// uninstall reverses install.
	uninstall func(project bool) (string, error)
	// check verifies the wiring end-to-end as far as possible without
	// spending agent/LLM invocations.
	check func(shellPath string, project bool) error
}

func dispatchInstallAgent(args []string) int {
	fs := flag.NewFlagSet("install-agent", flag.ExitOnError)
	shellPath := fs.String("shell", "", "shell binary to install (default: this bashy binary)")
	project := fs.Bool("project", false, "write project-level config instead of user-level (claude: .claude/settings.json; opencode: ./opencode.json)")
	check := fs.Bool("check", false, "verify the wiring instead of writing it (static; no LLM call)")
	probe := fs.Bool("probe", false, "verify LIVE: run the agent once and confirm its shell is bashy (spends one LLM call)")
	uninstall := fs.Bool("uninstall", false, "reverse a previous install")
	yes := fs.Bool("yes", false, "perform invasive steps without prompting (codex: attempt chsh)")
	mcpMode := fs.Bool("mcp", false, "register `bashy mcp serve` with the agent instead of wiring its shell")
	hooksMode := fs.Bool("hooks", false, "install turn-boundary inbox unread hooks (SessionStart + UserPromptSubmit) so the agent sees waiting mail within one turn")
	asName := fs.String("as", "", "with --hooks: bake this registered bashy identity into the hook command (same as `bashy inbox --as`); without it the hook stays silent unless the session is attributable")
	dryRun := fs.Bool("dry-run", false, "with --mcp or --hooks: print the entry instead of writing it")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: bashy install-agent <agent> [--shell PATH] [--project] [--check] [--uninstall]
       bashy install-agent <agent> --mcp [--project] [--dry-run] [--uninstall]
       bashy install-agent <agent> --hooks [--as NAME] [--project] [--dry-run] [--check] [--uninstall]

Wire a coding agent to use bashy as its shell.

agents:
  claude     Claude Code   settings.json env.CLAUDE_CODE_SHELL   (E2E verified)
  opencode   OpenCode      opencode.json "shell": "<path>"       (E2E verified)
  aider      Aider         $SHELL guidance (no config file)
  gemini     Gemini CLI    PATH shim dir (~/.bashy/shims)
  copilot    Copilot CLI   PATH shim dir (~/.bashy/shims)
  agy        Antigravity   PATH shim dir (~/.bashy/shims)
  codex      Codex CLI     login shell via chsh (reads /etc/passwd; invasive)

--hooks installs the native turn-boundary inbox hook both harnesses support
(claude: SessionStart + UserPromptSubmit settings hooks; codex: SessionStart +
UserPromptSubmit inline hooks in config.toml). The hook runs bashy inbox-hook
read-only: one model-visible unread hint when mail waits, silence otherwise,
and it never consumes mail. This is not the MCP skill export and not generic
notify: it is the inbox turn hook. Codex lists new hooks for trust review
(/hooks) before they first run; approve them there (or run that session with
--dangerously-bypass-hook-trust).

--mcp registers "bashy mcp serve" as an MCP server with the agent (claude: via
the claude CLI "mcp add" command; codex: ~/.codex/config.toml mcp_servers;
opencode: opencode.json "mcp"). --dry-run prints the entry instead of writing
it; other agents have no MCP config writer.

Note: bashy meet/chat/weave already force-inject the shell for spawned agents
(SHELL + PATH shim + CLAUDE_CODE_SHELL) — this command makes the wiring durable
for direct/interactive use. --probe runs the agent LIVE to confirm bashy is used.

With no agent, prints the wiring status of every known agent.
`)
	}
	// Accept the agent name anywhere among the flags (`install-agent claude
	// --check` and `install-agent --check claude` both work): Go's flag
	// package stops at the first positional, so pull the name out before
	// parsing. --shell and --as are the value-taking flags; keep their values
	// with them so `--as NAME` is never mistaken for the agent name.
	var name string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--shell" || a == "-shell" || a == "--as" || a == "-as" {
			rest = append(rest, a)
			if i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if name == "" && !strings.HasPrefix(a, "-") {
			name = a
			continue
		}
		rest = append(rest, a)
	}
	if err := fs.Parse(rest); err != nil {
		return 2
	}

	shell := *shellPath
	if shell == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: cannot resolve own path: %v\n", err)
			return 1
		}
		shell = exe
	}
	if abs, err := filepath.Abs(shell); err == nil {
		shell = abs
	}
	if _, err := os.Stat(shell); err != nil {
		fmt.Fprintf(os.Stderr, "install-agent: shell binary not found: %s\n", shell)
		return 1
	}

	installers := agentInstallers(*yes)
	if name == "" {
		printAgentStatus(installers, shell)
		return 0
	}
	ins, ok := installers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "install-agent: unknown agent %q (try: claude opencode aider gemini copilot codex agy)\n", name)
		return 2
	}

	if *mcpMode && *hooksMode {
		fmt.Fprint(os.Stderr, "install-agent: --mcp cannot be combined with --hooks\n")
		return 2
	}
	if *mcpMode {
		return dispatchInstallAgentMCP(name, *project, *dryRun, *uninstall, *check, *probe)
	}
	if *hooksMode {
		if *probe {
			fmt.Fprint(os.Stderr, "install-agent: --hooks cannot be combined with --probe (verify delivery by invoking the installed hook command with a hook payload; see the Sprint 321 recipe)\n")
			return 2
		}
		if name == "" {
			fmt.Fprint(os.Stderr, "install-agent: --hooks needs an agent (try: claude codex)\n")
			return 2
		}
		return dispatchInstallAgentHooks(name, shell, strings.TrimSpace(*asName), *project, *dryRun, *uninstall, *check)
	}
	if *dryRun {
		fmt.Fprint(os.Stderr, "install-agent: --dry-run requires --mcp or --hooks\n")
		return 2
	}

	switch {
	case *probe:
		if err := probeAgentLive(name, shell); err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s: PROBE FAILED: %v\n", name, err)
			return 1
		}
		fmt.Printf("install-agent: %s: PROBE OK — agent ran its shell under bashy\n", name)
		return 0
	case *check:
		if err := ins.check(shell, *project); err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s: CHECK FAILED: %v\n", name, err)
			return 1
		}
		fmt.Printf("install-agent: %s: OK\n", name)
		return 0
	case *uninstall:
		msg, err := ins.uninstall(*project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s: %v\n", name, err)
			return 1
		}
		fmt.Println(msg)
		return 0
	default:
		msg, err := ins.install(shell, *project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s: %v\n", name, err)
			return 1
		}
		fmt.Println(msg)
		return 0
	}
}

func agentInstallers(yes bool) map[string]agentInstaller {
	return map[string]agentInstaller{
		"claude":      claudeInstaller(),
		"opencode":    opencodeInstaller(),
		"aider":       aiderInstaller(),
		"gemini":      shimInstaller("gemini"),
		"copilot":     shimInstaller("copilot"),
		"agy":         shimInstaller("agy"),
		"antigravity": shimInstaller("antigravity"),
		"codex":       codexInstaller(yes),
	}
}

// --- claude ---------------------------------------------------------------

func claudeSettingsPath(project bool) string {
	if project {
		return filepath.Join(".claude", "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func claudeInstaller() agentInstaller {
	return agentInstaller{
		name: "claude",
		install: func(shell string, project bool) (string, error) {
			if runtime.GOOS == "windows" {
				return "", fmt.Errorf("CLAUDE_CODE_SHELL is ignored by Claude Code on Windows; the Windows path is CLAUDE_CODE_GIT_BASH_PATH=%s (experimental — see docs/agent-adoption/matrix.md)", shell)
			}
			path := claudeSettingsPath(project)
			err := mergeJSONFile(path, func(m map[string]any) {
				env, _ := m["env"].(map[string]any)
				if env == nil {
					env = map[string]any{}
				}
				env["CLAUDE_CODE_SHELL"] = shell
				m["env"] = env
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("claude: wrote env.CLAUDE_CODE_SHELL=%s to %s (restart claude to pick it up)", shell, path), nil
		},
		uninstall: func(project bool) (string, error) {
			path := claudeSettingsPath(project)
			err := mergeJSONFile(path, func(m map[string]any) {
				if env, ok := m["env"].(map[string]any); ok {
					delete(env, "CLAUDE_CODE_SHELL")
					if len(env) == 0 {
						delete(m, "env")
					}
				}
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("claude: removed env.CLAUDE_CODE_SHELL from %s", path), nil
		},
		check: func(shell string, project bool) error {
			path := claudeSettingsPath(project)
			m, err := readJSONFile(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			env, _ := m["env"].(map[string]any)
			got, _ := env["CLAUDE_CODE_SHELL"].(string)
			if got == "" {
				return fmt.Errorf("%s has no env.CLAUDE_CODE_SHELL", path)
			}
			// Claude Code generates its rc snapshot as `bash -c -l '<script>'`
			// (options after -c) — verify the configured shell handles that
			// exact shape, then the plain per-command shape.
			if err := probeShell(got, "-c", "-l", "echo ok"); err != nil {
				return fmt.Errorf("snapshot shape (-c -l) failed: %w", err)
			}
			return probeShell(got, "-c", "echo ok")
		},
	}
}

// --- opencode --------------------------------------------------------------

func opencodeConfigPath(project bool) string {
	if project {
		return "opencode.json"
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "opencode", "opencode.json")
}

func opencodeInstaller() agentInstaller {
	return agentInstaller{
		name: "opencode",
		install: func(shell string, project bool) (string, error) {
			path := opencodeConfigPath(project)
			// Verified against opencode v1.17.10: "shell" is a plain
			// string (an object form fails config validation).
			err := mergeJSONFile(path, func(m map[string]any) {
				if _, ok := m["$schema"]; !ok {
					m["$schema"] = "https://opencode.ai/config.json"
				}
				m["shell"] = shell
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("opencode: wrote \"shell\": %q to %s", shell, path), nil
		},
		uninstall: func(project bool) (string, error) {
			path := opencodeConfigPath(project)
			err := mergeJSONFile(path, func(m map[string]any) {
				delete(m, "shell")
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("opencode: removed \"shell\" from %s", path), nil
		},
		check: func(shell string, project bool) error {
			path := opencodeConfigPath(project)
			m, err := readJSONFile(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			got, _ := m["shell"].(string)
			if got == "" {
				return fmt.Errorf("%s has no \"shell\" key", path)
			}
			return probeShell(got, "-c", "echo ok")
		},
	}
}

// --- aider -----------------------------------------------------------------

func aiderInstaller() agentInstaller {
	guidance := func(shell string) string {
		return fmt.Sprintf(`aider selects its shell from $SHELL (no config file). Launch it as:

    SHELL=%s aider

or export SHELL in the profile of the account that runs aider.

Aider 0.86.2 honors SHELL on its interactive PTY/pexpect path. Its piped-stdin
subprocess path uses /bin/sh instead; run with a terminal/PTY to use this wiring.`, shell)
	}
	return agentInstaller{
		name: "aider",
		install: func(shell string, _ bool) (string, error) {
			return guidance(shell), nil
		},
		uninstall: func(_ bool) (string, error) {
			return "aider: nothing written by install-agent; stop exporting SHELL to revert", nil
		},
		check: func(shell string, _ bool) error {
			// aider spawns pexpect.spawn(shell, ["-i", "-c", cmd]) under a
			// PTY; verify the -i -c shape at least in forced-interactive
			// (pipe) mode.
			return probeShell(shell, "-i", "-c", "echo ok")
		},
	}
}

// --- gemini / copilot: PATH shim dir ----------------------------------------

func shimDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bashy", "shims")
}

// shimNames are the bare shell names an agent might resolve through PATH. zsh is
// included so an agent whose interactive/PTY path uses the default macOS shell
// (e.g. agy/antigravity) still lands on bashy.
var shimNames = []string{"bash", "sh", "zsh"}

func shimInstaller(agent string) agentInstaller {
	// launch command name (defaults to the agent name for agy/antigravity).
	launch := agent
	if l, ok := map[string]string{"gemini": "gemini", "copilot": "copilot"}[agent]; ok {
		launch = l
	}
	return agentInstaller{
		name: agent,
		install: func(shell string, _ bool) (string, error) {
			dir := shimDir()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			for _, name := range shimNames {
				if err := chat.WriteShellShim(filepath.Join(dir, name), shell); err != nil {
					return "", fmt.Errorf("shim %s: %w", name, err)
				}
			}
			return fmt.Sprintf(`%s resolves bare "bash" via PATH (gemini-family run_shell_command). Shims written to %s; launch it as:

    PATH=%q:"$PATH" %s

(bashy meet/chat/weave inject this automatically via the launcher.)`, agent, dir, dir, launch), nil
		},
		uninstall: func(_ bool) (string, error) {
			dir := shimDir()
			for _, name := range shimNames {
				_ = os.Remove(filepath.Join(dir, name))
			}
			return fmt.Sprintf("%s: removed bash/sh/zsh shims from %s", agent, dir), nil
		},
		check: func(_ string, _ bool) error {
			shim := filepath.Join(shimDir(), "bash")
			if _, err := os.Stat(shim); err != nil {
				return fmt.Errorf("shim missing: %s (run bashy install-agent %s first)", shim, agent)
			}
			return probeShell(shim, "-c", "echo ok")
		},
	}
}

// --- codex -------------------------------------------------------------------

// codexInstaller wires codex via the LOGIN SHELL. codex reads the /etc/passwd
// shell (getpwuid_r pw_shell), NOT $SHELL/PATH/config (verified against the
// codex-rs source: shell-command/src/shell_detect.rs), and derives the shell
// TYPE from the filename stem, then runs `<shell> -lc`. So the lever is a
// bash/zsh-NAMED shim to bashy set as the login shell via chsh. This is invasive
// (changes the login shell for ALL sessions), so it is guidance-only unless
// --yes, and it never edits /etc/shells (needs sudo) itself.
func codexInstaller(yes bool) agentInstaller {
	// A bash-NAMED shim so codex's detect_shell_type() classifies it as Bash and
	// runs `<shim> -lc` = bashy. (~/bin/bash — the lean drop-in — also works.)
	shimBash := filepath.Join(shimDir(), "bash")
	recipe := func(shell string) (string, error) {
		if runtime.GOOS == "windows" {
			return "", fmt.Errorf("codex login-shell route is unix-only")
		}
		if err := os.MkdirAll(shimDir(), 0o755); err != nil {
			return "", err
		}
		if err := chat.WriteShellShim(shimBash, shell); err != nil {
			return "", fmt.Errorf("shim %s: %w", shimBash, err)
		}
		steps := fmt.Sprintf(`codex reads the /etc/passwd login shell (not $SHELL/PATH). Route it via a
bash-named shim to bashy and set it as your login shell (invasive — changes the
login shell for ALL sessions: Terminal, ssh):

    echo %q | sudo tee -a /etc/shells >/dev/null   # once, if not already listed
    chsh -s %q

Wrote the shim: %s -> %s`, shimBash, shimBash, shimBash, shell)
		if !yes {
			return steps + "\n\n(re-run with --yes to attempt `chsh` for you; the sudo step is still manual.)", nil
		}
		// --yes: attempt chsh only if the shim is already an accepted shell
		// (/etc/shells); never sudo-edit /etc/shells silently.
		if !shellListed(shimBash) {
			return steps + "\n\ninstall-agent: --yes: shim not in /etc/shells yet — run the sudo line above first, then re-run.", nil
		}
		if out, err := exec.Command("chsh", "-s", shimBash).CombinedOutput(); err != nil {
			return "", fmt.Errorf("chsh -s %s: %v: %s", shimBash, err, strings.TrimSpace(string(out)))
		}
		return fmt.Sprintf("codex: set login shell to %s (bashy) — new codex sessions will run `%s -lc`", shimBash, shimBash), nil
	}
	return agentInstaller{
		name:    "codex",
		install: func(shell string, _ bool) (string, error) { return recipe(shell) },
		uninstall: func(_ bool) (string, error) {
			return fmt.Sprintf("codex: to revert, `chsh -s /bin/zsh` (or your prior shell); shim %s left in place", shimBash), nil
		},
		check: func(_ string, _ bool) error {
			// Static check: confirm the bash-named shim exists and is a working
			// bashy (handles `-lc`). It cannot confirm the login shell is set
			// without reading passwd — use --probe for the live end-to-end check.
			if _, err := os.Stat(shimBash); err != nil {
				return fmt.Errorf("shim missing: %s (run `bashy install-agent codex`)", shimBash)
			}
			return probeShell(shimBash, "-lc", "echo ok")
		},
	}
}

// shellListed reports whether path appears in /etc/shells (so chsh will accept
// it for a non-root user).
func shellListed(path string) bool {
	data, err := os.ReadFile("/etc/shells")
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == path {
			return true
		}
	}
	return false
}

// probeAgentLive runs the agent once (spending an LLM call) with a canary that
// prints its shell interpreter, and confirms bashy handled it. Relies on the
// chat launcher's shell forcing (BASHY_FORCE_AGENT_SHELL).
func probeAgentLive(name, shell string) error {
	const canary = `Run exactly this shell command using your shell/bash tool and report ONLY its raw stdout, nothing else: echo "BASHY_PROBE:$0:$(ps -o comm= -p $PPID 2>/dev/null)"`
	res, err := chat.Invoke(context.Background(), chat.Options{
		Agent:       name,
		Instruction: canary,
		Timeout:     5 * time.Minute,
	}, nil)
	if err != nil && res.Output == "" {
		return fmt.Errorf("agent run failed: %w", err)
	}
	base := filepath.Base(shell)
	if strings.Contains(res.Output, "bashy") || strings.Contains(res.Output, base) {
		return nil
	}
	return fmt.Errorf("agent output did not show a bashy shell (looked for %q/bashy):\n%s", base, strings.TrimSpace(res.Output))
}

// --- shared helpers ----------------------------------------------------------

// probeShell runs the shell with the given args (the last one an echo) and
// verifies it exits 0 and prints ok.
func probeShell(shell string, args ...string) error {
	out, err := exec.Command(shell, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", shell, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "ok") {
		return fmt.Errorf("%s %s: unexpected output: %s", shell, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// mergeJSONFile reads path (missing file = empty object), applies mutate, and
// writes the result back pretty-printed. The write is atomic (temp + rename)
// so an interrupted run never truncates an agent's settings file.
func mergeJSONFile(path string, mutate func(map[string]any)) error {
	m := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("refusing to rewrite %s: not valid JSON (%v)", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	mutate(m)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".install-agent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// --- MCP server entries ------------------------------------------------------

// mcpServerName is the MCP server name registered with agents.
const mcpServerName = "bashy"

// mcpWriter registers `bashy mcp serve` with one agent. Agents without a
// known MCP config surface have no writer: dispatch reports
// "no MCP config writer for <agent>" instead of guessing a format.
type mcpWriter struct {
	// entry renders the exact entry install would write (or the exact
	// command it would run) without touching anything.
	entry func(project bool) string
	// install writes the entry.
	install func(project bool) (string, error)
	// uninstall removes a previously installed entry.
	uninstall func(project bool) (string, error)
}

func mcpWriters() map[string]mcpWriter {
	return map[string]mcpWriter{
		"claude":   claudeMCPWriter(),
		"codex":    codexMCPWriter(),
		"opencode": opencodeMCPWriter(),
	}
}

func dispatchInstallAgentMCP(name string, project, dryRun, uninstall, check, probe bool) int {
	if check || probe {
		fmt.Fprint(os.Stderr, "install-agent: --mcp cannot be combined with --check or --probe\n")
		return 2
	}
	if dryRun && uninstall {
		fmt.Fprint(os.Stderr, "install-agent: --dry-run cannot be combined with --uninstall\n")
		return 2
	}
	w, ok := mcpWriters()[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "install-agent: no MCP config writer for %q\n", name)
		return 1
	}
	if dryRun {
		fmt.Print(w.entry(project))
		return 0
	}
	if uninstall {
		msg, err := w.uninstall(project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s: %v\n", name, err)
			return 1
		}
		fmt.Println(msg)
		return 0
	}
	msg, err := w.install(project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "install-agent: %s: %v\n", name, err)
		return 1
	}
	fmt.Println(msg)
	return 0
}

// claudeMCPWriter registers via the claude CLI's `mcp add` command:
// `add [options] <name> <commandOrUrl> [args...]`, stdio by default,
// `-s project` for the project scope (verified against claude mcp
// add/remove --help; remove with no -s takes whichever scope holds it).
func claudeMCPWriter() mcpWriter {
	addArgv := func(project bool) []string {
		args := []string{"mcp", "add"}
		if project {
			args = append(args, "-s", "project")
		}
		return append(args, mcpServerName, "--", "bashy", "mcp", "serve")
	}
	removeArgv := func(project bool) []string {
		args := []string{"mcp", "remove"}
		if project {
			args = append(args, "-s", "project")
		}
		return append(args, mcpServerName)
	}
	runClaude := func(args []string) error {
		if _, err := exec.LookPath("claude"); err != nil {
			return fmt.Errorf("claude CLI not found on PATH")
		}
		if out, err := exec.Command("claude", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("claude %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return mcpWriter{
		entry: func(project bool) string {
			return "claude " + strings.Join(addArgv(project), " ") + "\n"
		},
		install: func(project bool) (string, error) {
			if err := runClaude(addArgv(project)); err != nil {
				return "", err
			}
			scope := "the claude CLI default scope"
			if project {
				scope = "the project scope"
			}
			return fmt.Sprintf("claude: registered MCP server %q via claude mcp add (%s)", mcpServerName, scope), nil
		},
		uninstall: func(project bool) (string, error) {
			if err := runClaude(removeArgv(project)); err != nil {
				return "", err
			}
			return fmt.Sprintf("claude: removed MCP server %q via claude mcp remove", mcpServerName), nil
		},
	}
}

// codexMCPServerEntry is the exact TOML codex itself writes for
// `codex mcp add bashy -- bashy mcp serve` (verified against a scratch
// CODEX_HOME): appended verbatim so the file stays in a shape codex reads.
const codexMCPServerEntry = "[mcp_servers.bashy]\ncommand = \"bashy\"\nargs = [\"mcp\", \"serve\"]\n"

func codexMCPConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "config.toml")
}

// hasTOMLSection reports whether data contains a section header line exactly
// equal to [section] (commented headers do not count). No TOML parser: the
// file may hold arbitrary other tables that must pass through untouched.
func hasTOMLSection(data []byte, section string) bool {
	header := "[" + section + "]"
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == header {
			return true
		}
	}
	return false
}

// writeFileAtomic writes path via temp + rename so an interrupted run never
// truncates the agent's config file (same pattern as mergeJSONFile).
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".install-agent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func codexMCPWriter() mcpWriter {
	return mcpWriter{
		entry: func(_ bool) string { return codexMCPServerEntry },
		install: func(_ bool) (string, error) {
			path := codexMCPConfigPath()
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return "", err
			}
			if hasTOMLSection(data, "mcp_servers.bashy") {
				return fmt.Sprintf("codex: [mcp_servers.bashy] already present in %s", path), nil
			}
			var out string
			if strings.TrimSpace(string(data)) == "" {
				out = codexMCPServerEntry
			} else {
				out = strings.TrimRight(string(data), "\n") + "\n\n" + codexMCPServerEntry
			}
			if err := writeFileAtomic(path, []byte(out)); err != nil {
				return "", err
			}
			return fmt.Sprintf("codex: wrote [mcp_servers.bashy] to %s", path), nil
		},
		uninstall: func(_ bool) (string, error) {
			path := codexMCPConfigPath()
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Sprintf("codex: no [mcp_servers.bashy] entry in %s (nothing to remove)", path), nil
				}
				return "", err
			}
			if !hasTOMLSection(data, "mcp_servers.bashy") {
				return fmt.Sprintf("codex: no [mcp_servers.bashy] entry in %s (nothing to remove)", path), nil
			}
			lines := strings.Split(string(data), "\n")
			header := -1
			for i, line := range lines {
				if strings.TrimSpace(line) == "[mcp_servers.bashy]" {
					header = i
					break
				}
			}
			start := header
			if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
				start--
			}
			end := len(lines)
			for i := header + 1; i < len(lines); i++ {
				if t := strings.TrimSpace(lines[i]); strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
					end = i
					break
				}
			}
			rest := append(append([]string{}, lines[:start]...), lines[end:]...)
			out := strings.Join(rest, "\n")
			if out != "" && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			for strings.Contains(out, "\n\n\n") {
				out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
			}
			if err := writeFileAtomic(path, []byte(out)); err != nil {
				return "", err
			}
			return fmt.Sprintf("codex: removed [mcp_servers.bashy] from %s", path), nil
		},
	}
}

// opencodeMCPEntry renders the JSON fragment install merges under the "mcp"
// key (shape verified against the opencode config schema's McpLocalConfig:
// type/command required, command an argv array).
func opencodeMCPEntry() string {
	out, _ := json.MarshalIndent(map[string]any{
		"mcp": map[string]any{
			"bashy": map[string]any{
				"type":    "local",
				"command": []string{"bashy", "mcp", "serve"},
			},
		},
	}, "", "  ")
	return string(out) + "\n"
}

func opencodeMCPWriter() mcpWriter {
	return mcpWriter{
		entry: func(_ bool) string { return opencodeMCPEntry() },
		install: func(project bool) (string, error) {
			path := opencodeConfigPath(project)
			err := mergeJSONFile(path, func(m map[string]any) {
				servers, _ := m["mcp"].(map[string]any)
				if servers == nil {
					servers = map[string]any{}
				}
				servers["bashy"] = map[string]any{"type": "local", "command": []string{"bashy", "mcp", "serve"}}
				m["mcp"] = servers
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("opencode: wrote mcp.bashy to %s", path), nil
		},
		uninstall: func(project bool) (string, error) {
			path := opencodeConfigPath(project)
			err := mergeJSONFile(path, func(m map[string]any) {
				if servers, ok := m["mcp"].(map[string]any); ok {
					delete(servers, "bashy")
					if len(servers) == 0 {
						delete(m, "mcp")
					}
				}
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("opencode: removed mcp.bashy from %s", path), nil
		},
	}
}

// --- turn-boundary inbox hooks (Sprint 321) ----------------------------------

// inboxHookEvents are the turn-boundary events both harnesses fire and both
// honor additionalContext for: once per session and once per user turn. Any
// other event would either not be turn-scoped (PreToolUse) or not
// model-visible in the same way, so the installer wires exactly these two.
var inboxHookEvents = []string{"SessionStart", "UserPromptSubmit"}

// inboxHookCommand is the exact command a harness hook runs. as is the baked
// registered identity ("" leaves attribution ambient, in which case the hook
// stays silent unless the session resolves one itself).
func inboxHookCommand(shell, event, as string) string {
	cmd := shell + " inbox-hook --for " + event
	if as != "" {
		cmd += " --as " + as
	}
	return cmd
}

// hookWriter installs one agent's turn-boundary inbox hooks through the same
// merge-config mechanism as the shell/MCP writers: read the agent's config,
// add only our entries, write back everything else untouched.
type hookWriter struct {
	// entry renders exactly what install would write, without touching disk.
	entry func(shell, as string, project bool) string
	// install merges the hook entries into the agent's config.
	install func(shell, as string, project bool) (string, error)
	// uninstall removes previously installed hook entries.
	uninstall func(shell, as string, project bool) (string, error)
	// check verifies the entries are present (static; no harness launch).
	check func(shell, as string, project bool) error
}

func hookWriters() map[string]hookWriter {
	return map[string]hookWriter{
		"claude": claudeHookWriter(),
		"codex":  codexHookWriter(),
	}
}

func dispatchInstallAgentHooks(name, shell, as string, project, dryRun, uninstall, check bool) int {
	w, ok := hookWriters()[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "install-agent: no inbox-hook writer for %q (supported: claude codex) — generic notify and the MCP skill export are not inbox hooks\n", name)
		return 1
	}
	if dryRun && uninstall {
		fmt.Fprint(os.Stderr, "install-agent: --dry-run cannot be combined with --uninstall\n")
		return 2
	}
	if check && (dryRun || uninstall) {
		fmt.Fprint(os.Stderr, "install-agent: --check cannot be combined with --dry-run or --uninstall\n")
		return 2
	}
	switch {
	case dryRun:
		fmt.Print(w.entry(shell, as, project))
		return 0
	case check:
		if err := w.check(shell, as, project); err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s hooks: CHECK FAILED: %v\n", name, err)
			return 1
		}
		fmt.Printf("install-agent: %s hooks: OK\n", name)
		return 0
	case uninstall:
		msg, err := w.uninstall(shell, as, project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s hooks: %v\n", name, err)
			return 1
		}
		fmt.Println(msg)
		return 0
	default:
		msg, err := w.install(shell, as, project)
		if err != nil {
			fmt.Fprintf(os.Stderr, "install-agent: %s hooks: %v\n", name, err)
			return 1
		}
		fmt.Println(msg)
		return 0
	}
}

// claudeInboxHookEntry is one settings.json hook-group element for event: the
// documented shape (hooks-guide: {"hooks": {"SessionStart": [{"hooks":
// [{"type": "command", "command": ...}]}]}}). No matcher: SessionStart must
// fire for startup/resume/clear/compact, and UserPromptSubmit ignores
// matchers. timeout 30 bounds the hook past the slowest local snapshot
// (the cross-host delivery pass caps at 20s) without stalling the turn.
func claudeInboxHookEntry(command string) map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": command, "timeout": 30},
		},
	}
}

// claudeHookHasCommand reports whether the settings map already carries an
// inbox-hook command for event. as=="" matches any identity the command was
// baked with, so --check works without repeating --as.
func claudeHookHasCommand(m map[string]any, event string) bool {
	hooks, _ := m["hooks"].(map[string]any)
	arr, _ := hooks[event].([]any)
	want := "inbox-hook --for " + event
	for _, el := range arr {
		elm, _ := el.(map[string]any)
		sub, _ := elm["hooks"].([]any)
		for _, h := range sub {
			hm, _ := h.(map[string]any)
			if hm["type"] != "command" {
				continue
			}
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, want) {
				return true
			}
		}
	}
	return false
}

// mergeClaudeInboxHooks ensures both turn-boundary hook groups exist, adding
// only missing entries. Idempotent: a second run changes nothing.
func mergeClaudeInboxHooks(m map[string]any, shell, as string) (changed bool) {
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		m["hooks"] = hooks
		changed = true
	}
	for _, event := range inboxHookEvents {
		if claudeHookHasCommand(m, event) {
			continue
		}
		arr, _ := hooks[event].([]any)
		hooks[event] = append(arr, claudeInboxHookEntry(inboxHookCommand(shell, event, as)))
		changed = true
	}
	return changed
}

// stripClaudeInboxHooks removes every hook-group element that carries an
// inbox-hook command, pruning emptied events and the hooks key itself so
// uninstall leaves no residue. Unrelated hooks pass through untouched.
func stripClaudeInboxHooks(m map[string]any) (changed bool) {
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		return false
	}
	for _, event := range inboxHookEvents {
		arr, _ := hooks[event].([]any)
		if arr == nil {
			continue
		}
		kept := make([]any, 0, len(arr))
		for _, el := range arr {
			if claudeHookElementIsInbox(el) {
				changed = true
				continue
			}
			kept = append(kept, el)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else if len(kept) != len(arr) {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(m, "hooks")
	}
	return changed
}

func claudeHookElementIsInbox(el any) bool {
	elm, _ := el.(map[string]any)
	sub, _ := elm["hooks"].([]any)
	for _, h := range sub {
		hm, _ := h.(map[string]any)
		if hm["type"] == "command" {
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, "inbox-hook") {
				return true
			}
		}
	}
	return false
}

func claudeHookWriter() hookWriter {
	return hookWriter{
		entry: func(shell, as string, project bool) string {
			m := map[string]any{}
			mergeClaudeInboxHooks(m, shell, as)
			b, _ := json.MarshalIndent(map[string]any{"hooks": m["hooks"]}, "", "  ")
			scope := claudeSettingsPath(project)
			return fmt.Sprintf("# merge into %s under \"hooks\" (existing keys preserved):\n%s\n", scope, b)
		},
		install: func(shell, as string, project bool) (string, error) {
			path := claudeSettingsPath(project)
			changed := false
			err := mergeJSONFile(path, func(m map[string]any) {
				changed = mergeClaudeInboxHooks(m, shell, as)
			})
			if err != nil {
				return "", err
			}
			if !changed {
				return fmt.Sprintf("claude: inbox hooks already present in %s", path), nil
			}
			return fmt.Sprintf("claude: wrote SessionStart + UserPromptSubmit inbox hooks to %s (restart claude to pick them up)", path), nil
		},
		uninstall: func(_ string, _ string, project bool) (string, error) {
			path := claudeSettingsPath(project)
			changed := false
			err := mergeJSONFile(path, func(m map[string]any) {
				changed = stripClaudeInboxHooks(m)
			})
			if err != nil {
				return "", err
			}
			if !changed {
				return fmt.Sprintf("claude: no inbox hooks in %s", path), nil
			}
			return fmt.Sprintf("claude: removed inbox hooks from %s", path), nil
		},
		check: func(_ string, _ string, project bool) error {
			path := claudeSettingsPath(project)
			m, err := readJSONFile(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			for _, event := range inboxHookEvents {
				if !claudeHookHasCommand(m, event) {
					return fmt.Errorf("%s has no inbox-hook for %s (run `bashy install-agent claude --hooks [--as NAME]`)", path, event)
				}
			}
			return nil
		},
	}
}

// Codex reads turn hooks from inline [hooks] tables in config.toml (official
// hooks reference: [[hooks.SessionStart]] + [[hooks.SessionStart.hooks]] with
// type/command; UserPromptSubmit ignores matchers). Hooks are stable-enabled
// in Codex 0.157.1 (features list: hooks = stable true), so no [features]
// edit is needed. New hooks require one trust review (/hooks) before they
// first run — the installer says so; it cannot approve them itself.
const (
	codexHookBeginMark = "# BEGIN managed by `bashy install-agent codex --hooks` (inbox unread hints; remove with --hooks --uninstall)"
	codexHookEndMark   = "# END managed by `bashy install-agent codex --hooks`"
)

func codexInboxHookBlock(shell, as string) string {
	lines := []string{
		codexHookBeginMark,
		"[[hooks.SessionStart]]",
		"",
		"[[hooks.SessionStart.hooks]]",
		"type = \"command\"",
		fmt.Sprintf("command = %q", inboxHookCommand(shell, "SessionStart", as)),
		"timeout = 30",
		"",
		"[[hooks.UserPromptSubmit]]",
		"",
		"[[hooks.UserPromptSubmit.hooks]]",
		"type = \"command\"",
		fmt.Sprintf("command = %q", inboxHookCommand(shell, "UserPromptSubmit", as)),
		"timeout = 30",
		codexHookEndMark,
		" ",
	}
	return strings.Join(lines, "\n")
}

// replaceCodexHookBlock swaps any previously managed block for the new one so
// a changed shell path or --as identity updates in place; appends when absent.
func replaceCodexHookBlock(data, block string) string {
	trimmed := strings.TrimRight(block, " \n") + "\n"
	if begin, end := strings.Index(data, codexHookBeginMark), strings.Index(data, codexHookEndMark); begin >= 0 && end > begin {
		after := data[end+len(codexHookEndMark):]
		after = strings.TrimLeft(after, "\n")
		head := strings.TrimRight(data[:begin], "\n")
		if head == "" {
			return trimmed + after
		}
		if after == "" {
			return head + "\n\n" + trimmed
		}
		return head + "\n\n" + trimmed + "\n" + after
	}
	if strings.TrimSpace(data) == "" {
		return trimmed
	}
	return strings.TrimRight(data, "\n") + "\n\n" + trimmed
}

// removeCodexHookBlock deletes the managed block, collapsing the blank lines
// around it so uninstall restores the file's prior shape.
func removeCodexHookBlock(data string) (string, bool) {
	begin, end := strings.Index(data, codexHookBeginMark), strings.Index(data, codexHookEndMark)
	if begin < 0 || end < begin {
		return data, false
	}
	after := data[end+len(codexHookEndMark):]
	after = strings.TrimLeft(after, "\n")
	head := strings.TrimRight(data[:begin], "\n")
	switch {
	case head == "":
		return after, true
	case after == "":
		return head + "\n", true
	default:
		return head + "\n\n" + after, true
	}
}

func codexHookWriter() hookWriter {
	return hookWriter{
		entry: func(shell, as string, _ bool) string { return codexInboxHookBlock(shell, as) },
		install: func(shell, as string, _ bool) (string, error) {
			path := codexMCPConfigPath()
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return "", err
			}
			before := string(data)
			after := replaceCodexHookBlock(before, codexInboxHookBlock(shell, as))
			if after == before && strings.Contains(before, codexHookBeginMark) {
				return fmt.Sprintf("codex: inbox hooks already present in %s", path), nil
			}
			if err := writeFileAtomic(path, []byte(after)); err != nil {
				return "", err
			}
			return fmt.Sprintf("codex: wrote SessionStart + UserPromptSubmit inbox hooks to %s (approve them once under /hooks before they first run)", path), nil
		},
		uninstall: func(_ string, _ string, _ bool) (string, error) {
			path := codexMCPConfigPath()
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Sprintf("codex: no %s to clean", path), nil
				}
				return "", err
			}
			after, ok := removeCodexHookBlock(string(data))
			if !ok {
				return fmt.Sprintf("codex: no inbox hooks in %s", path), nil
			}
			if err := writeFileAtomic(path, []byte(after)); err != nil {
				return "", err
			}
			return fmt.Sprintf("codex: removed inbox hooks from %s", path), nil
		},
		check: func(_ string, _ string, _ bool) error {
			path := codexMCPConfigPath()
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			for _, event := range inboxHookEvents {
				if !strings.Contains(string(data), "inbox-hook --for "+event) {
					return fmt.Errorf("%s has no inbox-hook for %s (run `bashy install-agent codex --hooks [--as NAME]`)", path, event)
				}
			}
			return nil
		},
	}
}

func printAgentStatus(installers map[string]agentInstaller, shell string) {
	fmt.Printf("shell: %s\n\n", shell)
	for _, name := range []string{"claude", "opencode", "aider", "gemini", "copilot", "agy", "codex"} {
		ins := installers[name]
		onPath := ""
		if _, err := exec.LookPath(name); err == nil {
			onPath = " (installed)"
		}
		status := "not wired"
		if err := ins.check(shell, false); err == nil {
			status = "wired + check OK"
		}
		fmt.Printf("  %-9s%-13s %s\n", name, onPath, status)
	}
	fmt.Print("\nwire one: bashy install-agent <agent>   verify: bashy install-agent <agent> --check\n")
}
