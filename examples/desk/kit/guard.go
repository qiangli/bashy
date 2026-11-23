package kit

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
)

// SecurityOptions are the opt-in relaxations of the desk security headers.
// The zero value is ApplySecurityHeaders: framing forbidden.
type SecurityOptions struct {
	// Embeddable allows same-origin framing only: frame-ancestors 'self'
	// instead of 'none', for an app a page embeds as a block under the same
	// console origin (whiteboard, table, demo-queue projector). Never a
	// wildcard and never a foreign origin.
	Embeddable bool
}

// CSP is the Content-Security-Policy these options send.
func (o SecurityOptions) CSP() string {
	ancestors := "'none'"
	if o.Embeddable {
		ancestors = "'self'"
	}
	return "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors " + ancestors + "; base-uri 'none'; form-action 'self'"
}

// Apply sets the desk-wide response headers with these options.
func (o SecurityOptions) Apply(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", o.CSP())
}

// Wrap is WithSecurityHeaders with these options.
func (o SecurityOptions) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.Apply(w)
		h.ServeHTTP(w, r)
	})
}

// LANFlagName is the one escape hatch of the loopback guard: `--lan`.
const LANFlagName = "lan"

// LANFlag registers the explicit --lan override on a tool's serve flags.
func LANFlag(fs *flag.FlagSet) *bool {
	return fs.Bool(LANFlagName, false, "bind beyond loopback (LAN reachable, no LAN identity); logged on start")
}

// CheckBind is CheckLoopback with the explicit escape hatch every guard
// has. Without lan it refuses what CheckLoopback refuses, naming the
// override. With lan, any well-formed address is accepted, including a LAN address, 0.0.0.0 or ":port" (all
// interfaces), and the override is logged to logw (os.Stderr when nil) so
// whoever starts the tool sees that it is reachable beyond this host with
// no LAN identity. A malformed address is refused either way.
func CheckBind(addr string, lan bool, logw io.Writer) error {
	if !lan {
		if err := CheckLoopback(addr); err != nil {
			return fmt.Errorf("%w; --%s is the explicit, logged override", err, LANFlagName)
		}
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if logw == nil {
		logw = os.Stderr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		_, err = fmt.Fprintf(logw, "desk: --%s override given but %s is loopback; nothing beyond this host can reach it\n", LANFlagName, addr)
	} else {
		_, err = fmt.Fprintf(logw, "desk: --%s override: binding %s beyond loopback; anyone who can reach this host there can open the tool (no LAN identity)\n", LANFlagName, addr)
	}
	if err != nil {
		return errors.New("desk: the --lan override must be logged on start: " + err.Error())
	}
	return nil
}
