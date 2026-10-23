package agentos

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/qiangli/yoke/pkg/src"
)

// Full dependency revisions are needed by src fetch: Go's module build info
// carries only the 12-character pseudo-version suffix. Keep these in step
// with go.mod; the version check below fails closed after an unpaired bump.
var srcModulePins = map[string]struct{ module, commit string }{
	"yoke":        {"github.com/qiangli/yoke", "bdede1a9bd76ebe7f867a38e924dfe0ac2beb322"},
	"coreutils":   {"github.com/qiangli/coreutils", "42845e33d023245fd0631708986140cc512b186f"},
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

func runSrc(args []string) int {
	cmd := src.NewSrcCmd()
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
