// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
)

// Monitor policy defaults (docs/bashy-service-design.md §Policy). A unit that
// leaves a field empty gets these; every unit — seed, internal capability or
// operator declaration — is read through the same unitPolicy, so no unit has
// a policy of its own outside its YAML.
const (
	unitDefaultHealthInterval = 10 * time.Second
	unitDefaultHealthTimeout  = 2 * time.Second
	unitDefaultHealthFailures = 3
	unitDefaultStartBurst     = 5
	unitDefaultStartInterval  = 5 * time.Minute
)

// unitPolicy is a unit's declaration resolved to the values the monitor acts
// on: parsed durations with defaults applied.
type unitPolicy struct {
	restart      string
	minBackoff   time.Duration
	maxBackoff   time.Duration
	healthyAfter time.Duration
	burst        int
	burstWindow  time.Duration

	probe          bool // a declared http | tcp | exec probe
	healthInterval time.Duration
	healthTimeout  time.Duration
	healthFailures int
	startPeriod    time.Duration

	stopSignal syscall.Signal
	grace      time.Duration
}

func durOr(s string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return def
}

// policyOf resolves u. Validate has already refused malformed durations, so a
// parse failure here can only mean "unset".
func policyOf(u fleet.Service) unitPolicy {
	p := unitPolicy{
		restart:        u.Restart.Policy,
		minBackoff:     durOr(u.Restart.Backoff.Min, supervisordMinBackoff),
		maxBackoff:     durOr(u.Restart.Backoff.Max, supervisordMaxBackoff),
		healthyAfter:   durOr(u.Restart.Backoff.HealthyAfter, supervisordHealthyAfter),
		burst:          u.Restart.StartLimit.Burst,
		burstWindow:    durOr(u.Restart.StartLimit.Interval, unitDefaultStartInterval),
		probe:          u.Health.HTTP != "" || u.Health.TCP || len(u.Health.Exec) > 0,
		healthInterval: durOr(u.Health.Interval, unitDefaultHealthInterval),
		healthTimeout:  durOr(u.Health.Timeout, unitDefaultHealthTimeout),
		healthFailures: u.Health.Failures,
		startPeriod:    durOr(u.Health.StartPeriod, 0),
		stopSignal:     stopSignalOf(u.Stop.Signal),
		grace:          durOr(u.Stop.Timeout, supervisordGrace),
	}
	if p.restart == "" {
		p.restart = fleet.RestartOnFailure
	}
	if p.maxBackoff < p.minBackoff {
		p.maxBackoff = p.minBackoff
	}
	if p.burst == 0 {
		p.burst = unitDefaultStartBurst
	}
	if p.healthFailures == 0 {
		p.healthFailures = unitDefaultHealthFailures
	}
	return p
}

func stopSignalOf(s string) syscall.Signal {
	switch strings.TrimPrefix(strings.ToUpper(s), "SIG") {
	case "INT":
		return syscall.SIGINT
	case "HUP":
		return syscall.SIGHUP
	case "QUIT":
		return syscall.SIGQUIT
	case "KILL":
		return syscall.SIGKILL
	}
	return syscall.SIGTERM
}

// wantsRestart is the restart policy applied to one exit. A clean exit is a
// completion under on-failure; only always restarts it.
func (p unitPolicy) wantsRestart(ex childExit) bool {
	switch p.restart {
	case fleet.RestartNo:
		return false
	case fleet.RestartAlways:
		return true
	}
	return ex.code != 0 || ex.signaled || ex.err != nil
}

// backoffLadder is the bounded exponential restart delay both supervisord
// modes share: start at min, double per consecutive failure, stop at max, and
// start over once a run lasted healthyAfter.
type backoffLadder struct {
	min, max, healthyAfter time.Duration
	next                   time.Duration
}

func newBackoffLadder(min, max, healthyAfter time.Duration) backoffLadder {
	return backoffLadder{min: min, max: max, healthyAfter: healthyAfter, next: min}
}

// delay returns the wait before the next start after a run that lasted ran,
// and advances the ladder.
func (b *backoffLadder) delay(ran time.Duration) time.Duration {
	if ran >= b.healthyAfter || b.next == 0 {
		b.next = b.min
	}
	d := b.next
	b.next = min(b.next*2, b.max)
	return d
}

// startLimiter is the crash-loop guard: more than burst starts inside window
// and the unit is failed instead of started again (systemd StartLimitBurst /
// StartLimitIntervalSec). An explicit `service start` resets it.
type startLimiter struct {
	burst  int
	window time.Duration
	starts []time.Time
}

// allow records a start at now and reports whether it is within the limit.
func (l *startLimiter) allow(now time.Time) bool {
	keep := l.starts[:0]
	for _, t := range l.starts {
		if now.Sub(t) < l.window {
			keep = append(keep, t)
		}
	}
	l.starts = keep
	if len(l.starts) >= l.burst {
		return false
	}
	l.starts = append(l.starts, now)
	return true
}

func (l *startLimiter) reset() { l.starts = nil }
