package agentos

import (
	"context"
	"encoding/binary"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/room"
)

// A unified snapshot reads every MB post, Meet room header/transcript, Bus
// notification, and pending role buffer. On a busy host that can take longer
// than the old fixed 100 ms poll and keep an idle watcher near one full core.
// The watch loop therefore polls cheap file metadata, reads fully on change,
// and backs the metadata poll off to this bounded ceiling.
const (
	inboxPollMin    = 100 * time.Millisecond
	inboxPollMax    = time.Second
	inboxFullRescan = 30 * time.Second
)

type inboxPollRuntime struct {
	min, max, fullRescan time.Duration
	now                  func() time.Time
	wait                 func(context.Context, time.Duration) error
	snapshot             func(string, int, bool) (inboxBatch, error)
	fingerprint          func(string) (uint64, bool)
}

func defaultInboxPollRuntime() inboxPollRuntime {
	return inboxPollRuntime{
		min: inboxPollMin, max: inboxPollMax, fullRescan: inboxFullRescan,
		now: time.Now, wait: waitInboxPoll,
		snapshot: snapshotUnifiedInbox, fingerprint: inboxSourcesFingerprint,
	}
}

func waitInboxPoll(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// inboxPollGate fails open: an incomplete metadata sample always causes a full
// read. The fingerprint is sampled before that read and committed afterward;
// an event appended during the read therefore changes the next sample and
// cannot fall into the read/commit race.
type inboxPollGate struct {
	reader      string
	fingerprint func(string) (uint64, bool)
	fullRescan  time.Duration
	sampled     bool
	sum         uint64
	lastFull    time.Time
}

func (g *inboxPollGate) due(now time.Time) (read, changed bool, sum uint64, ok bool) {
	sum, ok = g.fingerprint(g.reader)
	switch {
	case !ok || !g.sampled:
		return true, false, sum, ok
	case sum != g.sum:
		return true, true, sum, ok
	case now.Sub(g.lastFull) >= g.fullRescan:
		return true, false, sum, ok
	default:
		return false, false, sum, ok
	}
}

func (g *inboxPollGate) commit(sum uint64, ok bool, now time.Time) {
	g.sampled = ok
	g.sum = sum
	g.lastFull = now
}

// inboxSourcesFingerprint hashes metadata for every durable input consulted by
// snapshotUnifiedInbox. Missing stores are a valid stable state; any other stat
// or enumeration error makes the sample incomplete so the gate fails open.
func inboxSourcesFingerprint(reader string) (uint64, bool) {
	f := sourceFingerprinter{h: fnv.New64a(), ok: true}

	// Message board posts, and Bus routing/materialization. Hash all pending and
	// subscription entries because the current reader may also hold a legacy
	// role inbox whose topic is discovered dynamically.
	f.file(filepath.Join(bus.BoardDir(), "posts.jsonl"))
	roomDir := room.Dir()
	f.file(filepath.Join(roomDir, "timeline.jsonl"))
	f.dir(filepath.Join(roomDir, "pending"), false)
	f.dir(filepath.Join(roomDir, "subs"), false)
	if bus.HostRoles != nil {
		for _, role := range bus.HostRoles() {
			f.string("role")
			f.string(role.Label)
			f.string(role.Topic)
			f.string(role.Holder)
		}
	}

	// Meet does not expose its store root. Mirror its documented
	// BASHY_MEET_DIR/default location, then stat each room's header and
	// transcript. state.json controls board membership; transcript.jsonl carries
	// delivery. This avoids parsing every room merely to learn that none moved.
	meetDir, ok := inboxMeetDir()
	if !ok {
		return 0, false
	}
	f.dir(meetDir, true)
	return f.h.Sum64(), f.ok
}

func inboxMeetDir() (string, bool) {
	if dir := strings.TrimSpace(os.Getenv("BASHY_MEET_DIR")); dir != "" {
		return dir, true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".bashy", "meet"), true
}

type sourceFingerprinter struct {
	h  hash.Hash64
	ok bool
}

func (f *sourceFingerprinter) file(path string) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			f.string(path + "\x00absent")
		} else {
			f.ok = false
		}
		return
	}
	f.stat(path, info)
}

// dir hashes the directory entry list and each entry's metadata. With children
// true, it additionally hashes state.json and transcript.jsonl in every room.
func (f *sourceFingerprinter) dir(path string, children bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			f.string(path + "\x00absent")
		} else {
			f.ok = false
		}
		return
	}
	if info, err := os.Stat(path); err == nil {
		f.stat(path, info)
	} else {
		f.ok = false
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			f.ok = false
			continue
		}
		entryPath := filepath.Join(path, entry.Name())
		f.stat(entryPath, info)
		if children && entry.IsDir() {
			f.file(filepath.Join(entryPath, "state.json"))
			f.file(filepath.Join(entryPath, "transcript.jsonl"))
		}
	}
}

func (f *sourceFingerprinter) stat(path string, info os.FileInfo) {
	f.string(path)
	f.integer(info.Size())
	f.integer(info.ModTime().UnixNano())
	f.integer(int64(info.Mode()))
}

func (f *sourceFingerprinter) string(value string) {
	_, _ = f.h.Write([]byte(value))
	_, _ = f.h.Write([]byte{0})
}

func (f *sourceFingerprinter) integer(value int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(value))
	_, _ = f.h.Write(buf[:])
}
