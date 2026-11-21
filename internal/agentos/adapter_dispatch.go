// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/pkg/shape/slot"
	"github.com/qiangli/yoke/pkg/src"
)

const (
	adapterExecEnv   = "BASHY_ADAPTER_EXEC"
	adapterInprocEnv = "BASHY_ADAPTER_INPROC"
	adaptersRootEnv  = "BASHY_ADAPTERS_ROOT"
	invokedAsEnv     = "BASHY_INVOKED_AS"
)

// adaptedVerbs maps front-door verb spellings (including aliases) to their
// folder and entry directory under the bashy repository.
var adaptedVerbs = map[string][2]string{
	"stats": {"yoke", "stats"},
}

func isAdaptedVerb(name string) bool {
	_, ok := adaptedVerbs[name]
	return ok
}

// runCommandAdapter routes a front-door command through its folder-organized
// Bash# adapter (<folder>/<entry>/<entry>.bsh), passing argv, stdio, env, and
// exit status through unchanged. If an active shaped CLI slot exists for the
// command, it executes that slot directly (honouring bashy shape).
func runCommandAdapter(folder, entry, invokedAs string, args []string) int {
	if invokedAs == "" {
		invokedAs = entry
	}
	if !certProfile() {
		for _, slotName := range uniqueStrings(invokedAs, entry) {
			if active, ok, err := shapeSlotSelect(slotName); err == nil && ok && active.Path != "" {
				env, envErr := slot.EntryEnvironment(active.Path, appendAdapterEnv(os.Environ(), invokedAs, false))
				if envErr == nil {
					return shapeSlotLaunch(active.Path, append([]string{invokedAs}, args...), env)
				}
			}
		}
	}

	scriptPath := resolveAdapterScript(folder, entry)
	if scriptPath == "" {
		fmt.Fprintf(os.Stderr, "bashy %s: adapter %s/%s/%s.bsh not found\n", invokedAs, folder, entry, entry)
		return 127
	}

	self := bashySelfPath()
	cmdArgs := make([]string, 0, 2+len(args))
	cmdArgs = append(cmdArgs, "--bashsharp", scriptPath)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(self, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = appendAdapterEnv(os.Environ(), invokedAs, true)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return childExitStatus(exitErr)
		}
		fmt.Fprintf(os.Stderr, "bashy %s: %v\n", invokedAs, err)
		return 126
	}
	return 0
}

func uniqueStrings(a, b string) []string {
	if a == "" {
		return []string{b}
	}
	if b == "" || a == b {
		return []string{a}
	}
	return []string{a, b}
}

func appendAdapterEnv(base []string, invokedAs string, markAdapterExec bool) []string {
	self := bashySelfPath()
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case adapterExecEnv, invokedAsEnv, "BASHY_SELF":
			continue
		}
		out = append(out, kv)
	}
	if markAdapterExec {
		out = append(out, adapterExecEnv+"=1")
	}
	out = append(out, invokedAsEnv+"="+invokedAs)
	if self != "" {
		out = append(out, "BASHY_SELF="+self)
	}
	return out
}

func stripEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}

// resolveAdapterScript locates <folder>/<entry>/<entry>.bsh across explicit
// overrides, the fetched source pin (~/.bashy/src/bashy), the executable tree,
// the working directory, GOWORK, and compile-time source location (tests).
func resolveAdapterScript(folder, entry string) string {
	rel := filepath.Join(folder, entry, entry+".bsh")

	if override := strings.TrimSpace(os.Getenv(adaptersRootEnv)); override != "" {
		p := filepath.Join(override, rel)
		if isRegularFile(p) {
			return p
		}
		return ""
	}

	if !certProfile() {
		if home := strings.TrimSpace(src.Home()); home != "" {
			if p := filepath.Join(home, "src", "bashy", rel); isRegularFile(p) {
				return p
			}
		}
	}

	for _, root := range candidateBashyRoots() {
		if p := filepath.Join(root, rel); isRegularFile(p) {
			return p
		}
	}
	return ""
}

func candidateBashyRoots() []string {
	var seeds []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		seeds = append(seeds, filepath.Dir(exe))
		if realExe, err := filepath.EvalSymlinks(exe); err == nil && realExe != "" && realExe != exe {
			seeds = append(seeds, filepath.Dir(realExe))
		}
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		seeds = append(seeds, wd, filepath.Join(wd, "bashy"))
	}
	if pwd := strings.TrimSpace(os.Getenv("PWD")); pwd != "" {
		seeds = append(seeds, pwd, filepath.Join(pwd, "bashy"))
	}
	if gowork := strings.TrimSpace(os.Getenv("GOWORK")); gowork != "" && gowork != "off" {
		seeds = append(seeds, filepath.Join(filepath.Dir(gowork), "bashy"))
	}
	if _, callerFile, _, ok := runtime.Caller(0); ok && filepath.IsAbs(callerFile) {
		seeds = append(seeds, filepath.Dir(callerFile))
	}

	seen := map[string]bool{}
	var roots []string
	for _, s := range seeds {
		if root, ok := findBashySourceRoot(s); ok && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
