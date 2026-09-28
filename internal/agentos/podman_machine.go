package agentos

import (
	"errors"
	"os/exec"
	"runtime"
	"sync"
)

// errPodmanUnreachable is the one answer when bashy's podman cannot be made
// reachable here.
var errPodmanUnreachable = errors.New("bashy's podman is not running (create its VM once: bashy podman machine init && bashy podman machine start)")

var podmanMachineReady struct {
	sync.Mutex
	ok bool
}

// ensurePodmanMachineHook is the seam callers use; tests replace it, since
// their self path is the test binary.
var ensurePodmanMachineHook = ensurePodmanMachine

// ensurePodmanMachine makes bashy's podman reachable before a caller needs it
// (a ~~~dockerfile/~~~k8s text fence, @contain's image provider). Linux runs
// podman natively. Elsewhere podman lives in a VM that a fresh ssh session
// finds stopped (Windows), or that reports running while its socket refuses
// the handshake (macOS) until it is restarted. Success is remembered for the
// process; a failure is retried on the next call.
func ensurePodmanMachine(self string) error {
	podmanMachineReady.Lock()
	defer podmanMachineReady.Unlock()
	if podmanMachineReady.ok {
		return nil
	}
	run := func(args ...string) error {
		return exec.Command(self, append([]string{"podman"}, args...)...).Run()
	}
	if err := ensurePodmanMachineWith(runtime.GOOS, run); err != nil {
		return err
	}
	podmanMachineReady.ok = true
	return nil
}

// ensurePodmanMachineWith is ensurePodmanMachine over an injected runner:
// probe; start and probe; stop, start and probe once more.
func ensurePodmanMachineWith(goos string, run func(args ...string) error) error {
	if goos == "linux" {
		return nil
	}
	if run("info") == nil {
		return nil
	}
	// start fails on a machine that is already running; the probe decides.
	_ = run("machine", "start")
	if run("info") == nil {
		return nil
	}
	_ = run("machine", "stop")
	_ = run("machine", "start")
	if run("info") == nil {
		return nil
	}
	return errPodmanUnreachable
}
