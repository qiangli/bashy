// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// `bashy doctor` is an environment self-diagnostic: it turns the footguns this
// shell documents (a wrapper shim shadowing `sh`, a stale `bashy` first on PATH,
// missing toolchain/engine, agent-mode state, bin cache) into a one-shot health
// table. Read-only and advisory; exits non-zero only when a check is a hard
// problem. Bashy-only (never the pure cmd/bash drop-in).
package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/binmgr"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

const doctorSchemaVersion = "bashy-doctor-v1"

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | info
	Detail string `json:"detail"`
}

func dispatchDoctor(args []string) int {
	asJSON := weavecli.IsAgent()
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "-h", "--help":
			fmt.Println("usage: doctor [--json]")
			fmt.Println("Diagnose the bashy environment: PATH/sh shadowing, stale binary,")
			fmt.Println("toolchain + container engine, agent mode, and the bin cache.")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "doctor: unknown option %q\n", a)
			return 2
		}
	}

	checks := collectDoctorChecks()

	warns := countDoctorWarnings(checks)

	if asJSON {
		b, _ := json.Marshal(map[string]any{
			"schema_version": doctorSchemaVersion,
			"checks":         checks,
			"warnings":       warns,
		})
		fmt.Println(string(b))
		return 0
	}

	printDoctorChecks(os.Stdout, checks, "bashy doctor")
	return 0
}

func collectDoctorChecks() []doctorCheck {
	checks := collectSelfChecks()
	add := func(name, status, detail string) {
		checks = append(checks, doctorCheck{name, status, detail})
	}

	// `bashy` on PATH — a different one first means the bare verb shims +
	// `command bashy` resolve to a stale build (a footgun we hit in practice).
	exe, _ := os.Executable()
	if lp, err := exec.LookPath("bashy"); err == nil && exe != "" && lp != exe {
		add("PATH: bashy", "warn", fmt.Sprintf("a different bashy is first on PATH (%s); this process is %s — bare verb shims will use the PATH one", lp, exe))
	} else if err == nil {
		add("PATH: bashy", "ok", lp)
	} else {
		add("PATH: bashy", "info", "not on PATH (run by absolute path)")
	}

	// `sh` on PATH — a wrapper shim shadowing it breaks `make test-bash` and
	// forked-shell tests; the documented fix is a clean PATH.
	if lp, err := exec.LookPath("sh"); err == nil {
		if strings.Contains(lp, "wrap") || strings.Contains(lp, "shim") || (lp != "/bin/sh" && lp != "/usr/bin/sh") {
			add("PATH: sh", "warn", fmt.Sprintf("sh resolves to %s — may be a wrapper shim; use a clean PATH (/bin:/usr/bin) for forked-shell work", lp))
		} else {
			add("PATH: sh", "ok", lp)
		}
	} else {
		add("PATH: sh", "warn", "no sh on PATH")
	}

	// Host Go is optional now that `bashy go` exists; still useful to report
	// because tests may accidentally pick it up.
	if lp, err := exec.LookPath("go"); err == nil {
		add("host go", "info", lp)
	} else {
		add("host go", "info", "no host go on PATH (use `bashy go`)")
	}

	if weavecli.IsAgent() {
		add("agent mode", "info", "ON (BASHY_AGENTIC truthy → JSON defaults)")
	} else {
		add("agent mode", "info", "off (set BASHY_AGENTIC=1 for JSON defaults)")
	}

	// Container engine reachability (best-effort, advisory).
	switch {
	case os.Getenv("CONTAINER_HOST") != "" || os.Getenv("DOCKER_HOST") != "":
		add("container engine", "info", "a CONTAINER_HOST/DOCKER_HOST is set")
	default:
		if _, err := exec.LookPath("podman"); err == nil {
			add("container engine", "info", "podman on PATH")
		} else if _, err := exec.LookPath("docker"); err == nil {
			add("container engine", "info", "docker on PATH")
		} else {
			add("container engine", "info", "none detected (bashy podman needs an engine build / host)")
		}
	}
	return checks
}

func collectSelfChecks() []doctorCheck {
	var checks []doctorCheck
	add := func(name, status, detail string) {
		checks = append(checks, doctorCheck{name, status, detail})
	}

	exe, _ := os.Executable()
	if exe == "" {
		add("bashy binary", "warn", "cannot resolve current executable")
	} else {
		add("bashy binary", "info", fmt.Sprintf("%s (%s/%s, %s)", exe, runtime.GOOS, runtime.GOARCH, runtime.Version()))
	}
	add("embedded git", "ok", "bashy git is compiled in and never falls back to system git")
	add("dag runner", "ok", "bashy dag is compiled in")
	add("managed go", "ok", "bashy go is compiled in; host go is optional")
	add("release target", "ok", fmt.Sprintf("%s/%s member %s", runtime.GOOS, runtime.GOARCH, releaseBinaryName()))

	if dir, err := binmgr.CacheDir(); err == nil {
		st := "ok"
		detail := dir
		if _, serr := os.Stat(dir); serr != nil {
			st, detail = "info", dir+" (not yet created)"
		}
		add("bin cache", st, detail)
	}

	if root, ok := findBashySourceRoot("."); ok {
		add("source tree", "ok", root)
		addSiblingLayoutChecks(&checks, root)
	} else {
		add("source tree", "info", "not running inside a bashy source checkout")
	}
	return checks
}

func addSiblingLayoutChecks(checks *[]doctorCheck, root string) {
	add := func(name, status, detail string) {
		*checks = append(*checks, doctorCheck{name, status, detail})
	}
	pinsPath := filepath.Join(root, ".sibling-pins")
	data, err := os.ReadFile(pinsPath)
	if err != nil {
		add("sibling pins", "warn", ".sibling-pins missing; standalone self-build cannot pin sibling repos")
		return
	}
	pins := parseSiblingPins(string(data))
	if len(pins) == 0 {
		add("sibling pins", "warn", ".sibling-pins has no active pins")
		return
	}
	names := make([]string, 0, len(pins))
	missing := make([]string, 0)
	for name := range pins {
		names = append(names, name)
		if _, err := os.Stat(filepath.Join(filepath.Dir(root), name, "go.mod")); err != nil {
			missing = append(missing, name)
		}
	}
	sort.Strings(names)
	sort.Strings(missing)
	add("sibling pins", "ok", strings.Join(names, ", "))
	if len(missing) == 0 {
		add("sibling repos", "ok", "all pinned siblings exist next to bashy")
		return
	}
	add("sibling repos", "warn", "missing next to bashy: "+strings.Join(missing, ", "))
}

func findBashySourceRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/qiangli/bashy") {
			return dir, true
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", false
		}
		dir = next
	}
}

func parseSiblingPins(src string) map[string]string {
	pins := map[string]string{}
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, sha, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		sha = strings.TrimSpace(sha)
		if name != "" && sha != "" {
			pins[name] = sha
		}
	}
	return pins
}

func countDoctorWarnings(checks []doctorCheck) int {
	warns := 0
	for _, c := range checks {
		if c.Status == "warn" {
			warns++
		}
	}
	return warns
}

func printDoctorChecks(w io.Writer, checks []doctorCheck, title string) {
	mark := map[string]string{"ok": "✓", "warn": "⚠", "info": "ⓘ"}
	for _, c := range checks {
		fmt.Fprintf(w, "  %s  %-18s %s\n", mark[c.Status], c.Name, c.Detail)
	}
	warns := countDoctorWarnings(checks)
	if warns == 0 {
		fmt.Fprintf(w, "\n%s: no warnings.\n", title)
	} else {
		fmt.Fprintf(w, "\n%s: %d warning(s) above.\n", title, warns)
	}
}
