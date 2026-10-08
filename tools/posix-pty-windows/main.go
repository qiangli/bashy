//go:build windows

// Windows ConPTY runner for the interactive POSIX survey. Build from the bashy
// module with GOOS=windows go build -o posix-pty-windows.exe
// ./scripts/posix-pty-windows.go. The six scripts mirror posix-parity-pty.sh.
// Without an independent upstream Bash 5.3 oracle this records execution,
// output, and exit status only; it does not assert POSIX conformance.
package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	pty "github.com/aymanbagabas/go-pty"
)

type probe struct {
	id, name string
	lines    []string
	unsetHF  bool
}

var probes = []probe{
	{"3", "interactive alias expansion", []string{"alias hi='printf \"alias-ok\\n\"'", "hi"}, false},
	{"29", "POSIX PS1 parameter and history expansion", []string{"PVAR=VALUE", "PS1='q:${PVAR}:!!>'"}, false},
	{"46", "interactive comments remain enabled", []string{"echo before # after"}, false},
	{"30", "POSIX default HISTFILE", []string{"printf 'hf=%s\\n' \"${HISTFILE-UNSET}\""}, true},
	{"31", "POSIX ! remains literal in double quotes", []string{"echo \"x!!y\""}, false},
	{"0", "interactive POSIX option state", []string{"case $- in *i*) echo interactive;; *) echo not-interactive;; esac", "set -o | grep '^posix'"}, false},
}

var ansi = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

func clean(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r", "")
	return ansi.ReplaceAllString(s, "")
}

func run(shell string, x probe) (string, error) {
	p, err := pty.New()
	if err != nil {
		return "", fmt.Errorf("create ConPTY: %w", err)
	}
	defer p.Close()
	if err := p.Resize(100, 30); err != nil {
		return "", err
	}
	c := p.Command(shell, "--noprofile", "--norc", "--posix", "-i")
	c.Env = append([]string{}, os.Environ()...)
	c.Env = append(c.Env, "TERM=xterm", "PVAR=VALUE", "PS1=@@@PROMPT@@@ ", "BASH_SILENCE_DEPRECATION_WARNING=1")
	if x.unsetHF {
		filtered := c.Env[:0]
		for _, e := range c.Env {
			if !strings.HasPrefix(strings.ToUpper(e), "HISTFILE=") {
				filtered = append(filtered, e)
			}
		}
		c.Env = filtered
	} else {
		c.Env = append(c.Env, "HISTFILE=NUL")
	}
	if err := c.Start(); err != nil {
		return "", fmt.Errorf("start shell: %w", err)
	}
	defer func() { _ = c.Process.Kill(); _ = c.Wait() }()
	data := make(chan []byte, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, e := p.Read(buf)
			if n > 0 {
				data <- bytes.Clone(buf[:n])
			}
			if e != nil {
				close(data)
				return
			}
		}
	}()
	// Startup output is retained for diagnosis, but delimiters isolate the case.
	start := "@@@P:" + x.id + "@@@"
	end := "@@@X:" + x.id + ":"
	lines := []string{"PS1='@@@PROMPT@@@ '", "printf '" + start + "\\n'"}
	lines = append(lines, x.lines...)
	lines = append(lines, "printf '"+end+"%s@@@\\n' \"$?\"", "exit")
	if _, err := p.Write([]byte(strings.Join(lines, "\r\n") + "\r\n")); err != nil {
		return "", fmt.Errorf("write probe: %w", err)
	}
	var out bytes.Buffer
	deadline := time.NewTimer(12 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case chunk, ok := <-data:
			if !ok {
				return clean(out.Bytes()), fmt.Errorf("shell closed before end marker")
			}
			out.Write(chunk)
			plain := clean(out.Bytes())
			// The input line contains the marker text too. Require a complete
			// executed output line with a numeric status before stopping.
			for _, line := range strings.Split(plain, "\n") {
				line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "@@@PROMPT@@@"))
				if strings.HasPrefix(line, end) && strings.HasSuffix(line, "@@@") {
					status := strings.TrimSuffix(strings.TrimPrefix(line, end), "@@@")
					if status == "0" || status == "1" || status == "2" {
						return plain, nil
					}
				}
			}
		case <-deadline.C:
			return clean(out.Bytes()), fmt.Errorf("end marker timeout")
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: posix-pty-windows.exe PATH_TO_BASHY")
		os.Exit(2)
	}
	fail := 0
	for _, x := range probes {
		out, err := run(os.Args[1], x)
		fmt.Printf("EXECUTED #%s %s\n%s\n", x.id, x.name, out)
		if err != nil {
			fmt.Printf("HARNESS ERROR #%s: %v\n", x.id, err)
			fail++
		}
	}
	fmt.Printf("=== %d interactive probes attempted / %d harness errors / no oracle comparison ===\n", len(probes), fail)
	if fail != 0 {
		os.Exit(2)
	}
}
