//go:build linux

package agentos

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// On Linux the child runs in fresh user + network namespaces: it sees only an
// unconfigured loopback, so it cannot reach any network. Rootless — no setuid
// helper — and pure Go.

func nativeContainSupported() error { return nil }

const nativeModelEgress = false

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

func runNativeContained(argv []string, _ bool) int {
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

// confineBackend: bashy's image (network none, bind mounts). The native
// namespace provider confines the network only, so it is refused for --dir.
func confineBackend(provider string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_CONTAIN_PROVIDER")))
	}
	switch p {
	case "", containBuiltin, containImageP:
		return confineImage, nil
	}
	return "", fmt.Errorf("file confinement (--dir) runs in bashy's image on linux; provider %q confines the network only", p)
}

func runSeatbeltConfined(*containConfine, []string) int { return containUnsupportedStatus }
