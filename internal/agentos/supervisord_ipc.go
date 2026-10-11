// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/service"
)

// The monitor's control socket (the provider contract, rule 7): a local
// stream socket at BASHY_HOME/service/supervisord.sock carrying one JSON
// request and one JSON response per connection. `bashy service
// status|start|stop|restart` are its clients; outpost's exposure reconciler
// and admin UI read the same `status` payload, and a process hosting internal
// capabilities (outpost, meet) uses `desired` + `report` to apply the units
// it owns. The payload is []service.Status — structured state, never text.
//
//	{"op":"status"[,"unit":"meet"]}      -> {"ok":true,"pid":N,"units":[Status…]}
//	{"op":"start|stop|restart","unit":X} -> {"ok":true,"units":[Status]}
//	{"op":"reload"}                      -> re-read the registry now
//	{"op":"desired","owner":"outpost"}   -> the internal units that owner hosts;
//	                                        units[i].enabled is "apply it now"
//	{"op":"report","status":{Status}}    -> the owner's view of one of them

// ServiceStatusSchema versions `bashy service status --json`.
const ServiceStatusSchema = "bashy-service-status-v1"

type ctlRequest struct {
	Op     string         `json:"op"`
	Unit   string         `json:"unit,omitempty"`
	Owner  string         `json:"owner,omitempty"`
	Status service.Status `json:"status,omitzero"`
}

type ctlResponse struct {
	OK    bool             `json:"ok"`
	Error string           `json:"error,omitempty"`
	PID   int              `json:"pid,omitempty"`
	Units []service.Status `json:"units,omitempty"`
}

// serviceHome is BASHY_HOME/service (default ~/.bashy/service): unit logs and
// the monitor socket.
func serviceHome() string {
	if h := strings.TrimSpace(os.Getenv("BASHY_HOME")); h != "" {
		return filepath.Join(h, "service")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".bashy", "service")
}

// supervisordSocket is the control socket path; BASHY_SUPERVISORD_SOCKET
// overrides it (a socket path must stay short on macOS).
func supervisordSocket() string {
	if p := strings.TrimSpace(os.Getenv("BASHY_SUPERVISORD_SOCKET")); p != "" {
		return p
	}
	return filepath.Join(serviceHome(), "supervisord.sock")
}

// errNoMonitor: nothing answers on the socket.
var errNoMonitor = errors.New("bashy supervisord is not running")

// ctlCall sends one request to the monitor at path.
func ctlCall(path string, req ctlRequest, timeout time.Duration) (ctlResponse, error) {
	c, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return ctlResponse{}, fmt.Errorf("%w (%s): %v", errNoMonitor, path, err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return ctlResponse{}, err
	}
	var resp ctlResponse
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&resp); err != nil {
		return ctlResponse{}, err
	}
	if !resp.OK && resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// serveControl listens on path and hands each request to the loop. A live
// monitor already answering there is refused — two monitors would fight over
// every unit; a stale socket file is removed.
func (m *monitor) serveControl(path string) (func(), error) {
	if _, err := ctlCall(path, ctlRequest{Op: "status"}, time.Second); err == nil {
		return nil, fmt.Errorf("another bashy supervisord is already serving %s", path)
	}
	_ = os.Remove(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.serveConn(c)
			}()
		}
	}()
	return func() {
		l.Close()
		_ = os.Remove(path)
		wg.Wait()
	}, nil
}

func (m *monitor) serveConn(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	var req ctlRequest
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(ctlResponse{Error: "bad request: " + err.Error()})
		return
	}
	reply := make(chan ctlResponse, 1)
	select {
	case m.msgs <- msgControl{req: req, reply: reply}:
	case <-m.ctx.Done():
		_ = json.NewEncoder(c).Encode(ctlResponse{Error: "supervisord is stopping"})
		return
	}
	var resp ctlResponse
	select {
	case resp = <-reply:
	case <-m.ctx.Done():
		resp = ctlResponse{Error: "supervisord is stopping"}
	}
	_ = json.NewEncoder(c).Encode(resp)
}

// ─── the internal provider, delegated to the owning process ───────────────

// delegatedInternal is how the monitor realizes `provider: internal` units
// whose capability lives in ANOTHER process (outpost, or the unit hosting it
// — meet hosts meet.events). The monitor owns the policy as for any unit; the
// owner applies the declaration to its registered capability with
// service.Internal and reports the result. Activate/Deactivate only change
// what `desired` tells the owner; status and health are what it reported.
// There is no child, no pid and no second flag: the unit's state IS whether
// the capability runs.
type delegatedInternal struct {
	now func() time.Time

	mu    sync.Mutex
	units map[string]*delegatedUnit
}

type delegatedUnit struct {
	want     bool
	reported bool
	status   service.Status
	emit     func(service.Event)
}

func newDelegatedInternal() *delegatedInternal {
	return &delegatedInternal{now: time.Now, units: map[string]*delegatedUnit{}}
}

func (d *delegatedInternal) Name() string { return fleet.ServiceProviderInternal }

func (d *delegatedInternal) Activate(_ context.Context, u fleet.Service, rt service.Runtime) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.units[u.Name] = &delegatedUnit{want: true, emit: rt.Emit,
		status: service.Status{Unit: u.Name, Provider: d.Name(), Enabled: u.Enabled, State: service.Starting,
			Reason: "awaiting owner " + u.Owner, Since: d.now()}}
	return nil
}

// Configure: the owner re-reads the declaration it applies; live by contract.
func (d *delegatedInternal) Configure(context.Context, fleet.Service, service.Runtime) error {
	return nil
}

func (d *delegatedInternal) Deactivate(_ context.Context, unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if u, ok := d.units[unit]; ok {
		u.want, u.reported = false, false
		u.status.State, u.status.Since = service.Inactive, d.now()
	}
	return nil
}

func (d *delegatedInternal) Health(_ context.Context, unit string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.units[unit]
	switch {
	case !ok || !u.want:
		return fmt.Errorf("service: %s is not active", unit)
	case !u.reported:
		return errHealthPending
	case u.status.State.Exposed():
		return nil
	}
	return fmt.Errorf("service: %s: owner reports %s: %s", unit, u.status.State, u.status.Reason)
}

func (d *delegatedInternal) Status(unit string) service.Status {
	d.mu.Lock()
	defer d.mu.Unlock()
	if u, ok := d.units[unit]; ok {
		return u.status
	}
	return service.Status{Unit: unit, Provider: d.Name(), State: service.Inactive}
}

// wants reports whether this monitor has actually activated the delegation.
// The declaration alone is not enough: an owner must not restart a unit while
// the monitor is backing off or has latched it down.
func (d *delegatedInternal) wants(unit string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	u, ok := d.units[unit]
	return ok && u.want
}

// report records the owner's view and forwards a transition to the monitor.
func (d *delegatedInternal) report(st service.Status) error {
	d.mu.Lock()
	u, ok := d.units[st.Unit]
	if !ok || !u.want {
		d.mu.Unlock()
		return fmt.Errorf("service: %s is not activated by this monitor", st.Unit)
	}
	from := u.status.State
	u.reported = true
	u.status.State, u.status.Reason, u.status.Health = st.State, st.Reason, st.Health
	if from != st.State {
		u.status.Since = d.now()
	}
	emit := u.emit
	d.mu.Unlock()
	if emit != nil && from != st.State {
		// report runs on the monitor loop; emit posts back to it.
		go emit(service.Event{Unit: st.Unit, From: from, To: st.State, Reason: st.Reason, At: d.now()})
	}
	return nil
}
