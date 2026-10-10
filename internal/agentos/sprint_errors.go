package agentos

import (
	"errors"
	"fmt"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/yoke/pkg/weave"
	"github.com/spf13/cobra"
)

// This file is the BASHY HALF of the sprint tree's self-reporting contract.
//
// yoke's weave.NewSprintCmd sets SilenceErrors/SilenceUsage on every command in
// the tree — subverbs print their own structured envelope and cobra must not
// double-print on top of it — and compensates by installing three reporters
// while it builds the tree: flag-parse failures (flagerr.go), positional-
// argument failures (argerr.go) and the errors a RunE returns itself
// (runerr.go). Anything that tree reports comes back as a STRUCTURED exit, and
// weave.IsStructuredExit is how a host is told "already printed, stay quiet".
//
// bashy then EDITS that finished tree (newSprintCmd): attachSprintWatch
// REPLACES start/take's RunE, and inbox-ack / monitor / wait are ADDED after
// yoke's reporters already walked it. Those bashy-owned paths are reported by
// nobody, and the host mapped every Execute() error to a bare `exit 1`. The
// result was the absence-of-evidence failure mode twice over — Sprint 412's
// `sprint start 412 --owner s412-manager --for 6h --watch` (todo 4a18c41ef997)
// and Sprint 329's `sprint inbox-ack 329 --as s329-manager` (todo 491a03436cbb)
// both exited 1 with ZERO bytes on both streams in an external manager harness,
// while the same commands WITHOUT --watch were loud. Two sprints lost time to a
// tool that refused and would not say so.
//
// The fix is structural rather than a message bolted onto the two reported
// call sites: the reporter is installed once by walking the tree AFTER bashy's
// edits, so a subcommand mounted here next year is loud without its author
// knowing this file exists. Errors yoke already reported pass through
// untouched, which is what keeps the message count at exactly one.

// sprintReportedError marks an error this file has ALREADY written to the
// command's own stderr, and carries the exit status the host should use. It is
// bashy's local equivalent of yoke's *exitCodeError, which is unexported — the
// host checks for it before falling back to printing, so nothing prints twice.
type sprintReportedError struct {
	err  error
	code int
}

func (e *sprintReportedError) Error() string { return e.err.Error() }
func (e *sprintReportedError) Unwrap() error { return e.err }
func (e *sprintReportedError) ExitCode() int { return e.code }

// sprintErrorReported reports whether err was already surfaced by this file.
func sprintErrorReported(err error) (int, bool) {
	var reported *sprintReportedError
	if errors.As(err, &reported) {
		return reported.code, true
	}
	return 0, false
}

// installSprintErrorReporting wraps every RunE in the tree so a bare error
// becomes a named, non-empty message on the command's own stderr plus a
// structured exit. Call it LAST, after every bashy edit, so the wrapper sits
// outside the closures bashy installed.
func installSprintErrorReporting(root *cobra.Command) {
	if root.RunE != nil {
		root.RunE = reportingSprintRunE(root.RunE)
	}
	for _, sub := range root.Commands() {
		installSprintErrorReporting(sub)
	}
}

// reportingSprintRunE reports one bare error exactly once.
//
// Errors that are already structured — yoke's own envelopes, and anything this
// reporter produced on an inner command — pass through: reporting again is the
// double-print yoke's SilenceErrors was set to prevent.
func reportingSprintRunE(orig func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := orig(cmd, args)
		if err == nil || weave.IsStructuredExit(err) {
			return err
		}
		if _, done := sprintErrorReported(err); done {
			return err
		}
		return reportSprintError(cmd, sprintExitCode(err), err)
	}
}

// reportSprintError writes err in the command's own output mode and returns it
// marked as reported.
func reportSprintError(cmd *cobra.Command, code int, err error) error {
	weavecli.EmitError(cmd.ErrOrStderr(), sprintErrOutputMode(cmd), cmd.CommandPath(), code, err)
	return &sprintReportedError{err: err, code: code}
}

// sprintExitCode classifies a bare error from a bashy-owned sprint path.
//
// yoke's runerr.go can assume exit 2: everything it wraps is a GUARD that
// rejected the invocation before the store was opened. bashy's additions are
// not all guards — attachSprintWatch's own argument checks are, but the watch
// loop it then enters runs for hours and can fail on state (the seat was taken
// over, the owning harness exited). Calling a lease that moved under us an
// "invalid argument" would be a lie to any agent reading the code, so the
// classification is explicit: usage-shaped failures keep yoke's exit 2, and a
// failure that happened while the command was doing its job is a generic
// failure (exit 1) — the status those paths already returned, so nothing that
// reads this exit status changes. Only the silence does.
func sprintExitCode(err error) int {
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	var usage *sprintUsageError
	if errors.As(err, &usage) {
		return weavecli.ExitInvalidArg
	}
	return weavecli.ExitGenericFail
}

// sprintUsageError marks a bashy-owned sprint failure that rejected the
// INVOCATION — a bad sprint number, an unusable identity, a precondition the
// caller can fix by typing something else. It exists so sprintExitCode does not
// have to guess from message text.
type sprintUsageError struct{ err error }

func (e *sprintUsageError) Error() string { return e.err.Error() }
func (e *sprintUsageError) Unwrap() error { return e.err }

func sprintUsage(err error) error {
	if err == nil {
		return nil
	}
	return &sprintUsageError{err: err}
}

func sprintUsagef(format string, a ...any) error {
	return sprintUsage(fmt.Errorf(format, a...))
}

// sprintErrOutputMode mirrors yoke's flagErrOutputMode: the reporter renders in
// whatever mode the invocation asked for, so an agent driving --json gets an
// envelope and a human gets a line.
func sprintErrOutputMode(cmd *cobra.Command) weavecli.OutputMode {
	boolFlag := func(name string) (set, val bool) {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			return false, false
		}
		return f.Changed, f.Value.String() == "true"
	}
	jsonSet, jsonVal := boolFlag("json")
	_, plain := boolFlag("plain")
	_, quiet := boolFlag("quiet")
	return weavecli.ResolveOutputModeEx(jsonSet, jsonVal, plain, quiet)
}
