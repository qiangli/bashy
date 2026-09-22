//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runLocaleProbe is deliberately independent of fixture execution. It records
// the host service before and after the prepared root replaces PATH, which
// distinguishes a missing host locale from provider/env propagation issues.
func runLocaleProbe(stdout, _ io.Writer) error {
	host := strings.TrimSpace(os.Getenv("BASHY_HOST_LOCALE"))
	root := strings.TrimSpace(os.Getenv("BASHY_ROOT"))
	if host == "" {
		return fmt.Errorf("locale probe requires BASHY_HOST_LOCALE; no direct host service was discovered")
	}
	if root == "" {
		return fmt.Errorf("locale probe requires the prepared Bashy root; pass -userland")
	}
	yoke := filepath.Join(root, "usr", "bin", "locale.exe")

	fmt.Fprintf(stdout, "locale-probe: host=%s\n", host)
	fmt.Fprintf(stdout, "locale-probe: prepared-root=%s\n", yoke)
	fmt.Fprintln(stdout, "locale-probe: direct host service")
	probeLocale(stdout, host, nil, "locale", "-a")
	for _, name := range corpusLocaleNames {
		env := localeProbeEnv(name)
		probeLocale(stdout, host, env, "LC_ALL="+name, "locale")
		probeLocale(stdout, host, env, "LC_ALL="+name, "locale", "charmap")
		for _, category := range localeProbeCategories {
			probeLocale(stdout, host, env, "LC_ALL="+name, "locale", "-k", category)
		}
	}

	fmt.Fprintln(stdout, "locale-probe: prepared-root yoke service")
	probeLocale(stdout, yoke, nil, "yoke locale -a")
	for _, name := range corpusLocaleNames {
		env := localeProbeEnv(name)
		for _, category := range localeProbeCategories {
			probeLocale(stdout, yoke, env, "yoke LC_ALL="+name+" locale -k "+category, "locale", "-k", category)
		}
	}
	return nil
}

var localeProbeCategories = []string{
	"LC_CTYPE", "LC_NUMERIC", "LC_TIME", "LC_COLLATE", "LC_MONETARY", "LC_MESSAGES",
}

func localeProbeEnv(locale string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "LC_ALL=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "LC_ALL="+locale)
}

func probeLocale(stdout io.Writer, path string, env []string, label string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	if env != nil {
		cmd.Env = env
	}
	out, errOut := new(strings.Builder), new(strings.Builder)
	cmd.Stdout = out
	cmd.Stderr = errOut
	err := cmd.Run()
	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	fmt.Fprintf(stdout, "locale-probe: command=%s exit=%d\n", label, exit)
	if out.Len() != 0 {
		fmt.Fprintf(stdout, "locale-probe: stdout=%s", out.String())
		if !strings.HasSuffix(out.String(), "\n") {
			fmt.Fprintln(stdout)
		}
	}
	if errOut.Len() != 0 {
		fmt.Fprintf(stdout, "locale-probe: stderr=%s", errOut.String())
		if !strings.HasSuffix(errOut.String(), "\n") {
			fmt.Fprintln(stdout)
		}
	}
	if err != nil && errOut.Len() == 0 {
		fmt.Fprintf(stdout, "locale-probe: error=%v\n", err)
	}
}
