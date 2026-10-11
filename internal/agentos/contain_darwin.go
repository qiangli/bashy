//go:build darwin

package agentos

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// On macOS the child runs under a Seatbelt profile that denies every network
// operation except local unix-domain sockets (which never leave the host),
// and, only for net "model", the host-local model door and the one loopback
// port of the model-egress proxy (contain_egress.go). Seatbelt cannot name a remote host, so provider
// hosts are never opened in the profile itself.

const seatbeltExec = "/usr/bin/sandbox-exec"

const seatbeltDenyNetwork = `(version 1)
(allow default)
(deny network*)
(allow network* (local unix-socket))
(allow network* (remote unix-socket))`

// seatbeltProfile is seatbeltDenyNetwork plus direct loopback TCP to ports.
func seatbeltProfile(loopbackPorts []int) string {
	var b strings.Builder
	b.WriteString(seatbeltDenyNetwork)
	for _, p := range loopbackPorts {
		fmt.Fprintf(&b, "\n(allow network-outbound (remote ip \"localhost:%d\"))", p)
	}
	return b.String()
}

func nativeContainSupported() error {
	if _, err := os.Stat(seatbeltExec); err != nil {
		return fmt.Errorf("network isolation needs %s: %v", seatbeltExec, err)
	}
	return nil
}

const nativeModelEgress = true

func runNativeContained(argv []string, model bool) int {
	var egress containEgress
	if model {
		egress = loadContainEgress()
	}
	ports := slices.Clone(egress.LoopbackPorts)
	var env []string
	if len(egress.Remote) > 0 {
		proxy, err := startContainProxy(egress.Remote)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy contain: model egress proxy: %v\n", err)
			return containUnsupportedStatus
		}
		defer proxy.close()
		_, portText, _ := net.SplitHostPort(proxy.addr())
		port, _ := strconv.Atoi(portText)
		ports = append(ports, port)
		env = containProxyEnv(proxy.addr())
	}
	cmd := exec.Command(seatbeltExec, append([]string{"-p", seatbeltProfile(ports), "--"}, argv...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
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
