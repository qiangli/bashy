package main

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/room"
)

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

// dir hashes the directory entry list and file metadata. Directory mtimes can
// settle after a read on Windows without any new mail; hashing them makes a
// repeated hook announce the same unread batch. Child names still detect
// additions/removals, while file metadata detects changed source records.
// With children true, it also hashes state.json and transcript.jsonl.
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
	f.string(path)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			f.ok = false
			continue
		}
		entryPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			f.string(entryPath)
		} else {
			f.stat(entryPath, info)
		}
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
