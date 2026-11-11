// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Package cast records a command's terminal output as asciicast v2 — the
// recorder behind the `cast record` skill-script (examples/cast/cast.bsh).
// The child runs under the platform pseudo-terminal (a creack/pty pair on
// Unix, ConPTY through aymanbagabas/go-pty on Windows) and every chunk it
// emits becomes one `[seconds,"o","data"]` event after the header line. The
// file is byte-identical in shape on every platform, so a scene scripted for
// examples/video renders the same whether it was captured on macOS, Linux or
// Windows. Replaces asciinema (GPL, no Windows build) for that one job; it is
// a recorder only — no player, no upload. It lives at example level, not in
// bashy's core: the command is a Bash# file whose Go fence imports this
// package through examples/cast/go.mod.
package cast

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Options configures one recording.
type Options struct {
	// Command is argv: Command[0] is the program. Required.
	Command []string
	// Cols and Rows size the pseudo-terminal; zero means 80×24. Deliberately
	// not read from the host terminal: a scene must not change shape with the
	// window it happened to be recorded from.
	Cols, Rows uint16
	// IdleLimit caps the gap recorded between two events (asciinema's
	// idle_time_limit): a long pause in the child becomes IdleLimit in the
	// cast. Zero keeps every pause verbatim.
	IdleLimit time.Duration
	// Dir and Env are the child's working directory and environment (nil Env
	// inherits the process environment). TERM=xterm-256color is appended.
	Dir string
	Env []string
	// Title is the optional asciicast header title.
	Title string
}

// Header is the asciicast v2 header line. Field order is the file order.
type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp"`
	Command   string            `json:"command,omitempty"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env"`
}

// Event is one recorded chunk: Time is seconds since the header timestamp.
type Event struct {
	Time float64
	Code string // "o" output (the only code this recorder writes), "i" input
	Data string
}

const (
	defaultCols = 80
	defaultRows = 24
	term        = "xterm-256color"
)

// Record runs opts.Command under a pseudo-terminal and streams the cast to w.
// It returns the child's exit status; err is non-nil only when the recorder
// itself failed (no pty, exec failure, write error), in which case the status
// is 127 by analogy with a shell that could not run the command.
func Record(w io.Writer, opts Options) (int, error) {
	if len(opts.Command) == 0 {
		return 127, errors.New("cast: no command")
	}
	cols, rows := opts.Cols, opts.Rows
	if cols == 0 {
		cols = defaultCols
	}
	if rows == 0 {
		rows = defaultRows
	}
	env := append(append([]string(nil), opts.Env...), "TERM="+term)
	if opts.Env == nil {
		env = nil // inherit; TERM is set by the platform start below
	}
	s, err := start(opts.Command, opts.Dir, env, cols, rows)
	if err != nil {
		return 127, fmt.Errorf("cast: %w", err)
	}
	started := time.Now()
	h := Header{
		Version: 2, Width: int(cols), Height: int(rows), Timestamp: started.Unix(),
		Command: strings.Join(opts.Command, " "), Title: opts.Title,
		Env: map[string]string{"SHELL": shellOf(opts.Env), "TERM": term},
	}
	bw := bufio.NewWriter(w)
	if err := writeHeader(bw, h); err != nil {
		_ = s.Close()
		return 127, err
	}

	// Read until the pty reports hangup. A read error here is the normal end
	// of a recording (EIO on Linux, EOF on macOS and on ConPTY once the
	// console is released by finish); only a write failure is reported.
	var werr error
	var carry []byte // trailing bytes of an incomplete UTF-8 sequence
	var shift time.Duration
	last := started
	buf := make([]byte, 32*1024)
	for werr == nil {
		n, rerr := s.Read(buf)
		if n > 0 {
			now := time.Now()
			if opts.IdleLimit > 0 && now.Sub(last) > opts.IdleLimit {
				shift += now.Sub(last) - opts.IdleLimit
			}
			last = now
			data, rest := splitRune(append(carry, buf[:n]...))
			carry = rest
			if len(data) > 0 {
				werr = writeEvent(bw, now.Sub(started)-shift, "o", string(data))
			}
		}
		if rerr != nil {
			break
		}
	}
	if werr == nil && len(carry) > 0 {
		werr = writeEvent(bw, time.Since(started)-shift, "o", string(carry))
	}
	code, waitErr := s.finish()
	if werr != nil {
		return 127, werr
	}
	if waitErr != nil {
		return 127, fmt.Errorf("cast: %w", waitErr)
	}
	return code, bw.Flush()
}

// splitRune returns b minus any incomplete trailing UTF-8 sequence, and that
// tail. A chunk boundary can fall inside a multi-byte rune; encoding the halves
// separately would turn each into U+FFFD, so the tail waits for the next read.
func splitRune(b []byte) (data, rest []byte) {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i], append([]byte(nil), b[i:]...)
			}
			break
		}
	}
	return b, nil
}

func writeHeader(w io.Writer, h Header) error {
	line, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", line)
	return err
}

func writeEvent(w io.Writer, at time.Duration, code, data string) error {
	if at < 0 {
		at = 0
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "[%s, %q, %s]\n", strconv.FormatFloat(at.Seconds(), 'f', 6, 64), code, payload)
	return err
}

func shellOf(env []string) string {
	if env == nil {
		return os.Getenv("SHELL")
	}
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "SHELL="); ok {
			return v
		}
	}
	return ""
}

// Parse reads an asciicast v2 stream back: the header and the event list.
func Parse(r io.Reader) (Header, []Event, error) {
	var h Header
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	if !sc.Scan() {
		return h, nil, errors.New("cast: empty stream")
	}
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
		return h, nil, fmt.Errorf("cast: header: %w", err)
	}
	if h.Version != 2 {
		return h, nil, fmt.Errorf("cast: unsupported version %d", h.Version)
	}
	var events []Event
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var raw []json.RawMessage
		if err := json.Unmarshal(sc.Bytes(), &raw); err != nil || len(raw) != 3 {
			return h, events, fmt.Errorf("cast: event %d: malformed", len(events)+1)
		}
		var ev Event
		if err := json.Unmarshal(raw[0], &ev.Time); err != nil {
			return h, events, fmt.Errorf("cast: event %d: time: %w", len(events)+1, err)
		}
		if err := json.Unmarshal(raw[1], &ev.Code); err != nil {
			return h, events, fmt.Errorf("cast: event %d: code: %w", len(events)+1, err)
		}
		if err := json.Unmarshal(raw[2], &ev.Data); err != nil {
			return h, events, fmt.Errorf("cast: event %d: data: %w", len(events)+1, err)
		}
		events = append(events, ev)
	}
	return h, events, sc.Err()
}
