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
	"yoke":        {"github.com/qiangli/yoke", "3c473f9428eced37ccc8f2a1cb73fd7f652c58fc"},
	"coreutils":   {"github.com/qiangli/coreutils", "ffa3754fa6df8bea8e9de5a957d6995620e6c0b3"},
	"sh":          {"github.com/qiangli/sh/v3", "de35c153b5f647eb988035abf4a7895552a3869f"},
	"filebrowser": {"github.com/qiangli/filebrowser/v2", "d1d2feab447319ff2b37b471829b59bc3cace390"},
}

func srcPinnedCommit(repo string) (string, error) {
	if repo == "bashy" {
		return src.ReleaseCommit(), nil
	}
	if repo == "managed" || repo == "posix-provider" {
		return "unknown", nil // each external has its own independent pin
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
