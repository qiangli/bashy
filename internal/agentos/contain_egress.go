package agentos

// Sprint: #421; Story: #1906; Story-ID: e853e9f1e2d9
//
// Contained model egress, opt-in as `bashy contain --net model` /
// @contain(net: "model"). `--net deny` stays fully offline (no proxy, no proxy
// variables): the effect layer reads it as proof of no network. A model scope
// is deny plus this allowlist, so it never discharges the net effect. The profile cannot name a host (Seatbelt accepts
// only "*" or "localhost" there), so the model providers are reached through a
// CONNECT-only proxy that bashy runs OUTSIDE the sandbox on a loopback port;
// the profile allows that one port and the host-local model door, and the
// proxy refuses every endpoint that is not on the allowlist. The allowlist is
// the registry's API models (the same base_url set `weave start --sealed`
// derives its egress from) plus BASHY_CONTAIN_ALLOW_HOST.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/yoke/pkg/broker/door"
	"github.com/qiangli/yoke/pkg/fleet"
)

const (
	// containAllowHostEnv adds endpoints (host, host:port or URL, comma
	// separated) to the model-egress allowlist.
	containAllowHostEnv = "BASHY_CONTAIN_ALLOW_HOST"
)

// containEgress is what a contained tree may reach besides local unix sockets.
type containEgress struct {
	// Remote holds "host:port" endpoints reached through the proxy.
	Remote []string
	// LoopbackPorts are host-local TCP ports reached directly (the model door).
	LoopbackPorts []int
}

func (e containEgress) empty() bool { return len(e.Remote) == 0 && len(e.LoopbackPorts) == 0 }

// containEndpoint normalizes a URL, host or host:port to a lower-case host and
// a port (the URL scheme's default when none is given).
func containEndpoint(raw string) (string, int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, fmt.Errorf("empty endpoint")
	}
	defaultPort := 443
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", 0, fmt.Errorf("endpoint %q is not a host, host:port or URL", raw)
	}
	if u.Scheme == "http" {
		defaultPort = 80
	}
	port := defaultPort
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", 0, fmt.Errorf("endpoint %q has an invalid port", raw)
		}
		port = n
	}
	return strings.ToLower(u.Hostname()), port, nil
}

func containIsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// buildContainEgress derives the allowlist from endpoint strings and the model
// door port. A loopback endpoint is a direct port; anything else goes through
// the proxy. Malformed entries are skipped: a bad registry row must not make
// every contained command fail.
func buildContainEgress(endpoints []string, doorPort int) containEgress {
	remote := map[string]bool{}
	ports := map[int]bool{}
	if doorPort > 0 {
		ports[doorPort] = true
	}
	for _, raw := range endpoints {
		host, port, err := containEndpoint(raw)
		if err != nil {
			continue
		}
		if containIsLoopbackHost(host) {
			ports[port] = true
			continue
		}
		remote[net.JoinHostPort(host, strconv.Itoa(port))] = true
	}
	out := containEgress{}
	for r := range remote {
		out.Remote = append(out.Remote, r)
	}
	sort.Strings(out.Remote)
	for p := range ports {
		out.LoopbackPorts = append(out.LoopbackPorts, p)
	}
	sort.Ints(out.LoopbackPorts)
	return out
}

// loadContainEgress is the host's allowlist: the registry's API models, the
// door, and BASHY_CONTAIN_ALLOW_HOST. A seam so tests never read the registry.
var loadContainEgress = func() containEgress {
	var endpoints []string
	if models, _ := fleet.New().Models(); models != nil {
		for _, m := range models {
			if m.Kind == fleet.ModelKindAPI && strings.TrimSpace(m.BaseURL) != "" {
				endpoints = append(endpoints, m.BaseURL)
			}
		}
	}
	for _, f := range strings.Split(os.Getenv(containAllowHostEnv), ",") {
		if f = strings.TrimSpace(f); f != "" {
			endpoints = append(endpoints, f)
		}
	}
	return buildContainEgress(endpoints, door.Port())
}

// containProxy is a CONNECT proxy (plus absolute-URI forwarding for http://
// endpoints) that serves only its allowlist.
type containProxy struct {
	allow     map[string]bool
	transport *http.Transport
	server    *http.Server
	listener  net.Listener
	wg        sync.WaitGroup
}

func (p *containProxy) allowed(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	return p.allow[net.JoinHostPort(strings.ToLower(strings.Trim(host, "[]")), port)]
}

func (p *containProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.connect(w, r)
		return
	}
	if r.URL.IsAbs() {
		p.forward(w, r)
		return
	}
	http.Error(w, "bashy contain: proxy request refused", http.StatusBadRequest)
}

func (p *containProxy) connect(w http.ResponseWriter, r *http.Request) {
	if !p.allowed(r.Host) {
		http.Error(w, "bashy contain: "+r.Host+" is not an allowed model endpoint", http.StatusForbidden)
		return
	}
	upstream, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(r.Context(), "tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "bashy contain: proxy cannot hijack", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	_, _ = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = buf.Flush()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		done := make(chan struct{}, 2)
		pipe := func(dst, src net.Conn) {
			_, _ = io.Copy(dst, src)
			if c, ok := dst.(interface{ CloseWrite() error }); ok {
				_ = c.CloseWrite()
			}
			done <- struct{}{}
		}
		go pipe(upstream, client)
		go pipe(client, upstream)
		<-done
		<-done
		client.Close()
		upstream.Close()
	}()
}

func (p *containProxy) forward(w http.ResponseWriter, r *http.Request) {
	port := r.URL.Port()
	if port == "" {
		port = "80"
		if r.URL.Scheme == "https" {
			port = "443"
		}
	}
	if !p.allowed(net.JoinHostPort(r.URL.Hostname(), port)) {
		http.Error(w, "bashy contain: "+r.URL.Host+" is not an allowed model endpoint", http.StatusForbidden)
		return
	}
	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.Header.Del("Proxy-Connection")
	out.Header.Del("Proxy-Authorization")
	resp, err := p.transport.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *containProxy) addr() string { return p.listener.Addr().String() }

func (p *containProxy) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = p.server.Shutdown(ctx)
	_ = p.server.Close()
	p.transport.CloseIdleConnections()
}

// startContainProxy serves allow (host:port entries) on a free loopback port.
func startContainProxy(allow []string) (*containProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &containProxy{
		allow:    map[string]bool{},
		listener: ln,
		// Proxy nil: an inherited HTTPS_PROXY must never chain this proxy to itself.
		transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext},
	}
	for _, a := range allow {
		p.allow[a] = true
	}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go p.server.Serve(ln)
	return p, nil
}

// containProxyEnv points a proxy-aware client at the proxy and keeps
// loopback (the door) direct.
func containProxyEnv(addr string) []string {
	u := "http://" + addr
	const noProxy = "localhost,127.0.0.1,::1"
	return []string{
		"HTTPS_PROXY=" + u, "https_proxy=" + u,
		"HTTP_PROXY=" + u, "http_proxy=" + u,
		"NO_PROXY=" + noProxy, "no_proxy=" + noProxy,
	}
}
