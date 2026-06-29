// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The space-time advisor's MEMORY: the "time" axis. The advisor accumulates
// what worked under which ambient conditions so it can turn a bare failure into
// a confident, history-grounded hint — the thing that makes bashy "know better"
// than a stateless agent looping in the dark.
//
// Two layers, both self-contained (no dependency on the deferred Batch-1 audit
// journal):
//
//   - A per-session failure counter, for doomed-loop detection (case 5): when an
//     agent retries the identical command and it keeps failing the same way, the
//     advisor escalates ("change the approach, not the parameters").
//   - A best-effort persisted host-success ledger keyed by a NETWORK FINGERPRINT
//     (a hash of the local subnets). It records that host X was reachable while
//     the machine was on network A. When X later fails to resolve while the
//     machine is on network B, the advisor can say "you reached X before from a
//     different network — you're likely off its network now," which is far more
//     actionable than a guess. The fingerprint is a RELATIVE signal (same
//     network as before?), so no absolute home/remote classification is needed.
//
// All persistence is best-effort: any IO error is swallowed, never surfacing to
// the shell. Disable persistence with BASHY_ADVISOR_NOMEM=1 (the in-memory layer
// still works); override the path with BASHY_ADVISOR_STATE.
package agentos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	advisorMemSchema = "bashy-advisor-mem-v1"
	loopThreshold    = 3   // failures of the identical command before escalation
	maxHostRecords   = 256 // cap the persisted ledger (LRU by last success)
)

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
	mu    sync.Mutex
	path  string // "" disables persistence
	nowFn func() int64
	hosts map[string]hostRecord
	fails map[string]int // session-only: loop key -> consecutive failures
}

// newMemory builds the memory and loads any persisted ledger (best-effort).
func newMemory() *memory {
	m := &memory{
		path:  defaultMemoryPath(),
		nowFn: func() int64 { return time.Now().Unix() },
		hosts: map[string]hostRecord{},
		fails: map[string]int{},
	}
	m.load()
	return m
}

// defaultMemoryPath returns the persisted-ledger path, or "" to disable
// persistence (BASHY_ADVISOR_NOMEM set, or no usable cache dir).
func defaultMemoryPath() string {
	switch strings.ToLower(os.Getenv("BASHY_ADVISOR_NOMEM")) {
	case "1", "true", "yes", "on":
		return ""
	}
	if p := os.Getenv("BASHY_ADVISOR_STATE"); p != "" {
		return p
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "bashy", "advisor", "hosts.json")
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
// count (the Nth consecutive identical failure).
func (m *memory) recordFail(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fails[key]++
	return m.fails[key]
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

// loopKey identifies "the same command" for doomed-loop detection: the full
// post-expansion argv. Re-running it identically yields the same key.
func loopKey(args []string) string {
	return strings.Join(args, "\x00")
}

// applyLoop escalates a hint when the identical command has failed loopThreshold
// or more times. With a base hint it forces retryable=false and prefixes the
// attempt count; without one it emits a standalone doomed-loop hint. Below the
// threshold it returns base unchanged.
func applyLoop(base *hint, cmd string, n int) *hint {
	if n < loopThreshold {
		return base
	}
	if base == nil {
		return &hint{
			dimension: "loop",
			retryable: false,
			text:      cmd + " has now failed repeatedly with the same result.",
			suggest:   "the environment is not changing between attempts — change the approach, not the parameters.",
		}
	}
	base.retryable = false
	if base.suggest != "" {
		base.suggest = "this has failed repeatedly for the same reason — " + base.suggest
	}
	return base
}

// networkFingerprint hashes the machine's current non-loopback subnets into a
// short, stable identifier. The same Wi-Fi/LAN yields the same fingerprint;
// moving to another network changes it. Empty on error (history then degrades
// gracefully to the live-probe hint).
func networkFingerprint() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var nets []string
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}
		// Mask to the network so per-host bits don't perturb the fingerprint.
		network := net.IPNet{IP: ip.Mask(ipn.Mask), Mask: ipn.Mask}
		nets = append(nets, network.String())
	}
	if len(nets) == 0 {
		return ""
	}
	sort.Strings(nets)
	sum := sha256.Sum256([]byte(strings.Join(nets, ",")))
	return hex.EncodeToString(sum[:8])
}
