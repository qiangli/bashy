// Command sprint is the standalone yoke/sprint adapter for `bashy sprint`.
//
// Every other file in this package is a DUPLICATE of the glue the in-process
// front door uses in internal/agentos: the active conductor loop (managed
// sprint owner), the sprint monitor, attachSprintWatch and the inbox stream it
// attaches, inbox-ack, wait and await. It is duplicated on purpose: the
// operator rule is that no non-internal command depends on another, even if
// that means duplicate code. Keep this package free of imports of any other
// bashy or yoke command package (AssertStandaloneDeps enforces it).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/weave"
)

func main() {
	os.Exit(runSprint(os.Args[1:], os.Stdout, os.Stderr))
}

// runSprint mirrors agentos runSprintDispatch: a structured exit or an error
// sprint_errors.go already reported stays silent and keeps its code; anything
// else is surfaced as "bashy sprint: ERR".
func runSprint(args []string, stdout, stderr io.Writer) int {
	// The host process wires these in agentos init(); this binary is its own
	// host, so it wires its own copies.
	wireMessageBoard()
	wireSprintOwner()
	cmd := newSprintCmd()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return adapter.RunCobraArgs(cmd, args, "", func(err error) int {
		if code, reported := sprintErrorReported(err); reported {
			return code
		}
		if weave.IsStructuredExit(err) {
			return weave.ExitCode(err)
		}
		fmt.Fprintf(stderr, "bashy sprint: %v\n", err)
		return weave.ExitCode(err)
	})
}
