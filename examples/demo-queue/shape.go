package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const linkMark = "demo-queue-v1"

func loopbackAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	n, portErr := strconv.ParseUint(port, 10, 16)
	return err == nil && portErr == nil && n != 0 && host == "127.0.0.1"
}

func linkGuard(role, key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if role != r.URL.Query().Get("role") || !equal(bearer(r), key) {
			fail(w, 403, "The private session link is invalid.")
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, linkMark)
	}
}

func serveShape(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := flags.String("addr", os.Getenv("BASHY_APP_ADDR"), "Console loopback address")
	data := flags.String("data", os.Getenv("BASHY_APP_DATA"), "Principal data directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !loopbackAddress(*addr) || *data == "" {
		return errors.New("serve requires the console-assigned loopback --addr and --data")
	}
	ln, err := net.Listen("tcp4", *addr)
	if err != nil {
		return err
	}
	server, done := serve(ln, shapedHandler())
	defer server.Close()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

func shapedHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { response(w, 200, map[string]bool{"ok": true}) })
	m.HandleFunc("GET /", page)
	m.HandleFunc("GET /api/", relaySession)
	m.HandleFunc("POST /api/", relaySession)
	return m
}

var relayTransport = &http.Transport{ResponseHeaderTimeout: 12 * time.Second}

func relaySession(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		fail(w, 403, "Open the Demo Queue page through the console.")
		return
	}
	q := r.URL.Query()
	addr, role := r.Header.Get("X-Demo-Target"), r.Header.Get("X-Demo-Role")
	key := bearer(r)
	if addr == "" {
		addr, role = q.Get("target"), q.Get("role")
	}
	if key == "" {
		key = q.Get("key")
	}
	if !loopbackAddress(addr) || !keyPattern.MatchString(key) {
		fail(w, 400, "Connect using a private local Demo Queue link.")
		return
	}
	switch role {
	case "control", "present", "projector":
	default:
		fail(w, 400, "Unknown session role.")
		return
	}
	// The native process proves the link at its own role endpoint before a
	// shaped page can forward an action. Only known Demo Queue routes are sent.
	switch r.Method + " " + r.URL.Path {
	case "GET /api/demo-link", "GET /api/events", "GET /api/chunk",
		"POST /api/control", "POST /api/viewer", "POST /api/rendered",
		"POST /api/prepare", "POST /api/member", "POST /api/upload":
	default:
		http.NotFound(w, r)
		return
	}
	base := "http://" + addr
	probeCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	probe, _ := http.NewRequestWithContext(probeCtx, "GET", base+"/api/demo-link?role="+role, nil)
	probe.Header.Set("Authorization", "Bearer "+key)
	res, err := relayTransport.RoundTrip(probe)
	if err != nil {
		fail(w, 502, "The local Demo Queue process could not be reached.")
		return
	}
	proof, _ := io.ReadAll(io.LimitReader(res.Body, 32))
	res.Body.Close()
	if res.StatusCode != 200 || string(proof) != linkMark {
		fail(w, 403, "The local Demo Queue link was refused.")
		return
	}
	if r.URL.Path == "/api/demo-link" {
		_, _ = io.WriteString(w, linkMark)
		return
	}
	q.Del("target")
	q.Del("role")
	q.Set("key", key)
	target := &url.URL{Scheme: "http", Host: addr, Path: r.URL.Path, RawQuery: q.Encode()}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		fail(w, 400, "Invalid session request.")
		return
	}
	req.ContentLength = r.ContentLength
	req.Header.Set("Authorization", "Bearer "+key)
	for _, name := range []string{"Content-Type", "X-Demo-Viewer"} {
		req.Header.Set(name, r.Header.Get(name))
	}
	out, err := relayTransport.RoundTrip(req)
	if err != nil {
		fail(w, 502, "The local Demo Queue request failed.")
		return
	}
	defer out.Body.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", out.Header.Get("Content-Type"))
	w.WriteHeader(out.StatusCode)
	if strings.HasPrefix(out.Header.Get("Content-Type"), "text/event-stream") {
		controller := http.NewResponseController(w)
		buf := make([]byte, 4096)
		for {
			n, err := out.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil || controller.Flush() != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	_, _ = io.Copy(w, out.Body)
}
