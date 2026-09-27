package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	sizes    map[string]Size
	hosts    []Host
	nextID   int
	created  []Spec
	deleted  []string
	onCreate func()
}

func newFake() *fakeProvider {
	return &fakeProvider{nextID: 1000, sizes: map[string]Size{
		"s-1vcpu-1gb":  {Slug: "s-1vcpu-1gb", PriceHourly: 0.00893},
		"s-2vcpu-4gb":  {Slug: "s-2vcpu-4gb", PriceHourly: 0.03571},
		"gpu-l40s-1x":  {Slug: "gpu-l40s-1x", PriceHourly: 1.57, GPU: true},
		"s-32vcpu-64g": {Slug: "s-32vcpu-64g", PriceHourly: 0.71429},
	}}
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Size(_ context.Context, slug string) (Size, error) {
	s, ok := f.sizes[slug]
	if !ok {
		return Size{}, fmt.Errorf("unknown size %q", slug)
	}
	return s, nil
}
func (f *fakeProvider) Create(_ context.Context, s Spec) (Host, error) {
	f.nextID++
	f.created = append(f.created, s)
	h := Host{ID: fmt.Sprint(f.nextID), Name: s.Name, Status: "active", IPv4: "192.0.2.10",
		Size: s.Size, Region: s.Region, PriceHourly: f.sizes[s.Size].PriceHourly, Tags: s.Tags}
	f.hosts = append(f.hosts, h)
	if f.onCreate != nil {
		f.onCreate()
	}
	return h, nil
}
func (f *fakeProvider) Get(_ context.Context, id string) (Host, error) {
	for _, h := range f.hosts {
		if h.ID == id {
			return h, nil
		}
	}
	return Host{}, ErrNotFound
}
func (f *fakeProvider) List(context.Context) ([]Host, error) {
	return append([]Host(nil), f.hosts...), nil
}
func (f *fakeProvider) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	for i, h := range f.hosts {
		if h.ID == id {
			f.hosts = append(f.hosts[:i], f.hosts[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

var t0 = time.Date(2026, 9, 27, 18, 0, 0, 0, time.UTC)

func testManager(t *testing.T) (*Manager, *fakeProvider) {
	t.Helper()
	prev := now
	now = func() time.Time { return t0 }
	t.Cleanup(func() { now = prev })
	f := newFake()
	return &Manager{Policy: DefaultPolicy(), Ledger: Ledger{Dir: t.TempDir()}, Provider: f, Seat: "gusset"}, f
}

func mustRefuse(t *testing.T, err error, want string) {
	t.Helper()
	if !IsRefused(err) {
		t.Fatalf("want refusal containing %q, got %v", want, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("refusal %q does not mention %q", err, want)
	}
}

func TestCreateRequiresTTLAndCap(t *testing.T) {
	m, f := testManager(t)
	ctx := context.Background()
	_, err := m.Create(ctx, CreateRequest{Name: "a", CapUSD: 1})
	mustRefuse(t, err, "--ttl is required")
	_, err = m.Create(ctx, CreateRequest{Name: "a", TTL: time.Hour})
	mustRefuse(t, err, "--cap is required")
	if len(f.created) != 0 {
		t.Fatalf("a refused request reached the provider: %v", f.created)
	}
}

func TestCreateRefusesOverPolicyAndPrice(t *testing.T) {
	m, f := testManager(t)
	ctx := context.Background()
	cases := []struct {
		req  CreateRequest
		want string
	}{
		{CreateRequest{Name: "a", TTL: 13 * time.Hour, CapUSD: 1}, "policy maximum 12h"},
		{CreateRequest{Name: "a", TTL: time.Hour, CapUSD: 11}, "policy maximum $10.00"},
		{CreateRequest{Name: "a", TTL: 10 * time.Hour, CapUSD: 5, Size: "s-32vcpu-64g"}, "over the $5.00 cap"},
		{CreateRequest{Name: "a", TTL: time.Hour, CapUSD: 5, Size: "gpu-l40s-1x"}, "GPU"},
		{CreateRequest{Name: "Bad.Name", TTL: time.Hour, CapUSD: 1}, "no dots"},
	}
	for _, c := range cases {
		_, err := m.Create(ctx, c.req)
		mustRefuse(t, err, c.want)
	}
	if len(f.created) != 0 {
		t.Fatalf("a refused request reached the provider: %v", f.created)
	}
}

func TestCreateRecordsLedgerAndTags(t *testing.T) {
	m, f := testManager(t)
	res, err := m.Create(context.Background(), CreateRequest{Name: "s311-a", TTL: 3 * time.Hour, CapUSD: 1, Sprint: "311"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Lease.ID == "" || res.Host.IPv4 == "" {
		t.Fatalf("result missing id/ip: %+v", res)
	}
	le, ok, err := m.Ledger.Find("s311-a")
	if err != nil || !ok {
		t.Fatalf("lease not in ledger: ok=%v err=%v", ok, err)
	}
	if !le.Deadline.Equal(t0.Add(3*time.Hour)) || le.Seat != "gusset" || le.Sprint != "311" || le.CapUSD != 1 {
		t.Fatalf("ledger lease wrong: %+v", le)
	}
	tags := f.created[0].Tags
	d, ok := DeadlineFromTags(tags)
	if !ok || !d.Equal(t0.Add(3*time.Hour)) {
		t.Fatalf("deadline tag missing or wrong: %v", tags)
	}
	for _, want := range []string{TagLease, "bashy-seat-gusset", "bashy-sprint-311"} {
		if !contains(tags, want) {
			t.Fatalf("tag %q missing from %v", want, tags)
		}
	}
	if info, err := os.Stat(filepath.Join(m.Ledger.Dir, "ledger.jsonl")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ledger file mode: %v %v", info, err)
	}
}

func TestCreateDryRunRentsNothing(t *testing.T) {
	m, f := testManager(t)
	res, err := m.Create(context.Background(), CreateRequest{Name: "dry", TTL: 2 * time.Hour, CapUSD: 1, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || res.CeilingUSD <= 0 {
		t.Fatalf("dry run result: %+v", res)
	}
	if len(f.created) != 0 {
		t.Fatal("dry run reached the provider")
	}
	if open, _ := m.Ledger.Open(); len(open) != 0 {
		t.Fatal("dry run wrote the ledger")
	}
}

func TestCreateRefusesDuplicateNameAndDailyCap(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "one", TTL: time.Hour, CapUSD: 10}); err != nil {
		t.Fatal(err)
	}
	_, err := m.Create(ctx, CreateRequest{Name: "one", TTL: time.Hour, CapUSD: 1})
	mustRefuse(t, err, "already exists")
	if _, err := m.Create(ctx, CreateRequest{Name: "two", TTL: time.Hour, CapUSD: 10}); err != nil {
		t.Fatal(err)
	}
	_, err = m.Create(ctx, CreateRequest{Name: "three", TTL: time.Hour, CapUSD: 10})
	mustRefuse(t, err, "daily cap")
	// 25 hours later the first two no longer count against the rolling day.
	now = func() time.Time { return t0.Add(25 * time.Hour) }
	if _, err := m.Create(ctx, CreateRequest{Name: "three", TTL: time.Hour, CapUSD: 10}); err != nil {
		t.Fatalf("rolling daily cap did not roll: %v", err)
	}
}

func TestProductionTripwireRefusesEverything(t *testing.T) {
	m, f := testManager(t)
	m.Policy.ProductionTripwire = []string{"prod.example.com"}
	f.hosts = append(f.hosts, Host{ID: "1", Name: "prod.example.com", Status: "active"})
	_, err := m.Create(context.Background(), CreateRequest{Name: "a", TTL: time.Hour, CapUSD: 1})
	mustRefuse(t, err, "production host")
	if len(f.created) != 0 {
		t.Fatal("tripwire did not stop the create")
	}
}

func TestLedgerWriteFailureRollsBackHost(t *testing.T) {
	m, f := testManager(t)
	// The ledger becomes unwritable between the provider's create and the
	// ledger append: a directory where the file should be.
	f.onCreate = func() {
		if err := os.MkdirAll(filepath.Join(m.Ledger.Dir, "ledger.jsonl"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_, err := m.Create(context.Background(), CreateRequest{Name: "a", TTL: time.Hour, CapUSD: 1})
	if err == nil || IsRefused(err) || !strings.Contains(err.Error(), "deleted again") {
		t.Fatalf("want a rolled-back ledger failure, got %v", err)
	}
	if len(f.created) != 1 || len(f.deleted) != 1 || len(f.hosts) != 0 {
		t.Fatalf("an unrecorded host was left behind: created=%d deleted=%v hosts=%v", len(f.created), f.deleted, f.hosts)
	}
}

func TestListJoinsLedgerAndProvider(t *testing.T) {
	m, f := testManager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, CreateRequest{Name: "mine", TTL: time.Hour, CapUSD: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, CreateRequest{Name: "vanished", TTL: time.Hour, CapUSD: 1}); err != nil {
		t.Fatal(err)
	}
	f.hosts = f.hosts[:1] // "vanished" destroyed outside bashy
	f.hosts = append(f.hosts, Host{ID: "77", Name: "stray", Status: "active"})
	rows, err := m.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		name := ""
		if r.Lease != nil {
			name = r.Lease.Name
		} else {
			name = r.Host.Name
		}
		got[name] = r.State
	}
	want := map[string]string{"mine": "active", "vanished": "missing", "stray": "untracked"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("state of %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	// Without a provider the ledger alone is shown.
	m.Provider = nil
	rows, err = m.List(ctx, false)
	if err != nil || len(rows) != 2 || rows[0].State != "unknown" {
		t.Fatalf("ledger-only list: %v %+v", err, rows)
	}
}

func TestLedgerFoldIgnoresEventsWithoutCreate(t *testing.T) {
	l := Ledger{Dir: t.TempDir()}
	must := func(e Event) {
		if err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	must(Event{Kind: "destroy", ID: "9"}) // never created by us
	must(Event{At: t0, Kind: "create", ID: "1", Name: "a", CapUSD: 2, PriceHourly: 1, Deadline: t0.Add(time.Hour)})
	must(Event{At: t0, Kind: "extend", ID: "1", Deadline: t0.Add(2 * time.Hour), CapUSD: 3})
	must(Event{At: t0.Add(90 * time.Minute), Kind: "destroy", ID: "1", CostUSD: 1.5})
	all, err := l.Leases()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 lease, got %+v", all)
	}
	le := all[0]
	if !le.Closed || le.CapUSD != 3 || !le.Deadline.Equal(t0.Add(2*time.Hour)) || le.SpentUSD(t0.Add(10*time.Hour)) != 1.5 {
		t.Fatalf("fold wrong: %+v", le)
	}
}

func TestPolicyFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(map[string]any{"max_ttl": "4h", "daily_cap_usd": 5, "production_tripwire": []string{"p"}})
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPolicy(dir)
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(p.MaxTTL) != 4*time.Hour || p.DailyCapUSD != 5 || p.MaxCapUSD != 10 || p.TokenSecret != "DO_EPHEMERAL_TOKEN" {
		t.Fatalf("policy merge wrong: %+v", p)
	}
}

func TestCommandCreateAndList(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("BASHY_AGENT", "gusset")
	prevNow, prevProv := now, newProvider
	now = func() time.Time { return t0 }
	f := newFake()
	newProvider = func(Policy) (Provider, error) { return f, nil }
	t.Cleanup(func() { now, newProvider = prevNow, prevProv })

	run := func(args ...string) (string, error) {
		cmd := NewCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	if _, err := run("create", "x"); err == nil || !strings.Contains(err.Error(), "--ttl is required") {
		t.Fatalf("create without flags: %v", err)
	}
	out, err := run("create", "s311-a", "--ttl", "2h", "--cap", "1", "--sprint", "311")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "created s311-a (id 1001)") || !strings.Contains(out, "ssh root@192.0.2.10") {
		t.Fatalf("create output: %s", out)
	}
	out, err = run("ls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "s311-a") || !strings.Contains(out, "gusset") || !strings.Contains(out, "2h0m0s") {
		t.Fatalf("ls output: %s", out)
	}
}

func TestNoTokenIsAnErrorForCreateButNotForList(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	prev := newProvider
	newProvider = func(Policy) (Provider, error) { return nil, errors.New("no DO_EPHEMERAL_TOKEN token") }
	t.Cleanup(func() { newProvider = prev })
	cmd := NewCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"create", "a", "--ttl", "1h", "--cap", "1"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "bashy secret set DO_EPHEMERAL_TOKEN") {
		t.Fatalf("create without a token: %v", err)
	}
	cmd = NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"ls"})
	if err := cmd.Execute(); err != nil || !strings.Contains(out.String(), "no ephemeral hosts") {
		t.Fatalf("ls without a token: %v %q", err, out.String())
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
