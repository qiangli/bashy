//go:build windows

package dhnt

import (
	"errors"
	"os/exec"
)

func configureRunnerProcess(command *exec.Cmd) {}

func stopRunnerProcessGroup(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}

func runnerExitStatus(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), false
	}
	return runnerLaunchExit, false
}
