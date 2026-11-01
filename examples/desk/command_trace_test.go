package desk

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

var deskTraceStart = time.Now()
var deskTraceMu sync.Mutex

// Write directly to the package output: testing.T.Log buffers successful
// subtest output and a package-wide timeout can lose the unfinished stage.
func deskStageLog(t *testing.T, format string, args ...any) {
	t.Helper()
	deskTraceMu.Lock()
	defer deskTraceMu.Unlock()
	fmt.Fprintf(os.Stderr, "desk-stage test=%q package_elapsed=%s %s\n", t.Name(), time.Since(deskTraceStart).Round(time.Millisecond), fmt.Sprintf(format, args...))
}

type deskCommandOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *deskCommandOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *deskCommandOutput) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// traceDeskCommand observes the existing command without adding a timeout,
// retry, replacement executable or changed arguments. Return all output to the
// original assertions; only diagnostic tails are bounded.
func traceDeskCommand(cmd *exec.Cmd, interval time.Duration, logf func(string, ...any)) ([]byte, error) {
	started := time.Now()
	label := strings.Join(cmd.Args, " ")
	var output deskCommandOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	logf("start command=%q", label)
	if err := cmd.Start(); err != nil {
		logf("end command=%q elapsed=%s error=%v", label, time.Since(started).Round(time.Millisecond), err)
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	report := func(stage string, err error) []byte {
		out := output.snapshot()
		tail := out
		if len(tail) > 4096 {
			tail = tail[len(tail)-4096:]
		}
		logf("%s command=%q pid=%d elapsed=%s error=%v output_bytes=%d output_tail=%q", stage, label, cmd.Process.Pid, time.Since(started).Round(time.Millisecond), err, len(out), tail)
		return out
	}
	for {
		select {
		case err := <-done:
			return report("end", err), err
		case <-tick.C:
			report("running", nil)
		}
	}
}

func runDeskCommand(t *testing.T, cmd *exec.Cmd) ([]byte, error) {
	t.Helper()
	return traceDeskCommand(cmd, 30*time.Second, func(format string, args ...any) { deskStageLog(t, format, args...) })
}
