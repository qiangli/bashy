package agentos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/foreman"
	"github.com/qiangli/coreutils/pkg/resources"
	"github.com/qiangli/coreutils/pkg/weave"
)

type sprintResourceAlert struct {
	ID        string    `json:"id"`
	Sprint    int64     `json:"sprint"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	At        time.Time `json:"at"`
	Owner     string    `json:"owner"`
	Coalesced int       `json:"coalesced,omitempty"`
}
type sprintAlertCondition struct {
	Version    int                   `json:"version"`
	Sprint     int64                 `json:"sprint"`
	Key        string                `json:"key"`
	Owner      string                `json:"owner"`
	Severity   string                `json:"severity"`
	Onset      time.Time             `json:"onset"`
	Recovery   time.Time             `json:"recovery"`
	ObservedAt time.Time             `json:"observed_at"`
	Generation uint64                `json:"generation"`
	Pending    []sprintResourceAlert `json:"pending"`
	Current    *sprintResourceAlert  `json:"current,omitempty"`
}
type sprintAlertSample struct {
	key, message     string
	at               time.Time
	value, high, low float64
	known            bool
}

func init() {
	foreman.ObserveSession = func(ctx context.Context, session, owner string) func() {
		if !strings.HasPrefix(session, "sprint-") || !strings.HasSuffix(session, "-manager") {
			return nil
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(session, "sprint-"), "-manager"), 10, 64)
		if err != nil || id <= 0 {
			return nil
		}
		return startSprintObservation(ctx, id, owner, 5*time.Second, observeSprintResources)
	}
}
func startSprintObservation(parent context.Context, id int64, owner string, every time.Duration, observe func(context.Context, int64, string)) func() {
	if observe == nil {
		return func() {}
	}
	if every <= 0 {
		every = 5 * time.Second
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if err := waitInboxPoll(ctx, every); err != nil {
				return
			}
			observe(ctx, id, owner)
		}
	}()
	return func() { cancel(); <-done }
}
func observeSprintResources(ctx context.Context, id int64, owner string) {
	s, err := collectSprintMonitor(ctx, sprintMonitorOptions{Sprint: id})
	if err != nil {
		return
	}
	// Ownership is re-read from the bounded shared inventory; no lease is renewed
	// by observation and a successor is never impersonated by this old worker.
	live := false
	if s.Inventory != nil {
		for _, seat := range s.Inventory.Sprints {
			if seat.ID == id && seat.Owner == owner && seat.Active {
				live = true
				break
			}
		}
	}
	if !live {
		return
	}
	_, _ = updateSprintAlerts(ctx, s, id, owner, true)
}
func activeSprintAlerts(ledger *resources.AlertLedger, id int64) []sprintResourceAlert {
	var alerts []sprintResourceAlert
	if ledger == nil {
		return alerts
	}
	for _, raw := range ledger.Entries {
		var c sprintAlertCondition
		if json.Unmarshal(raw, &c) != nil || c.Version != 1 || (id > 0 && c.Sprint != id) {
			continue
		}
		if c.Current != nil {
			alerts = append(alerts, *c.Current)
		}
	}
	// Deterministic keys make hash-based change detection independent of map order.
	for i := 1; i < len(alerts); i++ {
		for j := i; j > 0 && alerts[j].ID < alerts[j-1].ID; j-- {
			alerts[j], alerts[j-1] = alerts[j-1], alerts[j]
		}
	}
	return alerts
}
func sprintAlertSamples(s *sprintMonitorSnapshot) []sprintAlertSample {
	var samples []sprintAlertSample
	hostMissing := float64(0)
	if s.Host == nil || s.Host.System == nil || s.Host.Sections["cpu"].Stale || s.Host.Sections["memory"].Stale {
		hostMissing = 1
	}
	sourceAt := s.At.Truncate(5 * time.Second)
	if s.Host != nil && hostMissing == 0 {
		sourceAt = s.Host.At
	}
	samples = append(samples, sprintAlertSample{"source.host", "host resource source unavailable; inspect sprint monitor warnings", sourceAt, hostMissing, 1, 0, true})
	if s.Host != nil && s.Host.System != nil {
		h := s.Host
		system := h.System
		status := h.Sections["cpu"]
		known := status.Kind != "unknown" && !status.Stale && system.CPU.Source != ""
		at := status.At
		if at.IsZero() {
			at = h.At
		}
		samples = append(samples, sprintAlertSample{"host.cpu", fmt.Sprintf("host CPU pressure %.1f%% (%s); inspect competing workloads", system.CPU.UsagePercent, system.CPU.Source), at, system.CPU.UsagePercent, 90, 75, known})
		status = h.Sections["memory"]
		at = status.At
		if at.IsZero() {
			at = h.At
		}
		samples = append(samples, sprintAlertSample{"host.memory", fmt.Sprintf("host memory %.1f%% used; reduce owned work admission", system.Memory.UsedPercent), at, system.Memory.UsedPercent, 90, 80, status.Kind != "unknown" && !status.Stale && system.Memory.TotalBytes > 0})
		for _, d := range system.Disks {
			samples = append(samples, sprintAlertSample{"host.disk." + d.Mount, fmt.Sprintf("disk %s %.1f%% used; inspect eligible sprint artifacts", d.Mount, d.UsedPercent), h.At, d.UsedPercent, 90, 80, d.TotalBytes > 0 && !h.Sections["disks"].Stale && h.Sections["disks"].Kind != "unknown"})
		}
	}
	if s.Models != nil {
		for _, a := range s.Models.Accounts {
			for _, m := range a.Metrics {
				if m.Name != "quota.used_percent" {
					continue
				}
				value := float64(0)
				if m.Value != nil {
					value = *m.Value
				}
				window := ""
				if m.WindowStart != nil && m.WindowEnd != nil {
					window = m.WindowEnd.Sub(*m.WindowStart).String()
				}
				samples = append(samples, sprintAlertSample{"account." + a.Provider + "." + a.Account + "." + a.Pool + "." + m.Name + "." + m.Source + "." + window, fmt.Sprintf("%s shared pool %s quota %.1f%% used (%s); inspect models limits", a.Provider, a.Pool, value, m.Source), m.ObservedAt, value, 90, 75, m.Value != nil && m.Classification != "unknown" && m.Classification != "stale" && a.AccountKnown})
			}
		}
	}
	return samples
}
func updateSprintAlerts(ctx context.Context, s *sprintMonitorSnapshot, id int64, owner string, publish bool) ([]sprintResourceAlert, error) {
	return updateSprintAlertTargets(ctx, s, map[int64]string{id: owner}, id, publish)
}
func updateSprintAlertTargets(ctx context.Context, s *sprintMonitorSnapshot, targets map[int64]string, selected int64, publish bool) ([]sprintResourceAlert, error) {
	samples := sprintAlertSamples(s)
	ledger, err := resources.UpdateAlertState(ctx, resources.ResourcesStateDir(), func(ledger *resources.AlertLedger) error {
		if ledger.Entries == nil {
			ledger.Entries = map[string]json.RawMessage{}
		}
		for id, owner := range targets {
			for _, sample := range samples {
				digest := sha256.Sum256([]byte(sample.key))
				key := fmt.Sprintf("sprint:%d:%x", id, digest[:16])
				var c sprintAlertCondition
				changed := false
				if raw, ok := ledger.Entries[key]; ok {
					if err := json.Unmarshal(raw, &c); err != nil {
						return fmt.Errorf("alert state corrupt: %w", err)
					}
				} else if len(ledger.Entries) >= 255 {
					// Reserve one ledger row for an actionable overflow summary. The
					// snapshot remains the drill-down source for omitted metric detail.
					key = "resource-alert-overflow"
					c = sprintAlertCondition{Version: 1, Key: key, Sprint: id, Owner: owner}
					if raw, ok := ledger.Entries[key]; ok {
						_ = json.Unmarshal(raw, &c)
					}
					if c.Current == nil {
						appendSprintAlert(&c, "warning", "additional resource conditions exceed the detailed alert-state limit; inspect sprint monitor --json", s.At)
						b, _ := json.Marshal(c)
						ledger.Entries[key] = b
					}
					continue
				}
				if c.Version == 0 {
					c = sprintAlertCondition{Version: 1, Key: key, Sprint: id, Owner: owner}
					changed = true
				}
				if c.Owner != owner {
					changed = true
					priorPending := c.Pending
					c.Owner = owner
					// Pending old-owner events remain durable but get one successor
					// summary rather than multiplying a transition for each retry.
					c.Pending = nil
					if c.Current != nil {
						appendSprintAlert(&c, c.Severity, "handoff: "+c.Current.Message, s.At)
					} else if len(priorPending) > 0 {
						last := priorPending[len(priorPending)-1]
						appendSprintAlert(&c, last.Severity, "handoff: "+last.Message, s.At)
						c.Current = nil
					}
				}
				if !sample.known {
					changed = changed || !c.Recovery.IsZero() || (c.Current == nil && !c.Onset.IsZero())
					c.Recovery = time.Time{}
					if c.Current == nil {
						c.Onset = time.Time{}
					}
				}
				if sample.known && sample.at.After(c.ObservedAt) {
					changed = true
					c.ObservedAt = sample.at
					if sample.value >= sample.high {
						c.Recovery = time.Time{}
						if c.Onset.IsZero() {
							c.Onset = sample.at
						}
						if c.Current == nil && sample.at.Sub(c.Onset) >= 15*time.Second {
							appendSprintAlert(&c, "warning", sample.message, sample.at)
						}
					} else if sample.value <= sample.low {
						c.Onset = time.Time{}
						if c.Current != nil {
							if c.Recovery.IsZero() {
								c.Recovery = sample.at
							}
							if sample.at.Sub(c.Recovery) >= 30*time.Second {
								appendSprintAlert(&c, "recovered", sample.message, sample.at)
								c.Current = nil
								c.Severity = ""
							}
						}
					} else {
						c.Recovery = time.Time{}
						if c.Current == nil {
							c.Onset = time.Time{}
						}
					}
				}
				// All clients evaluate the same cached source sample. Keep its
				// existing bytes unless this transaction advanced condition state;
				// the store can then recognize the no-op without reserializing it.
				if !changed {
					continue
				}
				b, err := json.Marshal(c)
				if err != nil {
					return err
				}
				ledger.Entries[key] = b
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if publish {
		for key, raw := range ledger.Entries {
			var c sprintAlertCondition
			if json.Unmarshal(raw, &c) != nil || targets[c.Sprint] != c.Owner || c.Owner == "" {
				continue
			}
			for _, notice := range c.Pending {
				body, _ := json.Marshal(notice)
				err := weave.WithSprintObservationOwner(ctx, c.Sprint, c.Owner, func() error {
					_, err := bus.PostMessageOnce(ctx, notice.ID, bus.Post{From: "resource-observer", To: c.Owner, Topic: fmt.Sprintf("sprint.%d.resources", c.Sprint), Body: string(body)})
					return err
				})
				if err != nil {
					return activeSprintAlerts(ledger, selected), err
				}
				_, err = resources.UpdateAlertState(ctx, resources.ResourcesStateDir(), func(l *resources.AlertLedger) error {
					var current sprintAlertCondition
					if err := json.Unmarshal(l.Entries[key], &current); err != nil {
						return err
					}
					pending := current.Pending[:0]
					for _, p := range current.Pending {
						if p.ID != notice.ID {
							pending = append(pending, p)
						}
					}
					current.Pending = pending
					b, e := json.Marshal(current)
					if e == nil {
						l.Entries[key] = b
					}
					return e
				})
				if err != nil {
					return activeSprintAlerts(ledger, selected), err
				}
			}
		}
	}
	return activeSprintAlerts(ledger, selected), nil
}
func appendSprintAlert(c *sprintAlertCondition, severity, message string, at time.Time) {
	c.Generation++
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", c.Key, c.Generation, c.Owner)))
	if len(message) > 512 {
		message = message[:512]
	}
	notice := sprintResourceAlert{ID: "resource-" + hex.EncodeToString(sum[:16]), Sprint: c.Sprint, Severity: severity, Message: message, At: at, Owner: c.Owner}
	if len(c.Pending) >= 4 {
		notice.Coalesced = c.Pending[len(c.Pending)-1].Coalesced + 1
		c.Pending = c.Pending[:3]
	}
	c.Pending = append(c.Pending, notice)
	c.Severity = severity
	c.Current = &notice
}
