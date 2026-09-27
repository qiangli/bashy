package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Event is one line of the ledger. The ledger is append-only: a lease's
// current state is the fold of its events, and nothing is ever rewritten.
type Event struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"event"` // create | extend | destroy | reap | gone
	ID          string    `json:"id"`
	Name        string    `json:"name,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	Seat        string    `json:"seat,omitempty"`
	Sprint      string    `json:"sprint,omitempty"`
	Size        string    `json:"size,omitempty"`
	Region      string    `json:"region,omitempty"`
	PriceHourly float64   `json:"price_hourly,omitempty"`
	CapUSD      float64   `json:"cap_usd,omitempty"`
	Deadline    time.Time `json:"deadline,omitzero"`
	CostUSD     float64   `json:"cost_usd,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// Lease is the folded state of one host in the ledger.
type Lease struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Seat        string    `json:"seat"`
	Sprint      string    `json:"sprint,omitempty"`
	Size        string    `json:"size"`
	Region      string    `json:"region"`
	PriceHourly float64   `json:"price_hourly"`
	CapUSD      float64   `json:"cap_usd"`
	Created     time.Time `json:"created"`
	Deadline    time.Time `json:"deadline"`
	Closed      bool      `json:"closed"`
	ClosedAt    time.Time `json:"closed_at,omitzero"`
	CostUSD     float64   `json:"cost_usd,omitempty"`
}

// SpentUSD estimates what the lease has cost by now (or in total, once
// closed): hours held times the hourly price.
func (l Lease) SpentUSD(now time.Time) float64 {
	if l.Closed && l.CostUSD > 0 {
		return l.CostUSD
	}
	end := now
	if l.Closed {
		end = l.ClosedAt
	}
	return CostBetween(l.Created, end, l.PriceHourly)
}

// CostBetween is hours × price. Providers differ on rounding (per second,
// per started hour); this is the estimate the cap is checked against.
func CostBetween(from, to time.Time, priceHourly float64) float64 {
	h := to.Sub(from).Hours()
	if h < 0 {
		h = 0
	}
	return h * priceHourly
}

// Ledger is the append-only record at <dir>/ledger.jsonl.
type Ledger struct{ Dir string }

func (l Ledger) path() string { return filepath.Join(l.Dir, "ledger.jsonl") }

// Lock serializes check-then-act sequences (the daily cap, name
// uniqueness) across agents on this host.
func (l Ledger) Lock() (*fileLock, error) {
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return nil, err
	}
	return acquireLock(filepath.Join(l.Dir, "ledger.lock"), 2*time.Minute)
}

// Append writes one event. Each event is one small O_APPEND write.
func (l Ledger) Append(e Event) error {
	if e.ID == "" || e.Kind == "" {
		return errors.New("ledger: event needs id and kind")
	}
	if e.At.IsZero() {
		e.At = now().UTC()
	}
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path(), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Leases folds the ledger, oldest first.
func (l Ledger) Leases() ([]Lease, error) {
	f, err := os.Open(l.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byID := map[string]*Lease{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for n := 1; sc.Scan(); n++ {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("ledger line %d: %w", n, err)
		}
		le := byID[e.ID]
		if le == nil {
			if e.Kind != "create" {
				continue // an event for a lease we never created: ignore, never invent one
			}
			le = &Lease{}
			byID[e.ID] = le
			order = append(order, e.ID)
		}
		apply(le, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]Lease, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

func apply(le *Lease, e Event) {
	switch e.Kind {
	case "create":
		*le = Lease{ID: e.ID, Name: e.Name, Provider: e.Provider, Seat: e.Seat, Sprint: e.Sprint,
			Size: e.Size, Region: e.Region, PriceHourly: e.PriceHourly, CapUSD: e.CapUSD,
			Created: e.At, Deadline: e.Deadline}
	case "extend":
		if !e.Deadline.IsZero() {
			le.Deadline = e.Deadline
		}
		if e.CapUSD > 0 {
			le.CapUSD = e.CapUSD
		}
	case "destroy", "reap", "gone":
		le.Closed = true
		le.ClosedAt = e.At
		le.CostUSD = e.CostUSD
	}
}

// Open returns the leases not yet closed.
func (l Ledger) Open() ([]Lease, error) {
	all, err := l.Leases()
	if err != nil {
		return nil, err
	}
	var open []Lease
	for _, le := range all {
		if !le.Closed {
			open = append(open, le)
		}
	}
	return open, nil
}

// Find returns the open lease whose ID or name is ref.
func (l Ledger) Find(ref string) (Lease, bool, error) {
	open, err := l.Open()
	if err != nil {
		return Lease{}, false, err
	}
	for _, le := range open {
		if le.ID == ref || le.Name == ref {
			return le, true, nil
		}
	}
	return Lease{}, false, nil
}

// CommittedSince sums the caps of leases created at or after since: the most
// that set of hosts can cost, which is what the daily cap is checked against.
func CommittedSince(leases []Lease, since time.Time) float64 {
	var sum float64
	for _, le := range leases {
		if !le.Created.Before(since) {
			sum += le.CapUSD
		}
	}
	return sum
}
