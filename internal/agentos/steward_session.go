// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qiangli/coreutils/pkg/handoff"
	"github.com/qiangli/coreutils/pkg/steward"
)

// The RUNNING STEWARD is a separate fact from the SEAT, and this file is where
// that distinction is kept.
//
// The seat (pkg/steward's journal) answers "who is accountable for this host".
// It is authority, it is replayable, and it survives every process on the
// machine dying. A session record answers something much smaller and much more
// perishable: "is there an agent process here right now, which one, and how do I
// reach it". Storing the second inside the first would put a pid — the most
// disposable fact in the system — into an append-only record designed to outlive
// the hardware.
//
// So the session record is a single mutable file beside the journal, and losing
// it costs nothing the journal cannot rebuild.

const stewardSessionSchema = "bashy-steward-session-v1"

// stewardHandoffState is what the arriving steward was told about its
// predecessor's note — recorded so `steward status`/`stop` can report what the
// agent was ACTUALLY handed, rather than what we hoped it found.
type stewardHandoffState struct {
	Found     bool      `json:"found"`
	ID        string    `json:"id,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
	Age       string    `json:"age,omitempty"`
	Stale     bool      `json:"stale"`
	// Reason says WHY there is nothing usable — "no steward handoff has ever
	// been written here", "the newest one was superseded", "4d old". An absent
	// note with no reason reads as "probably fine", which is the one reading
	// that gets a successor to start from scratch without noticing.
	Reason     string `json:"reason,omitempty"`
	NextAction string `json:"next_action,omitempty"`
}

// stewardSession is the live-process record: which agent is stewarding, under
// which tenure, reachable where.
type stewardSession struct {
	SchemaVersion string `json:"schema_version"`

	Agent      string `json:"agent"` // canonical fleet name, e.g. claude-opus5
	Nick       string `json:"nick,omitempty"`
	Binding    string `json:"binding,omitempty"` // tool:model
	Tool       string `json:"tool,omitempty"`
	Model      string `json:"model,omitempty"`
	Band       int    `json:"band"`
	BandSource string `json:"band_source,omitempty"`
	Billing    string `json:"billing,omitempty"`
	WhyChosen  string `json:"why_chosen,omitempty"`

	SupervisorPID int       `json:"supervisor_pid"`
	StartedAt     time.Time `json:"started_at"`
	Cwd           string    `json:"cwd,omitempty"`
	LogPath       string    `json:"log_path,omitempty"`

	// Epoch is the fencing token the supervisor and the agent write under. Zero
	// means the session was started with --no-seat: it is running, and nothing
	// it claims can be recorded against the host.
	Epoch    uint64 `json:"epoch,omitempty"`
	SeatHeld bool   `json:"seat_held"`
	Room     string `json:"room,omitempty"`
	Topic    string `json:"topic,omitempty"`

	Handoff stewardHandoffState `json:"handoff"`
}

// stewardStopOutcome is what `stop` reports, written by the supervisor on its
// way out.
//
// It exists because the two halves of a stop run in different processes: `stop`
// asks, the supervisor performs. Without a written outcome, `stop` could only
// report that the process it signalled went away — which is true of a graceful
// wrap-up and of a crash, and those are not the same news.
type stewardStopOutcome struct {
	SchemaVersion string    `json:"schema_version"`
	Agent         string    `json:"agent"`
	StoppedAt     time.Time `json:"stopped_at"`

	// NoteWritten says whether a handoff note reached the store, and NoteBy says
	// WHO wrote it: the agent, or bashy's mechanical fallback. A fallback note
	// is a pointer at the journal, not a briefing, and reporting it as a
	// successful handover would be the same lie the fleet exists to prevent.
	NoteWritten bool   `json:"note_written"`
	NoteID      string `json:"note_id,omitempty"`
	NoteBy      string `json:"note_by,omitempty"` // "agent" | "fallback"
	NoteDetail  string `json:"note_detail,omitempty"`

	SeatReleased bool   `json:"seat_released"`
	RoomClosed   bool   `json:"room_closed"`
	Detail       string `json:"detail,omitempty"`
}

func stewardStateDir() (string, error) { return steward.DefaultDir() }

func stewardSessionPath() (string, error) {
	dir, err := stewardStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func stewardOutcomePath() (string, error) {
	dir, err := stewardStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "stop-outcome.json"), nil
}

func stewardLogPath() (string, error) {
	dir, err := stewardStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.log"), nil
}

func saveStewardSession(s *stewardSession) error {
	p, err := stewardSessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	s.SchemaVersion = stewardSessionSchema
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}

// loadStewardSession reads the record. A missing file is (nil, nil): no session
// is a state, not an error.
func loadStewardSession() (*stewardSession, error) {
	p, err := stewardSessionPath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var s stewardSession
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("steward: session record is unreadable (%w) — %s", err, p)
	}
	return &s, nil
}

func clearStewardSession() error {
	p, err := stewardSessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// liveStewardSession returns the session record only when its supervisor is
// still a live process.
//
// A STALE RECORD IS NOT A RUNNING STEWARD, and the difference is reported rather
// than smoothed over: `start` must not refuse because of a record left behind by
// a machine that rebooted, and `stop` must not claim it stopped something that
// was already gone.
func liveStewardSession() (sess *stewardSession, stale bool, err error) {
	s, err := loadStewardSession()
	if err != nil || s == nil {
		return nil, false, err
	}
	if s.SupervisorPID > 0 && stewardProcAlive(s.SupervisorPID) {
		return s, false, nil
	}
	return s, true, nil
}

func saveStewardOutcome(o *stewardStopOutcome) error {
	p, err := stewardOutcomePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	o.SchemaVersion = stewardSessionSchema
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}

func loadStewardOutcome() (*stewardStopOutcome, error) {
	p, err := stewardOutcomePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var o stewardStopOutcome
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// ─── the predecessor's note ───────────────────────────────────────────────────

// stewardHandoffStale is how old a steward's note may be before an arriving
// steward must treat it as a lead rather than a briefing.
//
// It is deliberately SHORTER than handoff.StaleAfter (3 days). That constant
// governs a parked TASK, which is still true a week later — the diff has not
// changed. A steward note describes the state of a whole host: what is running,
// what is blocked, what the human last asked for. A day of fleet activity can
// invalidate all of it while leaving the note looking perfectly current.
const stewardHandoffStale = 24 * time.Hour

// surveyStewardHandoff finds the note an arriving steward should pick up, and —
// when there is not a usable one — says exactly what it did see.
//
// The three outcomes are kept apart on purpose:
//
//	FRESH    a live steward-role handoff inside the staleness window. Resume it.
//	STALE    a live one, but old. Its next-action is a LEAD, not an instruction.
//	MISSING  none live. Whatever retired ones exist are named, because "no note"
//	         and "the last note was cancelled two hours ago" call for different
//	         first moves.
func surveyStewardHandoff(staleAfter time.Duration, now time.Time) (stewardHandoffState, *handoff.Record) {
	st := stewardHandoffState{}
	dir := handoff.DefaultDir()
	recs, err := handoff.List(dir)
	if err != nil {
		st.Reason = fmt.Sprintf("the handoff store could not be read (%v) — investigate before assuming there is nothing to resume", err)
		return st, nil
	}

	var live, newestRetired *handoff.Record
	for _, r := range recs {
		if r.Role != "steward" {
			continue
		}
		switch r.Status() {
		case "transferring", "active":
			if live == nil || r.CreatedAt.After(live.CreatedAt) {
				live = r
			}
		default:
			if newestRetired == nil || r.CreatedAt.After(newestRetired.CreatedAt) {
				newestRetired = r
			}
		}
	}

	if live == nil {
		switch {
		case newestRetired != nil:
			st.Reason = fmt.Sprintf("no live steward note; the newest one (%s, %s) is %s",
				newestRetired.ID, humanAge(now.Sub(newestRetired.CreatedAt)), newestRetired.Status())
		default:
			st.Reason = "no steward handoff note has ever been written on this host"
		}
		return st, nil
	}

	age := now.Sub(live.CreatedAt)
	st.Found = true
	st.ID = live.ID
	st.Status = live.Status()
	st.CreatedAt = live.CreatedAt
	st.Age = humanAge(age)
	st.NextAction = live.NextAction
	if age >= staleAfter {
		st.Stale = true
		st.Reason = fmt.Sprintf("the note is %s old (stale after %s) — a host changes faster than that",
			st.Age, humanAge(staleAfter))
	}
	return st, live
}

func humanAge(d time.Duration) string {
	switch {
	case d < 0:
		return "0s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
