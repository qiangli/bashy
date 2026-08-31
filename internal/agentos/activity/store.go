// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package activity

// The OUTBOX.
//
// This is the only state this package keeps, and naming it correctly matters:
// it is an outbox, not a mailbox. It answers three questions and no others —
// have I already published this event, what sequence does this source's next
// event get, and which publishes did I start but not finish. It holds no
// per-recipient read state, because pkg/bus already owns that and two answers
// to "have I read this" is worse than none.
//
// The log is append-only JSONL and folded by event id on read, latest line
// wins. Append-only is what makes the crash window safe: a record is written
// as unpublished BEFORE the durable bus append and rewritten as published
// after, so a crash in between leaves evidence that a delivery was owed rather
// than silence that looks like nothing happened.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

// StateDir resolves the outbox root, following the ladder audit, foreman and
// the skills store already use:
//
//	$BASHY_ACTIVITY_DIR    the specific override, most precise, wins
//	$BASHY_HOME/activity   the whole bashy home relocated (test isolation)
//	~/.config/bashy/activity
//
// $BASHY_HOME is load-bearing for the tests: a test that sets only the
// specific override still leaves anything that resolves the home writing to
// the operator's real one, which produces a suite that believes it is hermetic
// and is not. An empty return means no home could be determined; callers must
// treat it as "no store" rather than as a path.
func StateDir() string {
	if dir := strings.TrimSpace(os.Getenv("BASHY_ACTIVITY_DIR")); dir != "" {
		return dir
	}
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		return filepath.Join(home, "activity")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "bashy", "activity")
	}
	return ""
}

const (
	journalFile   = "outbox.jsonl"
	interestsFile = "interests.json"
	lockFile      = "activity.lock"

	// MaxJournalRecords bounds the folded outbox. Pruning drops the OLDEST
	// published records, and the consequence is stated rather than hidden: a
	// dedup key older than the retention window can publish a second time.
	// That is exactly the at-least-once semantic this package claims, so
	// pruning weakens nothing it promised — but a reader of this file should
	// not have to derive that.
	MaxJournalRecords = 5000

	lockWait = 5 * time.Second
)

// DeliveryState of one recipient's copy.
const (
	// WakeSteered — the recipient was live and the control socket took the frame.
	WakeSteered = "steered"
	// WakeQueued — durable only, to be read at the recipient's next turn boundary.
	WakeQueued = "queued"
	// WakeCoalesced — a wake for the same object was already delivered inside the
	// window, so this one stayed queued. The event itself was still published.
	WakeCoalesced = "coalesced"
	// WakeRateLimited — the per-minute wake cap bound. DEMOTED, never dropped.
	WakeRateLimited = "rate-limited"
	// WakeUnreachable — no live session and no control socket. Durable delivery
	// stands; this only records that nobody was woken.
	WakeUnreachable = "unreachable"
)

// Record is one journal line: an event, who it was routed to, and what
// happened to each delivery.
type Record struct {
	Event      Event       `json:"event"`
	Recipients []Recipient `json:"recipients,omitempty"`

	// Delivered records the DURABLE publish per recipient, and it is per
	// recipient rather than one flag for the record because that is what makes
	// recovery resumable. A single flag would force a crash midway through a
	// twelve-recipient fan-out to choose between re-publishing to all twelve or
	// none, and both answers are wrong.
	Delivered map[string]bool `json:"delivered,omitempty"`

	// Published is true once every recipient has been durably published to.
	Published bool `json:"published"`

	// Wakes maps recipient to wake outcome. A wake outcome is never a delivery
	// outcome: `queued` and `unreachable` both sit on top of a successful
	// durable append.
	Wakes map[string]string `json:"wakes,omitempty"`
	// Note carries a delivery-side failure so status can report it. It never
	// carries event content.
	Note string `json:"note,omitempty"`
}

// store is the on-disk outbox, held under a kernel lock for the duration of an
// emit so two concurrent bashy processes cannot assign the same source
// sequence or double-publish the same id.
type store struct {
	dir string
}

func openStore() (*store, error) {
	dir := StateDir()
	if dir == "" {
		return nil, fmt.Errorf("activity: no state directory (set BASHY_ACTIVITY_DIR or BASHY_HOME)")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("activity: creating %s: %w", dir, err)
	}
	return &store{dir: dir}, nil
}

func (s *store) lock(intent string) (*lockfile.Lock, error) {
	l, err := lockfile.AcquireWithin(filepath.Join(s.dir, lockFile), lockWait, lockfile.Holder{
		Name: "activity", PID: os.Getpid(), Intent: intent,
	})
	if err != nil {
		return nil, fmt.Errorf("activity: %w", err)
	}
	return l, nil
}

// load reads the journal and folds it by event id, latest line winning.
// Ordering within a source is preserved because Seq is assigned under the lock
// and the fold sorts by (source, seq).
func (s *store) load() ([]Record, error) {
	f, err := os.Open(filepath.Join(s.dir, journalFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	byID := map[string]Record{}
	var order []string
	sc := bufio.NewScanner(f)
	// An object reference is capped at 96 bytes and a record has a bounded
	// number of them, so a long line is corruption rather than a big event.
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if json.Unmarshal([]byte(line), &r) != nil {
			// A torn or corrupt line is skipped, not fatal. Refusing to read the
			// whole outbox because one line is unparseable would turn a cosmetic
			// fault into a total delivery outage.
			continue
		}
		if r.Event.ID == "" {
			continue
		}
		if _, seen := byID[r.Event.ID]; !seen {
			order = append(order, r.Event.ID)
		}
		byID[r.Event.ID] = r
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Event.Source != out[b].Event.Source {
			return out[a].Event.Source < out[b].Event.Source
		}
		return out[a].Event.Seq < out[b].Event.Seq
	})
	return out, nil
}

// append writes one record. It fsyncs before returning: the whole point of
// writing the record before the bus publish is that it survives the crash, and
// a record still in the page cache does not.
func (s *store) append(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, journalFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// nextSeq assigns the next per-source sequence from the folded journal.
//
// Per-source rather than global on purpose: the guarantee this package makes
// is ordering PER SOURCE. A global counter would imply a total order across
// subsystems that no single lock actually establishes — weave and meet commit
// independently — and an implied guarantee is the kind that gets relied on
// exactly once, in the incident.
func nextSeq(records []Record, source string) int64 {
	var high int64
	for _, r := range records {
		if r.Event.Source == source && r.Event.Seq > high {
			high = r.Event.Seq
		}
	}
	return high + 1
}

func findRecord(records []Record, id string) (Record, bool) {
	for _, r := range records {
		if r.Event.ID == id {
			return r, true
		}
	}
	return Record{}, false
}

// prune rewrites the journal keeping the newest MaxJournalRecords folded
// records, and always keeping every UNPUBLISHED one regardless of age: an
// unpublished record is an owed delivery, and dropping it to save space would
// convert at-least-once into sometimes.
func (s *store) prune(records []Record) error {
	if len(records) <= MaxJournalRecords {
		return nil
	}
	keep := make([]Record, 0, MaxJournalRecords)
	drop := len(records) - MaxJournalRecords
	for i, r := range records {
		if i < drop && r.Published {
			continue
		}
		keep = append(keep, r)
	}
	tmp := filepath.Join(s.dir, journalFile+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, r := range keep {
		b, merr := json.Marshal(r)
		if merr != nil {
			continue
		}
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, journalFile))
}

// --- interests -------------------------------------------------------------

// LoadInterests reads every declared interest, sorted by subscriber.
func LoadInterests() ([]Interest, error) {
	s, err := openStore()
	if err != nil {
		return nil, err
	}
	return s.loadInterests()
}

func (s *store) loadInterests() ([]Interest, error) {
	b, err := os.ReadFile(filepath.Join(s.dir, interestsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Interest
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("activity: interests are unreadable: %w", err)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Subscriber < out[b].Subscriber })
	return out, nil
}

func (s *store) saveInterests(in []Interest) error {
	sort.Slice(in, func(a, b int) bool { return in[a].Subscriber < in[b].Subscriber })
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, interestsFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Subscribe records or replaces one identity's interest.
//
// Replacement rather than merge is deliberate. A merge would make an interest
// only ever widen — every subscribe call adding sources it never removes —
// and the failure mode of this whole design is an interest that quietly grew
// into a firehose. `bashy activity subscribe` states the whole interest.
func Subscribe(in Interest) error {
	if strings.TrimSpace(in.Subscriber) == "" {
		return fmt.Errorf("activity: an interest needs a subscriber name")
	}
	s, err := openStore()
	if err != nil {
		return err
	}
	l, err := s.lock("subscribe")
	if err != nil {
		return err
	}
	defer l.Release()

	existing, err := s.loadInterests()
	if err != nil {
		return err
	}
	replaced := false
	for i := range existing {
		if strings.EqualFold(existing[i].Subscriber, in.Subscriber) {
			existing[i] = in
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, in)
	}
	return s.saveInterests(existing)
}

// Unsubscribe removes an identity's interest. Removing an interest stops
// SUBSCRIPTION, DEPENDENCY and MEMBERSHIP routing; it does not stop mention,
// assignment or ownership, because those are facts about the event rather than
// preferences of the recipient. An identity that wants silence sets Mute.
func Unsubscribe(subscriber string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	l, err := s.lock("unsubscribe")
	if err != nil {
		return err
	}
	defer l.Release()

	existing, err := s.loadInterests()
	if err != nil {
		return err
	}
	out := existing[:0]
	found := false
	for _, i := range existing {
		if strings.EqualFold(i.Subscriber, subscriber) {
			found = true
			continue
		}
		out = append(out, i)
	}
	if !found {
		return fmt.Errorf("activity: %q has no declared interest", subscriber)
	}
	return s.saveInterests(out)
}
