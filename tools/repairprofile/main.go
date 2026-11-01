// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// repairprofile runs one core component's repair test profile on the host it
// runs on and writes the bashy-repair-profile-v1 evidence record (S549.R4a).
// Driven by the ci/repair-profiles.dag.md recipe — see docs/repair-profiles.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/bashy/internal/agentos"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		component = flag.String("component", "", "component profile to run: bashy|outpost (required)")
		root      = flag.String("root", ".", "component checkout root (suite commands run there)")
		out       = flag.String("out", "", "evidence JSON path (default results/repair-profile-<component>-<os>-<arch>.json; empty file arg to -list needs none)")
		list      = flag.Bool("list", false, "print the plan for this host (suites, lanes, skip reasons) and exit")
		only      = flag.String("only", "", "comma-separated suite names to run (default: the full matrix)")
		timeoutS  = flag.Int("timeout", 0, "default per-suite wall bound in seconds (0 = built-in default)")
		cacheBase = flag.String("cache", "", "fixture cache base directory (default: the OS user cache dir)")
	)
	var overrides suiteOverrides
	flag.Var(&overrides, "suite", "override a suite command: -suite name=argv (whitespace-split), repeatable")
	flag.Parse()

	if *component == "" {
		fmt.Fprintln(os.Stderr, "repairprofile: -component is required (bashy|outpost)")
		return 2
	}
	if _, err := agentos.RepairSuitesFor(*component); err != nil && len(overrides) == 0 {
		fmt.Fprintln(os.Stderr, "repairprofile:", err)
		return 2
	}

	outPath := *out
	if outPath == "" {
		outPath = fmt.Sprintf("results/repair-profile-%s-%s-%s.json", *component, runtime.GOOS, runtime.GOARCH)
	}
	offline := boolEnv("BASHY_OFFLINE")

	opts := agentos.RepairProfileOptions{
		Component: *component,
		Root:      *root,
		Out:       outPath,
		Offline:   offline,
		CacheBase: *cacheBase,
		Stderr:    os.Stderr,
	}
	if *only != "" {
		for _, n := range strings.Split(*only, ",") {
			if n = strings.TrimSpace(n); n != "" {
				opts.Only = append(opts.Only, n)
			}
		}
	}
	if *timeoutS > 0 {
		opts.Timeout = time.Duration(*timeoutS) * time.Second
	}
	if len(overrides) > 0 {
		specs, err := agentos.RepairSuitesFor(*component)
		if err != nil {
			fmt.Fprintln(os.Stderr, "repairprofile:", err)
			return 2
		}
		opts.CustomSuites = specs
		for i := range opts.CustomSuites {
			if argv, ok := overrides[opts.CustomSuites[i].Name]; ok {
				opts.CustomSuites[i].Args = argv
				opts.CustomSuites[i].Kind = "argv"
				opts.CustomSuites[i].Command = strings.Join(argv, " ")
			}
		}
	}

	if *list {
		if _, err := agentos.ListRepairProfile(opts, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "repairprofile:", err)
			return 2
		}
		return 0
	}

	ev, err := agentos.RunRepairProfile(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repairprofile: %v\n", err)
		return 2
	}
	for _, s := range ev.Suites {
		status := "PASS"
		switch {
		case s.Skipped:
			status = "SKIP"
		case !s.Complete:
			status = "KILLED"
		case !s.Pass:
			status = "FAIL"
		}
		fmt.Fprintf(os.Stderr, "repairprofile: %-6s %-22s %s\n", status, s.Name, s.Reason)
	}
	verdict := "FAIL"
	if ev.Pass {
		verdict = "PASS"
	}
	fmt.Fprintf(os.Stderr, "repairprofile: %s component=%s complete=%v pass=%v evidence=%s\n",
		verdict, ev.Component, ev.Complete, ev.Pass, outPath)
	if !ev.Pass {
		return 1
	}
	return 0
}

func boolEnv(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	return err == nil && b
}

// suiteOverrides collects -suite name=argv flags.
type suiteOverrides map[string][]string

func (s suiteOverrides) String() string {
	var parts []string
	for k, v := range s {
		parts = append(parts, k+"="+strings.Join(v, " "))
	}
	return strings.Join(parts, ", ")
}

func (s *suiteOverrides) Set(val string) error {
	name, argv, ok := strings.Cut(val, "=")
	if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(argv) == "" {
		return fmt.Errorf("-suite wants name=argv (e.g. -suite make-test=make test)")
	}
	if *s == nil {
		*s = suiteOverrides{}
	}
	(*s)[strings.TrimSpace(name)] = strings.Fields(argv)
	return nil
}
