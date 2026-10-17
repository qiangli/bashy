// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// The service monitor (Sprint 418 S16; docs/bashy-service-design.md). One
// long-running `bashy supervisord` keeps every enabled unit in `bashy service
// list` in its desired state: it is inetd to outpost's systemd. It reads the
// declarations, drives each through its provider (process or internal — the
// unit's provider field picks, never its name) and applies ONE policy to all
// of them: requires ordering, the declared health probe, restart with bounded
// backoff, the start limit, and live reload of declaration changes.
//
// Concurrency model: a single loop goroutine owns every unitRun. Provider
// calls that block (activate, deactivate with its grace, probes) run on their
// own goroutines and report back as messages tagged with the unit's
// generation, so a late answer about an incarnation that has since been
// replaced is recognized and dropped. Nothing else mutates unit state.

// errHealthPending is a provider's "not yet" (an internal unit whose owner
// has not applied it): neither healthy nor a failure that counts.
var errHealthPending = errors.New("service: health pending")

// unitRun is the monitor's record of one unit.
type unitRun struct {
	decl  fleet.Service
	print string // declaration fingerprint for reload diffs
	pol   unitPolicy
	prov  service.Provider

	st service.Status // monitor-owned fields: State, Since, Restarts, Health, Reason

	gen     int  // incarnation; every async answer carries the gen it was asked under
	busy    bool // an activate/deactivate/configure call is in flight
	waiting bool // a backoff timer is pending; the timer clears it
	restart bool // the next activation is a restart (counts in Restarts)
	latched bool // failed or completed: stays down until start / a changed declaration
	held    bool // operator `service stop`: stays down until `service start`
	dirty   bool // the declaration changed while a call was in flight
	adhoc   bool // operator `service start` of a disabled unit (runtime only)
	gone    bool // removed or retired from the registry

	ladder    backoffLadder
	limit     startLimiter
	startedAt time.Time
	fails     int // consecutive failed probes
}

// monitor messages, all handled on the loop goroutine.
type (
	msgActivated struct {
		unit string
		gen  int
		err  error
	}
	msgDeactivated struct {
		unit string
		gen  int
		then func(*unitRun)
	}
	msgConfigured struct {
		unit string
		gen  int
		err  error
	}
	msgProvider struct {
		unit string
		gen  int
		ev   service.Event
	}
	msgProbe struct {
		unit string
		gen  int
		err  error
	}
	msgTimer struct {
		unit string
		gen  int
		kind string
	}
	msgControl struct {
		req   ctlRequest
		reply chan ctlResponse
	}
)

type monitor struct {
	// load returns the registry's units: the desired state.
	load func() ([]fleet.Service, []error)
	// providers realize units; the declaration picks one.
	providers service.Providers
	// resolve builds the Runtime the monitor resolves once per activation:
	// env with secrets, the provisioned download.
	resolve func(context.Context, fleet.Service) (service.Runtime, error)
	// probe runs the declared probe; checkPorts refuses foreign listeners.
	probe      func(context.Context, fleet.Service, unitPolicy) (error, bool)
	checkPorts func(fleet.Service) error
	// checkBuild is the supply-chain gate: a unit whose binary is not a
	// verified bashy-signed build is refused before activation.
	checkBuild func(fleet.Service) error
	// events receives every transition (the room timeline, S3); failed also
	// reaches notify (the sinks, S7). Both are called off the loop.
	events func(service.Event)
	notify func(service.Event)

	now       func() time.Time
	log       io.Writer
	pollEvery time.Duration // registry re-read; 0 = only on explicit reload

	// only/force are `service run`: just these units, enabled or not, and the
	// monitor returns once none of them wants to run any more.
	only  map[string]bool
	force bool
	// untilDone: return once no enabled unit wants to run any more (every one
	// completed or failed), like `service run` over the whole registry.
	untilDone bool

	msgs     chan any
	units    map[string]*unitRun
	ctx      context.Context
	cancel   context.CancelFunc
	stopping bool
	evq      chan service.Event
}

func newMonitor() *monitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &monitor{
		probe: func(ctx context.Context, u fleet.Service, p unitPolicy) (error, bool) {
			return runProbe(ctx, u, p, nil)
		},
		checkPorts: checkPorts,
		checkBuild: checkServiceBuildHook,
		now:        time.Now,
		log:        io.Discard,
		msgs:       make(chan any, 64),
		units:      map[string]*unitRun{},
		ctx:        ctx,
		cancel:     cancel,
		evq:        make(chan service.Event, 256),
	}
}

func (m *monitor) logf(format string, a ...any) {
	fmt.Fprintf(m.log, "supervisord: "+format+"\n", a...)
}

func (m *monitor) post(msg any) {
	select {
	case m.msgs <- msg:
	case <-m.ctx.Done():
	}
}

// later posts a timer message after d.
func (m *monitor) later(d time.Duration, unit string, gen int, kind string) {
	time.AfterFunc(d, func() { m.post(msgTimer{unit: unit, gen: gen, kind: kind}) })
}

// run is the loop. It returns when shutdown completes or, under `service run`,
// when no selected unit (--until-done: no enabled unit) wants to run any more; the code is 0 unless a unit
// it was asked to run ended failed.
func (m *monitor) run(signals <-chan os.Signal) int {
	defer m.cancel()
	go m.publish()
	defer close(m.evq)
	m.reload()
	m.stepAll()
	var poll <-chan time.Time
	if m.pollEvery > 0 {
		t := time.NewTicker(m.pollEvery)
		defer t.Stop()
		poll = t.C
	}
	for {
		if code, done := m.finished(); done {
			return code
		}
		select {
		case sig := <-signals:
			m.logf("%v: stopping every unit", sig)
			m.shutdown(m.stopping)
			m.stepAll()
		case <-poll:
			if !m.stopping {
				m.reload()
			}
		case msg := <-m.msgs:
			m.handle(msg)
		}
	}
}

// finished: shutting down and everything is down, or (service run) nothing
// selected wants to run.
func (m *monitor) finished() (int, bool) {
	if !m.stopping && m.only == nil && !m.untilDone {
		return 0, false
	}
	code := 0
	for _, u := range m.units {
		if u.busy || u.waiting || u.st.State != service.Inactive && u.st.State != service.Failed {
			return 0, false
		}
		if (m.only != nil || m.untilDone) && !m.stopping && m.wantUp(u) && !u.latched {
			return 0, false
		}
		if u.st.State == service.Failed {
			code = 1
		}
	}
	if m.stopping {
		code = 0
	}
	return code, true
}

func (m *monitor) handle(msg any) {
	switch v := msg.(type) {
	case msgActivated:
		m.onActivated(v)
	case msgDeactivated:
		if u := m.units[v.unit]; u != nil && u.gen == v.gen {
			u.busy = false
			if v.then != nil {
				v.then(u)
			}
		}
	case msgConfigured:
		m.onConfigured(v)
	case msgProvider:
		m.onProvider(v)
	case msgProbe:
		m.onProbe(v)
	case msgTimer:
		m.onTimer(v)
	case msgControl:
		v.reply <- m.control(v.req)
	}
	m.stepAll()
}

// ─── desired state ────────────────────────────────────────────────────────

func declFingerprint(u fleet.Service) string {
	u.Ring = 0
	b, _ := json.Marshal(u)
	return string(b)
}

// reload re-reads the registry and applies the difference: new units are
// added, changed ones reconfigured (live, or restarted when the provider
// says so), removed or retired ones stopped. Units whose declaration did not
// change are not touched — a reload never restarts a bystander.
func (m *monitor) reload() error {
	all, errs := m.load()
	for _, e := range errs {
		m.logf("registry: %v", e)
	}
	seen := map[string]bool{}
	for _, d := range all {
		if m.only != nil && !m.only[d.Name] {
			continue
		}
		if d.IsRetired() {
			continue
		}
		seen[d.Name] = true
		u, ok := m.units[d.Name]
		if !ok {
			m.add(d)
			continue
		}
		fp := declFingerprint(d)
		if fp == u.print && !u.gone {
			continue
		}
		wasEnabled := u.decl.Enabled
		u.decl, u.print, u.pol, u.gone = d, fp, policyOf(d), false
		u.ladder = newBackoffLadder(u.pol.minBackoff, u.pol.maxBackoff, u.pol.healthyAfter)
		u.limit.burst, u.limit.window = u.pol.burst, u.pol.burstWindow
		u.st.Enabled = d.Enabled
		if p, err := m.providers.For(d); err == nil {
			u.prov = p
		}
		// A changed declaration is the operator's answer to whatever latched it.
		u.latched = false
		if d.Enabled && !wasEnabled {
			u.held, u.limit.starts = false, nil
		}
		u.dirty = m.running(u)
	}
	for name, u := range m.units {
		if !seen[name] {
			u.gone = true
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (m *monitor) add(d fleet.Service) *unitRun {
	pol := policyOf(d)
	u := &unitRun{decl: d, print: declFingerprint(d), pol: pol,
		ladder: newBackoffLadder(pol.minBackoff, pol.maxBackoff, pol.healthyAfter),
		limit:  startLimiter{burst: pol.burst, window: pol.burstWindow},
		st:     service.Status{Unit: d.Name, Provider: d.ProviderName(), Enabled: d.Enabled, State: service.Inactive, Since: m.now()}}
	u.prov, _ = m.providers.For(d)
	m.units[d.Name] = u
	return u
}

// wantUp is the desired state: enabled (or started by hand, or selected by
// `service run`), not held down, not removed, and the monitor not stopping.
func (m *monitor) wantUp(u *unitRun) bool {
	if m.stopping || u.gone || u.held {
		return false
	}
	return u.decl.Enabled || u.adhoc || m.force
}

func (m *monitor) running(u *unitRun) bool {
	switch u.st.State {
	case service.Starting, service.Active, service.Degraded:
		return true
	}
	return false
}

// depsOf are the dependencies the monitor orders on. Under `service run`, a
// dependency outside the selected set is the operator's to provide.
func (m *monitor) depsOf(u *unitRun) []string {
	var out []string
	for _, d := range u.decl.Dependencies() {
		if m.only != nil && !m.only[d] {
			continue
		}
		out = append(out, d)
	}
	return out
}

// depsReady reports whether every dependency is up; otherwise the reason u
// waits and whether that reason is a failure.
func (m *monitor) depsReady(u *unitRun) (bool, string, bool) {
	for _, name := range m.depsOf(u) {
		d, ok := m.units[name]
		switch {
		case !ok || d.gone:
			return false, "requires unknown unit " + name, true
		case !m.wantUp(d) && !m.stopping:
			return false, "requires " + name + ", which is disabled or stopped", true
		case d.st.State.Exposed():
			continue
		case d.st.State == service.Failed:
			return false, "dependency " + name + " failed", true
		default:
			return false, "waiting for " + name, false
		}
	}
	return true, "", false
}

// stepAll moves every unit one step toward its desired state, to a fixpoint:
// a unit coming up may release dependents, one going down stops them.
func (m *monitor) stepAll() {
	names := make([]string, 0, len(m.units))
	for n := range m.units {
		names = append(names, n)
	}
	sort.Strings(names)
	for range len(names) + 1 {
		changed := false
		for _, n := range names {
			if m.step(m.units[n]) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for _, n := range names {
		if u := m.units[n]; u.gone && !u.busy && !m.running(u) && !u.waiting {
			delete(m.units, n)
		}
	}
}

// step takes at most one action for u and reports whether its state changed.
func (m *monitor) step(u *unitRun) bool {
	if u.busy {
		return false
	}
	before := u.st.State
	if m.running(u) {
		if !m.wantUp(u) {
			// Dependents go first (shutdown, disable): what requires u stops
			// before u does.
			if !m.hasRunningDependents(u) {
				m.deactivate(u, true, "stopped", nil)
			}
		} else if ok, why, _ := m.depsReady(u); !ok {
			m.deactivate(u, true, why, nil)
		} else if u.dirty {
			u.dirty = false
			m.configure(u)
		}
		return u.st.State != before
	}
	if !m.wantUp(u) || u.waiting || u.latched {
		if !m.wantUp(u) && !u.latched && u.st.State == service.Failed {
			m.transition(u, service.Inactive, "disabled")
		}
		return u.st.State != before
	}
	if m.stopping {
		return false
	}
	ok, why, failed := m.depsReady(u)
	if !ok {
		to := service.Inactive
		if failed {
			to = service.Failed
		}
		if u.st.Reason != why || u.st.State != to {
			u.st.Reason = why
			m.transition(u, to, why)
			return true
		}
		return false
	}
	m.activate(u, false)
	return u.st.State != before
}

// ─── transitions ──────────────────────────────────────────────────────────

func (m *monitor) transition(u *unitRun, to service.State, reason string) {
	from := u.st.State
	u.st.Reason = reason
	if from == to {
		return
	}
	u.st.State, u.st.Since = to, m.now()
	ev := service.Event{Unit: u.decl.Name, From: from, To: to, Reason: reason, At: u.st.Since}
	m.logf("%s: %s -> %s%s", ev.Unit, from, to, map[bool]string{true: " (" + reason + ")"}[reason != ""])
	select {
	case m.evq <- ev:
	default: // never block the loop on a slow sink
	}
}

// publish delivers transitions to the timeline and, for failed, the notify
// sinks — off the loop, in order.
func (m *monitor) publish() {
	for ev := range m.evq {
		if m.events != nil {
			m.events(ev)
		}
		if ev.To == service.Failed && m.notify != nil {
			m.notify(ev)
		}
	}
}

// fail latches u failed: it stays down until `service start` or a changed
// declaration, never an automatic retry.
func (m *monitor) fail(u *unitRun, reason string) {
	u.latched, u.st.Health = true, ""
	m.transition(u, service.Failed, reason)
}

// activate starts u through its provider. A start beyond the start limit,
// a foreign listener on a declared port or an unresolvable runtime fails the
// unit instead.
func (m *monitor) activate(u *unitRun, exempt bool) {
	if u.prov == nil {
		m.fail(u, fmt.Sprintf("no provider %q", u.decl.ProviderName()))
		return
	}
	if !exempt && !u.limit.allow(m.now()) {
		m.fail(u, fmt.Sprintf("start limit hit: %d starts within %v", u.pol.burst, u.pol.burstWindow))
		return
	}
	if m.checkBuild != nil {
		if err := m.checkBuild(u.decl); err != nil {
			m.fail(u, err.Error())
			return
		}
	}
	if u.decl.ProviderName() == fleet.ServiceProviderProcess && m.checkPorts != nil {
		if err := m.checkPorts(u.decl); err != nil {
			m.fail(u, err.Error())
			return
		}
	}
	if u.restart {
		u.st.Restarts++
		u.restart = false
	}
	u.gen++
	u.busy, u.fails, u.st.Health, u.dirty = true, 0, "", false
	u.startedAt = m.now()
	m.transition(u, service.Starting, "")
	gen, decl, prov := u.gen, u.decl, u.prov
	go func() {
		rt, err := service.Runtime{}, error(nil)
		if m.resolve != nil {
			rt, err = m.resolve(m.ctx, decl)
		}
		if err == nil {
			rt.Emit = func(ev service.Event) { m.post(msgProvider{unit: decl.Name, gen: gen, ev: ev}) }
			err = prov.Activate(m.ctx, decl, rt)
		}
		m.post(msgActivated{unit: decl.Name, gen: gen, err: err})
	}()
}

func (m *monitor) onActivated(v msgActivated) {
	u := m.units[v.unit]
	if u == nil || u.gen != v.gen {
		return
	}
	u.busy = false
	if v.err != nil {
		m.fail(u, v.err.Error())
		return
	}
	m.tick(u, true)
}

// tick runs one health check for the current incarnation: the declared probe
// when there is one, otherwise the provider's liveness.
func (m *monitor) tick(u *unitRun, first bool) {
	gen, decl, pol, prov := u.gen, u.decl, u.pol, u.prov
	delay := pol.healthInterval
	if first {
		delay = 0
		if pol.probe && pol.startPeriod > 0 {
			delay = min(pol.startPeriod, pol.healthInterval)
		}
	}
	run := func() {
		err, declared := m.probe(m.ctx, decl, pol)
		if !declared {
			err = prov.Health(m.ctx, decl.Name)
		}
		m.post(msgProbe{unit: decl.Name, gen: gen, err: err})
	}
	if delay == 0 {
		go run()
		return
	}
	time.AfterFunc(delay, run)
}

func (m *monitor) onProbe(v msgProbe) {
	u := m.units[v.unit]
	if u == nil || u.gen != v.gen || !m.running(u) {
		return // an incarnation that is gone: its probe chain ends here
	}
	if u.busy { // a live reconfigure is in flight: judge the next probe
		m.tick(u, false)
		return
	}
	switch {
	case v.err == nil:
		u.fails, u.st.Health = 0, "ok"
		if u.st.State != service.Active {
			m.transition(u, service.Active, "")
		}
	case errors.Is(v.err, errHealthPending):
		u.st.Health = "pending"
	default:
		u.st.Health = v.err.Error()
		inStartPeriod := u.st.State == service.Starting && m.now().Sub(u.startedAt) < u.pol.startPeriod
		if !inStartPeriod {
			u.fails++
			if u.st.State == service.Active {
				m.transition(u, service.Degraded, "health: "+v.err.Error())
			}
			if u.fails >= u.pol.healthFailures {
				reason := fmt.Sprintf("health: %d consecutive failures: %v", u.fails, v.err)
				m.deactivate(u, true, reason, func(u *unitRun) { m.afterExit(u, childExit{code: 1}, reason) })
				return
			}
		}
	}
	m.tick(u, false)
}

// onProvider handles a transition the provider observed itself: a child that
// exited, a capability that failed or reported in.
func (m *monitor) onProvider(v msgProvider) {
	u := m.units[v.unit]
	if u == nil || u.gen != v.gen || !m.running(u) {
		return
	}
	switch v.ev.To {
	case service.Inactive, service.Failed:
		ex := childExit{code: 1}
		if st := u.prov.Status(u.decl.Name); st.LastExit != nil {
			ex.code = *st.LastExit
		} else if v.ev.To == service.Inactive && u.decl.ProviderName() != fleet.ServiceProviderInternal {
			ex.code = 0
		}
		reason := v.ev.Reason
		if reason == "" && u.decl.ProviderName() == fleet.ServiceProviderInternal {
			reason = "owner went inactive"
		}
		// Clean up what the child left (its group, its log) quietly — this is
		// not a stop anyone asked for — then apply the restart policy.
		m.deactivate(u, false, reason, func(u *unitRun) { m.afterExit(u, ex, reason) })
	case service.Active:
		if u.st.State == service.Starting && !u.pol.probe {
			m.post(msgProbe{unit: u.decl.Name, gen: u.gen}) // the owner applied it
		}
	}
}

// afterExit applies the restart policy to an exit of the current incarnation.
func (m *monitor) afterExit(u *unitRun, ex childExit, reason string) {
	ran := m.now().Sub(u.startedAt)
	u.st.Health = "" // the verdict was about an incarnation that is gone
	m.refreshEnabled(u)
	if !m.wantUp(u) {
		m.transition(u, service.Inactive, reason)
		return
	}
	if u.pol.wantsRestart(ex) {
		d := u.ladder.delay(ran)
		u.waiting, u.restart = true, true
		m.transition(u, service.Inactive, fmt.Sprintf("%s; restarting in %v", reason, d))
		m.later(d, u.decl.Name, u.gen, "restart")
		return
	}
	if ex.code == 0 && !ex.signaled && ex.err == nil {
		u.latched = true
		m.transition(u, service.Inactive, "completed ("+reason+")")
		return
	}
	m.fail(u, reason)
}

// refreshEnabled rereads u's enabled bit from the registry right before a
// restart decision, instead of trusting reload's last cached snapshot: a
// disable written directly to the catalog (fleet's SetServiceEnabled, what
// `bashy llm down --service` calls, rather than `bashy service disable`
// which also pokes this monitor to reload) would otherwise not be seen
// until the next poll tick, during which an exit the disable raced can
// still be restarted (Sprint 455 F9).
func (m *monitor) refreshEnabled(u *unitRun) {
	if m.load == nil {
		return
	}
	all, _ := m.load()
	for _, d := range all {
		if d.Name == u.decl.Name {
			u.decl.Enabled = d.Enabled
			return
		}
	}
}

func (m *monitor) onTimer(v msgTimer) {
	u := m.units[v.unit]
	if u == nil || u.gen != v.gen {
		return
	}
	if v.kind == "restart" {
		u.waiting = false
	}
}

// deactivate takes u down through its provider; then runs on the loop once
// it is down. announce=false is the quiet cleanup after an exit.
func (m *monitor) deactivate(u *unitRun, announce bool, reason string, then func(*unitRun)) {
	u.gen++
	u.busy = true
	if announce {
		m.transition(u, service.Stopping, reason)
	}
	gen, name, prov := u.gen, u.decl.Name, u.prov
	if then == nil {
		then = func(u *unitRun) { m.transition(u, service.Inactive, reason) }
	}
	go func() {
		if prov != nil {
			if err := prov.Deactivate(m.ctx, name); err != nil {
				m.logf("%s: deactivate: %v", name, err)
			}
		}
		m.post(msgDeactivated{unit: name, gen: gen, then: then})
	}()
}

// configure applies a changed declaration to a running unit: live when the
// provider can, otherwise a restart that does not count against the limit.
func (m *monitor) configure(u *unitRun) {
	u.busy = true
	gen, decl, prov := u.gen, u.decl, u.prov
	go func() {
		rt, err := service.Runtime{}, error(nil)
		if m.resolve != nil {
			rt, err = m.resolve(m.ctx, decl)
		}
		if err == nil {
			err = prov.Configure(m.ctx, decl, rt)
		}
		m.post(msgConfigured{unit: decl.Name, gen: gen, err: err})
	}()
}

func (m *monitor) onConfigured(v msgConfigured) {
	u := m.units[v.unit]
	if u == nil || u.gen != v.gen {
		return
	}
	u.busy = false
	if v.err == nil {
		m.logf("%s: reconfigured live", u.decl.Name)
		return
	}
	reason := "reconfigured"
	if !errors.Is(v.err, service.ErrRestartRequired) {
		reason = "reconfigure: " + v.err.Error()
	}
	m.deactivate(u, true, reason, func(u *unitRun) {
		m.transition(u, service.Inactive, reason)
		if m.wantUp(u) {
			u.restart = true
			m.activate(u, true)
		}
	})
}

// shutdown stops every unit, dependents before what they require (step
// orders it); force (a second signal) cuts every grace short.
func (m *monitor) shutdown(force bool) {
	m.stopping = true
	// A unit sleeping out its restart backoff has nothing to stop: the signal
	// ends the wait now instead of after the delay.
	for _, u := range m.units {
		u.waiting = false
	}
	if force {
		m.cancel()
	}
}

// hasRunningDependents: some unit that requires u is still up or on its way.
func (m *monitor) hasRunningDependents(u *unitRun) bool {
	for _, o := range m.units {
		if o == u || !(m.running(o) || o.busy || o.st.State == service.Stopping) {
			continue
		}
		for _, d := range m.depsOf(o) {
			if d == u.decl.Name {
				return true
			}
		}
	}
	return false
}

// ─── status ───────────────────────────────────────────────────────────────

// status overlays the provider's view (pid, last exit) with the monitor's
// policy fields and the declaration's ports and exposure.
func (m *monitor) status(u *unitRun) service.Status {
	st := u.st
	st.Unit, st.Provider, st.Enabled = u.decl.Name, u.decl.ProviderName(), u.decl.Enabled
	if u.prov != nil {
		ps := u.prov.Status(u.decl.Name)
		if m.running(u) || u.st.State == service.Stopping {
			st.PID = ps.PID
		}
		st.LastExit = ps.LastExit
	}
	st.Ports, st.Exposed = unitPorts(u.decl), nil
	if st.State.Exposed() {
		st.Exposed = unitExposure(u.decl)
	}
	return st
}

func unitPorts(u fleet.Service) []string {
	var out []string
	for _, p := range u.Ports {
		bind, proto := p.Bind, p.Protocol
		if bind == "" {
			bind = "loopback"
		}
		if proto == "" {
			proto = "tcp"
		}
		out = append(out, bind+":"+strconv.Itoa(p.Port)+"/"+proto)
	}
	return out
}

func unitExposure(u fleet.Service) []string {
	var out []string
	if a := u.Expose.App; a != nil {
		name := a.Name
		if name == "" {
			name = u.Name
		}
		out = append(out, "app:"+name)
	}
	for _, ms := range u.Expose.Mesh {
		out = append(out, "mesh:"+ms.Name)
	}
	return out
}

func (m *monitor) statuses(names []string) []service.Status {
	var out []service.Status
	if len(names) == 0 {
		for n := range m.units {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	for _, n := range names {
		if u, ok := m.units[n]; ok {
			out = append(out, m.status(u))
		}
	}
	return out
}

// control is the socket's verbs, applied on the loop.
func (m *monitor) control(req ctlRequest) ctlResponse {
	resp := ctlResponse{OK: true, PID: os.Getpid()}
	unit := func() (*unitRun, bool) {
		u, ok := m.units[req.Unit]
		if !ok {
			resp.OK, resp.Error = false, fmt.Sprintf("no service %q", req.Unit)
		}
		return u, ok
	}
	switch req.Op {
	case "status":
		var names []string
		if req.Unit != "" {
			if _, ok := unit(); !ok {
				return resp
			}
			names = []string{req.Unit}
		}
		resp.Units = m.statuses(names)
	case "start":
		u, ok := unit()
		if !ok || !m.buildAllowed(u, &resp) {
			return resp
		}
		u.held, u.latched, u.waiting = false, false, false
		u.adhoc = !u.decl.Enabled
		u.limit.reset()
		resp.Units = []service.Status{m.status(u)}
	case "stop":
		u, ok := unit()
		if !ok {
			return resp
		}
		u.held, u.adhoc, u.waiting = true, false, false
		resp.Units = []service.Status{m.status(u)}
	case "restart":
		u, ok := unit()
		if !ok || !m.buildAllowed(u, &resp) {
			return resp
		}
		u.held, u.latched, u.waiting = false, false, false
		u.limit.reset()
		if m.running(u) && !u.busy {
			m.deactivate(u, true, "restart requested", func(u *unitRun) {
				m.transition(u, service.Inactive, "restart requested")
				u.restart = true
			})
		}
		resp.Units = []service.Status{m.status(u)}
	case "reload":
		if err := m.reload(); err != nil {
			resp.Error = err.Error() // a bad record is reported; the rest applied
		}
	case "report", "desired":
		return m.hosted(req)
	default:
		resp.OK, resp.Error = false, fmt.Sprintf("unknown op %q", req.Op)
	}
	return resp
}

// buildAllowed refuses an explicit start/restart of a unit whose binary is
// not a verified bashy-signed build, so the operator sees the refusal at the
// prompt rather than only as a failed state.
func (m *monitor) buildAllowed(u *unitRun, resp *ctlResponse) bool {
	if m.checkBuild == nil {
		return true
	}
	if err := m.checkBuild(u.decl); err != nil {
		resp.OK, resp.Error = false, err.Error()
		return false
	}
	return true
}

// checkServiceBuildHook is the gate newMonitor installs; tests substitute it.
var checkServiceBuildHook = checkServiceBuild

// checkServiceBuild is the default build gate: the host catalog's
// fleet.(*Catalog).CheckServiceBuild. A unit naming no tool pin passes
// without reading the catalog.
func checkServiceBuild(u fleet.Service) error {
	if strings.TrimSpace(u.Tool) == "" {
		return nil
	}
	return fleet.New().CheckServiceBuild(u)
}

// hosted serves the internal-unit owner side of the protocol through the
// delegated provider when one is installed.
func (m *monitor) hosted(req ctlRequest) ctlResponse {
	d, ok := m.providers[fleet.ServiceProviderInternal].(*delegatedInternal)
	if !ok {
		return ctlResponse{Error: "internal units are not delegated by this monitor"}
	}
	if req.Op == "report" {
		if err := d.report(req.Status); err != nil {
			return ctlResponse{Error: err.Error()}
		}
		return ctlResponse{OK: true, PID: os.Getpid()}
	}
	var out []service.Status
	for _, u := range m.units {
		if u.decl.ProviderName() == fleet.ServiceProviderInternal && u.decl.Owner == req.Owner {
			st := m.status(u)
			st.Enabled = m.wantUp(u) && d.wants(u.decl.Name)
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Unit < out[j].Unit })
	return ctlResponse{OK: true, PID: os.Getpid(), Units: out}
}

// splitUnits trims and drops empties.
func splitUnits(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
