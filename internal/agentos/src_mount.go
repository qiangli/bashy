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
var srcModulePins = map[string]struct{ module, commit string }{
	"yoke":        {"github.com/qiangli/yoke", "281e79d3243dbba184963d9c4de712d3f0c5e147"},
	"coreutils":   {"github.com/qiangli/coreutils", "211789fadf75a29b73772b8ab2923ac03b6935ec"},
	"sh":          {"github.com/qiangli/sh/v3", "6663c93a2d3cd3528a8d270080cfb10ead828135"},
	"filebrowser": {"github.com/qiangli/filebrowser/v2", "d1d2feab447319ff2b37b471829b59bc3cace390"},
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
	pin, ok := srcModulePins[repo]
	if !ok {
		return "", nil // managed externals and providers have their own pins
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("src: cannot read module versions for %s", repo)
	}
	for _, dep := range bi.Deps {
		for mod := dep; mod != nil; mod = mod.Replace {
			if mod.Path != pin.module {
				continue
			}
			if !strings.HasSuffix(mod.Version, "-"+pin.commit[:12]) {
				return "", fmt.Errorf("src: %s pin is out of sync with the linked module version %s", repo, mod.Version)
			}
			return pin.commit, nil
		}
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
