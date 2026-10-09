package agentos

import (
	"fmt"
	"os"
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

// applyGitDirs performs the -C directory changes cumulatively, like git.
func applyGitDirs(dirs []string) error {
	for _, d := range dirs {
		if err := os.Chdir(d); err != nil {
			return &gitExitError{code: 128, msg: fmt.Sprintf("fatal: cannot change to '%s': %s", d, chdirReason(err))}
		}
	}
	return nil
}

func chdirReason(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		err = pe.Err
	}
	s := err.Error()
	if s != "" {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
