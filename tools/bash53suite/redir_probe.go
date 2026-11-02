package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type redirectProbePhase struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	Expected *string  `json:"expected_combined_output,omitempty"`
}

func redirectProbePhases() []redirectProbePhase {
	authored := []struct{ name, script, want string }{
		{"integer-prefix", `declare -i target=2; target=9 printf 'marker\n' >&$(echo $target)`, "marker\n"},
		{"integer-prefix-eval", `declare -i target=2; target=9 eval printf "'marker\n'" >&$(echo $target)`, "marker\n"},
		{"integer-reset", `target=42; unset target; typeset -i target; target=4 eval echo $(echo target+=3); target=2; target=9 echo marker >&$(echo $target)`, "target+=3\nmarker\n"},
		{"capture-bytes", `declare -i target=2; target=9 printf '<%s>\n' "$(echo $target)"`, "<2>\n"},
	}
	var phases []redirectProbePhase
	for _, tc := range authored {
		want := tc.want
		phases = append(phases, redirectProbePhase{tc.name, []string{"-c", tc.script}, &want})
	}
	return append(phases,
		redirectProbePhase{"direct-subfixture", []string{"./redir11.sub"}, nil},
		redirectProbePhase{"nested-subfixture", []string{"-c", `${THIS_SH} ./redir11.sub`}, nil},
		redirectProbePhase{"trace-subfixture", []string{"-x", "./redir11.sub"}, nil},
	)
}

func probeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// runRedirectProbe is deliberately unscored. Run the original redir measurement
// separately first; these child invocations never change its fixture or verdict.
func runRedirectProbe(root, testsDir, bashPath, userland, destination string, stdout io.Writer) error {
	destination, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	// Never replace evidence from an earlier invocation.
	if err = os.Mkdir(destination, 0755); err != nil {
		return err
	}
	paths := map[string]string{
		"candidate": bashPath, "userland": userland,
		"nested_candidate": filepath.Join(os.Getenv("BASHY_ROOT"), "usr", "bin", "bash"),
		"redir.tests":      filepath.Join(testsDir, "redir.tests"),
		"redir.right":      filepath.Join(testsDir, "redir.right"),
		"redir11.sub":      filepath.Join(testsDir, "redir11.sub"),
	}
	hashes := make(map[string]string)
	for name, path := range paths {
		hashes[name], err = probeFileHash(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", name, err)
		}
	}
	if hashes["candidate"] != hashes["nested_candidate"] {
		return fmt.Errorf("nested candidate differs from measured binary")
	}
	manifest := struct {
		Scope          string               `json:"scope"`
		TimeoutSeconds int                  `json:"timeout_seconds"`
		Paths          map[string]string    `json:"paths"`
		SHA256         map[string]string    `json:"sha256"`
		Phases         []redirectProbePhase `json:"phases"`
	}{"diagnostic only; no full-suite or subfixture pass claim; combined raw stdout/stderr", 60, paths, hashes, redirectProbePhases()}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(destination, "manifest.json"), append(b, '\n'), 0644); err != nil {
		return err
	}
	var failures []error
	for _, phase := range manifest.Phases {
		// A previous phase must not seed the next with its fixture-directory writes.
		phaseTests, cleanup, err := hermeticTree(testsDir)
		if err != nil {
			return err
		}
		raw, timedOut, runErr, startErr := runFixtureProcess(root, phaseTests, bashPath, fixture{Name: "redir", Test: "redir11.sub"}, 60*time.Second, phase.Args)
		for _, name := range []string{"redir.tests", "redir.right", "redir11.sub"} {
			got, hashErr := probeFileHash(filepath.Join(phaseTests, name))
			if hashErr != nil || got != hashes[name] {
				startErr = errors.Join(startErr, fmt.Errorf("phase input changed: %s (%v)", name, hashErr))
			}
		}
		cleanup()
		if err = os.WriteFile(filepath.Join(destination, phase.Name+".raw"), raw, 0644); err != nil {
			return err
		}
		status := struct {
			ExitCode            int    `json:"exit_code"`
			TimedOut            bool   `json:"timed_out"`
			ProcessError        string `json:"process_error"`
			InfrastructureError string `json:"infrastructure_error"`
			AuthoredAssertion   *bool  `json:"authored_assertion,omitempty"`
		}{TimedOut: timedOut}
		if timedOut {
			status.ExitCode = -1
		}
		if runErr != nil {
			status.ProcessError = runErr.Error()
			status.ExitCode = -1
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				status.ExitCode = exitErr.ExitCode()
			}
		}
		if startErr != nil {
			status.InfrastructureError = startErr.Error()
			status.ExitCode = -1
		}
		if phase.Expected != nil {
			pass := !timedOut && runErr == nil && startErr == nil && string(raw) == *phase.Expected
			status.AuthoredAssertion = &pass
			if !pass {
				failures = append(failures, fmt.Errorf("authored probe %s failed", phase.Name))
			}
		}
		if timedOut || startErr != nil {
			failures = append(failures, fmt.Errorf("probe %s incomplete", phase.Name))
		}
		b, err = json.MarshalIndent(status, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(destination, phase.Name+".json"), append(b, '\n'), 0644); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "redir-probe: %s timed_out=%t process_error=%q infrastructure_error=%q\n", phase.Name, timedOut, status.ProcessError, status.InfrastructureError)
		// A diagnostic must not silently move to another binary or alter a fixture.
		for name, path := range paths {
			got, hashErr := probeFileHash(path)
			if hashErr != nil || got != hashes[name] {
				return fmt.Errorf("probe input changed: %s (%v)", name, hashErr)
			}
		}
	}
	return errors.Join(failures...)
}
