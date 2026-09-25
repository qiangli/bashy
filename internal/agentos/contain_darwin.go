//go:build darwin

package agentos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// On macOS the child runs under a Seatbelt profile that denies every network
// operation except local unix-domain sockets (which never leave the host).

const seatbeltExec = "/usr/bin/sandbox-exec"

const seatbeltDenyNetwork = `(version 1)
(allow default)
(deny network*)
(allow network* (local unix-socket))
(allow network* (remote unix-socket))`

func containSupported() error {
	if _, err := os.Stat(seatbeltExec); err != nil {
		return fmt.Errorf("network isolation needs %s: %v", seatbeltExec, err)
	}
	return nil
}

func runContained(argv []string) int {
	cmd := exec.Command(seatbeltExec, append([]string{"-p", seatbeltDenyNetwork, "--"}, argv...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	return 0
}
