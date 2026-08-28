package agentos

import (
	"context"
	"encoding/binary"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/rjeczalik/notify"
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
	close                func()
}

func defaultInboxPollRuntime(follow bool) inboxPollRuntime {
	if !follow {
		return inboxPollRuntime{
			min: inboxPollMin, max: inboxPollMax, fullRescan: inboxFullRescan,
			now: time.Now, wait: waitInboxPoll,
			snapshot:    snapshotUnifiedInbox,
			fingerprint: func(string) (uint64, bool) { return 1, true },
		}
	}
	changes := newInboxChangeNotifier()
	return inboxPollRuntime{
		min: inboxPollMin, max: inboxPollMax, fullRescan: inboxFullRescan,
		now: time.Now, wait: changes.wait,
		snapshot: snapshotUnifiedInbox, fingerprint: changes.fingerprint,
		close: changes.close,
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

// inboxChangeNotifier turns native filesystem notifications into a constant-
// cost generation fingerprint. The stores are durable, so the periodic full
// rescan remains the correctness backstop for an unavailable watcher, an OS
// queue overflow, or a platform-specific notification gap.
//
// A missing store is watched through its nearest existing parent. The event
// which creates it increments the generation and lets armRoots install the
// recursive watch; the full read triggered by that same event covers writes
// which raced ahead of watch installation.
type inboxChangeNotifier struct {
	epoch  atomic.Uint64
	closed atomic.Bool
	wake   chan struct{}
	events chan notify.EventInfo
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once

	mu    sync.Mutex
	roots []string
	armed map[string]struct{}
}

func newInboxChangeNotifier() *inboxChangeNotifier {
	meetDir, _ := inboxMeetDir()
	n := &inboxChangeNotifier{
		wake: make(chan struct{}, 1), events: make(chan notify.EventInfo, 256),
		stop: make(chan struct{}), done: make(chan struct{}),
		roots: []string{bus.BoardDir(), room.Dir(), meetDir},
		armed: make(map[string]struct{}),
	}
	n.epoch.Store(1)
	n.armRoots()
	go n.run()
	return n
}

func (n *inboxChangeNotifier) fingerprint(string) (uint64, bool) {
	return n.epoch.Load(), true
}

func (n *inboxChangeNotifier) wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.wake:
		return nil
	case <-timer.C:
		return nil
	}
}

func (n *inboxChangeNotifier) run() {
	defer close(n.done)
	for {
		select {
		case <-n.stop:
			return
		case <-n.events:
			n.epoch.Add(1)
			select {
			case n.wake <- struct{}{}:
			default:
			}
			n.armRoots()
		}
	}
}

func (n *inboxChangeNotifier) armRoots() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed.Load() {
		return
	}
	for _, root := range n.roots {
		if root == "" {
			continue
		}
		target := nearestExistingDir(root)
		if target == "" {
			continue
		}
		if sameFilePath(target, root) {
			target = filepath.Join(target, "...")
		}
		if _, ok := n.armed[target]; ok {
			continue
		}
		if err := notify.Watch(target, n.events, notify.All); err == nil {
			n.armed[target] = struct{}{}
		}
	}
}

func nearestExistingDir(path string) string {
	for path != "" {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
	return ""
}

func sameFilePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func (n *inboxChangeNotifier) close() {
	n.once.Do(func() {
		n.closed.Store(true)
		close(n.stop)
		n.mu.Lock()
		notify.Stop(n.events)
		n.mu.Unlock()
		<-n.done
	})
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

// inboxSourcesFingerprint is retained as a diagnostic/benchmark oracle for the
// complete durable source set. The live watcher does not call it: enumerating
// every retained Meet room once a second was itself an unbounded idle workload.
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
