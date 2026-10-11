// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/acp"
	"github.com/qiangli/yoke/pkg/atlas"
)

// `bashy acp` is the one-token spelling of `bashy herald acp`: an ACP host
// (Zed's agent_servers entry, a Codync bot's `command:`) configures ONE command
// string, and `herald` is a bashy word that operator has no reason to know.
//
// These tests pin the three things the alias has to get right — it is
// catalogued, it is NOT a bare shell shim, and it serves the real protocol —
// without re-testing yoke's herald, whose body it shares.

func TestACPFrontDoorIsCataloguedAsHeraldsAlias(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")

	if !slices.Contains(directFrontDoorVerbs, "acp") {
		t.Error("acp must be a direct front door: `bashy acp` is what an ACP host configures")
	}
	// A bare `acp` shim would put a JSON-RPC server behind a three-letter word
	// in every interactive shell. Same rule as llm/ping/approve.
	if slices.Contains(alwaysShimVerbs, "acp") ||
		strings.Contains(Preamble(), "\nacp()") || strings.HasPrefix(Preamble(), "acp()") {
		t.Error("acp must never become a bare shell shim")
	}
	// It inherits herald's maturity claim exactly, because it is herald's code:
	// out of the taught catalog, back under --all, dispatch untouched.
	if !isCuratedHidden("acp") || !isCuratedHidden("herald") {
		t.Error("acp and herald must carry the same (experimental) maturity claim")
	}
	_, _, verbs := commandsCatalog()
	if slices.Contains(verbs, "acp") {
		t.Error("experimental acp should not appear in the default catalog")
	}
	if !slices.Contains(hiddenVerbsCatalog(), "acp") {
		t.Error("acp must be catalogued under --all")
	}
	if synopsisOf("acp") == "" {
		t.Error("acp needs a synopsis; `commands --all` prints one per name")
	}
	// `bashy acp --help` routes through dispatchFormattedHelp, which refuses a
	// verb with no atlas entry. Both lookups are load-bearing.
	if !isHelpVerb("acp") {
		t.Error("acp must be a help verb so `bashy acp --help` renders")
	}
	entry, ok := lookupEntry("acp")
	if !ok {
		t.Fatal("acp has no atlas entry")
	}
	if entry.Stability != atlas.StabilityExperimental {
		t.Errorf("acp stability = %q, want %q", entry.Stability, atlas.StabilityExperimental)
	}
	// Answering an ACP client still execs the local agent on metered
	// inference: the direction of the connection changes, the cost does not.
	for _, effect := range []string{atlas.EffExec, atlas.EffNet, atlas.EffSpend} {
		if !slices.Contains(entry.Effects, effect) {
			t.Errorf("acp effects = %v, want %q among them", entry.Effects, effect)
		}
	}
}

// acpTestBinary resolves the bashy under test the same way the synergy fixture
// does — BASHY_E2E_BIN, then the checkout's bin/bashy, then (outside -short) a
// plain build. `bashy acp` is a process-level surface: an ACP host launches a
// command, so nothing short of a real process proves the dispatch arm exists.
func acpTestBinary(t *testing.T) string {
	t.Helper()
	root, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Skip("not inside the bashy checkout")
	}
	if p := strings.TrimSpace(os.Getenv("BASHY_E2E_BIN")); p != "" {
		return p
	}
	built := filepath.Join(root, "bin", "bashy")
	if runtime.GOOS == "windows" {
		built += ".exe"
	}
	if st, err := os.Stat(built); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
		return built
	}
	if testing.Short() {
		t.Skip("needs a built bashy: run `make build-bashy` or set BASHY_E2E_BIN (not built under -short)")
	}
	out := filepath.Join(t.TempDir(), "bashy")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/bashy")
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bashy: %v\n%s", err, b)
	}
	return out
}

// acpTestEnv isolates the server from the invoking agent's own identity and
// stores, and keeps it offline: a help render and a refused turn must touch
// neither the operator's fleet nor the network.
func acpTestEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + tmp,
		"BASHY_HOME=" + filepath.Join(home, "bashy-home"),
		"BASHY_FLEET_DIR=" + filepath.Join(home, "fleet"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"BASHY_HINTS=off",
		"BASHY_HELP_FORMAT=classic",
	}
}

// TestACPHelpIsHeraldACPHelpByteForByte is the parity claim stated as a
// comparison rather than a copy of the prose: `bashy acp` prepends the
// subcommand and nothing else, so the two help texts cannot drift apart. A
// future flag added to `herald acp` is documented by `bashy acp --help` the
// day it lands, with no edit here.
func TestACPHelpIsHeraldACPHelpByteForByte(t *testing.T) {
	bin := acpTestBinary(t)
	env := acpTestEnv(t)

	run := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = env
		cmd.Stdin = strings.NewReader("")
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("`bashy %s` did not finish: %v", strings.Join(args, " "), ctx.Err())
		}
		if err != nil {
			t.Fatalf("`bashy %s` failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	alias := run("acp", "--help")
	direct := run("herald", "acp", "--help")
	if alias != direct {
		t.Errorf("`bashy acp --help` and `bashy herald acp --help` differ:\nalias=%q\ndirect=%q", alias, direct)
	}
	// Guard against both being empty, which would make the comparison vacuous.
	for _, want := range []string{"Serve this machine as an ACP agent over stdin/stdout", "--gate", "--timeout"} {
		if !strings.Contains(alias, want) {
			t.Errorf("`bashy acp --help` is missing %q:\n%s", want, alias)
		}
	}
}

// TestACPProcessLevelInitializeSessionPrompt drives the REAL `bashy acp`
// process over the protocol's own transport, with yoke's ACP client as the
// host: initialize (strict v1), session/new, session/prompt.
//
// The prompt is deliberately whitespace. chatRunner refuses an empty turn
// BEFORE it starts a local agent, so this exercises the whole JSON-RPC path —
// dispatch arm, server construction, session map, response encoding — with no
// agent binary, no pty and no inference. What it does NOT prove is a completed
// turn with real text: that needs a pinned ACP adapter and belongs to the open
// half of this story (Zed and a Codync bot each completing a prompt, and a
// permission request reaching `bashy approve`).
func TestACPProcessLevelInitializeSessionPrompt(t *testing.T) {
	bin := acpTestBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "acp")
	cmd.Env = acpTestEnv(t)
	cmd.Stderr = os.Stderr

	// NewClient initializes and strictly rejects a protocol version other than
	// the one it speaks, so reaching a non-nil client IS the initialize claim.
	client, err := acp.NewClient(ctx, acp.BaseHandler{}, cmd)
	if err != nil {
		t.Fatalf("`bashy acp` did not complete the ACP handshake: %v", err)
	}
	defer client.Close()

	if got := client.ProtocolVersion(); got != acp.ProtocolVersionNumber {
		t.Errorf("negotiated protocol version = %d, want %d", got, acp.ProtocolVersionNumber)
	}

	cwd := t.TempDir()
	first, err := client.NewSession(ctx, cwd)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if strings.TrimSpace(first) == "" {
		t.Fatal("session/new returned an empty session id")
	}

	reason, err := client.Prompt(ctx, first, "   ")
	if err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if reason != acp.StopReasonRefusal {
		t.Errorf("empty-prompt stop reason = %q, want %q", reason, acp.StopReasonRefusal)
	}

	// A second session on the same connection: the server multiplexes sessions
	// and survived the first turn, which is what an editor keeping one agent
	// process across several conversations depends on.
	second, err := client.NewSession(ctx, cwd)
	if err != nil {
		t.Fatalf("second session/new: %v", err)
	}
	if second == first {
		t.Errorf("second session reused id %q; sessions must be distinct", first)
	}
	if reason, err := client.Prompt(ctx, second, ""); err != nil {
		t.Fatalf("second session/prompt: %v", err)
	} else if reason != acp.StopReasonRefusal {
		t.Errorf("second-session stop reason = %q, want %q", reason, acp.StopReasonRefusal)
	}

	// An unknown session is an error, not a silent new session: a host that
	// lost track of its ids must be told, not quietly given a fresh context.
	if _, err := client.Prompt(ctx, "no-such-session", "hello"); err == nil {
		t.Error("session/prompt on an unknown session must fail")
	}
}
