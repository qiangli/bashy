package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/dag"
)

func applyBashySelfArgv0() {
	self := adapter.BashySelf()
	adapter.StripAdapterEnv()
	if self != "" && len(os.Args) > 0 {
		os.Args[0] = self
	}
}

func runDagArgs(args []string, cmdFn func() (*os.File, *os.File)) int {
	applyBashySelfArgv0()
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, dag.CapacityServices{})
	if cmdFn != nil {
		out, errOut := cmdFn()
		if out != nil {
			cmd.SetOut(out)
		}
		if errOut != nil {
			cmd.SetErr(errOut)
		}
	}
	return adapter.RunCobraArgs(cmd, args, "", func(err error) int {
		var dagErr *dag.Error
		if err != nil && !errors.As(err, &dagErr) {
			fmt.Fprintf(cmd.ErrOrStderr(), "bashy dag: %v\n", err)
		}
		return dag.ExitCodeOf(err)
	})
}

func main() {
	os.Exit(runDagArgs(os.Args[1:], nil))
}
