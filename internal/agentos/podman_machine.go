package agentos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/lockfile"
)

// errPodmanUnreachable is the one answer when bashy's podman cannot be made
// reachable here.
var errPodmanUnreachable = errors.New("bashy's podman is not running (create its VM once: bashy podman machine init && bashy podman machine start)")

var podmanMachineReady struct {
	sync.Mutex
	ok bool
}

// podmanEnsuringEnv marks the podman children ensurePodmanMachine runs, so
// their own front door does not try to ensure the machine again.
const podmanEnsuringEnv = "BASHY_PODMAN_ENSURING"

// podmanMachineLockWait bounds how long a caller waits for another process's
// first-time init (an image download plus VM boot).
const podmanMachineLockWait = 20 * time.Minute

// ensurePodmanMachineHook is the seam callers use; tests replace it, since
// their self path is the test binary.
var ensurePodmanMachineHook = ensurePodmanMachine

// ensurePodmanMachine makes bashy's podman reachable before a caller needs it
// (`bashy oci run`, a ~~~dockerfile/~~~k8s text fence, @contain's image
// provider). Linux runs podman natively. Elsewhere podman lives in a VM that
// may not exist yet (a fresh host), that a fresh ssh session finds stopped
// (Windows), or that reports running while its socket refuses the handshake
// (macOS) until it is restarted. Success is remembered for the process; a
// failure is retried on the next call. Concurrent first calls from several
// processes are serialised by a lock file, so only one of them inits.
func ensurePodmanMachine(self string) error {
	if runtime.GOOS == "linux" {
		return nil
	}
	podmanMachineReady.Lock()
	defer podmanMachineReady.Unlock()
	if podmanMachineReady.ok {
		return nil
	}
	if p := podmanMachineLockPath(); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err == nil {
			host, _ := os.Hostname()
			l, err := lockfile.AcquireWithin(p, podmanMachineLockWait, lockfile.Holder{Name: host, PID: os.Getpid(), Intent: "ensure bashy podman machine", Since: time.Now()})
			if err != nil {
				return fmt.Errorf("%w (waiting for %s: %v)", errPodmanUnreachable, p, err)
			}
			defer l.Release()
		}
	}
	run := func(args ...string) error {
		cmd := exec.Command(self, append([]string{"podman"}, args...)...)
		cmd.Env = append(os.Environ(), podmanEnsuringEnv+"=1")
		if runtime.GOOS == "windows" && os.Getenv("CONTAINERS_MACHINE_PROVIDER") == "" {
			cmd.Env = append(cmd.Env, "CONTAINERS_MACHINE_PROVIDER=wsl")
		}
		if len(args) > 0 && args[0] == "machine" {
			// init downloads an image and boots a VM: show its progress.
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		}
		return cmd.Run()
	}
	if err := ensurePodmanMachineWith(runtime.GOOS, run); err != nil {
		return err
	}
	podmanMachineReady.ok = true
	return nil
}

// podmanMachineLockPath is the cross-process lock for ensurePodmanMachine:
// $BASHY_HOME/locks/podman-machine.lock, else ~/.bashy/locks/….
func podmanMachineLockPath() string {
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(h) == "" {
			return ""
		}
		home = filepath.Join(h, ".bashy")
	}
	return filepath.Join(home, "locks", "podman-machine.lock")
}

// ensurePodmanMachineWith is ensurePodmanMachine over an injected runner:
// probe; start and probe; then, when no machine exists, init + start and
// probe, else stop + start and probe once more.
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
	if run("machine", "inspect") != nil {
		// No machine yet (a fresh host): create bashy's own, then boot it.
		if err := run("machine", "init"); err != nil {
			return errPodmanUnreachable
		}
		_ = run("machine", "start")
		if run("info") == nil {
			return nil
		}
		return errPodmanUnreachable
	}
	_ = run("machine", "stop")
	_ = run("machine", "start")
	if run("info") == nil {
		return nil
	}
	return errPodmanUnreachable
}

// podmanEngineSubcommands are the podman subcommands that talk to the engine
// and so need its machine running. `machine`, `info`, `version`, `help`,
// `system connection` and the like are deliberately absent: they manage or
// probe the machine itself.
var podmanEngineSubcommands = map[string]bool{
	"run": true, "create": true, "start": true, "stop": true, "restart": true,
	"pull": true, "push": true, "exec": true, "ps": true, "images": true,
	"image": true, "container": true, "build": true, "buildx": true, "logs": true,
	"rm": true, "rmi": true, "inspect": true, "kill": true, "cp": true,
	"volume": true, "network": true, "pod": true, "tag": true, "load": true,
	"save": true, "import": true, "export": true, "commit": true, "attach": true,
	"wait": true, "top": true, "stats": true, "port": true, "pause": true,
	"unpause": true, "kube": true, "play": true, "compose": true, "search": true,
	"history": true, "diff": true, "mount": true, "unmount": true, "rename": true,
	"prune": true, "events": true, "manifest": true, "secret": true,
}

// podmanNeedsMachine reports whether a podman front-door invocation must find
// bashy's machine running first: a non-Linux host, an engine subcommand, the
// default bashy target, and no explicit host engine selected.
func podmanNeedsMachine(goos string, args []string, getenv func(string) string) bool {
	if goos == "linux" || getenv(podmanEnsuringEnv) != "" || strings.TrimSpace(getenv("CONTAINER_HOST")) != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(getenv("BASHY_PODMAN_SYSTEM"))) {
	case "1", "true", "yes", "on":
		return false
	}
	if podmanMachineName(args) != "bashy" {
		return false
	}
	sub := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-h" || a == "--help" {
			return false
		}
		if sub != "" {
			continue
		}
		if !strings.HasPrefix(a, "-") {
			sub = a
			continue
		}
		name, _, hasVal := strings.Cut(a, "=")
		if (podmanGlobalValueFlags[name] || name == "--connection" || name == "-c" || name == "--machine") && !hasVal {
			i++
		}
	}
	return podmanEngineSubcommands[sub]
}

// ensureEngineReady is the engine front door's pre-flight: `bashy oci run …`
// (and its sandbox/docker/podman spellings) makes bashy's podman machine
// reachable — creating it on a fresh host — before passing through. A machine
// that cannot be made reachable fails closed with 125, never running anything.
func ensureEngineReady(name string, args []string) int {
	if name != "podman" || !podmanNeedsMachine(runtime.GOOS, args, os.Getenv) {
		return 0
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
		return 125
	}
	if err := ensurePodmanMachineHook(self); err != nil {
		fmt.Fprintf(os.Stderr, "bashy podman: %v\n", err)
		return 125
	}
	return 0
}
