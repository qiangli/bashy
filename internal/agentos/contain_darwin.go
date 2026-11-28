//go:build darwin

package agentos

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
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

// confineBackend: Seatbelt natively (builtin/native), or bashy's image
// through the podman machine when the image provider is asked for.
func confineBackend(provider string) (string, error) {
	switch resolveContainProvider(provider) {
	case containNative:
		if err := nativeContainSupported(); err != nil {
			return "", fmt.Errorf("file confinement (--dir): %v", err)
		}
		return confineSeatbelt, nil
	case containImageP:
		return confineImage, nil
	}
	return "", errors.New("file confinement (--dir) has no custom provider: the wrapper's confinement cannot be verified")
}

// containAccountHome is the account's home from the OS user record (never
// $HOME, which the caller controls); a seam for tests.
var containAccountHome = func() (string, error) {
	u, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" || !filepath.IsAbs(u.HomeDir) {
		return "", fmt.Errorf("user record of uid %d has no absolute home", os.Getuid())
	}
	return u.HomeDir, nil
}

// seatbeltDeveloperDir is the selected developer directory (xcode-select's
// link target) when it is outside the system roots: /usr/bin/git and the
// other shims exec from it.
func seatbeltDeveloperDir() []string {
	t, err := os.Readlink("/private/var/db/xcode_select_link")
	if err != nil || !filepath.IsAbs(t) {
		return nil
	}
	if r, err := filepath.EvalSymlinks(t); err == nil && sbplSafePath(r) == nil && r != "/" {
		return []string{r}
	}
	return nil
}

// seatbeltConfinedHost resolves the host side of the profile: the account
// home from the user record (fail closed without one), $HOME, the user temp
// root and a fresh private temp inside it (the caller removes it).
func seatbeltConfinedHost() (seatbeltHost, error) {
	home, err := containAccountHome()
	if err != nil {
		return seatbeltHost{}, fmt.Errorf("account home from the user record: %v", err)
	}
	userTemp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return seatbeltHost{}, fmt.Errorf("temp root: %v", err)
	}
	tmp, err := os.MkdirTemp(userTemp, "bashy-confine-")
	if err != nil {
		return seatbeltHost{}, fmt.Errorf("private temp: %v", err)
	}
	return seatbeltHost{
		Tmp:    tmp,
		Deny:   containConfineDenyRoots(home, os.Getenv("HOME"), userTemp),
		Self:   containConfineSelf(),
		System: seatbeltDeveloperDir(),
	}, nil
}

// runSeatbeltConfined runs argv under seatbeltConfinedProfile in c.Dir with
// the scrubbed environment and a private temp removed afterwards.
func runSeatbeltConfined(c *containConfine, argv []string) int {
	host, err := seatbeltConfinedHost()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v; nothing ran\n", err)
		return containUnsupportedStatus
	}
	defer os.RemoveAll(host.Tmp)
	env := confinedEnv(os.Environ(), c.Env, host.Tmp)
	return runContainChild(seatbeltExec, append([]string{"-p", seatbeltConfinedProfile(c, host), "--"}, argv...), c.Dir, env)
}
