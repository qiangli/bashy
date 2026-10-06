// Package httpproxy implements an HTTP/1.1 forward proxy and opaque CONNECT tunnels.
package httpproxy

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

// Config configures a proxy. Empty Auth disables authentication. DialContext
// can route both forwarded requests and CONNECT tunnels over a peer transport.
type Config struct {
	Auth        string
	DialContext func(context.Context, string, string) (net.Conn, error)
	Logger      *slog.Logger
}

// Handler is safe for concurrent requests. Close terminates tunnels and releases idle origin connections.
type Handler struct {
	ctx       context.Context
	cancel    context.CancelFunc
	auth      string
	dial      func(context.Context, string, string) (net.Conn, error)
	logger    *slog.Logger
	transport *http.Transport
	forward   *httputil.ReverseProxy
}

// New creates a reusable proxy handler.
func New(c Config) (*Handler, error) {
	if c.Auth != "" {
		u, _, ok := strings.Cut(c.Auth, ":")
		if !ok || u == "" {
			return nil, fmt.Errorf("auth must be user:pass with a nonempty user")
		}
	}
	if c.DialContext == nil {
		c.DialContext = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	// Use an independent transport: neither environment proxy settings nor
	// process-wide replacements of DefaultTransport should affect this server.
	tr := &http.Transport{
		DialContext:           c.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handler{ctx: ctx, cancel: cancel, auth: c.Auth, dial: c.DialContext, logger: c.Logger, transport: tr}
	h.forward = &httputil.ReverseProxy{
		Transport: tr,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.Out.Header.Del("Proxy-Authorization")
			p.Out.Header.Del("Proxy-Connection")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			h.logger.Warn("proxy forward failed", "method", r.Method, "target", r.URL.Host)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}
	return h, nil
}

func (h *Handler) Close() {
	h.cancel()
	h.transport.CloseIdleConnections()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.auth != "" {
		scheme, token, ok := strings.Cut(r.Header.Get("Proxy-Authorization"), " ")
		decoded, err := base64.StdEncoding.DecodeString(token)
		got, want := sha256.Sum256(decoded), sha256.Sum256([]byte(h.auth))
		if !ok || !strings.EqualFold(scheme, "Basic") || err != nil || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("Proxy-Authenticate", `Basic realm="bashy", charset="UTF-8"`)
			http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
			h.logger.Info("proxy authentication rejected", "remote", r.RemoteAddr)
			return
		}
	}
	if r.Method == http.MethodConnect {
		h.connect(w, r)
		return
	}
	if r.URL.Scheme != "http" || r.URL.Host == "" || r.URL.User != nil {
		http.Error(w, "absolute http URL required", http.StatusBadRequest)
		return
	}
	h.logger.Info("proxy forward", "method", r.Method, "target", r.URL.Host)
	h.forward.ServeHTTP(w, r)
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || host == "" || port == "" {
		http.Error(w, "CONNECT requires host:port", http.StatusBadRequest)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unavailable", http.StatusInternalServerError)
		return
	}
	upstream, err := h.dial(r.Context(), "tcp", r.Host)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	client, buffer, err := hj.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	// Use the handler lifetime: net/http cancels the request context on a
	// client read EOF, which is a valid half-close for a tunnel.
	stop := context.AfterFunc(h.ctx, func() { client.Close(); upstream.Close() })
	defer stop()
	if _, err = buffer.WriteString("HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	if err = buffer.Flush(); err != nil {
		return
	}
	h.logger.Info("proxy tunnel opened", "target", r.Host)
	done := make(chan struct{})
	go func() {
		_, err := io.Copy(upstream, buffer) // Includes bytes read ahead with CONNECT.
		if err != nil {
			upstream.Close()
			client.Close()
		} else {
			closeWrite(upstream)
		}
		close(done)
	}()
	_, err = io.Copy(client, upstream)
	if err != nil {
		client.Close()
		upstream.Close()
	} else {
		closeWrite(client)
	}
	<-done
	h.logger.Info("proxy tunnel closed", "target", r.Host)
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	} else {
		c.Close()
	}
}

// Serve owns l and blocks until the listener fails or ctx is canceled. Cancellation
// closes HTTP connections and tunnels; a canceled server returns nil.
func Serve(ctx context.Context, l net.Listener, c Config) error {
	h, err := New(c)
	if err != nil {
		l.Close()
		return err
	}
	defer h.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	defer srv.Close()
	stop := context.AfterFunc(ctx, func() { h.Close(); srv.Close() })
	defer stop()
	err = srv.Serve(l)
	if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
		return nil
	}
	return err
}
