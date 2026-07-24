// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Command installbashy installs a freshly built bash/bashy pair without leaving
// an older bashy earlier on PATH. It exists because `go install` always writes
// GOBIN even when `command -v bashy` resolves somewhere else; that split let a
// stale release silently shadow a current developer build.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var requiredAgentOSVerbs = []string{
	"agents",
	"commands",
	"dag",
	"invoke",
	"judge",
	"kb",
	"meet",
	"models",
	"skills",
	"tools",
	"weave",
}

type commandRunner func(exe string, args ...string) ([]byte, error)

func main() {
	var bashSource, bashySource, installDir string
	flag.StringVar(&bashSource, "bash", filepath.Join("bin", executableName("bash")), "freshly built bash binary")
	flag.StringVar(&bashySource, "bashy", filepath.Join("bin", executableName("bashy")), "freshly built bashy binary")
	flag.StringVar(&installDir, "dir", defaultInstallDir(), "installation directory (default: $DHNT_BIN_DIR or ~/.local/bin)")
	flag.Parse()

	dir, err := filepath.Abs(installDir)
	if err != nil {
		fatal(err)
	}
	bashyTarget := filepath.Join(dir, executableName("bashy"))
	bashTarget := filepath.Join(dir, executableName("bash"))

	if err := verifyBashySurface(bashySource, runCommand); err != nil {
		fatal(fmt.Errorf("refusing to install incomplete bashy: %w", err))
	}
	if err := installExecutable(bashSource, bashTarget); err != nil {
		fatal(fmt.Errorf("install bash: %w", err))
	}
	if err := installExecutable(bashySource, bashyTarget); err != nil {
		fatal(fmt.Errorf("install bashy: %w", err))
	}
	if err := verifyBashySurface(bashyTarget, runCommand); err != nil {
		fatal(fmt.Errorf("installed bashy failed verification: %w", err))
	}

	fmt.Printf("installed %s and %s (dhnt user bin)\n", bashTarget, bashyTarget)
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func defaultInstallDir() string {
	if dir := strings.TrimSpace(os.Getenv("DHNT_BIN_DIR")); dir != "" {
		return dir
	}
	// Compatibility for callers that already supplied the bashy-specific name.
	if dir := strings.TrimSpace(os.Getenv("BASHY_INSTALL_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

func runCommand(exe string, args ...string) ([]byte, error) {
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "BASHY_AGENTIC=1")
	// Keep stderr separate: telemetry diagnostics are legitimate stderr and
	// must not corrupt JSON emitted on stdout by `commands --json`.
	return cmd.Output()
}

func verifyBashySurface(exe string, run commandRunner) error {
	out, err := run(exe, "-c", "-l", "echo ok")
	if err != nil || !strings.Contains(string(out), "ok") {
		return fmt.Errorf("-c -l shell probe failed: %v: %s", err, strings.TrimSpace(string(out)))
	}

	out, err = run(exe, "commands", "--json")
	if err != nil {
		return fmt.Errorf("commands probe failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	var surface struct {
		Verbs []string `json:"verbs"`
	}
	if err := json.Unmarshal(out, &surface); err != nil {
		return fmt.Errorf("decode commands surface: %w", err)
	}
	have := make(map[string]bool, len(surface.Verbs))
	for _, verb := range surface.Verbs {
		have[verb] = true
	}
	var missing []string
	for _, verb := range requiredAgentOSVerbs {
		if !have[verb] {
			missing = append(missing, verb)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("AgentOS command surface is missing: %s", strings.Join(missing, ", "))
	}

	for _, verb := range []string{"judge", "agents"} {
		out, err = run(exe, verb, "--help")
		if err != nil {
			return fmt.Errorf("%s dispatch probe failed: %v: %s", verb, err, strings.TrimSpace(string(out)))
		}
		if strings.Contains(strings.ToLower(string(out)), "command not found") {
			return fmt.Errorf("%s fell through to the shell: %s", verb, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func installExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	// Windows cannot rename over an existing file. This path is used only when
	// the destination isn't the currently running executable; Windows locks a
	// running image and will correctly reject its replacement.
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, dst)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "installbashy:", err)
	os.Exit(1)
}
