package agentos

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/pkg/src"
)

// Full dependency revisions are needed by src fetch: Go's module build info
// carries only the 12-character pseudo-version suffix. Keep these in step
// with go.mod; the version check below fails closed after an unpaired bump.
var srcModulePins = map[string]struct{ module, upstream, commit string }{
	"yoke":        {"github.com/qiangli/yoke", "", "556b27f01f72d1178011a11931300bbb5f52b79a"},
	"coreutils":   {"github.com/qiangli/coreutils", "", "2ff8ab0f9ed3fec2c7ab139b364949a6db262522"},
	"sh":          {"github.com/qiangli/sh/v3", "mvdan.cc/sh/v3", "dc51034ce22ac1bd2f89db2e0207dfb8b494bc1e"},
	"filebrowser": {"github.com/qiangli/filebrowser/v2", "github.com/filebrowser/filebrowser/v2", "d1d2feab447319ff2b37b471829b59bc3cace390"},
}

func srcPinnedCommit(repo string) (string, error) {
	if repo == "bashy" {
		return src.ReleaseCommit(), nil
	}
	if repo == "managed" || repo == "posix-provider" {
		return "unknown", nil // each external has its own independent pin
	}
	if repo == src.RegisteredRepo {
		// A registered command is a file of its own, not a linked module:
		// its history is the file's, so there is no release pin to report.
		return "registered (see the source file's own history)", nil
	}
	_, ok := srcModulePins[repo]
	if !ok {
		return "", nil // managed externals and providers have their own pins
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("src: cannot read module versions for %s", repo)
	}
	return srcPinnedCommitFromBuildInfo(repo, bi)
}

// Only the effective linked module can authenticate a release pin. A local
// workspace/replacement has no published revision in its build metadata, so
// it must fail closed rather than borrow the replaced version's commit or
// inspect an unrelated checkout at runtime.
func srcPinnedCommitFromBuildInfo(repo string, bi *debug.BuildInfo) (string, error) {
	pin, ok := srcModulePins[repo]
	if !ok || bi == nil {
		return "", fmt.Errorf("src: cannot read module versions for %s", repo)
	}
	for _, dep := range bi.Deps {
		if dep.Path != pin.module && (pin.upstream == "" || dep.Path != pin.upstream) {
			continue
		}
		mod := dep
		for mod.Replace != nil {
			mod = mod.Replace
		}
		if mod.Version == "" || mod.Version == "(devel)" {
			return "", fmt.Errorf("src: %s is an unversioned local module; no release commit can be authenticated", repo)
		}
		if mod.Path != pin.module {
			return "", fmt.Errorf("src: %s linked module %s is not the pinned module %s", repo, mod.Path, pin.module)
		}
		if !strings.HasSuffix(mod.Version, "-"+pin.commit[:12]) {
			return "", fmt.Errorf("src: %s pin is out of sync with the linked module version %s", repo, mod.Version)
		}
		return pin.commit, nil
	}
	return "", fmt.Errorf("src: %s module is absent from this build", repo)
}

// bashySrcCmd is the `bashy src` tree: yoke's pkg/src rod plus bashy's own
// run-local dispatcher verb (see src_local.go). Both the front-door mount
// and the cobra schema use this constructor so they never diverge.
func bashySrcCmd() *cobra.Command {
	cmd := src.NewSrcCmd()
	cmd.AddCommand(newSrcRunCmd())
	return cmd
}

func runSrc(args []string) int {
	cmd := bashySrcCmd()
	setCobraArgs(cmd, args)
	// The upstream src command reads one process VCS stamp. A bashy binary
	// contains several source repos, so select the linked repo's own pin for
	// show/fetch before executing its Cobra tree.
	if os.Getenv("BASHY_SRC_COMMIT") == "" && len(args) > 0 {
		repo := ""
		switch args[0] {
		case "show":
			if len(args) > 1 {
				repo, _, _ = src.Source(args[1])
			}
		case "fetch":
			repo = "yoke"
			if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
				repo = args[1]
			}
		}
		if repo != "" {
			pin, err := srcPinnedCommit(repo)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if pin != "" {
				_ = os.Setenv("BASHY_SRC_COMMIT", pin)
				defer os.Unsetenv("BASHY_SRC_COMMIT")
			}
		}
	}
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
