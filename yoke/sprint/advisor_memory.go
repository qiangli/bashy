package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	advisorMemSchema = "bashy-advisor-mem-v1"
	loopThreshold    = 3   // failures of the identical command before escalation
	loopWindowSecs   = 300 // failures more than this far apart aren't one loop
	maxHostRecords   = 256 // cap the persisted ledger (LRU by last success)
)

// failMark tracks a loop key's consecutive-failure count and WHEN the last
// failure happened. Without the timestamp, a long-lived interactive shell would
// mistake the same argv failing a handful of times over an hour — in genuinely
// different contexts — for one tight retry loop, and emit a spurious
// "change your approach" hint. A doomed loop is failures in quick succession.
type failMark struct {
	n    int
	last int64 // unix seconds of the last failure
}

// hostRecord is one host's reachability memory.
type hostRecord struct {
	NetFP       string `json:"netfp"`        // network fingerprint at last success
	LastSuccess int64  `json:"last_success"` // unix seconds
	Successes   int    `json:"successes"`
}

// memoryFile is the on-disk shape.
type memoryFile struct {
	Schema string                `json:"schema_version"`
	Hosts  map[string]hostRecord `json:"hosts"`
}

// memory holds the advisor's accumulated knowledge. All methods are safe for
// concurrent use (subshells/pipelines run ExecHandler in parallel goroutines).
type memory struct {
	mu     sync.Mutex
	path   string // "" disables persistence
	nowFn  func() int64
	hosts  map[string]hostRecord
	fails  map[string]failMark // session-only: loop key -> recent-failure mark
	hinted map[string]bool     // session-only: proactive-hint keys already shown
}

// firstHint reports whether key has not been hinted this session, marking it so
// subsequent calls return false. This is the once-per-(tool,session) rate-limit
// that keeps proactive nudges from becoming ignorable noise.
func (m *memory) firstHint(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hinted[key] {
		return false
	}
	m.hinted[key] = true
	return true
}

// load reads the persisted ledger into memory; any error leaves it empty.
func (m *memory) load() {
	maps.Copy(m.hosts, readMemoryFile(m.path))
}

// readMemoryFile parses the ledger at path; returns nil on any problem.
func readMemoryFile(path string) map[string]hostRecord {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f memoryFile
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	return f.Hosts
}

// recordFail bumps the session failure counter for key and returns the new
// count (the Nth consecutive identical failure). If the previous failure of the
// same key is older than loopWindowSecs, the count restarts: failures that far
// apart are separate uses of the command, not one retry loop.
func (m *memory) recordFail(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.nowFn()
	fm := m.fails[key]
	if fm.n > 0 && now-fm.last > loopWindowSecs {
		fm.n = 0
	}
	fm.n++
	fm.last = now
	m.fails[key] = fm
	return fm.n
}

// clearFail resets a key's failure counter after it succeeds.
func (m *memory) clearFail(key string) {
	m.mu.Lock()
	delete(m.fails, key)
	m.mu.Unlock()
}

// priorSuccess returns the stored reachability record for host, if any.
func (m *memory) priorSuccess(host string) (hostRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.hosts[host]
	return r, ok
}

// recordSuccess notes that host was reachable under fingerprint netfp. It
// persists (best-effort) only when the record is new or the fingerprint
// changed, to keep disk writes infrequent.
func (m *memory) recordSuccess(host, netfp string) {
	if host == "" {
		return
	}
	m.mu.Lock()
	r := m.hosts[host]
	changed := r.Successes == 0 || r.NetFP != netfp
	r.NetFP = netfp
	r.LastSuccess = m.nowFn()
	r.Successes++
	m.hosts[host] = r
	m.mu.Unlock()
	if changed {
		m.persist()
	}
}

// persist writes the ledger atomically (temp + rename), merged with whatever is
// already on disk (so concurrent bashy processes don't clobber each other) and
// capped to the most-recently-successful maxHostRecords hosts. Best-effort.
func (m *memory) persist() {
	if m.path == "" {
		return
	}
	m.mu.Lock()
	merged := make(map[string]hostRecord, len(m.hosts))
	maps.Copy(merged, m.hosts)
	m.mu.Unlock()

	for h, r := range readMemoryFile(m.path) {
		if cur, ok := merged[h]; !ok || r.LastSuccess > cur.LastSuccess {
			merged[h] = r
		}
	}
	capHostRecords(merged, maxHostRecords)

	b, err := json.Marshal(memoryFile{Schema: advisorMemSchema, Hosts: merged})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".hosts-*.json")
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
	_ = os.Rename(tmpName, m.path)
}

// capHostRecords trims hosts in place to the n most-recently-successful entries.
func capHostRecords(hosts map[string]hostRecord, n int) {
	if len(hosts) <= n {
		return
	}
	type kv struct {
		host string
		when int64
	}
	all := make([]kv, 0, len(hosts))
	for h, r := range hosts {
		all = append(all, kv{h, r.LastSuccess})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].when > all[j].when })
	for _, e := range all[n:] {
		delete(hosts, e.host)
	}
}
