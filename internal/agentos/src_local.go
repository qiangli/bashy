package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/src"
)

// src_local.go — the run-local dispatcher (M2b deliverable 5).
//
// Edits under $BASHY_HOME/src live on the host's own `local` branch and
// merge through local PRs. This dispatcher decides, per command, what
// actually runs:
//
//   - an edited skill-tier Bash# unit (.bsh) is run from its edited source,
//     interpreted by this binary — the worktree file when the checkout
//     sits on `local`, else the `local`-tip blob materialized into the
//     run cache;
//   - a Go change runs from a `bashy self build --local` cached build, and
//     only when that build's source differs from the release (a fresh
//     manifest at the current local tip). A build whose diff touches core
//     paths is never built and a running core never executes a local edit:
//     both fall back to the release binary.
//   - pristine (local == release) always restores the release binary.
//
// A modified-protected build may run in the foreground but is never
// started as a service (see serviceSrcGuard, wired into self install).

// Local run resolution kinds.
const (
	localRunRelease = "release"
	localRunSource  = "source"
	localRunBinary  = "binary"
)

// localRunResolution is what the dispatcher picked for one command.
type localRunResolution struct {
	Kind   string `json:"kind"`
	Repo   string `json:"repo,omitempty"`
	State  string `json:"state,omitempty"`
	Base   string `json:"base,omitempty"`
	Local  string `json:"local,omitempty"`
	Unit   string `json:"unit,omitempty"`
	Binary string `json:"binary,omitempty"`
	Notice string `json:"notice,omitempty"`
}

// resolveLocalRun maps command to what should run: the edited Bash# unit,
// a cached local build, or the release binary. Resolution is read-only —
// it never fetches, builds, or mutates a checkout.
func resolveLocalRun(home, command string) (localRunResolution, error) {
	release := func(notice string) (localRunResolution, error) {
		return localRunResolution{Kind: localRunRelease, Notice: notice}, nil
	}
	repo, pkg, err := src.Source(command)
	if err != nil {
		return release("unknown command: running the release binary")
	}
	if home == "" {
		home = src.Home()
	}
	dir := filepath.Join(src.SrcDir(home), repo)
	if st, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !st.IsDir() {
		return release("repo " + repo + " is not fetched: running the release binary")
	}
	st, err := src.State(home, repo)
	if err != nil {
		// Never report pristine without checking: an undeterminable
		// state falls back to release, loudly.
		r := localRunResolution{Kind: localRunRelease, Repo: repo, State: "unknown"}
		r.Notice = "state of " + repo + " could not be determined: running the release binary"
		return r, nil
	}
	if st.State == src.StatePristine {
		return localRunResolution{Kind: localRunRelease, Repo: repo, State: st.State, Base: st.Base, Local: st.Local}, nil
	}
	files, err := localChangedFiles(dir, st.Base, st.Local)
	if err != nil {
		return release("cannot diff " + repo + ": running the release binary")
	}
	var core []string
	for _, f := range files {
		if src.PathClass(repo, f) == src.ClassCore {
			core = append(core, f)
		}
	}
	if len(core) > 0 {
		// A running core never executes a local edit.
		return localRunResolution{Kind: localRunRelease, Repo: repo, State: st.State, Base: st.Base, Local: st.Local,
			Notice: "core path(s) " + strings.Join(core, ", ") + " differ from release: running the release binary"}, nil
	}
	if unit, ok, err := localEditedUnit(home, dir, repo, pkg, files, st.Local, command); err != nil {
		return localRunResolution{}, err
	} else if ok {
		return localRunResolution{Kind: localRunSource, Repo: repo, State: st.State, Base: st.Base, Local: st.Local, Unit: unit}, nil
	}
	if bin, ok := freshLocalBuild(home, repo, st); ok {
		return localRunResolution{Kind: localRunBinary, Repo: repo, State: st.State, Base: st.Base, Local: st.Local, Binary: bin}, nil
	}
	return localRunResolution{Kind: localRunRelease, Repo: repo, State: st.State, Base: st.Base, Local: st.Local,
		Notice: "no fresh cached build for " + repo + " local " + shortSHA12(st.Local) + " (run `bashy self build --local --repo " + repo + "`): running the release binary"}, nil
}

// localChangedFiles lists repo-relative paths differing between base and
// local, parsed from the unified diff headers.
func localChangedFiles(dir, base, local string) ([]string, error) {
	text, err := ygit.DiffCommits(dir, base, local)
	if err != nil {
		return nil, err
	}
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		a := strings.TrimPrefix(fields[2], "a/")
		b := strings.TrimPrefix(fields[3], "b/")
		p := b
		if b == "/dev/null" {
			p = a
		}
		p = strings.TrimPrefix(p, "./")
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

// repoUnitDir maps a src package path to the repo-relative directory whose
// edited files belong to the command. It reports false when the package
// cannot be attributed to a directory (shell-mounted commands, externals,
// whole-module mappings outside yoke), in which case no source unit is
// eligible and only the cached-build path remains.
func repoUnitDir(repo, pkg string) (string, bool) {
	if pkg == "" || pkg == "builtin" || strings.HasPrefix(pkg, "managed:") || strings.HasPrefix(pkg, "provider:") {
		return "", false
	}
	if repo == "yoke" {
		const mod = "github.com/qiangli/yoke"
		if pkg == mod {
			return "", true
		}
		if rest, ok := strings.CutPrefix(pkg, mod+"/"); ok {
			return rest, true
		}
	}
	return "", false
}

// isInterpretedUnit reports whether a changed skill-tier file is a Bash#
// unit bashy runs interpreted. .bsh is the interpreted Bash# source form
// (see the source-routing contract: `bashy x.bsh` runs interpreted).
func isInterpretedUnit(path string) bool { return strings.HasSuffix(path, ".bsh") }

// localEditedUnit finds the edited Bash# unit for command among the
// changed skill-tier files under its package dir: exactly one interpretable
// unit, or the one named <command>.bsh. Anything else is ambiguous and an
// error — the dispatcher never guesses which file the command means.
func localEditedUnit(home, dir, repo, pkg string, files []string, localTip, command string) (string, bool, error) {
	pkgdir, ok := repoUnitDir(repo, pkg)
	if !ok {
		return "", false, nil
	}
	var units []string
	for _, f := range files {
		if !isInterpretedUnit(f) {
			continue
		}
		if src.PathClass(repo, f) != src.ClassSkill && src.PathClass(repo, f) != src.ClassProtected {
			continue
		}
		if pkgdir != "" && f != pkgdir && !strings.HasPrefix(f, pkgdir+"/") {
			continue
		}
		units = append(units, f)
	}
	switch len(units) {
	case 0:
		return "", false, nil
	case 1:
		path, err := materializeUnit(home, dir, repo, localTip, units[0])
		return path, err == nil, err
	default:
		for _, u := range units {
			if filepath.Base(u) == command+".bsh" {
				path, err := materializeUnit(home, dir, repo, localTip, u)
				return path, err == nil, err
			}
		}
		return "", false, fmt.Errorf("src run: %d edited Bash# units under %s/%s; name one by running it directly", len(units), repo, pkgdir)
	}
}

// materializeUnit returns a runnable path for the unit at the local tip:
// the worktree file when the checkout sits on local, else the tip blob
// materialized into the run cache (the worktree may sit on a topic branch
// and must not be moved by a run).
func materializeUnit(home, dir, repo, localTip, rel string) (string, error) {
	if rev, err := ygit.RevParse(ygit.RevParseOptions{RepoPath: dir}); err == nil && rev.Hash == localTip {
		cand := filepath.Join(dir, filepath.FromSlash(rel))
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	body, err := blobAtRevision(dir, localTip, rel)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(src.SrcDir(home), ".run", repo, shortSHA12(localTip), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, []byte(body), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// blobAtRevision reads one file at a revision through go-git: pure Go, no
// worktree mutation, no shell-out.
func blobAtRevision(dir, rev, rel string) (string, error) {
	r, err := gogit.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	h, err := r.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return "", fmt.Errorf("src run: resolve %s: %w", shortSHA12(rev), err)
	}
	c, err := r.CommitObject(*h)
	if err != nil {
		return "", err
	}
	f, err := c.File(filepath.ToSlash(rel))
	if err != nil {
		return "", fmt.Errorf("src run: %s not found at %s", rel, shortSHA12(rev))
	}
	return f.Contents()
}

// localBuildManifest records which source a cached local build came from.
// The dispatcher trusts it only when Local/Base match the live State: a
// revert (local == pristine) or any new commit invalidates it, and the
// release binary is restored.
type localBuildManifest struct {
	Repo    string `json:"repo"`
	Base    string `json:"base"`
	Local   string `json:"local"`
	State   string `json:"state"`
	Binary  string `json:"binary"`
	BuiltAt string `json:"built_at"`
}

func localBuildsDir(home string) string {
	if home == "" {
		home = src.Home()
	}
	return filepath.Join(src.SrcDir(home), ".builds")
}

func writeLocalBuildManifest(home, repo string, m localBuildManifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(localBuildsDir(home), repo, shortSHA12(m.Local))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), append(raw, '\n'), 0o644)
}

func readLocalBuildManifest(home, repo, local string) (localBuildManifest, bool) {
	var m localBuildManifest
	raw, err := os.ReadFile(filepath.Join(localBuildsDir(home), repo, shortSHA12(local), "manifest.json"))
	if err != nil {
		return m, false
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return localBuildManifest{}, false
	}
	return m, true
}

// freshLocalBuild prefers the cached build only when its source differs
// from the release and still matches the live tip: manifest pins equal the
// current State and the binary exists. Anything else falls back to release.
func freshLocalBuild(home, repo string, st src.RepoState) (string, bool) {
	if st.State == src.StatePristine {
		return "", false
	}
	m, ok := readLocalBuildManifest(home, repo, st.Local)
	if !ok || m.Local != st.Local || m.Base != st.Base || m.Binary == "" {
		return "", false
	}
	if st, err := os.Stat(m.Binary); err != nil || st.IsDir() {
		return "", false
	}
	return m.Binary, true
}

// serviceSrcGuard refuses to start a service from a modified-protected
// tree: guard-rail rewrites may run in the foreground under their
// approval, but a modified-protected build is never started as a service.
func serviceSrcGuard(rows []srcRepoProvenance) error {
	for _, r := range rows {
		if r.State == src.StateModifiedProtected {
			return fmt.Errorf("refusing: src repo %s is modified-protected (local %s rewrites guard-rail paths): a modified-protected build is never started as a service", r.Repo, shortSHA12(r.Local))
		}
	}
	return nil
}

// newSrcRunCmd is the run-local dispatcher verb: `bashy src run <command>
// [args...]`. Pristine restores the release binary; an edited Bash# unit
// runs interpreted; a Go change runs the fresh cached local build.
func newSrcRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <command> [args...]",
		Short: "Run a command from its local src checkout (edited unit, cached build, or release)",
		Long: `Run a command from its local src checkout.

  src run <command> [args...]

The dispatcher picks what runs: an edited skill-tier Bash# unit runs from
its edited source (interpreted); a Go change runs the fresh ` + "`bashy self build --local`" + ` cached build, only when its source differs from the
release and never when the diff touches core paths; pristine (local ==
release) restores the release binary. Extra arguments pass through
verbatim (no flag parsing past the command name).`,
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := resolveLocalRun("", args[0])
			if err != nil {
				return err
			}
			rest := append([]string{}, args[1:]...)
			switch res.Kind {
			case localRunSource:
				noticeSrcRun(os.Stderr, res)
				exe, err := os.Executable()
				if err != nil || exe == "" {
					return fmt.Errorf("src run: cannot resolve the interpreter binary")
				}
				return execPassthrough(exe, append([]string{res.Unit}, rest...))
			case localRunBinary:
				noticeSrcRun(os.Stderr, res)
				return execPassthrough(res.Binary, rest)
			default:
				if res.Notice != "" {
					fmt.Fprintln(os.Stderr, "src run: "+res.Notice)
				}
				exe, err := os.Executable()
				if err != nil || exe == "" {
					return fmt.Errorf("src run: cannot resolve the release binary")
				}
				return execPassthrough(exe, args)
			}
		},
	}
	return cmd
}

func noticeSrcRun(w *os.File, res localRunResolution) {
	fmt.Fprintf(w, "src run: %s from %s local %s (release %s)\n", res.Kind, res.Repo, shortSHA12(res.Local), shortSHA12(res.Base))
}

// execPassthrough runs path with args, passing stdio through, and returns
// an error carrying the child's exit status.
func execPassthrough(path string, args []string) error {
	c := exec.Command(path, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("src run: %w", err)
	}
	return nil
}

// localBuildCorePaths returns the pristine..local diff paths that are core
// tier in repo: any entry refuses the build.
func localBuildCorePaths(home, repo string, st src.RepoState) ([]string, error) {
	dir := filepath.Join(src.SrcDir(home), repo)
	files, err := localChangedFiles(dir, st.Base, st.Local)
	if err != nil {
		return nil, err
	}
	var core []string
	for _, f := range files {
		if src.PathClass(repo, f) == src.ClassCore {
			core = append(core, f)
		}
	}
	return core, nil
}
