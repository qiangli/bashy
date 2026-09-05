// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// R0-pre — register the agent session nobody else knows about.
//
// `bashy agents list` is the address book, and its entries are `tool:model`
// bindings. Using it as an address book ASSUMES the singleton rule: one
// identity, one live occupant. That assumption is currently FALSE, and it was
// measured rather than suspected.
//
// A session launched by `bashy chat` claims its identity through room.Join, so
// it appears in `chat sessions` and can be reached. A session launched the
// ordinary way — a human types `claude` in a terminal, with bashy configured as
// its shell — claims NOTHING. It is invisible to the room, invisible to
// `chat sessions`, and its identity looks free. On this host that produced two
// live `claude-opus5`: one registered at a pid the room knew, and one that had
// never been seen. A DM to that binding would reach the registered process,
// miss the other entirely, and report success.
//
// bashy is the shell under every one of those tools, which makes it the only
// process positioned to notice. So it registers what it sees.
//
// # A presence card, NOT a claim
//
// The card is filed under a `shell:` id, deliberately outside the namespace
// room.Join claims for real sessions. That is the whole design:
//
//   - it can never refuse a later `bashy chat` launch of the same binding — a
//     shell that locks the operator out of their own agent is a shell nobody
//     runs, the same reasoning the weave guard follows;
//   - several raw launches of one tool coexist, each visible, because a human
//     may legitimately run two;
//   - the address book groups by BINDING, so a collision becomes visible
//     instead of silent, which is the entire point.
//
// # What it does not know, and does not guess
//
// The MODEL is not discoverable for a raw launch: DetectAgent reads env markers
// that name the TOOL (`CLAUDECODE=1` -> claude) and nothing carries the model.
// So Model is left EMPTY. A card claiming `claude:opus5` on the strength of a
// guess would put a fabricated address in the address book, which is worse than
// an incomplete one — the whole feature exists because a message went somewhere
// nobody was.
package agentos

import (
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/room"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
)

// shellSessionPrefix namespaces presence cards away from the ids room.Join
// claims exclusively. Nothing may ever file a real session under it.
const shellSessionPrefix = "shell:"

// shellSessionMode is the Card.Mode value for a presence card. It sits beside
// the existing interactive|weave|foreman|meet modes and means: running under
// this tool, reachable only if it looks — there is no control socket.
const shellSessionMode = "shell"

// registerShellSession files a presence card for the agent this shell runs
// under, if any.
//
// Best-effort and silent: every failure is discarded. A shell that refused to
// start because a presence card could not be written would be trading the
// operator's session for a status board.
func registerShellSession() {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BASHY_SHELL_PRESENCE")), "off") {
		return
	}
	tool, ok := coreskills.DetectAgent()
	if !ok || strings.TrimSpace(tool) == "" {
		return
	}

	// The AGENT's pid, not this shell's. Agent CLIs run each command in a fresh
	// shell, so keying on os.Getpid() would file a new card per command and
	// leave a trail of dead ones. The parent is the agent process itself —
	// verified on this host: every `bashy` invocation shares one ppid, the
	// claude process driving it.
	//
	// The failure mode is bounded and self-correcting: inside a nested subshell
	// the parent is that shell rather than the agent, so the card describes a
	// shorter-lived occupant and is pruned when it exits. It never names
	// something that is not running.
	agentPID := os.Getppid()
	if agentPID <= 1 {
		return
	}

	// Already registered by a path that DOES claim — a `bashy chat` launch files
	// a real card at this pid. Filing a presence card beside it would double
	// count one occupant, and the address book would report a collision that
	// does not exist.
	if members, err := room.Members(); err == nil {
		for _, m := range members {
			if m.PID == agentPID {
				return
			}
		}
	}

	cwd, _ := os.Getwd()
	_ = room.Join(room.Card{
		ID:      shellSessionPrefix + tool + ":" + strconv.Itoa(agentPID),
		Tool:    tool,
		Binding: tool, // model unknown for a raw launch; never guessed
		Mode:    shellSessionMode,
		PID:     agentPID,
		Cwd:     cwd,
		// CtlSock is deliberately EMPTY. There is no socket, so nothing can be
		// pushed into this session — presence must report it as reachable only
		// by polling, and an advertised socket that does not exist would be the
		// exact lie this whole line of work is removing.
	})
}

// IsShellPresenceCard reports whether a card is a presence registration rather
// than a claimed session. Readers use it to render reach honestly: a presence
// card means "can send, cannot be pushed to".
func IsShellPresenceCard(c room.Card) bool {
	return strings.HasPrefix(c.ID, shellSessionPrefix)
}

// fleetAgentNames lists the address book — every agent the catalog knows.
//
// It is the input to `bus subscriptions --reconcile`: the fleet IS the set that
// needs inboxes, because `bashy agents list` is what a human or an agent reads
// when deciding who to message. A catalog that will not load yields no names
// rather than an error; reconciliation is a repair, and a repair that fails
// loudly on a read it did not need is worse than one that does nothing.
func fleetAgentNames() []string {
	agents, _ := fleet.New().Agents()
	out := make([]string, 0, len(agents))
	for _, a := range agents {
		if n := strings.TrimSpace(a.Name); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// fleetSelectAudience answers a `bashy mb send --band/--tool/--provider`
// selector from the CATALOG — the same catalog `bashy agents list` prints.
//
// That identity is the point. A selector resolved from a copy kept elsewhere is
// a second opinion about who is L4, and a second opinion can drift; answering
// from the catalog means the address book and the audience can never disagree.
//
// Criteria are ANDed, matching the flag documentation. A cascade agent has no
// plain model binding (Base is set and Model ignored), so a provider or band
// filter cannot describe it and it is skipped rather than guessed at — the same
// no-attribution-no-row rule the capability ledger uses.
func fleetSelectAudience(aud bus.Audience) ([]string, error) {
	cat := fleet.New()
	agents, _ := cat.Agents()
	var out []string
	for _, a := range agents {
		if aud.Tool != "" && !strings.EqualFold(a.Tool, aud.Tool) {
			continue
		}
		if aud.Band != 0 || aud.Provider != "" || aud.Family != "" || aud.Version != "" {
			m, ok := cat.Model(a.Model)
			if !ok {
				continue // cascade or unresolvable binding: not describable, not guessed
			}
			if aud.Band != 0 && m.Band != aud.Band {
				continue
			}
			if aud.Provider != "" && !strings.EqualFold(m.Provider, aud.Provider) {
				continue
			}
			if aud.Family != "" && !strings.EqualFold(m.Family, aud.Family) {
				continue
			}
			if aud.Version != "" && !strings.EqualFold(m.Version, aud.Version) {
				continue
			}
		}
		if n := strings.TrimSpace(a.Name); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// fleetResolveAgentName maps any spelling of an agent to its canonical fleet
// name — the one `bashy agents list` prints and `mb send` addresses.
//
// This is what lets a bare `bashy mb` work. A reader's environment does not
// carry its fleet name: a bashy-launched agent has
// BASHY_PRINCIPAL=dhnt:agent/<Nick>, and everything else falls back to $USER.
// Posts are addressed to the fleet name, so without this the send and read
// sides resolve different identities and a post lands where its recipient never
// looks — which is precisely what happened before this existed.
//
// An unknown string comes back EMPTY rather than guessed at, so the caller can
// fall through to using it verbatim: a human at a terminal is a legitimate board
// participant under their login name, not an agent to be resolved.
func fleetResolveAgentName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Canonicalize any PRINCIPAL, not agents only.
	//
	// This is the board's one name-folding hook: bus.BoardIdentity routes every
	// sender and every addressee through it. Resolving agents alone meant a
	// human's name never folded, so the operator was whatever string each
	// surface happened to hand over — `qiangli` from the CLI, the cloud handle
	// from the browser — and their posts and their inbox split between the two.
	// An agent got one canonical name; a person got none.
	if canonical, _, ok := fleet.New().CanonicalPrincipal(s); ok {
		return canonical
	}
	return ""
}
