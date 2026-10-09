package agentos

import (
	"fmt"
	"strings"
)

// gitNoopGlobals are real-git global options with no effect on the native
// engine (it never pages, locks optimistically, or prints advice hints).
var gitNoopGlobals = map[string]bool{
	"--no-pager": true, "-P": true, "--paginate": true, "-p": true,
	"--no-optional-locks": true, "--no-advice": true,
	"--no-replace-objects": true, "--no-lazy-fetch": true,
}

// gitValueGlobals are global options the native engine cannot honor; they
// are valid real-git syntax, so they get a pointer to --external instead of
// cobra's "unknown flag".
var gitValueGlobals = map[string]bool{
	"-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--config-env": true, "--exec-path": true, "--super-prefix": true,
}

// splitGitGlobals consumes git's global options that precede the verb the
// way real git does. -C DIR is returned (in order) for the caller to apply;
// no-op options are dropped; options the native engine cannot apply fail
// with a hint naming --external. Parsing stops at the first non-option, and
// at options it does not know (--version, --help, …), which pass through.
func splitGitGlobals(args []string) (dirs, rest []string, err error) {
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-C":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("no directory given for '-C'")
			}
			dirs = append(dirs, args[i+1])
			i += 2
		case gitNoopGlobals[a]:
			i++
		case gitValueGlobals[a] || gitValueGlobals[strings.SplitN(a, "=", 2)[0]]:
			return nil, nil, fmt.Errorf("git %s is not applied by bashy's native git engine; retry with --external to run it on a host git", a)
		default:
			return dirs, args[i:], nil
		}
	}
	return dirs, args[i:], nil
}

// gitAwdRun runs `bashy awd` (a seam: tests must not re-exec the binary).
var gitAwdRun = dispatchAwd

// gitAwdArgs rewrites `bashy git -C D1 [-C D2 ...] REST` as
// `awd D1 -- bashy git [-C D2 ...] REST`. Each hop runs in the previous
// directory, so repeated and relative -C resolve exactly as in git.
func gitAwdArgs(self string, dirs, rest []string) []string {
	args := []string{dirs[0], "--", self, "git"}
	for _, d := range dirs[1:] {
		args = append(args, "-C", d)
	}
	return append(args, rest...)
}
