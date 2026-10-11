// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
)

// The declared probe is the monitor's (the provider contract, rule 4): one of
// http | tcp | exec against the unit's own listener, with the same timeout
// and failure count for every unit. It replaces reading a daemon's status
// text for "stopped" — a verdict is an error value, never parsed prose.

// probeAddr is host:port for a listener; a wildcard bind is probed on
// loopback, which every wildcard listener also answers.
func probeAddr(p fleet.ServicePort) string {
	return net.JoinHostPort(probeHost(p.Bind), strconv.Itoa(p.Port))
}

func probeHost(bind string) string {
	switch bind {
	case "", "loopback", "localhost", "lan", "0.0.0.0", "*", "all":
		return "127.0.0.1"
	case "::":
		return "::1"
	}
	return bind
}

// listenHost is the address a unit's listener would bind.
func listenHost(bind string) string {
	switch bind {
	case "", "loopback", "localhost":
		return "127.0.0.1"
	case "lan", "*", "all":
		return "0.0.0.0"
	}
	return bind
}

// runProbe executes u's declared probe once. ok=false means none is declared
// (health is then the provider's liveness alone).
func runProbe(ctx context.Context, u fleet.Service, pol unitPolicy, r *reaper) (err error, ok bool) {
	h := u.Health
	if !pol.probe {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, pol.healthTimeout)
	defer cancel()
	port, _ := u.PortNamed(h.Port)
	switch {
	case h.HTTP != "":
		req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+probeAddr(port)+h.HTTP, nil)
		if rerr != nil {
			return rerr, true
		}
		resp, herr := http.DefaultClient.Do(req)
		if herr != nil {
			return fmt.Errorf("http %s: %w", h.HTTP, herr), true
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("http %s: status %d", h.HTTP, resp.StatusCode), true
		}
		return nil, true
	case h.TCP:
		var d net.Dialer
		c, derr := d.DialContext(ctx, "tcp", probeAddr(port))
		if derr != nil {
			return fmt.Errorf("tcp %s: %w", probeAddr(port), derr), true
		}
		c.Close()
		return nil, true
	default:
		cmd := exec.Command(h.Exec[0], h.Exec[1:]...)
		cmd.Env = os.Environ()
		cmd.SysProcAttr = ownProcessGroupAttr()
		child, serr := startTracked(cmd, r)
		if serr != nil {
			return fmt.Errorf("exec %s: %w", h.Exec[0], serr), true
		}
		done := make(chan childExit, 1)
		go func() { done <- child.Wait() }()
		select {
		case ex := <-done:
			if ex.code != 0 || ex.signaled || ex.err != nil {
				return fmt.Errorf("exec %s: %s", h.Exec[0], ex), true
			}
			return nil, true
		case <-ctx.Done():
			_ = child.Kill()
			<-done
			return fmt.Errorf("exec %s: timed out after %v", h.Exec[0], pol.healthTimeout), true
		}
	}
}

// errForeignListener marks a start refused because something outside the
// monitor already listens on a declared port. It is a failed state with a
// reason, never a restart loop: retrying cannot evict another program.
var errForeignListener = errors.New("held by a foreign listener")

// checkPorts verifies every declared listener is free before a process unit
// starts. The registry already refuses two ENABLED units on one port, so a
// held port here is a process the monitor does not own.
func checkPorts(u fleet.Service) error {
	for _, p := range u.Ports {
		addr := net.JoinHostPort(listenHost(p.Bind), strconv.Itoa(p.Port))
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		var err error
		if proto == "tcp" {
			// A listener on the wildcard does not stop a SO_REUSEADDR bind of a
			// specific address on BSD/macOS, so ask the port itself first.
			if c, derr := net.DialTimeout("tcp", probeAddr(p), 250*time.Millisecond); derr == nil {
				c.Close()
				return fmt.Errorf("port %d/%s (%s) is %w", p.Port, proto, addr, errForeignListener)
			}
		}
		if proto == "udp" {
			var c net.PacketConn
			if c, err = net.ListenPacket("udp", addr); err == nil {
				c.Close()
			}
		} else {
			var l net.Listener
			if l, err = net.Listen("tcp", addr); err == nil {
				l.Close()
			}
		}
		if err != nil {
			if errors.Is(err, syscall.EADDRINUSE) || isAddrInUse(err) {
				return fmt.Errorf("port %d/%s (%s) is %w", p.Port, proto, addr, errForeignListener)
			}
			return fmt.Errorf("port %d/%s (%s): %v", p.Port, proto, addr, err)
		}
	}
	return nil
}
