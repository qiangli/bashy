// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	check := fs.Bool("check", false, "verify the wiring instead of writing it")
	uninstall := fs.Bool("uninstall", false, "reverse a previous install")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `usage: bashy install-agent <agent> [--shell PATH] [--project] [--check] [--uninstall]

Wire a coding agent to use bashy as its shell.

agents:
  claude     Claude Code   settings.json env.CLAUDE_CODE_SHELL   (E2E verified)
  opencode   OpenCode      opencode.json "shell": "<path>"       (E2E verified)
  aider      Aider         $SHELL guidance (no config file)
  gemini     Gemini CLI    PATH shim dir (~/.bashy/shims)
  copilot    Copilot CLI   PATH shim dir (~/.bashy/shims)
  codex      Codex CLI     status only (no override surface on macOS)

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

	installers := agentInstallers()
	if name == "" {
		printAgentStatus(installers, shell)
		return 0
	}
	ins, ok := installers[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "install-agent: unknown agent %q (try: claude opencode aider gemini copilot codex)\n", name)
		return 2
	}

	switch {
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

func agentInstallers() map[string]agentInstaller {
	return map[string]agentInstaller{
		"claude":   claudeInstaller(),
		"opencode": opencodeInstaller(),
		"aider":    aiderInstaller(),
		"gemini":   shimInstaller("gemini"),
		"copilot":  shimInstaller("copilot"),
		"codex":    codexInstaller(),
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

func shimInstaller(agent string) agentInstaller {
	launch := map[string]string{"gemini": "gemini", "copilot": "copilot"}[agent]
	return agentInstaller{
		name: agent,
		install: func(shell string, _ bool) (string, error) {
			dir := shimDir()
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			for _, name := range []string{"bash", "sh"} {
				link := filepath.Join(dir, name)
				_ = os.Remove(link)
				if err := os.Symlink(shell, link); err != nil {
					return "", fmt.Errorf("shim %s: %w", link, err)
				}
			}
			return fmt.Sprintf(`%s resolves bare "bash" via PATH. Shims written to %s; launch it as:

    PATH=%q:"$PATH" %s`, agent, dir, dir, launch), nil
		},
		uninstall: func(_ bool) (string, error) {
			dir := shimDir()
			for _, name := range []string{"bash", "sh"} {
				_ = os.Remove(filepath.Join(dir, name))
			}
			return fmt.Sprintf("%s: removed bash/sh shims from %s", agent, dir), nil
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

func codexInstaller() agentInstaller {
	status := `codex has no shell-override surface here: on macOS it spawns /bin/zsh by
absolute path (verified against codex-cli 0.142.5), so neither a config key
nor a PATH shim reaches it. Tracked as upstream work (a portable-bash backend
beside codex's ZshFork) — see docs/agent-adoption/matrix.md.`
	return agentInstaller{
		name: "codex",
		install: func(_ string, _ bool) (string, error) {
			return "", fmt.Errorf("%s", status)
		},
		uninstall: func(_ bool) (string, error) {
			return "codex: nothing to remove", nil
		},
		check: func(_ string, _ bool) error {
			return fmt.Errorf("%s", status)
		},
	}
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
	for _, name := range []string{"claude", "opencode", "aider", "gemini", "copilot", "codex"} {
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
