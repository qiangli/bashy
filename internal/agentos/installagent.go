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

	"github.com/qiangli/coreutils/pkg/chat"
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
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: bashy install-agent <agent> [--shell PATH] [--project] [--check] [--uninstall]

Wire a coding agent to use bashy as its shell.

agents:
  claude     Claude Code   settings.json env.CLAUDE_CODE_SHELL   (E2E verified)
  opencode   OpenCode      opencode.json "shell": "<path>"       (E2E verified)
  aider      Aider         $SHELL guidance (no config file)
  gemini     Gemini CLI    PATH shim dir (~/.bashy/shims)
  copilot    Copilot CLI   PATH shim dir (~/.bashy/shims)
  agy        Antigravity   PATH shim dir (~/.bashy/shims)
  codex      Codex CLI     login shell via chsh (reads /etc/passwd; invasive)

Note: bashy meet/chat/weave already force-inject the shell for spawned agents
(SHELL + PATH shim + CLAUDE_CODE_SHELL) — this command makes the wiring durable
for direct/interactive use. --probe runs the agent LIVE to confirm bashy is used.

With no agent, prints the wiring status of every known agent.
`)
	}
	// Accept the agent name anywhere among the flags (`install-agent claude
	// --check` and `install-agent --check claude` both work): Go's flag
	// package stops at the first positional, so pull the name out before
	// parsing. --shell is the only value-taking flag; keep its value with it.
	var name string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--shell" || a == "-shell" {
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

or export SHELL in the profile of the account that runs aider.`, shell)
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
				link := filepath.Join(dir, name)
				_ = os.Remove(link)
				if err := os.Symlink(shell, link); err != nil {
					return "", fmt.Errorf("shim %s: %w", link, err)
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
		if tgt, err := os.Readlink(shimBash); err != nil || tgt != shell {
			_ = os.Remove(shimBash)
			if err := os.Symlink(shell, shimBash); err != nil {
				return "", fmt.Errorf("shim %s: %w", shimBash, err)
			}
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
