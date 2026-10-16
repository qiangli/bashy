// Command pause is a pure-Go port of Kubernetes' build/pause/linux/pause.c
// (Apache-2.0), the process that holds a pod sandbox's namespaces open.
// Behaviour is the C program's, line for line:
//
//   - any argument equal (case-insensitively) to -v prints the version, exit 0;
//   - a warning on stderr when not PID 1 (not an error: pause sees use outside
//     infra containers);
//   - SIGINT or SIGTERM: "Shutting down, got signal: <name>" on stderr, exit 0;
//   - SIGCHLD: reap every exited child without blocking (PID 1 in a pod with
//     a shared PID namespace inherits orphans);
//   - otherwise sleep forever.
//
// bashy-build/build.yml refuses to build when the pinned tag's pause.c is not
// the file this port was reviewed against (PAUSE_C_SHA256 in the PIN).
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// version is stamped by the build: v<PAUSE_VERSION>-<kubernetes tag>-bashy.
var version = "HEAD"

func main() {
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "-v") {
			fmt.Printf("pause.go %s\n", version)
			os.Exit(0)
		}
	}
	if os.Getpid() != 1 {
		fmt.Fprintln(os.Stderr, "Warning: pause should be the first process")
	}
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGCHLD)
	for s := range sigs {
		if s == syscall.SIGCHLD {
			reap()
			continue
		}
		fmt.Fprintf(os.Stderr, "Shutting down, got signal: %s\n", strings.ToUpper(s.String()[:1])+s.String()[1:])
		os.Exit(0)
	}
}

// reap is pause.c's sigreap: waitpid(-1, NULL, WNOHANG) until none is left.
func reap() {
	for {
		pid, err := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
		if pid <= 0 || err != nil {
			return
		}
	}
}
