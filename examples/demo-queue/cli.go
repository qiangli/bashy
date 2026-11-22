package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{32}$`)
var sessionClient = &http.Client{
	Transport:     &http.Transport{ResponseHeaderTimeout: 10 * time.Second},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

type endpoint struct{ addr, role, key, via, mount, viewer string }

func fromLink(link string) (endpoint, error) {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "http" || !loopbackAddress(u.Host) || u.User != nil ||
		u.RawQuery != "" || !keyPattern.MatchString(u.Fragment) {
		return endpoint{}, errors.New("pass the private session link from host or join via --url or DEMO_QUEUE_URL")
	}
	role := strings.TrimPrefix(u.Path, "/")
	switch role {
	case "control", "present", "projector":
		return endpoint{addr: u.Host, role: role, key: u.Fragment}, nil
	default:
		return endpoint{}, errors.New("unknown session role")
	}
}

func (e *endpoint) discover(ctx context.Context) error {
	bin := os.Getenv("BASHY_BIN")
	if bin == "" {
		bin = "bashy"
	}
	out, err := exec.CommandContext(ctx, bin, "app", "show", "demo-queue", "--runtime", "--json").Output()
	if err != nil {
		return errors.New("open Demo Queue in the local Bashy console before using --via-shape")
	}
	var runtime struct{ Addr, URL string }
	if json.Unmarshal(out, &runtime) != nil || !loopbackAddress(runtime.Addr) {
		return errors.New("Demo Queue console runtime is unavailable")
	}
	mount, err := url.Parse(runtime.URL)
	if err != nil || (mount.Scheme != "http" && mount.Scheme != "https") || mount.Host == "" || mount.User != nil {
		return errors.New("Demo Queue console mount is unavailable")
	}
	e.via, e.mount = runtime.Addr, runtime.URL
	return nil
}

func (e endpoint) do(ctx context.Context, method, path, typ string, q url.Values, body io.Reader) (*http.Response, error) {
	if q == nil {
		q = make(url.Values)
	}
	addr := e.addr
	if e.via != "" {
		addr = e.via
		q.Set("target", e.addr)
		q.Set("role", e.role)
	}
	if path == "events" {
		q.Set("key", e.key)
		if e.viewer != "" {
			q.Set("viewer", e.viewer)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+addr+"/api/"+path+"?"+q.Encode(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	if typ != "" {
		req.Header.Set("Content-Type", typ)
	}
	if e.viewer != "" {
		req.Header.Set("X-Demo-Viewer", e.viewer)
	}
	res, err := sessionClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach the %s Demo Queue session: %w", e.role, err)
	}
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return res, nil
	}
	defer res.Body.Close()
	var fault struct{ Error string }
	_ = json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&fault)
	if fault.Error == "" {
		fault.Error = res.Status
	}
	return nil, errors.New(fault.Error)
}

func (e endpoint) post(ctx context.Context, path string, a action, dry bool) (*http.Response, error) {
	b, _ := json.Marshal(a)
	q := make(url.Values)
	if dry {
		q.Set("dry_run", "1")
	}
	return e.do(ctx, "POST", path, "application/json", q, bytes.NewReader(b))
}

func printJSON(value any) error { return json.NewEncoder(os.Stdout).Encode(value) }

type sessionState struct {
	Turn      *broadcast   `json:"turn"`
	Ended     bool         `json:"ended"`
	Me        *participant `json:"me"`
	Projector bool         `json:"projector"`
}

func (e endpoint) watch(ctx context.Context, fn func([]byte) (bool, error)) error {
	res, err := e.do(ctx, "GET", "events", "", nil, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	scan := bufio.NewScanner(res.Body)
	scan.Buffer(make([]byte, 4096), 4<<20)
	for scan.Scan() {
		b, ok := bytes.CutPrefix(scan.Bytes(), []byte("data: "))
		if !ok {
			continue
		}
		done, err := fn(b)
		if done || err != nil {
			return err
		}
	}
	if err := scan.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (e endpoint) snapshot(ctx context.Context) (sessionState, error) {
	var state sessionState
	err := e.watch(ctx, func(data []byte) (bool, error) { return true, json.Unmarshal(data, &state) })
	return state, err
}

func command(ctx context.Context, args []string) error {
	verb := args[0]
	flags := flag.NewFlagSet(verb, flag.ContinueOnError)
	link := flags.String("url", os.Getenv("DEMO_QUEUE_URL"), "Private native session link")
	via := flags.Bool("via-shape", false, "Use the running Bashy shaped app instance")
	_ = flags.Bool("json", true, "Print machine-readable results")
	dry := flags.Bool("dry-run", false, "Validate without changing state")
	id := flags.String("id", "", "Participant ID for assign")
	limit := flags.Int("limit", 120, "Turn length in seconds")
	warning := flags.Int("warning", 15, "Warning in seconds")
	viewer := flags.String("viewer", "", "Projector viewer token")
	turn := flags.Uint64("turn", 0, "Turn ID")
	generation := flags.Uint64("generation", 0, "Media generation")
	seq := flags.Uint64("seq", 0, "Media sequence")
	watch := flags.Bool("watch", false, "Stream state as NDJSON")
	input := flags.String("input", "", "VP8 WebM file or - for a live WebM stream on stdin")
	audio := flags.Bool("audio", false, "The supplied WebM stream contains Opus audio")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument to %s", verb)
	}
	e, err := fromLink(*link)
	if err != nil {
		return err
	}
	e.viewer = *viewer
	if *via {
		if err := e.discover(ctx); err != nil {
			return err
		}
	}
	a := action{Action: verb, ID: *id, Limit: *limit, Warning: *warning, Turn: *turn, Generation: *generation}
	path := ""
	switch verb {
	case "status":
		return e.watch(ctx, func(b []byte) (bool, error) { _, err := fmt.Fprintln(os.Stdout, string(b)); return !*watch, err })
	case "view":
		if *dry {
			if err := e.validate(ctx); err != nil {
				return err
			}
			return printJSON(map[string]any{"action": "view", "dryRun": true})
		}
		return e.view(ctx, os.Stdout)
	case "links":
		if e.mount == "" {
			return printJSON(map[string]string{e.role: *link})
		}
		u, err := url.Parse(e.mount)
		if err != nil {
			return err
		}
		u.Path = strings.TrimRight(u.Path, "/") + "/" + e.role
		u.Fragment = *link
		return printJSON(map[string]string{e.role: u.String()})
	case "invite", "start", "assign", "skip", "end":
		path = "control"
	case "leave", "finish":
		path = "member"
		if verb == "leave" {
			a.Action = "withdraw"
		}
	case "viewer", "rendered":
		path = verb
	case "chunk":
		if *dry {
			return errors.New("chunk is a binary read")
		}
		res, err := e.do(ctx, "GET", "chunk", "", mediaQuery(*turn, *generation, *seq), nil)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, err = io.Copy(os.Stdout, res.Body)
		return err
	case "prepare":
		if e.role != "present" {
			return errors.New("prepare requires a presenter link")
		}
		return e.present(ctx, *input, *audio, *dry)
	default:
		return fmt.Errorf("unknown action %q", verb)
	}
	if (verb == "skip" || verb == "finish") && a.Turn == 0 {
		state, err := e.snapshot(ctx)
		if err != nil {
			return err
		}
		if state.Turn == nil {
			return errors.New("there is no current turn")
		}
		a.Turn = state.Turn.ID
	}
	res, err := e.post(ctx, path, a, *dry)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 204 {
		return printJSON(map[string]any{"action": verb, "ok": true, "dryRun": *dry})
	}
	_, err = io.Copy(os.Stdout, res.Body)
	return err
}

func mediaQuery(turn, gen, seq uint64) url.Values {
	return url.Values{"turn": {strconv.FormatUint(turn, 10)}, "generation": {strconv.FormatUint(gen, 10)}, "seq": {strconv.FormatUint(seq, 10)}}
}

func (e endpoint) present(ctx context.Context, input string, audio, dry bool) error {
	if input == "" {
		return errors.New("prepare requires --input with a VP8 WebM file or live stdin")
	}
	in := os.Stdin
	if input != "-" {
		var err error
		in, err = os.Open(input)
		if err != nil {
			return err
		}
		defer in.Close()
	}
	source := bufio.NewReader(in)
	header, err := source.Peek(4)
	if err != nil || !bytes.Equal(header, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return errors.New("input must begin with a WebM header")
	}
	if dry {
		if err := e.validate(ctx); err != nil {
			return err
		}
		return printJSON(map[string]any{"action": "prepare", "dryRun": true})
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	res, err := e.do(probeCtx, "POST", "prepare", "application/octet-stream", nil, bytes.NewReader(make([]byte, probeSize)))
	if err != nil {
		cancel()
		return err
	}
	res.Body.Close()
	cancel()
	res, err = e.post(ctx, "member", action{Action: "ready"}, false)
	if err != nil {
		return err
	}
	res.Body.Close()
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		if result, err := e.post(c, "member", action{Action: "withdraw"}, false); err == nil {
			result.Body.Close()
		}
	}()
	if err := printJSON(map[string]string{"status": "Ready"}); err != nil {
		return err
	}
	return e.sendSource(ctx, source, audio)
}

func (e endpoint) validate(ctx context.Context) error {
	res, err := e.do(ctx, "GET", "demo-link", "", url.Values{"role": {e.role}}, nil)
	if err != nil {
		return err
	}
	return res.Body.Close()
}

func (e endpoint) sendSource(ctx context.Context, source io.Reader, audio bool) error {
	mime := "video/webm;codecs=vp8"
	if audio {
		mime += ",opus"
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var turn *broadcast
	var stopped <-chan error
	return e.watch(ctx, func(raw []byte) (bool, error) {
		var snap sessionState
		if err := json.Unmarshal(raw, &snap); err != nil {
			return true, err
		}
		if snap.Ended || (turn != nil && (snap.Turn == nil || snap.Turn.ID != turn.ID || snap.Turn.Generation != turn.Generation)) {
			return true, nil
		}
		if stopped != nil {
			select {
			case err := <-stopped:
				if err != nil {
					return true, err
				}
				stopped = nil
			default:
			}
		}
		if turn == nil && snap.Turn != nil && snap.Projector {
			turn = snap.Turn
			result := make(chan error, 1)
			stopped = result
			go func() { result <- e.uploadSource(ctx, source, mime, turn) }()
		}
		return false, nil
	})
}

func (e endpoint) uploadSource(ctx context.Context, source io.Reader, mime string, turn *broadcast) error {
	// Send a bounded read at a time. Live input controls its own rate. A
	// small delay on finite input gives the current projector time to append.
	buf := make([]byte, 96<<10)
	for seq := uint64(0); ; seq++ {
		n, err := source.Read(buf)
		if n > 0 {
			res, uploadErr := e.do(ctx, "POST", "upload", mime, mediaQuery(turn.ID, turn.Generation, seq), bytes.NewReader(buf[:n]))
			if uploadErr != nil {
				return uploadErr
			}
			res.Body.Close()
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(200 * time.Millisecond):
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// view publishes one turn as an ordered WebM stream to the supplied sink.
func (e endpoint) view(ctx context.Context, out io.Writer) error {
	if e.role != "projector" {
		return errors.New("view requires a projector link")
	}
	res, err := e.post(ctx, "viewer", action{}, false)
	if err != nil {
		return err
	}
	var connection struct{ Viewer string }
	err = json.NewDecoder(res.Body).Decode(&connection)
	res.Body.Close()
	if err != nil {
		return err
	}
	e.viewer = connection.Viewer
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var turn *broadcast
	var finished <-chan error
	return e.watch(ctx, func(raw []byte) (bool, error) {
		var snap sessionState
		if err := json.Unmarshal(raw, &snap); err != nil {
			return true, err
		}
		if finished != nil {
			select {
			case err := <-finished:
				if snap.Turn != nil && turn.ID == snap.Turn.ID && turn.Generation == snap.Turn.Generation {
					return true, err
				}
				return true, nil
			default:
			}
		}
		if snap.Ended || (turn != nil && (snap.Turn == nil || turn.ID != snap.Turn.ID || turn.Generation != snap.Turn.Generation)) {
			return true, nil
		}
		if turn == nil && snap.Turn != nil {
			turn = snap.Turn
			result := make(chan error, 1)
			finished = result
			go func() { result <- e.receiveMedia(ctx, out, turn) }()
		}
		return false, nil
	})
}

func (e endpoint) receiveMedia(ctx context.Context, out io.Writer, turn *broadcast) error {
	for seq := uint64(0); ; {
		res, err := e.do(ctx, "GET", "chunk", "", mediaQuery(turn.ID, turn.Generation, seq), nil)
		if err != nil {
			return err
		}
		if res.StatusCode == 204 {
			res.Body.Close()
			continue
		}
		_, err = io.Copy(out, res.Body)
		res.Body.Close()
		if err != nil {
			return err
		}
		if seq == 0 {
			res, err := e.post(ctx, "rendered", action{Turn: turn.ID, Generation: turn.Generation}, false)
			if err != nil {
				return err
			}
			res.Body.Close()
		}
		seq++
	}
}
