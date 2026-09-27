package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
)

// now is the clock; tests replace it.
var now = time.Now

func seatName() string {
	for _, k := range []string{"BASHY_AGENT", "WEAVE_AGENT", "USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "unknown"
}

// Manager applies the policy and keeps the ledger honest. Provider may be nil
// for ledger-only views (no token available).
type Manager struct {
	Policy   Policy
	Ledger   Ledger
	Provider Provider
	Seat     string
}

// CreateRequest is one rental. TTL and CapUSD are mandatory: there is no
// such thing as a host without a deadline and a budget.
type CreateRequest struct {
	Name   string
	Size   string
	Region string
	Image  string
	Sprint string
	TTL    time.Duration
	CapUSD float64
	DryRun bool
}

// CreateResult reports what was (or, for a dry run, would be) rented.
type CreateResult struct {
	DryRun     bool    `json:"dry_run,omitempty"`
	Lease      Lease   `json:"lease"`
	Host       Host    `json:"host"`
	CeilingUSD float64 `json:"ceiling_usd"` // price × TTL: the most it can cost if destroyed on time
}

// RefusedError is a policy refusal: the request is well-formed but not
// allowed. It is distinct from provider or I/O failures.
type RefusedError struct{ Reason string }

func (e *RefusedError) Error() string { return "refused: " + e.Reason }

func refuse(format string, a ...any) error { return &RefusedError{Reason: fmt.Sprintf(format, a...)} }

// IsRefused reports whether err is a policy refusal.
func IsRefused(err error) bool {
	var r *RefusedError
	return errors.As(err, &r)
}

func (m *Manager) needProvider() error {
	if m.Provider == nil {
		return fmt.Errorf("no provider token: store it with `bashy ask --name %s --stdout | bashy secret set %s`",
			m.Policy.TokenSecret, m.Policy.TokenSecret)
	}
	return nil
}

// checkTripwire lists every host the token can see and refuses if any of
// them is a production host: the token is then not an ephemeral-account
// token, and nothing this package does with it is safe.
func (m *Manager) checkTripwire(ctx context.Context) ([]Host, error) {
	hosts, err := m.Provider.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, h := range hosts {
		if slices.Contains(m.Policy.ProductionTripwire, h.Name) {
			return nil, refuse("the token can see production host %q: it is not an ephemeral-account token; replace %s",
				h.Name, m.Policy.TokenSecret)
		}
	}
	return hosts, nil
}

// Create validates the request against the policy, rents the host and
// records it. The host is in the ledger before this returns, so a crash
// while waiting for it to boot still leaves it reapable.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (CreateResult, error) {
	p := m.Policy
	if err := m.needProvider(); err != nil {
		return CreateResult{}, err
	}
	if !ValidName(req.Name) {
		return CreateResult{}, refuse("name %q: use lower-case letters, digits and dashes (max 63, no dots)", req.Name)
	}
	if req.TTL <= 0 {
		return CreateResult{}, refuse("--ttl is required: every host has a deadline")
	}
	if req.CapUSD <= 0 {
		return CreateResult{}, refuse("--cap is required: every host has a budget in USD")
	}
	if max := time.Duration(p.MaxTTL); max > 0 && req.TTL > max {
		return CreateResult{}, refuse("--ttl %s exceeds the policy maximum %s", req.TTL, max)
	}
	if p.MaxCapUSD > 0 && req.CapUSD > p.MaxCapUSD {
		return CreateResult{}, refuse("--cap $%.2f exceeds the policy maximum $%.2f", req.CapUSD, p.MaxCapUSD)
	}
	if req.Size == "" {
		req.Size = p.DefaultSize
	}
	if req.Region == "" {
		req.Region = p.DefaultRegion
	}
	if req.Image == "" {
		req.Image = p.DefaultImage
	}

	size, err := m.Provider.Size(ctx, req.Size)
	if err != nil {
		return CreateResult{}, err
	}
	if size.GPU && !p.AllowGPU {
		return CreateResult{}, refuse("size %s is a GPU size and the policy does not allow GPU hosts", size.Slug)
	}
	ceiling := size.PriceHourly * req.TTL.Hours()
	if ceiling > req.CapUSD {
		return CreateResult{}, refuse("%s at $%.4f/h for %s costs $%.2f, over the $%.2f cap",
			size.Slug, size.PriceHourly, req.TTL, ceiling, req.CapUSD)
	}

	hosts, err := m.checkTripwire(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	for _, h := range hosts {
		if h.Name == req.Name {
			return CreateResult{}, refuse("a host named %q already exists (id %s)", req.Name, h.ID)
		}
	}

	lock, err := m.Ledger.Lock()
	if err != nil {
		return CreateResult{}, fmt.Errorf("ledger lock: %w", err)
	}
	defer lock.Release()

	leases, err := m.Ledger.Leases()
	if err != nil {
		return CreateResult{}, err
	}
	for _, le := range leases {
		if !le.Closed && le.Name == req.Name {
			return CreateResult{}, refuse("the ledger already holds an open host named %q (id %s)", req.Name, le.ID)
		}
	}
	t := now().UTC()
	if p.DailyCapUSD > 0 {
		committed := CommittedSince(leases, t.Add(-24*time.Hour))
		if committed+req.CapUSD > p.DailyCapUSD {
			return CreateResult{}, refuse("the last 24h already commit $%.2f of the $%.2f daily cap; $%.2f more does not fit",
				committed, p.DailyCapUSD, req.CapUSD)
		}
	}

	deadline := t.Add(req.TTL)
	lease := Lease{Name: req.Name, Provider: m.Provider.Name(), Seat: m.Seat, Sprint: req.Sprint,
		Size: size.Slug, Region: req.Region, PriceHourly: size.PriceHourly, CapUSD: req.CapUSD,
		Created: t, Deadline: deadline}
	res := CreateResult{DryRun: req.DryRun, Lease: lease, CeilingUSD: ceiling}
	if req.DryRun {
		return res, nil
	}

	h, err := m.Provider.Create(ctx, Spec{Name: req.Name, Size: size.Slug, Region: req.Region,
		Image: req.Image, Tags: leaseTags(m.Seat, req.Sprint, deadline)})
	if err != nil {
		return CreateResult{}, err
	}
	lease.ID = h.ID
	if err := m.Ledger.Append(Event{At: t, Kind: "create", ID: h.ID, Name: lease.Name, Provider: lease.Provider,
		Seat: lease.Seat, Sprint: lease.Sprint, Size: lease.Size, Region: lease.Region,
		PriceHourly: lease.PriceHourly, CapUSD: lease.CapUSD, Deadline: deadline}); err != nil {
		// A host we cannot record is a host nobody may destroy: give it back now.
		if derr := m.Provider.Delete(context.WithoutCancel(ctx), h.ID); derr != nil {
			return CreateResult{}, fmt.Errorf("host %s created but the ledger write failed (%v) and so did the rollback delete (%v); its deadline tag still lets the reaper find it", h.ID, err, derr)
		}
		return CreateResult{}, fmt.Errorf("ledger write failed, host %s deleted again: %w", h.ID, err)
	}
	res.Lease, res.Host = lease, h
	return res, nil
}

// Wait polls until the host is active with a public address.
func (m *Manager) Wait(ctx context.Context, id string, timeout time.Duration, poll time.Duration) (Host, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		h, err := m.Provider.Get(ctx, id)
		if err == nil && h.Status == "active" && h.IPv4 != "" {
			return h, nil
		}
		select {
		case <-ctx.Done():
			if err == nil {
				err = fmt.Errorf("host %s still %q after %s", id, h.Status, timeout)
			}
			return h, err
		case <-time.After(poll):
		}
	}
}

// Row is one line of `list`: a ledger lease, a provider host, or both.
type Row struct {
	Lease *Lease `json:"lease,omitempty"`
	Host  *Host  `json:"host,omitempty"`
	// State: the provider status (active, new, off, ...), "missing" (open in
	// the ledger, not at the provider), "untracked" (at the provider, not in
	// the ledger), "closed", or "unknown" (no provider to ask).
	State string `json:"state"`
}

// List joins the ledger with what the provider can see. With all, closed
// leases are included. Hosts the ledger does not know are always shown: in an
// ephemeral account every one of them is somebody's forgotten box.
func (m *Manager) List(ctx context.Context, all bool) ([]Row, error) {
	leases, err := m.Ledger.Leases()
	if err != nil {
		return nil, err
	}
	var hosts []Host
	if m.Provider != nil {
		if hosts, err = m.Provider.List(ctx); err != nil {
			return nil, err
		}
	}
	byID := map[string]*Host{}
	for i := range hosts {
		byID[hosts[i].ID] = &hosts[i]
	}
	seen := map[string]bool{}
	var rows []Row
	for i := range leases {
		le := &leases[i]
		if le.Closed {
			if all {
				rows = append(rows, Row{Lease: le, State: "closed"})
			}
			continue
		}
		seen[le.ID] = true
		switch h := byID[le.ID]; {
		case m.Provider == nil:
			rows = append(rows, Row{Lease: le, State: "unknown"})
		case h == nil:
			rows = append(rows, Row{Lease: le, State: "missing"})
		default:
			rows = append(rows, Row{Lease: le, Host: h, State: h.Status})
		}
	}
	for i := range hosts {
		if !seen[hosts[i].ID] {
			rows = append(rows, Row{Host: &hosts[i], State: "untracked"})
		}
	}
	return rows, nil
}
