// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// `bashy release` is the distribution verb: what bytes leave this machine, and
// under what name. It is deliberately NOT an orchestration verb — every one of
// those (weave/sprint/dag/supervise) answers *who does the work and in what
// order*, and none of them owns an artifact's identity. The artifact names are
// a contract a third party consumes BY NAME (the fleet-upgrade receiver,
// binmgr), which is why the stages are subcommands of one verb rather than a
// new top-level family. See docs/plan-bashy-release-t0.md.
//
// This is the T0 slice: build → archive → checksum, in-process, over the
// .goreleaser.yaml subset implemented by coreutils/pkg/release. It is
// LOCAL-FIRST — `--snapshot` needs no network, no credentials, and no tag. The
// unimplemented tail (sign/sbom/publish/announce) is refused BY NAME by the
// config loader, never silently skipped, so a run can never ship fewer assets
// than the config declares while still reporting success.
//
// Why the whole GoReleaser CLI is not imported: measured at 77.3 MB / 277 new
// linked modules / 6 new MPL-2.0 deps (docs/bashy-release-pipeline-design.md
// §4), and the three stages T0 needs live in its internal/pipe/* and are not
// importable at any price.
package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	outgit "github.com/qiangli/coreutils/git"
	"github.com/qiangli/coreutils/pkg/release"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// releasePlanSchema tags the `release plan` envelope. The artifact ledger
// carries its own schema tag (release.LedgerSchema, "bashy-release-v1").
const releasePlanSchema = "bashy-release-plan-v1"

// releaseLedgerName is where the artifact ledger is written inside the output
// directory. The ledger is the machine-readable product of a run: the assets
// with their digests and sizes. It is written into dist/ (goreleaser writes its
// own artifacts.json there) and is NOT covered by the checksum manifest, which
// lists archives only.
const releaseLedgerName = "release-ledger.json"

// releaseSnapshotBase is the version a snapshot counts from when the caller
// names none. A snapshot is by definition unreleasable, so inventing a version
// from the newest tag would be a guess presented as a fact — and tag ordering
// is not lexical. `--version` states it explicitly instead; deriving one from a
// tag is Tier 1 (`release publish`), which this slice does not implement.
const releaseSnapshotBase = "0.0.0"

// releaseConfigNames are the config file names probed in the project directory,
// in order, when --config is not given.
var releaseConfigNames = []string{".goreleaser.yaml", ".goreleaser.yml"}

// errReleaseUsage marks a usage error so dispatch can exit 2 rather than 1.
var errReleaseUsage = errors.New("usage")

type releaseOptions struct {
	dir       string
	config    string
	dist      string
	version   string
	skipBuild bool
	asJSON    bool
}

func (o *releaseOptions) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&o.dir, "dir", "C", "", "Project root holding the config and the output directory (default: cwd)")
	f.StringVarP(&o.config, "config", "f", "", "Config path (default: .goreleaser.yaml, then .goreleaser.yml, in the project root)")
	f.StringVar(&o.dist, "dist", "", "Output directory, relative to the project root (default: the config's dist, or 'dist')")
	f.StringVar(&o.version, "version", "", "Version to build under (default: the config's snapshot version template)")
	f.BoolVar(&o.skipBuild, "skip-build", false, "Package binaries already present in the output directory")
	f.BoolVar(&o.asJSON, "json", weavecli.IsAgent(), "Emit JSON (default under $BASHY_AGENTIC)")
}

func releaseCmd() *cobra.Command {
	var opts releaseOptions
	var snapshot bool

	root := &cobra.Command{
		Use:   "release",
		Short: "Build, archive and checksum this project's release artifacts",
		Long: `bashy release turns a .goreleaser.yaml into named, checksummed artifacts.

T0 implements the local-first half in-process: build -> archive -> checksum,
emitting the archives, the checksum manifest, and a ` + release.LedgerSchema + ` artifact
ledger. No network, no credentials, no tag required.

Stages this tier does not implement (sign, sbom, publish, announce, packages)
are refused BY NAME when the config declares them, so a run never ships fewer
assets than the config asks for while still reporting success.

  bashy release --snapshot          build + archive + checksum (no tag needed)
  bashy release plan                what a run would produce, without building
  bashy release check               validate the config and the name templates`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !snapshot {
				// Bare `bashy release` is not a no-op and not a full release:
				// say which invocation exists rather than printing help and
				// exiting 0, which would read as "done".
				return fmt.Errorf("%w: nothing to do — this tier builds only snapshots: `bashy release --snapshot` (tagged publishing is not implemented)", errReleaseUsage)
			}
			return runReleaseSnapshot(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), &opts)
		},
	}
	root.Flags().BoolVar(&snapshot, "snapshot", false, "Run the snapshot pipeline (build, archive, checksum)")
	opts.bind(root)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", errReleaseUsage, err)
	})

	root.AddCommand(releaseSnapshotCmd(), releasePlanCmd(), releaseCheckCmd())
	return root
}

func releaseSnapshotCmd() *cobra.Command {
	var opts releaseOptions
	cmd := &cobra.Command{
		Use:          "snapshot",
		Short:        "Build, archive and checksum without a tag (same as `release --snapshot`)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReleaseSnapshot(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), &opts)
		},
	}
	opts.bind(cmd)
	return cmd
}

func releasePlanCmd() *cobra.Command {
	var opts releaseOptions
	cmd := &cobra.Command{
		Use:          "plan",
		Short:        "Print what a run would build and package, without building it",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReleasePlan(cmd.OutOrStdout(), &opts)
		},
	}
	opts.bind(cmd)
	return cmd
}

func releaseCheckCmd() *cobra.Command {
	var opts releaseOptions
	cmd := &cobra.Command{
		Use:          "check",
		Short:        "Validate the config, its stages and its name templates",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, dir, err := loadReleaseConfig(&opts)
			if err != nil {
				return err
			}
			fields, err := resolveReleaseFields(dir, cfg, &opts)
			if err != nil {
				return err
			}
			plan, err := release.BuildPlan(cfg, fields)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return releaseJSON(cmd.OutOrStdout(), map[string]any{
					"schema_version": releasePlanSchema,
					"ok":             true,
					"project_name":   plan.ProjectName,
					"version":        plan.Version,
					"dist":           cfg.Dist,
					"targets":        len(plan.Targets),
					"archives":       len(plan.Archives),
					"checksum":       plan.ChecksumName,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "release: config ok — %s %s, %d targets, %d archives, checksum %s\n",
				plan.ProjectName, plan.Version, len(plan.Targets), len(plan.Archives), plan.ChecksumName)
			return nil
		},
	}
	opts.bind(cmd)
	return cmd
}

// runReleaseSnapshot is the T0 pipeline: resolve config + fields, build,
// archive, checksum, then write the artifact ledger.
func runReleaseSnapshot(ctx context.Context, stdout, stderr io.Writer, opts *releaseOptions) error {
	cfg, dir, err := loadReleaseConfig(opts)
	if err != nil {
		return err
	}
	fields, err := resolveReleaseFields(dir, cfg, opts)
	if err != nil {
		return err
	}
	if dirty, known := releaseWorktreeDirty(dir); known && dirty {
		// Not a failure — a snapshot of uncommitted work is a legitimate thing
		// to want. But the artifacts then correspond to no commit, and the
		// version string names one, so say so instead of letting the name imply
		// a provenance it does not have.
		fmt.Fprintf(stderr, "release: worktree is dirty — these artifacts do not correspond to commit %s as built\n", fields.ShortCommit)
	}

	// Builds run through the package's own GoBuilder (the Go toolchain doing
	// its own job); its diagnostics belong on our stderr, not swallowed.
	ledger, err := release.Run(ctx, cfg, release.Options{
		Dir:       dir,
		Builder:   release.GoBuilder{Stderr: stderr},
		SkipBuild: opts.skipBuild,
		Snapshot:  true,
		Fields:    fields,
	})
	if err != nil {
		return err
	}

	ledgerPath := filepath.Join(dir, cfg.Dist, releaseLedgerName)
	if err := writeReleaseLedger(ledgerPath, ledger); err != nil {
		return err
	}
	if opts.asJSON {
		b, err := ledger.JSON()
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, string(b))
		return nil
	}
	printReleaseLedger(stdout, ledger, filepath.Join(cfg.Dist, releaseLedgerName))
	return nil
}

func runReleasePlan(stdout io.Writer, opts *releaseOptions) error {
	cfg, dir, err := loadReleaseConfig(opts)
	if err != nil {
		return err
	}
	fields, err := resolveReleaseFields(dir, cfg, opts)
	if err != nil {
		return err
	}
	plan, err := release.BuildPlan(cfg, fields)
	if err != nil {
		return err
	}
	if opts.asJSON {
		targets := make([]map[string]any, 0, len(plan.Targets))
		for _, t := range plan.Targets {
			targets = append(targets, map[string]any{
				"build": t.BuildID, "goos": t.Goos, "goarch": t.Goarch,
				"binary": t.Binary, "path": t.Path,
			})
		}
		archives := make([]map[string]any, 0, len(plan.Archives))
		for _, a := range plan.Archives {
			archives = append(archives, map[string]any{
				"name": a.Name, "format": a.Format, "path": a.Path,
				"goos": a.Target.Goos, "goarch": a.Target.Goarch,
			})
		}
		return releaseJSON(stdout, map[string]any{
			"schema_version": releasePlanSchema,
			"project_name":   plan.ProjectName,
			"version":        plan.Version,
			"snapshot":       true,
			"dist":           cfg.Dist,
			"targets":        targets,
			"archives":       archives,
			"checksum":       plan.ChecksumName,
		})
	}
	fmt.Fprintf(stdout, "%s %s (snapshot) → %s\n", plan.ProjectName, plan.Version, cfg.Dist)
	for _, t := range plan.Targets {
		fmt.Fprintf(stdout, "  build    %s/%s  %s\n", t.Goos, t.Goarch, t.Path)
	}
	for _, a := range plan.Archives {
		fmt.Fprintf(stdout, "  archive  %s/%s  %s\n", a.Target.Goos, a.Target.Goarch, a.Path)
	}
	if plan.ChecksumName != "" {
		fmt.Fprintf(stdout, "  checksum %s\n", filepath.Join(cfg.Dist, plan.ChecksumName))
	}
	return nil
}

// loadReleaseConfig resolves the project root and the config, applies the
// goreleaser defaults, and honours a --dist override. It returns the ABSOLUTE
// project root, which every path in the run is joined onto.
func loadReleaseConfig(opts *releaseOptions) (*release.Config, string, error) {
	dir := opts.dir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		dir = wd
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, "", fmt.Errorf("release: project root %s is not a directory", dir)
	}

	path := opts.config
	switch {
	case path == "":
		for _, name := range releaseConfigNames {
			candidate := filepath.Join(dir, name)
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
		if path == "" {
			return nil, "", fmt.Errorf("release: no release config in %s (looked for %s) — pass --config PATH",
				dir, strings.Join(releaseConfigNames, ", "))
		}
	case !filepath.IsAbs(path):
		path = filepath.Join(dir, path)
	}

	cfg, err := release.LoadConfig(path)
	if err != nil {
		return nil, "", err
	}
	if err := cfg.ApplyDefaults(dir); err != nil {
		return nil, "", err
	}
	if opts.dist != "" {
		// The output directory is joined onto the project root, so an absolute
		// or upward path would write outside the project — the same escape the
		// artifact-name check refuses one level down.
		if filepath.IsAbs(opts.dist) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(opts.dist)), "../") {
			return nil, "", fmt.Errorf("release: --dist %q must be a path inside the project root", opts.dist)
		}
		cfg.Dist = filepath.Clean(opts.dist)
	}
	return cfg, dir, nil
}

// resolveReleaseFields fills the name-template context. The version is either
// stated by --version or rendered from the config's snapshot template over the
// resolved commit; there is no third, guessed source.
func resolveReleaseFields(dir string, cfg *release.Config, opts *releaseOptions) (release.Fields, error) {
	fields := release.Fields{ProjectName: cfg.ProjectName}
	// Pure-Go git (go-git via coreutils), not a `git` subprocess: `bashy
	// release` must work on a node whose only toolchain is bashy itself.
	if rp, err := outgit.RevParse(outgit.RevParseOptions{RepoPath: dir, Short: 7}); err == nil {
		fields.Commit, fields.ShortCommit = rp.Hash, rp.Short
	}
	if v := strings.TrimSpace(opts.version); v != "" {
		fields.Version = strings.TrimPrefix(v, "v")
		return fields, nil
	}
	if fields.ShortCommit == "" {
		return fields, fmt.Errorf("release: %s is not a git repository, so a snapshot has no commit to name itself after — pass --version VERSION", dir)
	}
	version, err := release.Render(cfg.Snapshot.VersionTemplate, release.Fields{
		ProjectName: cfg.ProjectName,
		Version:     releaseSnapshotBase,
		Commit:      fields.Commit,
		ShortCommit: fields.ShortCommit,
	})
	if err != nil {
		return fields, fmt.Errorf("release: snapshot.version_template: %w (or pass --version VERSION)", err)
	}
	fields.Version = version
	return fields, nil
}

// releaseWorktreeDirty reports whether dir's worktree has uncommitted changes.
// The second return says whether the question could be answered at all — a
// non-repo is "unknown", never "clean", so absence of evidence is not reported
// as a clean tree.
func releaseWorktreeDirty(dir string) (dirty, known bool) {
	rp, err := outgit.RevParse(outgit.RevParseOptions{RepoPath: dir})
	if err != nil {
		return false, false
	}
	return rp.Dirty, true
}

func writeReleaseLedger(path string, ledger *release.Ledger) error {
	b, err := ledger.JSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func printReleaseLedger(w io.Writer, ledger *release.Ledger, ledgerPath string) {
	mode := "release"
	if ledger.Snapshot {
		mode = "snapshot"
	}
	fmt.Fprintf(w, "%s %s (%s) → %s\n", ledger.ProjectName, ledger.Version, mode, ledger.Dist)
	width := 0
	for _, a := range ledger.Artifacts {
		if len(a.Name) > width {
			width = len(a.Name)
		}
	}
	for _, a := range ledger.Artifacts {
		fmt.Fprintf(w, "  %-*s  %9d  %s\n", width, a.Name, a.Size, a.SHA256[:12])
	}
	fmt.Fprintf(w, "ledger: %s (%s, %d artifacts)\n", ledgerPath, ledger.Schema, len(ledger.Artifacts))
}

func releaseJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// dispatchRelease is the front-door entry point. Usage errors exit 2 so an
// agent can tell "I called it wrong" from "the release failed".
func dispatchRelease(args []string) int {
	cmd := releaseCmd()
	cmd.SetArgs(args)
	cmd.SilenceErrors = true
	if err := cmd.Execute(); err != nil {
		msg := err.Error()
		if errors.Is(err, errReleaseUsage) {
			fmt.Fprintf(os.Stderr, "release: %s\n", strings.TrimPrefix(msg, "usage: "))
			return 2
		}
		if !strings.HasPrefix(msg, "release:") {
			msg = "release: " + msg
		}
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}
	return 0
}
