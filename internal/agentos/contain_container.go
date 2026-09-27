package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a
//
// @contain's two backends. `native` is the kernel primitive where one exists
// (Linux user+network namespaces, macOS Seatbelt): fast, the host's own
// toolchain. `container` runs the child in bashy's own image — all you need
// is bashy — through bashy's podman with no network and only the working
// directory mounted, so it also keeps the child away from everything else on
// the host. Native is used where it exists, the container elsewhere
// (Windows); BASHY_CONTAIN_BACKEND=container|native chooses explicitly.
// Either way a restriction that cannot be enforced fails closed (125).

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/qiangli/bashy/internal/cli"
)

const (
	containNative    = "native"
	containContainer = "container"
)

// containBackend picks the backend: BASHY_CONTAIN_BACKEND, else native where
// the OS has it, else the container.
func containBackend() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_CONTAIN_BACKEND"))) {
	case containNative:
		return containNative
	case containContainer:
		return containContainer
	}
	if nativeContainSupported() == nil {
		return containNative
	}
	return containContainer
}

func containSupported() error {
	if containBackend() == containNative {
		return nativeContainSupported()
	}
	return nil // the container backend reports its own failures (125)
}

func runContained(argv []string) int {
	if containBackend() == containNative {
		return runNativeContained(argv)
	}
	return runContainerContained(argv)
}

// containToolchains is the toolchain a contained child needs preloaded in the
// image: the container has no network, so nothing is fetched inside it.
func containToolchains(argv []string) []string {
	switch strings.TrimSuffix(baseName(argv[0]), ".exe") {
	case "python", "python3", "pip", "pip3", "uv", "pytest":
		return []string{"python"}
	case "go", "gofmt":
		return []string{"go"}
	}
	if isBashyExecutable(argv[0]) && len(argv) > 1 {
		return containToolchains(argv[1:])
	}
	return nil
}

// containImage is the bashy image a contained child runs in:
// BASHY_CONTAIN_IMAGE, else `bashy self image`'s tag for the toolchain.
func containImage(argv []string) (string, []string) {
	with := containToolchains(argv)
	if img := strings.TrimSpace(os.Getenv("BASHY_CONTAIN_IMAGE")); img != "" {
		return img, with
	}
	return selfImageTag(firstNonEmpty(cli.BashyVersion(), "dev"), runtime.GOARCH, with), with
}

// containerCommand is the command line the image's bashy runs: the child's
// name (bashy's own path becomes plain bashy subcommands; a host path becomes
// its base name, resolved inside the image) and its arguments, quoted.
func containerCommand(argv []string) string {
	words := make([]string, 0, len(argv))
	start := 0
	if isBashyExecutable(argv[0]) {
		start = 1
		words = append(words, "bashy")
	} else {
		words = append(words, shellQuote(strings.TrimSuffix(baseName(argv[0]), ".exe")))
		start = 1
	}
	for _, a := range argv[start:] {
		words = append(words, shellQuote(a))
	}
	return strings.Join(words, " ")
}

// containerRunArgs are the `bashy podman` arguments for one contained child.
func containerRunArgs(image, workdir, command string) []string {
	return []string{"podman", "run", "--rm", "-i", "--network=none",
		"-v", workdir + ":/work", "-w", "/work",
		"-e", "BASHY_AGENTIC=1", "-e", "BASHY_HINTS=off",
		image, "-c", command}
}

func runContainerContained(argv []string) int {
	self := bashySelfPath()
	image, with := containImage(argv)
	if err := ensureContainImage(self, image, with); err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	workdir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	cmd := exec.Command(self, containerRunArgs(image, workdir, containerCommand(argv))...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// podman's own failures are 125 too: the run was refused, not run.
			return exit.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy contain: %v\n", err)
		return containUnsupportedStatus
	}
	return 0
}

// ensureContainImage builds the bashy image once when it is missing
// (`bashy self image [--with TOOLCHAIN]`); the build fetches what it needs,
// the contained child never does. A custom BASHY_CONTAIN_IMAGE is never built.
func ensureContainImage(self, image string, with []string) error {
	if exec.Command(self, "podman", "image", "exists", image).Run() == nil {
		return nil
	}
	if strings.TrimSpace(os.Getenv("BASHY_CONTAIN_IMAGE")) != "" {
		return fmt.Errorf("image %s not found (BASHY_CONTAIN_IMAGE)", image)
	}
	fmt.Fprintf(os.Stderr, "bashy contain: building %s (first contained run with this toolchain)\n", image)
	args := []string{"self", "image", "--tag", image}
	if len(with) > 0 {
		args = append(args, "--with", strings.Join(with, ","))
	}
	build := exec.Command(self, args...)
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building %s: %v (bashy podman must work here; a dev build needs BASHY_SCRATCH_BIN)", image, err)
	}
	return nil
}
