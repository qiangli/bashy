package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var client = &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{}}
var peerPattern = regexp.MustCompile(`^[a-zA-Z0-9]{16,160}$`)
var servicePattern = regexp.MustCompile(`^demo-[A-Za-z0-9_-]{12}$`)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		report(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: demo-queue host | join | serve | status | invite | start | assign | skip | end | prepare | leave | finish | view | viewer | rendered | chunk | links (run VERB --help)")
		return nil
	}
	switch args[0] {
	case "host":
		flags := flag.NewFlagSet("host", flag.ContinueOnError)
		output := flags.Bool("json", false, "Print links as JSON")
		dry := flags.Bool("dry-run", false, "Validate without starting a session")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: demo-queue host [--json] [--dry-run]")
		}
		if *dry {
			if _, err := meshStatus(ctx); err != nil {
				return err
			}
			return printJSON(map[string]any{"action": "host", "dryRun": true})
		}
		return host(ctx, *output)
	case "serve":
		return serveShape(ctx, args[1:])
	case "join":
		flags := flag.NewFlagSet("join", flag.ContinueOnError)
		name := flags.String("name", "", "Presenter display name")
		output := flags.Bool("json", false, "Print presenter URL as JSON")
		dry := flags.Bool("dry-run", false, "Validate the invite without joining")
		invite := flags.String("invite-file", "", "Read the invite from a file or - for stdin")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: demo-queue join [--name NAME]")
		}
		return join(ctx, *name, *invite, *output, *dry)
	default:
		return command(ctx, args)
	}
}

func meshCommand(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "outpost", append([]string{"mesh"}, args...)...).Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("outpost mesh %s: %w %s", args[0], err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("outpost mesh %s: %w", args[0], err)
	}
	return out, nil
}

type peerStatus struct {
	Status *struct {
		PeerID string `json:"peer_id"`
		Peers  []struct {
			ID     string `json:"id"`
			Direct bool   `json:"direct"`
		} `json:"peers"`
	} `json:"status"`
}

func meshStatus(ctx context.Context) (peerStatus, error) {
	var status peerStatus
	b, err := meshCommand(ctx, "status", "--json")
	if err != nil {
		return status, err
	}
	if err := json.Unmarshal(b, &status); err != nil {
		return status, fmt.Errorf("read Outpost mesh status: %w", err)
	}
	if status.Status == nil || !peerPattern.MatchString(status.Status.PeerID) {
		return status, errors.New("start a paired Outpost with the mesh enabled")
	}
	return status, nil
}

func cleanupMesh(args ...string) {
	if _, err := meshCommand(context.Background(), args...); err != nil {
		report(err)
	}
}

func openLocal() (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }

func serve(ln net.Listener, handler http.Handler) (*http.Server, <-chan error) {
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 4 * time.Second,
		ReadTimeout: 12 * time.Second, IdleTimeout: 20 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	done := make(chan error, 1)
	go func() {
		defer close(done)
		if err := server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			done <- err
		}
	}()
	return server, done
}

func host(ctx context.Context, jsonOutput bool) error {
	status, err := meshStatus(ctx)
	if err != nil {
		return err
	}
	s := &room{
		session: newSession(), owner: secret(), display: secret(),
		peer: status.Status.PeerID, service: "demo-" + secret()[:12],
	}
	local, err := openLocal()
	if err != nil {
		return err
	}
	defer local.Close()
	inbound, err := openLocal()
	if err != nil {
		return err
	}
	defer inbound.Close()
	if _, err := meshCommand(ctx, "expose", s.service, inbound.Addr().String()); err != nil {
		return err
	}
	defer cleanupMesh("unexpose", s.service)
	meshServer, meshDone := serve(inbound, s.mesh())
	defer meshServer.Close()
	localServer, localDone := serve(local, s.local(local.Addr().String()))
	defer localServer.Close()
	clockDone := make(chan struct{})
	defer close(clockDone)
	go s.clock(clockDone)
	if jsonOutput {
		if err := printJSON(map[string]string{"control": "http://" + local.Addr().String() + "/control#" + s.owner, "projector": "http://" + local.Addr().String() + "/projector#" + s.display}); err != nil {
			return err
		}
	} else {
		fmt.Printf("Organizer (private): http://%s/control#%s\n", local.Addr(), s.owner)
		fmt.Printf("Projector (local):   http://%s/projector#%s\n", local.Addr(), s.display)
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-meshDone:
		return err
	case err := <-localDone:
		return err
	}
}

func decodeInvite(input string) ([]string, error) {
	if len(input) > 512 {
		return nil, errors.New("invalid invite")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(input))
	var parts []string
	if err != nil || json.Unmarshal(b, &parts) != nil || len(parts) != 3 ||
		!peerPattern.MatchString(parts[0]) || !servicePattern.MatchString(parts[1]) || len(parts[2]) != 32 {
		return nil, errors.New("invalid invite")
	}
	return parts, nil
}

func join(ctx context.Context, name, inviteFile string, jsonOutput, dry bool) error {
	input := os.Stdin
	if inviteFile != "" && inviteFile != "-" {
		file, err := os.Open(inviteFile)
		if err != nil {
			return err
		}
		defer file.Close()
		input = file
	}
	console := bufio.NewScanner(input)
	if inviteFile == "" {
		fmt.Fprint(os.Stderr, "Private invite: ")
	}
	if !console.Scan() {
		return errors.New("a private invite is required")
	}
	parts, err := decodeInvite(console.Text())
	if err != nil {
		return err
	}
	if name == "" {
		if input != os.Stdin {
			return errors.New("--name is required with an invite file")
		}
		fmt.Fprint(os.Stderr, "Display name: ")
		if !console.Scan() {
			return errors.New("a display name is required")
		}
		name = console.Text()
	}
	if strings.TrimSpace(name) == "" || len(strings.TrimSpace(name)) > 80 {
		return errors.New("enter a display name of up to 80 bytes")
	}
	peer, service, key := parts[0], parts[1], parts[2]
	if dry {
		return printJSON(map[string]any{"action": "join", "dryRun": true, "name": name})
	}
	out, err := meshCommand(ctx, "listen", peer, service, "--local", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := strings.TrimSpace(string(out))
	tcp, err := net.ResolveTCPAddr("tcp4", addr)
	if err != nil || !tcp.IP.IsLoopback() || tcp.Port == 0 {
		return errors.New("Outpost did not return a loopback forward")
	}
	defer cleanupMesh("unlisten", addr)
	payload, _ := json.Marshal(action{Name: name})
	target := &url.URL{Scheme: "http", Host: addr}
	req, err := http.NewRequestWithContext(ctx, "POST", target.String()+"/api/join", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reach organizer over Outpost mesh: %w", err)
	}
	var joined struct{ Lease, Error string }
	err = json.NewDecoder(resp.Body).Decode(&joined)
	resp.Body.Close()
	if err != nil || joined.Lease == "" {
		if joined.Error != "" {
			return errors.New(joined.Error)
		}
		return errors.New("the organizer declined the join request")
	}
	ln, err := openLocal()
	if err != nil {
		return err
	}
	defer ln.Close()
	localKey := secret()
	m := presenterProxy(target, peer, key, joined.Lease, localKey)
	server, done := serve(ln, loopback(ln.Addr().String(), m))
	defer server.Close()
	presenterURL := "http://" + ln.Addr().String() + "/present#" + localKey
	if jsonOutput {
		if err := printJSON(map[string]string{"presenter": presenterURL}); err != nil {
			return err
		}
	} else {
		fmt.Printf("Presenter (local): %s\n", presenterURL)
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

func presenterProxy(target *url.URL, peer, key, lease, localKey string) http.Handler {
	proxy := &httputil.ReverseProxy{
		Transport:     &http.Transport{ResponseHeaderTimeout: 12 * time.Second},
		FlushInterval: -1,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Header.Del("Cookie")
			r.Out.Header.Set("Authorization", "Bearer "+key)
			r.Out.Header.Set("X-Demo-Lease", lease)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			fail(w, 502, "The organizer is unreachable over the Outpost mesh.")
		},
	}
	m := http.NewServeMux()
	m.HandleFunc("GET /", page)
	m.HandleFunc("GET /api/demo-link", linkGuard("present", localKey))
	m.Handle("GET /api/events", proxy)
	m.Handle("POST /api/member", proxy)
	m.Handle("POST /api/upload", proxy)
	m.HandleFunc("POST /api/prepare", func(w http.ResponseWriter, r *http.Request) {
		status, err := meshStatus(r.Context())
		if err == nil {
			for _, p := range status.Status.Peers {
				if p.ID == peer && p.Direct {
					proxy.ServeHTTP(w, r)
					return
				}
			}
		}
		fail(w, 409, "The organizer needs a direct Outpost mesh connection. Check that both devices are on the same LAN, then retry.")
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/demo-link" &&
			!equal(bearer(r), localKey) && !equal(r.URL.Query().Get("key"), localKey) {
			fail(w, 403, "Open the private presenter URL.")
			return
		}
		m.ServeHTTP(w, r)
	})
}
