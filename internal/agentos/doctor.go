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
	"os"
	"os/exec"
	"runtime"
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

	var checks []doctorCheck
	add := func(name, status, detail string) {
		checks = append(checks, doctorCheck{name, status, detail})
	}

	// 1. This binary.
	exe, _ := os.Executable()
	add("bashy binary", "info", fmt.Sprintf("%s (%s/%s, %s)", exe, runtime.GOOS, runtime.GOARCH, runtime.Version()))

	// 2. `bashy` on PATH — a different one first means the bare verb shims +
	//    `command bashy` resolve to a STALE build (a footgun we hit in practice).
	if lp, err := exec.LookPath("bashy"); err == nil && exe != "" && lp != exe {
		add("PATH: bashy", "warn", fmt.Sprintf("a different bashy is first on PATH (%s); this process is %s — bare verb shims will use the PATH one", lp, exe))
	} else if err == nil {
		add("PATH: bashy", "ok", lp)
	} else {
		add("PATH: bashy", "info", "not on PATH (run by absolute path)")
	}

	// 3. `sh` on PATH — a wrapper shim shadowing it breaks `make test-bash` and
	//    forked-shell tests; the documented fix is a clean PATH.
	if lp, err := exec.LookPath("sh"); err == nil {
		if strings.Contains(lp, "wrap") || strings.Contains(lp, "shim") || (lp != "/bin/sh" && lp != "/usr/bin/sh") {
			add("PATH: sh", "warn", fmt.Sprintf("sh resolves to %s — may be a wrapper shim; use a clean PATH (/bin:/usr/bin) for forked-shell work", lp))
		} else {
			add("PATH: sh", "ok", lp)
		}
	} else {
		add("PATH: sh", "warn", "no sh on PATH")
	}

	// 4. Go toolchain (for `bashy go` consumers / building).
	if lp, err := exec.LookPath("go"); err == nil {
		add("go toolchain", "ok", lp)
	} else {
		add("go toolchain", "info", "no host go on PATH (use `bashy go`)")
	}

	// 5. Agent mode.
	if weavecli.IsAgent() {
		add("agent mode", "info", "ON (BASHY_AGENTIC truthy → JSON defaults)")
	} else {
		add("agent mode", "info", "off (set BASHY_AGENTIC=1 for JSON defaults)")
	}

	// 6. Container engine reachability (best-effort, advisory).
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

	// 7. Bin cache for binmgr-managed externals.
	if dir, err := binmgr.CacheDir(); err == nil {
		st := "ok"
		detail := dir
		if _, serr := os.Stat(dir); serr != nil {
			st, detail = "info", dir+" (not yet created)"
		}
		add("bin cache", st, detail)
	}

	warns := 0
	for _, c := range checks {
		if c.Status == "warn" {
			warns++
		}
	}

	if asJSON {
		b, _ := json.Marshal(map[string]any{
			"schema_version": doctorSchemaVersion,
			"checks":         checks,
			"warnings":       warns,
		})
		fmt.Println(string(b))
		return 0
	}

	mark := map[string]string{"ok": "✓", "warn": "⚠", "info": "ⓘ"}
	for _, c := range checks {
		fmt.Printf("  %s  %-18s %s\n", mark[c.Status], c.Name, c.Detail)
	}
	if warns == 0 {
		fmt.Println("\nbashy doctor: no warnings.")
	} else {
		fmt.Printf("\nbashy doctor: %d warning(s) above.\n", warns)
	}
	return 0
}
