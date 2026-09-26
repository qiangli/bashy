package agentos

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
)

// forwardSignals relays the interrupt and termination signals bashy
// receives to a child process until the returned stop is called.
func forwardSignals(p *os.Process) func() {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, append([]os.Signal{os.Interrupt}, stewardTermSignals()...)...)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-signals:
				_ = p.Signal(s)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

// runSelf runs this bashy binary with args, stdio passed through and the
// interrupt/termination signals forwarded, and returns its exit status.
func runSelf(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(bashySelfPath(), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Start(); err != nil {
		return 127
	}
	stop := forwardSignals(cmd.Process)
	err := cmd.Wait()
	stop()
	if err == nil {
		return 0
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() >= 0 {
		return exit.ExitCode()
	}
	return 1
}
