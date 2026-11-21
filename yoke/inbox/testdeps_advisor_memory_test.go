package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// inboxHintArrivals is the cross-process half of unread-hint suppression. Each
// `bashy` CLI invocation is a new process, so in-memory session state cannot
// deduplicate hints across commands: the arrival fingerprint (see
// inboxSourcesFingerprint) is persisted per reader instead. A hint fires only
// when the fingerprint differs from the last announced one, so duplicate
// notices within a turn are suppressed while newly arrived mail re-hints.
// Probed records the last fingerprint fully snapshotted (re-sampled after the
// read, so first-read cursor materialization never re-triggers), so a quiet
// store is never re-read; Hinted records the last fingerprint announced, so
// an arrival already surfaced on one channel (a command hint, a hook) is not
// repeated on another. Best-effort throughout: IO errors leave an empty state,
// which fails open toward at most one extra hint, never toward silence.
//
// The reader name is the instance key (legacy compatibility). When the
// UUID-instance foundation (#1268) lands, its principal record plugs in here;
// nothing here fabricates an identity.
type inboxHintArrivals struct {
	Probed map[string]uint64 `json:"probed,omitempty"`
	Hinted map[string]uint64 `json:"hinted,omitempty"`
}

// inboxHintsStatePath returns the persisted-arrivals path, or "" to disable
// persistence (BASHY_ADVISOR_NOMEM set, or no usable cache dir). Override with
// BASHY_HINTS_STATE. A disabled path keeps per-process suppression only.
func inboxHintsStatePath() string {
	switch strings.ToLower(os.Getenv("BASHY_ADVISOR_NOMEM")) {
	case "1", "true", "yes", "on":
		return ""
	}
	if p := os.Getenv("BASHY_HINTS_STATE"); p != "" {
		return p
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "bashy", "inbox-hints.json")
}

// save persists the arrival state atomically (temp + rename), best-effort.
func (st inboxHintArrivals) save() {
	path := inboxHintsStatePath()
	if path == "" {
		return
	}
	if st.Probed == nil {
		st.Probed = map[string]uint64{}
	}
	if st.Hinted == nil {
		st.Hinted = map[string]uint64{}
	}
	b, err := json.Marshal(struct {
		Schema string            `json:"schema_version"`
		State  inboxHintArrivals `json:"state"`
	}{Schema: "bashy-inbox-hints-v1", State: st})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".inbox-hints-*.json")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, path)
}
