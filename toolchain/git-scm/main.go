package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/gitscm"
)

func splitGitExternal(args []string) (bool, []string, error) {
	var external bool
	var rest []string
	for _, a := range args {
		if a == "--external" {
			external = true
			continue
		}
		if v, ok := strings.CutPrefix(a, "--external="); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return false, nil, fmt.Errorf("invalid --external value %q: want true or false", v)
			}
			external = b
			continue
		}
		rest = append(rest, a)
	}
	return external, rest, nil
}

func run(args []string) int {
	_, rest, err := splitGitExternal(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd := gitscm.NewGitSCMCmd()
	cmd.SilenceErrors = true
	return adapter.RunCobraArgs(cmd, rest, "", func(err error) int {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return adapter.ChildExitStatus(err)
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	})
}

func main() {
	os.Exit(run(os.Args[1:]))
}
