//go:build !windows

package agentos

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/yoke/external/meshagent"
)

// Drive the production process boundary with a fake outpost. Checking the
// child's descriptors catches wrappers that accidentally replace a terminal
// with buffered pipes. No SSH server, credentials, or network are involved.
func TestConnectTerminalAndExitPassthrough(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "outpost")
	const script = `#!/bin/sh
[ "$#" = 3 ] && [ "$1" = ssh ] && [ "$2" = -- ] && [ "$3" = elephant ] || exit 90
[ -t 0 ] && [ -t 1 ] && [ -t 2 ] || exit 91
IFS= read -r line
[ "$line" = ping ] || exit 92
printf 'remote stdout\n'
printf 'remote stderr\n' >&2
exit 37
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OUTPOST_BIN", fake)
	if got, ok := meshagent.Resolve(); !ok || got != fake {
		t.Fatalf("fake transport resolution = %q, %v", got, ok)
	}
	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminal.Close()
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = terminal, terminal, terminal
	defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr }()
	if _, err := master.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	if code := dispatchConnect([]string{"elephant"}); code != 37 {
		t.Fatalf("exit = %d, want remote status 37", code)
	}
	// Drain before closing the slave: Darwin may discard unread output on
	// close. Bound the read so a missing output marker cannot hang the gate.
	output := make(chan string, 1)
	go func() {
		var out strings.Builder
		reader := bufio.NewReader(master)
		for {
			line, err := reader.ReadString('\n')
			out.WriteString(line)
			if err != nil || strings.Contains(line, "remote stderr") {
				output <- out.String()
				return
			}
		}
	}()
	var out string
	select {
	case out = <-output:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading fake transport output")
	}
	for _, want := range []string{"remote stdout", "remote stderr"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in terminal output %q", want, out)
		}
	}
}
