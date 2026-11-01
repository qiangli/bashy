package desk

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDeskTraceChild(t *testing.T) {
	mode := os.Getenv("DESK_TRACE_CHILD")
	if mode == "" {
		return
	}
	fmt.Print("stdout:")
	fmt.Fprint(os.Stderr, "stderr:")
	if mode == "wait" {
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Print(strings.Repeat("x", 5000))
	if mode == "fail" {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestDeskCommandTrace(t *testing.T) {
	for _, mode := range []string{"wait", "fail"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDeskTraceChild$")
			cmd.Env = append(os.Environ(), "DESK_TRACE_CHILD="+mode)
			var logs []string
			out, err := traceDeskCommand(cmd, 10*time.Millisecond, func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) })
			want := "stdout:stderr:" + strings.Repeat("x", 5000)
			if string(out) != want {
				t.Fatalf("output changed: got %d bytes, want %d", len(out), len(want))
			}
			if mode == "fail" {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 7 {
					t.Fatalf("exit status changed: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			all := strings.Join(logs, "\n")
			for _, part := range []string{"start command=", "end command=", "elapsed=", "output_bytes=5014", "output_tail="} {
				if !strings.Contains(all, part) {
					t.Errorf("missing %q in trace", part)
				}
			}
			if mode == "wait" && !strings.Contains(all, "running command=") {
				t.Fatal("missing pending-stage output")
			}
			if strings.Contains(logs[len(logs)-1], want) {
				t.Fatal("diagnostic output was not bounded")
			}
		})
	}
}
