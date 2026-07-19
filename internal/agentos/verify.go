// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// `bashy verify` codifies bashy's formal test batteries as first-class
// subcommands. The four are a PRECISION LADDER, not synonyms — the subcommand
// names keep the distinction visible so a claim is never overstated:
//
//	compat       GNU Bash 5.3 COMPATIBILITY   — behaves like the reference impl
//	conformance  mined POSIX CONFORMANCE       — passes a standard's suite (measured)
//	compliance   official POSIX CERTIFICATION  — granted by the authority (licensed)
//	benchmark    agentic BENCHMARK             — performance/efficacy (not correctness)
//
// Licensing posture (codified in suiteRegistry): the HARNESS is permissive and
// shipped; the TEST SUITES are never vendored. Public suites (GPL bash tests,
// yash) are fetched at runtime via an embedded URL into a gitignored cache — that
// is use, not redistribution, so no copyleft propagates to bashy. The official
// POSIX suite (Open Group VSC-PCTS) is LICENSED, not OSS: it is never auto-fetched;
// `compliance` is a stub that documents how to obtain a license and accepts a
// user-supplied local suite (`--suite PATH`).
//
// These orchestrate the existing dev harnesses (Makefile targets + scripts/), so
// they run from a bashy source checkout — the natural home for conformance tooling.
package agentos

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// suiteLicense classifies how a suite's tests may be obtained — the load-bearing
// policy distinction (see package doc).
type suiteLicense int

const (
	licensePublicFetch  suiteLicense = iota // GPL/public: fetch at runtime, gitignored, never vendored
	licenseUserSupplied                     // licensed/proprietary: BYO local path, never auto-fetch
	licenseHarnessOnly                      // no external suite (harness-native)
)

type suiteSpec struct {
	Name     string // subcommand
	Aliases  []string
	Kind     string // the precise claim term
	Summary  string
	License  suiteLicense
	FetchURL string // embedded runtime-download URL (public suites only; "" otherwise)
	Ready    bool   // false = stub
	Run      func(root string, args []string) int
}

// bash53TarballURL is the embedded runtime-download URL for the GPL Bash 5.3
// source; only its tests/ tree is used, fetched into a gitignored cache. bashy
// never vendors it.
const bash53TarballURL = "https://ftp.gnu.org/gnu/bash/bash-5.3.tar.gz"

func suiteRegistry() []suiteSpec {
	return []suiteSpec{
		{
			Name: "compat", Kind: "compatibility",
			Summary: "GNU Bash 5.3 compatibility — the 86/86 fixture gate (drives bin/bash)",
			License: licensePublicFetch, FetchURL: bash53TarballURL, Ready: true, Run: runVerifyCompat,
		},
		{
			Name: "conformance", Aliases: []string{"yash"}, Kind: "conformance",
			Summary: "mined POSIX conformance — the yash -p suite vs a reference-shell panel",
			License: licensePublicFetch, FetchURL: "https://github.com/magicant/yash.git", Ready: true, Run: runVerifyConformance,
		},
		{
			Name: "compliance", Aliases: []string{"posix"}, Kind: "certification",
			Summary: "official POSIX certification (Open Group VSC-PCTS) — licensed suite [STUB]",
			License: licenseUserSupplied, Ready: false, Run: runVerifyCompliance,
		},
		{
			Name: "benchmark", Kind: "benchmark",
			Summary: "agentic benchmark — bashy vs GNU Bash 5.3 across codex/claude/agy",
			License: licenseHarnessOnly, Ready: true, Run: runVerifyBenchmark,
		},
	}
}

func verifyCmd() *cobra.Command {
	var list bool
	cmd := &cobra.Command{
		Use:   "conform [suite] [flags]",
		Short: "run bashy's formal test batteries (compat/conformance/compliance/benchmark)",
		Long: "verify runs bashy's formal test batteries. The four suites are a precision\n" +
			"ladder — compatibility (vs GNU bash), conformance (measured vs POSIX suites),\n" +
			"compliance (official certification), benchmark (agentic performance) — named so\n" +
			"a claim is never overstated. Test suites are fetched at runtime (public) or\n" +
			"user-supplied (licensed); bashy never vendors them.",
		RunE: func(c *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				printVerifyList(c.OutOrStdout())
				return nil
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "list the suites and their setup status")
	for _, s := range suiteRegistry() {
		cmd.AddCommand(suiteSubcommand(s))
	}
	return cmd
}

func suiteSubcommand(s suiteSpec) *cobra.Command {
	sub := &cobra.Command{
		Use:                s.Name,
		Aliases:            s.Aliases,
		Short:              s.Summary,
		DisableFlagParsing: true, // pass flags straight through to the underlying harness
		RunE: func(c *cobra.Command, args []string) error {
			root, ok := findBashySourceRoot(mustGetwd())
			if !ok {
				return fmt.Errorf("bashy verify %s must run from a bashy source checkout (needs the Makefile + scripts/)", s.Name)
			}
			code := s.Run(root, args)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
	return sub
}

func printVerifyList(w io.Writer) {
	fmt.Fprintln(w, "bashy verify — formal test batteries (a precision ladder, not synonyms):")
	fmt.Fprintln(w)
	lic := map[suiteLicense]string{
		licensePublicFetch:  "public suite, fetched at runtime (gitignored, never vendored)",
		licenseUserSupplied: "licensed suite, user-supplied (--suite PATH; never auto-fetched)",
		licenseHarnessOnly:  "harness-native (no external suite)",
	}
	for _, s := range suiteRegistry() {
		status := "ready"
		if !s.Ready {
			status = "STUB"
		}
		fmt.Fprintf(w, "  %-12s %-14s [%s]\n      %s\n      suite: %s\n", s.Name, "("+s.Kind+")", status, s.Summary, lic[s.License])
		if s.FetchURL != "" {
			fmt.Fprintf(w, "      fetch: %s\n", s.FetchURL)
		}
		fmt.Fprintln(w)
	}
}

// --- compat: GNU Bash 5.3 compatibility (86/86) ---

func runVerifyCompat(root string, args []string) int {
	if _, err := ensureBash53Fixtures(root); err != nil {
		fmt.Fprintf(os.Stderr, "verify compat: %v\n", err)
		return 1
	}
	// The canonical gate. test-bash-parallel builds bin/bash then runs 86 fixtures.
	fmt.Fprintln(os.Stderr, "verify compat: GNU Bash 5.3 compatibility gate (86/86)…")
	return runHarness(root, "make", append([]string{"test-bash-parallel"}, args...)...)
}

// ensureBash53Fixtures makes external/bash-5.3/tests present, fetching the GPL
// Bash 5.3 source tarball at runtime into a gitignored user-cache and symlinking
// it in — the embedded-URL pattern. Idempotent; a no-op when already present.
func ensureBash53Fixtures(root string) (string, error) {
	link := filepath.Join(root, "external", "bash-5.3")
	if fi, err := os.Stat(filepath.Join(link, "tests")); err == nil && fi.IsDir() {
		return link, nil // symlink or dir already satisfies the harness
	}
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(cacheBase, "bashy", "conformance", "bash-5.3")
	if fi, err := os.Stat(filepath.Join(dst, "tests")); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "verify compat: fetching GPL Bash 5.3 tests (gitignored cache, not vendored)\n  %s\n", bash53TarballURL)
		if err := fetchBash53Tests(bash53TarballURL, dst); err != nil {
			return "", fmt.Errorf("fetch bash-5.3 tests: %w", err)
		}
	}
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	_ = os.Remove(link)
	if err := os.Symlink(dst, link); err != nil {
		return "", fmt.Errorf("symlink external/bash-5.3: %w", err)
	}
	return link, nil
}

// fetchBash53Tests streams the tarball and extracts only bash-5.3/tests/** into
// dst/tests (never the whole GPL source — only what the harness needs).
func fetchBash53Tests(url, dst string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	const want = "bash-5.3/tests/"
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		idx := strings.Index(h.Name, want)
		if idx < 0 {
			continue
		}
		rel := h.Name[idx+len("bash-5.3/"):] // -> tests/...
		if strings.Contains(rel, "..") {
			continue // path-traversal guard
		}
		target := filepath.Join(dst, rel)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	if fi, err := os.Stat(filepath.Join(dst, "tests")); err != nil || !fi.IsDir() {
		return fmt.Errorf("tarball did not contain bash-5.3/tests/")
	}
	return nil
}

// --- conformance: mined POSIX (yash panel) ---

func runVerifyConformance(root string, args []string) int {
	script := filepath.Join(root, "scripts", "yash-posix-suite.sh")
	if _, err := os.Stat(script); err != nil {
		fmt.Fprintf(os.Stderr, "verify conformance: missing %s\n", script)
		return 1
	}
	// Auto-resolve a container runtime so an agent isn't stuck: docker, else a
	// podman on PATH, else the binmgr-cached podman — starting the machine if it's
	// stopped. The script's own detection only knew docker/bashy-podman; passing a
	// resolved OCI removes that wall.
	oci, err := ensureContainerRuntime()
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify conformance: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "verify conformance: yash POSIX (-p) suite via %s — auto-clones yash (GPL, gitignored) + builds the shell panel…\n", oci)
	return runHarnessEnv(root, []string{"OCI=" + oci}, script, args...)
}

// ensureContainerRuntime finds a usable OCI runtime and makes sure it's ready
// (starts a stopped podman VM). Returns the command the harness should use as
// $OCI, or an actionable error — so `verify conformance/benchmark` "just runs"
// for any agent that has a runtime installed, instead of a bare "need docker".
func ensureContainerRuntime() (string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	oci := ""
	if p, err := exec.LookPath("podman"); err == nil {
		oci = p
	} else if cb, err := os.UserCacheDir(); err == nil {
		// binmgr caches bashy's own podman here (same tree as vfkit/gvproxy).
		if cand := filepath.Join(cb, "bashy", "bin", "podman"); isExecutable(cand) {
			oci = cand
		}
	}
	if oci == "" {
		return "", fmt.Errorf("no container runtime found — install docker or podman (macOS: also `podman machine init`), then retry")
	}
	// A stopped podman VM makes every command fail; start it once.
	if err := exec.Command(oci, "info").Run(); err != nil {
		fmt.Fprintln(os.Stderr, "verify: container runtime not responding — starting the podman machine…")
		if serr := exec.Command(oci, "machine", "start").Run(); serr != nil {
			return "", fmt.Errorf("podman machine is not running and could not be started (try: %s machine start): %v", oci, serr)
		}
		fmt.Fprintf(os.Stderr, "verify: podman machine started — stop it with `%s machine stop` when done to free CPU\n", oci)
	}
	return oci, nil
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		// Windows has no unix exec bit — a file copied/extracted there is
		// -rw-rw-rw-. Runnability is by extension, so a bashy-managed podman.exe
		// in the cache must still resolve.
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".bat", ".cmd", ".com":
			return true
		}
		return false
	}
	return fi.Mode()&0o111 != 0
}

// --- compliance: official POSIX (Open Group VSC-PCTS) — STUB ---

func runVerifyCompliance(root string, args []string) int {
	var suite string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--suite" && i+1 < len(args):
			i++
			suite = args[i]
		case strings.HasPrefix(args[i], "--suite="):
			suite = args[i][len("--suite="):]
		}
	}
	w := os.Stdout
	fmt.Fprintln(w, "bashy verify compliance — official POSIX certification (Open Group VSC-PCTS)")
	fmt.Fprintln(w, "STATUS: stub. The certification suite is LICENSED (not open source), so bashy")
	fmt.Fprintln(w, "does not — and legally cannot — auto-download it. Obtain it under license, then")
	fmt.Fprintln(w, "point this command at your local copy:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  bashy verify compliance --suite /path/to/vsc-pcts")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Obtaining a license:")
	fmt.Fprintln(w, "  • The Open Group POSIX/UNIX certification + the VSC-PCTS test suite:")
	fmt.Fprintln(w, "    https://www.opengroup.org/certifications  (application in progress)")
	fmt.Fprintln(w, "  • The runner will land here once a licensed suite is available to test against.")
	if suite != "" {
		if _, err := os.Stat(suite); err != nil {
			fmt.Fprintf(w, "\nnote: --suite %q not found: %v\n", suite, err)
			return 2
		}
		fmt.Fprintf(w, "\nnote: found a suite at %q, but the VSC-PCTS runner is not implemented yet (stub).\n", suite)
	}
	return 0
}

// --- benchmark: agentic (bashy vs bash 5.3 × agents) ---

func runVerifyBenchmark(root string, args []string) int {
	runner := filepath.Join(root, "eval", "agent-shell", "run-container-task.sh")
	if _, err := os.Stat(runner); err != nil {
		fmt.Fprintf(os.Stderr, "verify benchmark: missing %s\n", runner)
		return 1
	}
	if len(args) == 0 {
		w := os.Stdout
		fmt.Fprintln(w, "bashy verify benchmark — agentic shell benchmark (bashy vs GNU Bash 5.3)")
		fmt.Fprintln(w, "Container-enforced arms × codex/claude/agy. Needs a container runtime + the")
		fmt.Fprintln(w, "agent CLIs; see docs/agent-shell-eval/. Pass harness args through, e.g.:")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  bashy verify benchmark --task wrong-cwd-recovery --tool codex")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "(no args → this help; the harness is orchestrated by the eval campaign)")
		return 0
	}
	return runHarness(root, runner, args...)
}

// --- shared ---

func runHarness(dir, bin string, args ...string) int {
	return runHarnessEnv(dir, nil, bin, args...)
}

func runHarnessEnv(dir string, extraEnv []string, bin string, args ...string) int {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		return 1
	}
	return 0
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
