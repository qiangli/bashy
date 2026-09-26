package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/qiangli/bashy/pkg/ladder"
	"mvdan.cc/sh/v3/interp"
)

// The default agent TUI (Sprint #301 T4) is bashy's own interactive shell
// with an agent attached: one input line (the readline fork: history, vi/emacs
// editing), the terminal's scrollback as the transcript, and a prompt that
// carries the agent and its session. There is no slash parser; every line goes
// through the action ladder (pkg/ladder):
//
//	rung 0  a literal command runs as typed, byte-identically
//	rung 1  a misspelled command name is repaired, echoed, then run
//	last    free text becomes an agent turn: `bashy ycode -- 'TEXT'`
//
// The session is shell state: the terminal exports the agent config
// (YCODE_CONFIG) and a session pointer (BASHY_YCODE_SESSION_FILE), so
// `bashy ycode status|new|resume|session …` typed at the prompt — or run from
// a script in it — act on the session this terminal is on.
const (
	agentTUIEnv     = "BASHY_AGENT_TUI"          // the agent's name; set in the TUI shell
	agentConfigEnv  = "YCODE_CONFIG"             // the engine's bootstrap env: the agent YAML
	agentSessionEnv = "BASHY_YCODE_SESSION_FILE" // ycodecli.SessionFileEnv
)

// AgentOSOwnedCommand and AgentOSOwnedNames report the commands the bashy
// binary serves in-process (its pure-Go userland), which need not be on PATH.
// The pure bash drop-in leaves them empty; it has no agent TUI.
var (
	AgentOSOwnedCommand = func(string) bool { return false }
	AgentOSOwnedNames   = func() []string { return nil }
)

// RunAgentTerminal hosts an agent's session on this terminal: it starts
// bashy's interactive shell with the agent attached and returns when the user
// leaves it. Wired as ycodecli.TerminalUI by cmd/bashy.
func RunAgentTerminal(ctx context.Context, agent, config, session string) error {
	pointer, err := newSessionPointer(session)
	if err != nil {
		return err
	}
	defer os.Remove(pointer)
	self, err := selfExecutable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, self, "-i")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The launching process already reported telemetry once; the shell and
	// every turn or command it runs stay quiet about it.
	cmd.Env = append(os.Environ(), agentTUIEnv+"="+agent, agentConfigEnv+"="+config, agentSessionEnv+"="+pointer, "BASHY_TELEMETRY_QUIET=1")
	// Leaving the terminal is not a failure, whatever the last command's
	// status was when the user left.
	if err := cmd.Run(); err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); !ok {
			return err
		}
	}
	return nil
}

// newSessionPointer writes the session pointer under $BASHY_HOME (else
// ~/.bashy, else the temp dir) — never a per-OS config dir.
func newSessionPointer(session string) (string, error) {
	base := os.Getenv("BASHY_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = filepath.Join(home, ".bashy")
		} else {
			base = filepath.Join(os.TempDir(), "bashy")
		}
	}
	dir := filepath.Join(base, "ycode", "terminals")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var nonce [6]byte
	_, _ = rand.Read(nonce[:])
	path := filepath.Join(dir, fmt.Sprintf("%d-%s.session", os.Getpid(), hex.EncodeToString(nonce[:])))
	return path, os.WriteFile(path, []byte(session+"\n"), 0o600)
}

// selfExecutable prefers the unix launcher over the Go program it execs
// (bin/bashy over bin/bashy.real) so the shell keeps its signal snapshot.
func selfExecutable() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if launcher, ok := strings.CutSuffix(self, ".real"); ok {
		if _, err := os.Stat(launcher); err == nil {
			return launcher, nil
		}
	}
	return self, nil
}

// agentTerminal is the TUI state inside the interactive shell.
type agentTerminal struct {
	name    string
	pointer string
	self    string
	stderr  io.Writer
}

func agentTerminalFromEnv(stderr io.Writer) *agentTerminal {
	name := os.Getenv(agentTUIEnv)
	if name == "" || !AgentOSBashPPDefault {
		return nil
	}
	self, err := selfExecutable()
	if err != nil {
		return nil
	}
	return &agentTerminal{name: name, pointer: os.Getenv(agentSessionEnv), self: self, stderr: stderr}
}

func (t *agentTerminal) session() string {
	data, err := os.ReadFile(t.pointer)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// prompt prefixes the shell prompt with the agent and its session.
func (t *agentTerminal) prompt(ps1 string) string {
	id := t.session()
	if len(id) > 8 {
		id = id[:8]
	}
	return fmt.Sprintf("[%s %s] %s", t.name, id, ps1)
}

func (t *agentTerminal) greeting() string {
	return fmt.Sprintf("%s: type a command, or ask in plain words. Session commands: bashy ycode status | new | resume [SESSION] | session list. Ctrl-D leaves.\n", t.name)
}

// route places one input line on the ladder and returns the line the shell
// runs: the line itself (rung 0), the repaired line (rung 1, echoed), or the
// agent turn that carries free text.
func (t *agentTerminal) route(_ context.Context, r *interp.Runner, line string) string {
	d := ladder.Resolve(line, runnerCommands{r: r})
	switch d.Rung {
	case ladder.Repair:
		fmt.Fprintf(t.stderr, "%s  (%s)\n", d.Line, d.Note)
		return d.Line
	case ladder.Free:
		return singleQuote(t.self) + " ycode -- " + singleQuote(d.Line)
	}
	return line
}

// runnerCommands answers the ladder from the live shell: keywords (the
// ladder's own), builtins, functions, aliases, bashy's in-process commands,
// then PATH.
type runnerCommands struct{ r *interp.Runner }

func (c runnerCommands) Known(name string) bool {
	if interp.IsBuiltin(name) || c.r.Funcs[name] != nil || c.r.AliasDefined(name) || AgentOSOwnedCommand(name) {
		return true
	}
	return onPath(name, c.r.LiveVar("PATH").String())
}

func (c runnerCommands) Names() []string {
	names := interp.BuiltinNames()
	for name := range c.r.Funcs {
		names = append(names, name)
	}
	names = append(names, AgentOSOwnedNames()...)
	return append(names, pathNames(c.r.LiveVar("PATH").String())...)
}

// GlobMatches resolves a pattern in the shell's working directory.
func (c runnerCommands) GlobMatches(pattern string) bool {
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(c.r.Dir, pattern)
	}
	matches, err := filepath.Glob(pattern)
	return err == nil && len(matches) > 0
}

func onPath(name, path string) bool {
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = append(exts, ".exe", ".cmd", ".bat", ".com")
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			if info, err := os.Stat(filepath.Join(dir, name+ext)); err == nil && !info.IsDir() && executable(info) {
				return true
			}
		}
	}
	return false
}

var pathNamesCache struct {
	sync.Mutex
	path  string
	names []string
}

// pathNames lists the executables on path, cached per PATH value.
func pathNames(path string) []string {
	pathNamesCache.Lock()
	defer pathNamesCache.Unlock()
	if pathNamesCache.path == path && pathNamesCache.names != nil {
		return pathNamesCache.names
	}
	var names []string
	for _, dir := range filepath.SplitList(path) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if info, err := e.Info(); err == nil && !info.IsDir() && executable(info) {
				name := e.Name()
				if runtime.GOOS == "windows" {
					name = strings.TrimSuffix(name, filepath.Ext(name))
				}
				names = append(names, name)
			}
		}
	}
	pathNamesCache.path, pathNamesCache.names = path, names
	return names
}

func executable(info os.FileInfo) bool {
	return info.Mode()&0o111 != 0 || filepath.Ext(info.Name()) == ".exe"
}
