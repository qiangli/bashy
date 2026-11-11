// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package cast

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The child under test is this binary itself, re-entered through TestMain, so
// the same test runs on every platform without a printf(1) or sleep(1) on
// PATH — Windows included; there is no build tag anywhere in this package's
// tests.
func TestMain(m *testing.M) {
	switch os.Getenv("CAST_TEST_CHILD") {
	case "":
		os.Exit(m.Run())
	case "red":
		fmt.Print("\033[31mred\033[0m\n")
		time.Sleep(150 * time.Millisecond)
		fmt.Print("done\n")
		os.Exit(3)
	case "idle":
		fmt.Print("a\n")
		time.Sleep(400 * time.Millisecond)
		fmt.Print("b\n")
		os.Exit(0)
	case "split":
		// Colour codes, then a 3-byte rune (✓ U+2713) and a 4-byte rune
		// (🚀 U+1F680) each cut in two by a pause long enough that the
		// recorder's two reads see the halves separately.
		fmt.Print("\033[32mok\033[0m ")
		os.Stdout.Write([]byte{0xe2, 0x9c})
		time.Sleep(120 * time.Millisecond)
		os.Stdout.Write([]byte{0x93, ' ', 0xf0, 0x9f})
		time.Sleep(120 * time.Millisecond)
		os.Stdout.Write([]byte{0x9a, 0x80, '\n'})
		os.Exit(0)
	}
}

func record(t *testing.T, mode string, opts Options) (Header, []Event, int, string) {
	t.Helper()
	opts.Command = []string{os.Args[0]}
	opts.Env = append(os.Environ(), "CAST_TEST_CHILD="+mode)
	var buf bytes.Buffer
	code, err := Record(&buf, opts)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	h, events, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, buf.String())
	}
	return h, events, code, buf.String()
}

func TestRecordRedAndSleep(t *testing.T) {
	h, events, code, raw := record(t, "red", Options{Cols: 100, Rows: 30})
	if code != 3 {
		t.Errorf("exit status = %d, want the child's 3", code)
	}
	if h.Version != 2 || h.Width != 100 || h.Height != 30 {
		t.Errorf("header = %+v, want version 2, 100x30", h)
	}
	if h.Timestamp == 0 || h.Env["TERM"] != "xterm-256color" {
		t.Errorf("header timestamp/env = %d %v", h.Timestamp, h.Env)
	}
	if len(events) < 1 {
		t.Fatalf("no events in:\n%s", raw)
	}
	var all strings.Builder
	prev := -1.0
	for i, ev := range events {
		if ev.Code != "o" {
			t.Errorf("event %d code %q", i, ev.Code)
		}
		if ev.Time < prev {
			t.Errorf("event %d time %f < previous %f: not monotonic", i, ev.Time, prev)
		}
		prev = ev.Time
		all.WriteString(ev.Data)
	}
	out := all.String()
	for _, want := range []string{"\033[31m", "red", "\033[0m", "done"} {
		if !strings.Contains(out, want) {
			t.Errorf("recorded output lacks %q:\n%q", want, out)
		}
	}
	if events[len(events)-1].Time < 0.1 {
		t.Errorf("last event at %fs, want the 150ms sleep reflected", events[len(events)-1].Time)
	}
	if !strings.HasPrefix(raw, `{"version":2,"width":100,"height":30,"timestamp":`) {
		t.Errorf("header line is not the asciicast v2 shape: %s", strings.SplitN(raw, "\n", 2)[0])
	}
}

// A multi-byte rune cut by a read boundary must reach the cast intact: the
// recorder carries the incomplete tail into the next event instead of
// encoding each half as U+FFFD.
func TestRecordUTF8SplitAcrossReads(t *testing.T) {
	h, events, code, raw := record(t, "split", Options{Cols: 40, Rows: 8})
	if code != 0 {
		t.Errorf("exit status = %d", code)
	}
	if h.Width != 40 || h.Height != 8 {
		t.Errorf("header = %+v, want 40x8", h)
	}
	if len(events) < 2 {
		t.Fatalf("want the split to produce at least two events, got %d:\n%s", len(events), raw)
	}
	var all strings.Builder
	prev := -1.0
	for i, ev := range events {
		if ev.Time < prev {
			t.Errorf("event %d time %f < previous %f: not monotonic", i, ev.Time, prev)
		}
		prev = ev.Time
		if !utf8.ValidString(ev.Data) {
			t.Errorf("event %d is not valid UTF-8: %q", i, ev.Data)
		}
		if strings.ContainsRune(ev.Data, utf8.RuneError) {
			t.Errorf("event %d carries U+FFFD: %q", i, ev.Data)
		}
		all.WriteString(ev.Data)
	}
	if got, want := all.String(), "\033[32mok\033[0m \u2713 \U0001F680\r\n"; got != want {
		t.Errorf("recorded bytes = %q, want %q", got, want)
	}
}

func TestRecordIdleLimit(t *testing.T) {
	_, events, code, raw := record(t, "idle", Options{IdleLimit: 100 * time.Millisecond})
	if code != 0 {
		t.Errorf("exit status = %d", code)
	}
	last := events[len(events)-1]
	if !strings.Contains(last.Data, "b") {
		t.Fatalf("last event is not the post-sleep line:\n%s", raw)
	}
	// The child slept 400ms; with a 100ms idle limit the cast must be well
	// under that, and still after the first line.
	if last.Time > 0.3 {
		t.Errorf("idle limit not applied: last event at %fs\n%s", last.Time, raw)
	}
	var first *Event
	for i := range events {
		if strings.Contains(events[i].Data, "a") {
			first = &events[i]
			break
		}
	}
	if first == nil || last.Time < first.Time {
		t.Errorf("events not monotonic: first %v last %v", first, last)
	}
}

func TestRecordMissingCommand(t *testing.T) {
	var buf bytes.Buffer
	if code, err := Record(&buf, Options{}); err == nil || code != 127 {
		t.Errorf("empty command: code %d err %v", code, err)
	}
	if code, err := Record(&buf, Options{Command: []string{"definitely-not-a-program-" + fmt.Sprint(os.Getpid())}}); err == nil || code != 127 {
		t.Errorf("unknown program: code %d err %v", code, err)
	}
}

func TestSplitRune(t *testing.T) {
	data, rest := splitRune([]byte("ab\xe2\x9c"))
	if string(data) != "ab" || string(rest) != "\xe2\x9c" {
		t.Errorf("split = %q %q", data, rest)
	}
	data, rest = splitRune([]byte("ab\xe2\x9c\x93"))
	if string(data) != "ab\xe2\x9c\x93" || rest != nil {
		t.Errorf("complete = %q %q", data, rest)
	}
	data, rest = splitRune([]byte("\x1b[31m"))
	if string(data) != "\x1b[31m" || rest != nil {
		t.Errorf("ascii = %q %q", data, rest)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	if _, _, err := Parse(strings.NewReader("")); err == nil {
		t.Error("empty stream accepted")
	}
	if _, _, err := Parse(strings.NewReader(`{"version":1,"width":80,"height":24}` + "\n")); err == nil {
		t.Error("v1 accepted")
	}
	if _, _, err := Parse(strings.NewReader(`{"version":2,"width":80,"height":24}` + "\n[1,\"o\"]\n")); err == nil {
		t.Error("two-field event accepted")
	}
}
