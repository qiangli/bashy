//go:build linux

package agentos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// On Linux the child runs in fresh user + network namespaces: it sees only an
// unconfigured loopback, so it cannot reach any network. Rootless — no setuid
// helper — and pure Go.

func containSupported() error { return nil }

func containCmd(argv []string) *exec.Cmd {
	cmd := exec.Command(argv[0], argv[1:]...)
	uid, gid := os.Getuid(), os.Getgid()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
		GidMappingsEnableSetgroups: false,
	}
	return cmd
}

func runContained(argv []string) int {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %s: %v\n", argv[0], err)
		return 127
	}
	cmd := containCmd(append([]string{path}, argv[1:]...))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		// user namespaces disabled (sysctl, AppArmor): fail closed.
		fmt.Fprintf(os.Stderr, "bashy contain: cannot create an isolated network namespace: %v\n", err)
		return containUnsupportedStatus
	}
	return 0
}
