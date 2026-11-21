// Command inbox is the standalone yoke/inbox adapter for `bashy inbox`.
//
// Every other file in this package is a DUPLICATE of the glue the in-process
// front door uses in internal/agentos (session relays, turn delivery, the
// watcher claim and the poll loop). It is duplicated on purpose: the operator
// rule is that no non-internal command depends on another, even if that means
// duplicate code. Keep this package free of imports of any other bashy or yoke
// command package (AssertStandaloneDeps enforces it).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
)

func main() {
	os.Exit(runInbox(os.Args[1:], os.Stdout, os.Stderr))
}

// runInbox mirrors the agentos `case "inbox"` contract: silent exits stay
// silent, everything else is reported as "bashy inbox: ERR", and the exit
// status comes from inboxCommandExitCode.
func runInbox(args []string, stdout, stderr io.Writer) int {
	wireMessageBoard()
	cmd := newUnifiedInboxCmd()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return adapter.RunCobraArgs(cmd, args, "", func(err error) int {
		if !inboxSilentExit(err) {
			fmt.Fprintf(stderr, "bashy inbox: %v\n", err)
		}
		return inboxCommandExitCode(err)
	})
}
