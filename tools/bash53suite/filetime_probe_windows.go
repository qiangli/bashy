//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runFiletimeProbe keeps the platform measurement in the same prepared
// context as the Windows fixture runner. In particular, /tmp is the runner's
// private TEMP and BASHY_ROOT/PATH point at bashy's yoke userland.
func runFiletimeProbe(root, testsDir, bashPath string, stdout, stderr io.Writer) error {
	if strings.TrimSpace(os.Getenv("BASHY_ROOT")) == "" {
		return fmt.Errorf("filetime probe requires the prepared Bashy root; pass -userland")
	}
	probePath := filepath.Join(os.TempDir(), "test.newer")
	_ = os.Remove(probePath)
	snapshot := filepath.Join(root, "scripts", "windows-filetime-snapshot.ps1")
	if _, err := os.Stat(snapshot); err != nil {
		return fmt.Errorf("filetime snapshot helper: %v", err)
	}

	fmt.Fprintf(stdout, "filetime-probe: fixture root=%s\n", os.Getenv("BASHY_ROOT"))
	fmt.Fprintf(stdout, "filetime-probe: native path=%s\n", probePath)
	fmt.Fprintln(stdout, "filetime-probe: before state=absent (no content read)")
	if err := runFiletimeSnapshot(snapshot, probePath, "before", stdout); err != nil {
		// The pre-sequence state is intentionally absent; absence is the only
		// expected result before test.tests creates /tmp/test.newer.
		fmt.Fprintf(stdout, "filetime-probe: before native snapshot skipped: %v\n", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bashPath, "-c", `t() { test "$@"; }; touch /tmp/test.newer ; sleep 1; echo "hello" > /tmp/test.newer; echo 't -N /tmp/test.newer'; t -N /tmp/test.newer`)
	cmd.Dir = testsDir
	cmd.Env = fixtureEnv(root, testsDir, bashPath, "test")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("test.tests:185-187 sequence failed: %v\n%s", err, output.String())
	}
	fmt.Fprintln(stdout, "filetime-probe: sequence=touch /tmp/test.newer ; sleep 1; echo \\\"hello\\\" > /tmp/test.newer; echo 't -N /tmp/test.newer'; t -N /tmp/test.newer")
	if output.Len() != 0 {
		fmt.Fprintf(stdout, "filetime-probe: sequence output=%s", output.String())
	}
	if err := runFiletimeSnapshot(snapshot, probePath, "after", stdout); err != nil {
		return err
	}
	return nil
}

func runFiletimeSnapshot(script, path, phase string, stdout io.Writer) error {
	cmd := exec.Command("pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-File", script, path, phase)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	return cmd.Run()
}
