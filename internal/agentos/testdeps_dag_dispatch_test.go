package agentos

import (
	"errors"
	"fmt"
	"github.com/qiangli/yoke/pkg/dag"
	"io"
)

// runDagDispatch mounts the DAG runner with the capacity endpoint and runs it.
//
// Two things here are load-bearing. dag.NewDagCmd sets SilenceErrors, so an
// error cobra raises BEFORE RunE (an unknown flag, a parse failure — and, until
// coreutils 539b8fa7 declared the root's positional contract, every target
// once `capacity` was mounted) reaches this caller as the only evidence that
// anything went wrong. Mapping it straight to an exit code, as this used to,
// produced a silent `exit 1` that two sprint-163 workers spent their last ten
// minutes on. dag's own failures are *dag.Error values that emitErr has
// already written as an envelope; those are not printed twice.
func runDagDispatch(args []string, stdout, stderr io.Writer) int {
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, sprintCapacityServices())
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	setCobraArgs(cmd, args)
	err := cmd.Execute()
	var dagErr *dag.Error
	if err != nil && !errors.As(err, &dagErr) {
		fmt.Fprintf(stderr, "bashy dag: %v\n", err)
	}
	return dag.ExitCodeOf(err)
}
