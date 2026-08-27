// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/steward"
)

// `bashy steward start` / `stop` — running the seat, rather than describing it.
//
// Everything else under `bashy steward` reads or writes the RECORD: who holds
// the host, what they claimed, what nobody checked. None of it puts an agent on
// the seat, so a host could have a perfectly maintained journal and nobody
// attending it — and every instruction that said "be the steward" was addressed
// to a human who happened to be reading.
//
// These two verbs close that. `start` selects an agent, takes the seat, opens
// the room, hands the agent its predecessor's note, and leaves it running with a
// supervisor that heartbeats and relays. `stop` asks for a note, checks that one
// arrived, closes the room, and releases the seat.
//
// WHY IT IS AN INTERACTIVE SESSION AND NOT A HEADLESS RUN. A steward is the
// human's continuous point of contact for the whole host; a `--print`-style
// one-shot is deaf by construction, and a deaf steward is the failure this is
// meant to prevent. So the agent runs under a pty with a control socket — it is
// backgrounded, not headless. It stays addressable through the seat room, the
// bus, `bashy chat attach`, and `bashy coach attach`.

type stewardStartOptions struct {
	AgentName string
	Tool      string
	Band      int

	Force    bool
	NoSeat   bool
	KeepSeat bool
	NoNote   bool

	AllowPremium bool
	Yolo         bool

	Sidecar       bool
	Mediator      bool
	MediatorAgent string
	MediatorBand  int

	StaleAfter  time.Duration
	StopTimeout time.Duration
	NudgeWait   time.Duration

	Cwd    string
	AsJSON bool

	// RandomSelection is used only by the lazy permanent-room starter. A human
	// explicitly running `steward start` keeps the explainable cost/quota order.
	RandomSelection bool
}

// newStewardStartCmd builds `bashy steward start`.
func newStewardStartCmd() *cobra.Command {
	var opt stewardStartOptions
	var supervise bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "put an agent on the seat: pick one, take the seat, open its room, resume the handoff note",
		Long: `start runs a steward on this host, in the background.

It is the verb that turns the seat from a record into a post. In order:

  1. SELECT   an agent. --agent names one; otherwise the strongest COST-AWARE
              match at band L` + fmt.Sprint(stewardDefaultBand) + `+ is chosen: a subscription seat you have
              already paid for before a metered API key, and among equals the
              one with the most quota left, read from the same meter the budget
              gate enforces. An agent the gate would block is never selected.
              Below L` + fmt.Sprint(stewardBandFloor) + ` you get a loud warning: a sub-L` + fmt.Sprint(stewardBandFloor) + ` agent in an
              orchestrating seat does not fail cleanly, it LOOPS and reports
              success anyway — worse than an unstewarded host, because it looks
              attended.
  2. ACQUIRE  the seat, through the ordinary authorized path (` + "`steward authorize`" + `
              then ` + "`claim`" + `/` + "`takeover`" + `). Both halves are attended and ask you to
              type the epoch back; there is deliberately no way to skip that,
              because a start verb that could take host authority unattended is
              exactly the capability the confirmation exists to withhold.
              A seat you already hold is reused, not re-acquired.
  3. OPEN     the seat's room, so conductors and other agents can reach the
              holder, and the seat's bus inbox, which needs no room at all.
  4. RESUME   the predecessor's handoff note. A FRESH note is a briefing to
              continue from. A STALE one is a lead to verify. NO note is an
              instruction to investigate and WRITE one before doing anything
              else — that mending step is the steward's job, not a nicety.
  5. SUPERVISE it: heartbeat the seat, host the bus sidecar, and nudge the agent
              at a turn boundary when a message arrives.

ALREADY RUNNING? Starting the same agent again prints its status and changes
nothing. Starting a DIFFERENT one is refused unless --force, which stops the
incumbent (asking it for a note first) before taking over.`,
		Example: `  bashy steward start                        # pick an L` + fmt.Sprint(stewardDefaultBand) + ` agent on cost/quota
  bashy steward start --agent claude-opus5   # name one
  bashy steward start --band 3               # settle for L3
  bashy steward start --force --agent codex-gpt-5.5   # replace the running steward
  bashy steward stop                         # ask for a note, then stand down`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if supervise {
				return runStewardSupervisor(cmd.Context(), cmd.ErrOrStderr(), opt)
			}
			return startStewardSession(cmd.OutOrStdout(), cmd.ErrOrStderr(), opt)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opt.AgentName, "agent", "", "hold the seat with this agent (nick, canonical name, or tool:model)")
	f.StringVar(&opt.Tool, "tool", "", "select any operable agent using this tool")
	f.IntVar(&opt.Band, "band", 0, fmt.Sprintf("select any operable agent at this capability band or above (default %d)", stewardDefaultBand))
	f.BoolVar(&opt.Force, "force", false, "stop a steward run by a DIFFERENT agent and take the post")
	f.BoolVar(&opt.NoSeat, "no-seat", false,
		"run the agent WITHOUT acquiring the seat. It will be unaccountable: every journal write it "+
			"attempts is fenced, and `steward ping` still reports no steward. For a scratch session only")
	f.BoolVar(&opt.KeepSeat, "keep-seat", false, "on stop, leave the seat held (it will lapse on its own)")
	f.BoolVar(&opt.NoNote, "no-note", false, "on stop, do not ask for a handoff note (nothing is written)")
	f.BoolVar(&opt.AllowPremium, "allow-premium", false, "bypass LLM budget/subscription gates for urgent human-authorized work")
	f.BoolVar(&opt.Yolo, "yolo", false,
		"keep the agent CLI's approval-gate kill-switches and accept the uncontained-host risk. "+
			"Without it the tool's own gate stays on, and a background steward with nobody at its terminal "+
			"STALLS on the first approval prompt rather than failing — take the keyboard with `bashy chat attach` "+
			"if that happens, or start with this flag and let the room be the oversight")
	f.BoolVar(&opt.Sidecar, "sidecar", true, "host the bus sidecar in the supervisor (free; it is what makes an interrupt reach a running turn)")
	f.BoolVar(&opt.Mediator, "mediator", true, "triage new messages with a cheap low-band agent before nudging the steward")
	f.StringVar(&opt.MediatorAgent, "mediator-agent", "", "use this agent for message triage")
	f.IntVar(&opt.MediatorBand, "mediator-band", 0, fmt.Sprintf("pick the message-triage agent at this band (default %d; must be below the steward's)", stewardMediatorBand))
	f.DurationVar(&opt.StaleAfter, "stale-after", stewardHandoffStale, "treat a handoff note older than this as a lead rather than a briefing")
	f.DurationVar(&opt.StopTimeout, "stop-timeout", 5*time.Minute, "how long the wrap-up waits for the agent to write its note")
	f.DurationVar(&opt.NudgeWait, "nudge-wait", 10*time.Minute, "how long a message notice waits for a turn boundary before trying again")
	f.StringVar(&opt.Cwd, "cwd", "", "working directory for the steward agent (default: here)")
	f.BoolVar(&opt.AsJSON, "json", false, "emit the bashy-steward-session-v1 envelope")
	f.BoolVar(&supervise, "supervise", false, "internal: run as the supervisor process (spawned by start)")
	_ = f.MarkHidden("supervise")
	return cmd
}

// startStewardSession is the parent side: check, select, acquire, spawn.
func startStewardSession(out, errW io.Writer, opt stewardStartOptions) error {
	// ─── is one already running? ───────────────────────────────────────────
	live, stale, err := liveStewardSession()
	if err != nil {
		return err
	}
	if stale && live != nil {
		fmt.Fprintf(errW, "steward: a session record names %s (pid %d) but that process is gone — clearing it\n",
			live.Agent, live.SupervisorPID)
		_ = clearStewardSession()
		live = nil
	}
	if live != nil {
		same, err := sameStewardAgent(live.Agent, opt)
		if err != nil {
			return err
		}
		if same {
			// The Meet service may have restarted after the supervisor. Rebind the
			// durable @steward alias to the live agent before reporting success.
			if line := steward.EnsureRoom(live.Agent); line != "" {
				fmt.Fprint(errW, line)
			}
			// The idempotent case: a supervisor tick, a second terminal, a
			// script run twice. Report and change nothing.
			return reportStewardSession(out, live, opt.AsJSON, "already running")
		}
		if !opt.Force {
			return fmt.Errorf("steward: %s already holds this host (pid %d, since %s). "+
				"Starting a different agent would give one host two stewards, which is the ownership collapse the "+
				"seat exists to prevent. Reach them first (`bashy steward ping`), or `--force` to stop them and take the post",
				live.Agent, live.SupervisorPID, live.StartedAt.Local().Format(time.RFC3339))
		}
		fmt.Fprintf(errW, "steward: --force — stopping %s before taking the post\n", live.Agent)
		if _, err := stopStewardSession(errW, live, opt.StopTimeout, false); err != nil {
			return fmt.Errorf("steward: the incumbent could not be stopped: %w", err)
		}
	}

	// ─── who ───────────────────────────────────────────────────────────────
	sel, err := selectStewardAgent(opt.AgentName, opt.Tool, opt.Band)
	if err != nil {
		return err
	}
	if opt.RandomSelection && strings.TrimSpace(opt.AgentName) == "" {
		if err := randomizeStewardSelection(sel); err != nil {
			return err
		}
	}
	writeStewardSelection(errW, sel)

	cwd := opt.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	sess := &stewardSession{
		Agent: sel.Chosen.Name, Nick: sel.Chosen.Nick, Binding: sel.Chosen.Binding,
		Tool: sel.Chosen.Tool, Model: sel.Chosen.Model,
		Band: sel.Chosen.Band, BandSource: sel.Chosen.BandSource,
		Billing: sel.Chosen.Billing, WhyChosen: sel.Why,
		StartedAt: time.Now().UTC(), Cwd: cwd,
	}

	// ─── the seat ──────────────────────────────────────────────────────────
	if !opt.NoSeat {
		epoch, err := acquireStewardSeat(errW, sel.Chosen.Name)
		if err != nil {
			return err
		}
		sess.Epoch, sess.SeatHeld = epoch, true
	} else {
		fmt.Fprintln(errW, "steward: --no-seat — this agent is NOT accountable for the host. "+
			"Its journal writes will be fenced and `steward ping` will report no steward.")
	}

	// ─── the room and the inbox ────────────────────────────────────────────
	sess.Topic = steward.Assignment().Topic()
	if line := steward.EnsureRoom(sel.Chosen.Name); line != "" {
		fmt.Fprint(errW, line)
	}
	if c, err := steward.SeatContact(); err == nil && c != nil {
		sess.Room = c.String()
	}
	if _, err := bus.EnsureRoleInbox(sess.Topic); err != nil {
		fmt.Fprintf(errW, "steward: the seat inbox could not be opened (%v) — pings will be stored and unreadable\n", err)
	}

	// ─── the predecessor's note ────────────────────────────────────────────
	state, _ := surveyStewardHandoff(opt.StaleAfter, time.Now().UTC())
	sess.Handoff = state
	writeStewardHandoffState(errW, state)

	// ─── spawn ─────────────────────────────────────────────────────────────
	logPath, err := stewardLogPath()
	if err != nil {
		_ = steward.ReleaseRoom(sel.Chosen.Name)
		return err
	}
	sess.LogPath = logPath
	if err := saveStewardSession(sess); err != nil {
		_ = steward.ReleaseRoom(sel.Chosen.Name)
		return err
	}

	pid, err := spawnStewardSupervisor(opt, logPath)
	if err != nil {
		_ = clearStewardSession()
		_ = steward.ReleaseRoom(sel.Chosen.Name)
		return err
	}
	sess.SupervisorPID = pid
	if err := saveStewardSession(sess); err != nil {
		return err
	}

	return reportStewardSession(out, sess, opt.AsJSON, "started")
}

// spawnStewardSupervisor re-execs this binary as the detached supervisor.
//
// Re-exec rather than a goroutine because the supervisor must OUTLIVE the shell
// that started it: a steward that dies when the operator closes their terminal
// is not a background steward, it is a foreground one with extra steps.
func spawnStewardSupervisor(opt stewardStartOptions, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	args := []string{"steward", "start", "--supervise"}
	if opt.AgentName != "" {
		args = append(args, "--agent", opt.AgentName)
	}
	if opt.MediatorAgent != "" {
		args = append(args, "--mediator-agent", opt.MediatorAgent)
	}
	if opt.MediatorBand != 0 {
		args = append(args, "--mediator-band", fmt.Sprint(opt.MediatorBand))
	}
	args = append(args,
		"--stop-timeout", opt.StopTimeout.String(),
		"--nudge-wait", opt.NudgeWait.String(),
		fmt.Sprintf("--sidecar=%t", opt.Sidecar),
		fmt.Sprintf("--mediator=%t", opt.Mediator),
		fmt.Sprintf("--keep-seat=%t", opt.KeepSeat),
		fmt.Sprintf("--no-note=%t", opt.NoNote),
		fmt.Sprintf("--allow-premium=%t", opt.AllowPremium),
		fmt.Sprintf("--yolo=%t", opt.Yolo),
	)

	cmd := exec.Command(exe, args...)
	// A daemon has nowhere to write: its parent is about to exit and its stdout
	// is a terminal that will close. Without this, everything the supervisor
	// reports — including why it kept dying — vanishes.
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return 0, err
	}
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, err
	}
	defer lf.Close()
	cmd.Stdout, cmd.Stderr = lf, lf
	cmd.Stdin = nil
	stewardDetach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Release, never Wait: this process exits and the supervisor must outlive it.
	_ = cmd.Process.Release()
	return pid, nil
}

// ─── stop ─────────────────────────────────────────────────────────────────────

func newStewardStopCmd() *cobra.Command {
	var timeout time.Duration
	var asJSON, force bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "stand the steward down: ask for a handoff note, close the room, release the seat",
		Long: `stop ends the background steward, in the order that makes the ending USEFUL.

  1. ASK      the agent to record a handoff note (` + "`bashy handoff --as steward`" + `).
  2. VERIFY   that one actually landed. Asking is not being answered, and an
              agent's own "done" is a claim like any other — so stop looks in
              the store. If nothing is there, it writes a MECHANICAL record
              instead, marked as such: a pointer at the journal saying a tenure
              ended untidily, never a briefing pretending to be one.
  3. CLOSE    the seat's room, before the seat is released, so the host never
              advertises a channel to a seat nobody holds.
  4. RELEASE  the seat, fenced with the epoch it was held under.

--force skips step 1 and kills the supervisor. The mechanical note is still
written: a steward that was killed is exactly the case a successor must not
mistake for an idle host.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, errW := cmd.OutOrStdout(), cmd.ErrOrStderr()
			live, stale, err := liveStewardSession()
			if err != nil {
				return err
			}
			if live == nil {
				fmt.Fprintln(out, "steward: no session is running here.")
				return printSeatLine(out)
			}
			if stale {
				fmt.Fprintf(out, "steward: the session record named %s (pid %d), but that process is already gone.\n",
					live.Agent, live.SupervisorPID)
				_ = clearStewardSession()
				return printSeatLine(out)
			}
			outcome, err := stopStewardSession(errW, live, timeout, force)
			if err != nil {
				return err
			}
			if asJSON {
				return emitStewardJSON(out, outcome)
			}
			return writeStewardOutcome(out, live, outcome)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 6*time.Minute, "how long to wait for the wrap-up (the note is an LLM turn)")
	cmd.Flags().BoolVar(&force, "force", false, "kill the supervisor without asking for a note (one is still written mechanically)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the stop outcome as JSON")
	return cmd
}

// stopStewardSession signals the supervisor and waits for it to finish its
// wrap-up, then reads what it actually managed to do.
func stopStewardSession(errW io.Writer, sess *stewardSession, timeout time.Duration, force bool) (*stewardStopOutcome, error) {
	// Clear any previous outcome first, so a wrap-up that never runs cannot be
	// reported using the LAST stop's result — the most convincing possible way
	// to report a success that did not happen.
	if p, err := stewardOutcomePath(); err == nil {
		_ = os.Remove(p)
	}

	if force {
		_ = stewardForceStop(sess.SupervisorPID)
	} else if err := stewardSignalStop(sess.SupervisorPID); err != nil {
		return nil, fmt.Errorf("steward: could not signal the supervisor (pid %d): %w", sess.SupervisorPID, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !stewardProcAlive(sess.SupervisorPID) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if stewardProcAlive(sess.SupervisorPID) {
		fmt.Fprintf(errW, "steward: the wrap-up did not finish within %s — killing the supervisor\n", timeout)
		_ = stewardForceStop(sess.SupervisorPID)
		time.Sleep(time.Second)
	}

	o, err := loadStewardOutcome()
	if err != nil {
		return nil, err
	}
	if o == nil {
		// The supervisor went away without recording an outcome — a crash, a
		// kill, a machine that lost power. Do the parts that can still be done
		// from here rather than leaving the host advertising a dead steward.
		o = &stewardStopOutcome{
			Agent: sess.Agent, StoppedAt: time.Now().UTC(),
			Detail: "the supervisor exited without recording an outcome; the wrap-up below was performed by `steward stop`",
		}
		if line := steward.ReleaseRoom(sess.Agent); line != "" {
			o.RoomClosed = !strings.Contains(line, "could not be closed")
			fmt.Fprint(errW, line)
		}
		if rec, err := writeStewardFallbackNote(sess, "the supervisor exited without a wrap-up"); err == nil {
			o.NoteWritten, o.NoteID, o.NoteBy = true, rec.ID, "fallback"
			o.NoteDetail = "written by `steward stop` — a pointer at the journal, not a briefing"
		}
		if sess.SeatHeld && sess.Epoch > 0 {
			if err := stewardRelease(sess.Epoch, "steward stop (supervisor gone)"); err == nil {
				o.SeatReleased = true
			}
		}
		_ = saveStewardOutcome(o)
		_ = clearStewardSession()
	}
	return o, nil
}

// ─── the seat ─────────────────────────────────────────────────────────────────

// acquireStewardSeat gets the tenure the supervisor will write under.
//
// It does NOT invent a shortcut. Acquiring the seat goes through
// `steward authorize` + `claim`/`takeover` exactly as it always has, including
// the two typed confirmations at a real terminal — because a `start` verb that
// could take host authority unattended would hand every cron job and runaway
// agent loop on the machine precisely the capability those confirmations exist
// to withhold. What it does add is picking the RIGHT one of claim/takeover from
// the seat state, so nobody has to know which situation they are in.
func acquireStewardSeat(errW io.Writer, agent string) (uint64, error) {
	st, err := steward.Open("")
	if err != nil {
		return 0, err
	}
	view, err := st.Status(time.Now())
	if err != nil {
		return 0, err
	}

	// Already ours and alive? Reuse the tenure. A restart of the agent is not a
	// change of authority, and re-acquiring would burn an epoch for nothing.
	self := steward.Self()
	if !view.Authority.Vacant && view.Authority.Holder.Episode == self.Episode && view.Liveness == steward.LivenessLive {
		fmt.Fprintf(errW, "steward: reusing the seat you already hold (epoch %d)\n", view.Authority.Epoch)
		if err := steward.ExportEpoch(view.Authority.Epoch); err != nil {
			return 0, err
		}
		return view.Authority.Epoch, nil
	}

	// A tenure exported into this shell by an earlier `claim --export` is the
	// operator saying "I already did this". Trust it, but VERIFY it against the
	// journal rather than believing the environment.
	if e := steward.EpochFromEnv(); e != 0 && e == view.Authority.Epoch && !view.Authority.Vacant {
		fmt.Fprintf(errW, "steward: using the tenure already exported in this shell (epoch %d)\n", e)
		return e, nil
	}

	fmt.Fprintf(errW, "\nsteward: the seat must be acquired before %s can hold it.\n", agent)
	fmt.Fprintln(errW, "This is deliberately ATTENDED — you will be asked to type the epoch back, twice.")
	fmt.Fprintln(errW, "There is no unattended path, and there is no --yes.")
	fmt.Fprintln(errW)

	action := steward.ActionTakeover
	if view.Authority.Vacant || view.Claimable {
		action = steward.ActionClaim
	}
	return 0, fmt.Errorf("run these two, then `steward start` again:\n"+
		"  grant=$(bashy steward authorize --action %s --actor \"$USER\" --reason \"running a steward\" --json | jq -r .id)\n"+
		"  eval \"$(bashy steward %s --grant \"$grant\" --intent \"background steward: %s\" --export)\"\n\n"+
		"Or start without accountability (a scratch session; nothing it claims is recorded): `steward start --no-seat`",
		action, action, agent)
}

func printSeatLine(out io.Writer) error {
	if err := steward.SeatSummary(out, ""); err != nil {
		fmt.Fprintf(out, "seat: cannot read (%v)\n", err)
	}
	return nil
}

// ─── reporting ────────────────────────────────────────────────────────────────

func sameStewardAgent(running string, opt stewardStartOptions) (bool, error) {
	if strings.TrimSpace(opt.AgentName) == "" && opt.Band == 0 && opt.Tool == "" {
		// No selector: "start the steward" against a host that already has one
		// means the running one, whatever it is.
		return true, nil
	}
	sel, err := selectStewardAgent(opt.AgentName, opt.Tool, opt.Band)
	if err != nil {
		return false, err
	}
	return sel.Chosen.Name == running, nil
}

func writeStewardSelection(w io.Writer, sel *stewardSelection) {
	c := sel.Chosen
	fmt.Fprintf(w, "steward: %s", c.Name)
	if c.Nick != "" {
		fmt.Fprintf(w, " (%s)", c.Nick)
	}
	fmt.Fprintf(w, " — %s, band L%d\n", c.Binding, c.Band)
	fmt.Fprintf(w, "  why:      %s\n", sel.Why)
	if len(sel.Runners) > 0 {
		var names []string
		for i, r := range sel.Runners {
			if i == 3 {
				names = append(names, fmt.Sprintf("+%d more", len(sel.Runners)-3))
				break
			}
			names = append(names, fmt.Sprintf("%s (L%d, %s)", r.Name, r.Band, r.Billing))
		}
		fmt.Fprintf(w, "  runners:  %s\n", strings.Join(names, ", "))
	}
	for _, s := range sel.Skipped {
		fmt.Fprintf(w, "  skipped:  %s — %s\n", s.Name, s.Reason)
	}
	if sel.BandFloor {
		// LOUD, and specific about the failure mode. "Low band" is an
		// abstraction; "it loops and reports success" is what actually happens
		// and what an operator can recognise when it starts.
		fmt.Fprintf(w, "\n  ⚠  WARNING: L%d is BELOW the L%d floor for this seat.\n", c.Band, stewardBandFloor)
		fmt.Fprintln(w, "     A sub-L3 agent in an orchestrating seat is not merely weaker — the measured failure")
		fmt.Fprintln(w, "     is that it LOOPS: it repeats the same tool calls, never converges, and reports")
		fmt.Fprintln(w, "     success anyway. A host stewarded by one is worse than an unstewarded host,")
		fmt.Fprintln(w, "     because it LOOKS attended. Verify its work yourself, and prefer --band 3.")
		fmt.Fprintln(w)
	}
}

func writeStewardHandoffState(w io.Writer, st stewardHandoffState) {
	switch {
	case st.Found && !st.Stale:
		fmt.Fprintf(w, "  handoff:  %s (%s old) — the agent will resume from it\n", st.ID, st.Age)
	case st.Found && st.Stale:
		fmt.Fprintf(w, "  handoff:  %s is STALE (%s) — the agent will treat it as a lead and mend it\n", st.ID, st.Age)
	default:
		fmt.Fprintf(w, "  handoff:  none usable (%s) — the agent will investigate and WRITE one first\n", st.Reason)
	}
}

func reportStewardSession(out io.Writer, s *stewardSession, asJSON bool, what string) error {
	if asJSON {
		return emitStewardJSON(out, s)
	}
	fmt.Fprintf(out, "\nsteward %s: %s (%s, L%d)\n", what, s.Agent, s.Binding, s.Band)
	if s.SeatHeld {
		fmt.Fprintf(out, "  seat:      held at epoch %d\n", s.Epoch)
	} else {
		fmt.Fprintln(out, "  seat:      NOT HELD (--no-seat) — nothing it does is recorded against this host")
	}
	if s.Room != "" {
		fmt.Fprintf(out, "  room:      %s\n", s.Room)
	}
	if s.Topic != "" {
		fmt.Fprintf(out, "  inbox:     bus %s  (`bashy steward ping --body ...`)\n", s.Topic)
	}
	fmt.Fprintf(out, "  pid:       %d\n", s.SupervisorPID)
	fmt.Fprintf(out, "  log:       %s\n", s.LogPath)
	fmt.Fprintln(out, "\n  watch:     bashy steward status · bashy chat sessions")
	fmt.Fprintln(out, "  talk:      bashy steward ping --body \"...\" · bashy mb send --to steward ...")
	fmt.Fprintln(out, "  take over: bashy chat attach "+s.Agent)
	fmt.Fprintln(out, "  stop:      bashy steward stop")
	return nil
}

func writeStewardOutcome(out io.Writer, s *stewardSession, o *stewardStopOutcome) error {
	fmt.Fprintf(out, "steward stopped: %s\n", o.Agent)
	if o.Detail != "" {
		fmt.Fprintf(out, "  reason:    %s\n", o.Detail)
	}
	switch {
	case o.NoteWritten && o.NoteBy == "agent":
		fmt.Fprintf(out, "  handoff:   %s — written by the agent\n", o.NoteID)
		if o.NoteDetail != "" {
			fmt.Fprintf(out, "             %s\n", o.NoteDetail)
		}
	case o.NoteWritten:
		fmt.Fprintf(out, "  handoff:   %s — MECHANICAL fallback\n", o.NoteID)
		fmt.Fprintln(out, "             The agent left no briefing. The next steward must reconcile the journal;")
		fmt.Fprintln(out, "             do not read this as a clean handover.")
	default:
		fmt.Fprintln(out, "  handoff:   NONE — nothing was recorded. The next steward starts blind.")
	}
	fmt.Fprintf(out, "  room:      %s\n", yesNoLabel(o.RoomClosed, "closed", "still open"))
	if s.SeatHeld {
		fmt.Fprintf(out, "  seat:      %s\n", yesNoLabel(o.SeatReleased, "released", "still held (it will lapse on its own)"))
	}
	fmt.Fprintln(out, "\n  resume:    bashy resume        # read the note")
	fmt.Fprintln(out, "             bashy steward start # put an agent back on the seat")
	return nil
}

func yesNoLabel(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func emitStewardJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ─── the brief ────────────────────────────────────────────────────────────────

// stewardBootstrapBrief is the first thing the agent is told, and it is the
// actual product of this feature.
//
// A background steward that is merely LAUNCHED does nothing useful: it has no
// task, no memory of the host, and no reason to believe it is accountable for
// anything. The brief is where "you are the steward now" becomes a set of
// actions with an order, and where the three handoff-note situations get three
// DIFFERENT instructions — because "there is no note" and "the note is a day
// old" call for opposite first moves, and an agent given one instruction for
// both will pick the reassuring reading.
func stewardBootstrapBrief(sess *stewardSession, opt stewardStartOptions) (string, stewardHandoffState) {
	state, rec := surveyStewardHandoff(opt.StaleAfter, time.Now().UTC())

	var b strings.Builder
	b.WriteString("You are now the STEWARD of this host. This is a background session: no human is watching\n")
	b.WriteString("your terminal continuously, and everything you are accountable for is reachable through bashy.\n")
	b.WriteString("At the start of EVERY turn, first process any newly delivered human\n")
	b.WriteString("instructions and rebuild the turn TODO; those instructions may replace the task or the TODO itself.\n")
	b.WriteString("Only then inspect state and derive the remaining task-specific supervisory checklist.\n\n")

	b.WriteString("FIRST, LOAD THE ROLE:\n")
	b.WriteString("    bashy steward skill        # your operating contract — read it before acting\n")
	b.WriteString("    bashy steward status       # the seat, its liveness, and the board\n\n")

	if sess.SeatHeld {
		fmt.Fprintf(&b, "The seat is HELD in your name at epoch %d, and $%s is already exported into your\n",
			sess.Epoch, steward.EpochEnv)
		b.WriteString("environment — every journal write presents it automatically. You do not need to claim anything.\n\n")
	} else {
		b.WriteString("THE SEAT IS NOT HELD (started with --no-seat). Every journal write you attempt will be\n")
		b.WriteString("FENCED, and anyone pinging this host will be told there is no steward. Say so if you are\n")
		b.WriteString("asked to record anything; do not work around it.\n\n")
	}

	fmt.Fprintf(&b, "YOU ARE REACHABLE at bus topic %s", sess.Topic)
	if sess.Room != "" {
		fmt.Fprintf(&b, " and in room %s", sess.Room)
	}
	b.WriteString(".\nA supervisor process heartbeats the seat for you and will tell you, between turns, when a message\n")
	b.WriteString("arrives. It tells you the COUNT, never the contents — read them yourself so the read is\n")
	b.WriteString("recorded against you:\n")
	b.WriteString("    bashy steward inbox        # messages addressed to the SEAT (including your predecessors')\n")
	b.WriteString("    bashy mb                   # the host message board\n\n")

	// ── the note ──
	b.WriteString("YOUR PREDECESSOR'S HANDOFF NOTE\n")
	switch {
	case rec != nil && !state.Stale:
		fmt.Fprintf(&b, "There is a current one (%s, %s old). Pick it up and continue from where it left off:\n", rec.ID, state.Age)
		b.WriteString("    bashy resume               # read it\n")
		b.WriteString("    bashy resume --claim       # TAKE it — this is what makes you its owner\n\n")
		if s := strings.TrimSpace(rec.Continuity); s != "" {
			b.WriteString("Its brief:\n")
			b.WriteString(indentBlock(s))
			b.WriteString("\n")
		}
		if s := strings.TrimSpace(rec.NextAction); s != "" {
			b.WriteString("Its stated next action:\n")
			b.WriteString(indentBlock(s))
			b.WriteString("\n")
		}
		b.WriteString("VERIFY BEFORE YOU TRUST IT. The note says what a predecessor BELIEVED. Confirm the world\n")
		b.WriteString("still matches (`bashy board`, `bashy weave status`, `git status`) before acting on it.\n\n")

	case rec != nil && state.Stale:
		fmt.Fprintf(&b, "There is one, but it is STALE: %s\n", state.Reason)
		fmt.Fprintf(&b, "Record %s says:\n", rec.ID)
		b.WriteString(indentBlock(strings.TrimSpace(rec.Continuity)))
		if s := strings.TrimSpace(rec.NextAction); s != "" {
			b.WriteString("\nIts next action was:\n")
			b.WriteString(indentBlock(s))
		}
		b.WriteString("\nTREAT IT AS A LEAD, NOT A BRIEFING. A host changes faster than that note is old: work may\n")
		b.WriteString("have finished, failed, or been abandoned since. MENDING IT IS PART OF YOUR JOB — do this\n")
		b.WriteString("before you start anything new:\n")
		b.WriteString(stewardInvestigationBlock())
		b.WriteString("    # then record what is ACTUALLY true now, superseding the stale note:\n")
		b.WriteString("    bashy handoff --as steward -m \"<what is really in flight, and what the stale note got wrong>\" \\\n")
		b.WriteString("        --next \"<the real next action>\"\n\n")

	default:
		fmt.Fprintf(&b, "There is NO usable note: %s\n", state.Reason)
		b.WriteString("This is not permission to assume the host was idle — it is the opposite. Something was\n")
		b.WriteString("probably in flight and nobody wrote it down. RECONSTRUCTING IT IS YOUR FIRST TASK:\n")
		b.WriteString(stewardInvestigationBlock())
		b.WriteString("    # then write the note that should have existed, so the next steward is not blind:\n")
		b.WriteString("    bashy handoff --as steward -m \"<what you found in flight, and how you established it>\" \\\n")
		b.WriteString("        --next \"<the next action>\"\n\n")
		b.WriteString("Say plainly in that note what you could NOT establish. An unknown recorded as an unknown is\n")
		b.WriteString("worth more than a guess recorded as a fact.\n\n")
	}

	b.WriteString("THEN RUN YOUR LOOP (the skill has the full version):\n")
	b.WriteString("  reconcile → read messages → partition authority → appoint conductors → monitor → verify → record.\n")
	b.WriteString("Record what you do as you go — `bashy steward record`, `bashy steward decide`. A claim with\n")
	b.WriteString("nothing to point at projects as UNKNOWN, and a claim nobody checked projects as ASSERTED.\n")
	b.WriteString("Only `bashy steward verify` makes something verified.\n\n")
	b.WriteString("Stay responsive. You will be interrupted with messages between turns; answer them. When you are\n")
	b.WriteString("asked to stand down, you will be told explicitly — write the handoff note then.\n")

	return b.String(), state
}

// stewardInvestigationBlock is the mending recipe: what to look at when the note
// is missing or stale. It is the same list in both cases on purpose — the
// question ("what is actually in flight here?") is identical; only the reason
// for asking differs.
func stewardInvestigationBlock() string {
	return "" +
		"    bashy steward reconcile    # what the journal can and cannot establish — run this FIRST\n" +
		"    bashy steward log --degraded  # claims nobody ever checked\n" +
		"    bashy board                # todo + sprints + weave runs across the host\n" +
		"    bashy weave status         # workspaces still open, and whether they committed anything\n" +
		"    bashy chat sessions        # agents live on this host right now\n" +
		"    bashy resume --all         # every handoff record, with status\n" +
		"    bashy kb search <topic>    # what previous agents wrote down\n" +
		"    git -C <repo> log --oneline -20 && git -C <repo> status   # per repo: what moved, what is dirty\n"
}

// stewardWrapUpInstruction is the last thing the agent is told.
//
// It is explicit about the format and the verb because this is the message that
// has to work when the agent is tired, mid-task, and about to be terminated: a
// vague "wrap up please" produces prose in the transcript, which dies with the
// session. Only a `bashy handoff` record survives it.
func stewardWrapUpInstruction(sess *stewardSession) string {
	var b strings.Builder
	b.WriteString("STAND DOWN. Your steward session is ending now. Do exactly one thing before you stop:\n\n")
	b.WriteString("Record the handoff note for the next steward — which may well be you, cold, with none of\n")
	b.WriteString("this context. Run it as a COMMAND; prose in this conversation dies with the session:\n\n")
	b.WriteString("    bashy handoff --as steward \\\n")
	b.WriteString("      -m \"<what you were doing and why · what you established and HOW · what you could not\n")
	b.WriteString("           establish · what changed on this host during your tenure>\" \\\n")
	b.WriteString("      --next \"<the single next action, stated so a COLD agent in a DIFFERENT tool can act\n")
	b.WriteString("              on it without re-deriving your plan>\" \\\n")
	b.WriteString("      --blocker \"<anything in the way>\"\n\n")
	b.WriteString("Be honest about the unfinished and the unverified. A successor who inherits an accurate\n")
	b.WriteString("\"I could not confirm X\" is far better off than one who inherits a confident summary that\n")
	b.WriteString("turns out to be wrong — that is the whole reason this record exists.\n\n")
	if sess.SeatHeld {
		b.WriteString("Do NOT release the seat or close the room yourself; bashy does that after your note lands.\n")
	}
	b.WriteString("Reply when the handoff command has run.\n")
	return b.String()
}

func indentBlock(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n") + "\n"
}
