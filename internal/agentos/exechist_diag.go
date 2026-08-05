// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import "context"

// The diagnosis hand-off between the advisor and the recorder.
//
// The advisor already does the hard part. On a failure it runs a probe-grounded
// pattern library — does that relative path exist at the repo root, is the host
// reachable under this network fingerprint, is the disk full or read-only, has
// this exact command failed three times in five minutes — and produces a
// DIMENSION (cwd | network | compute | disk | state | loop) plus whether
// re-running as-is could ever work.
//
// Then it prints one line to stderr and throws all of it away.
//
// That diagnosis is the difference between a pitfall that says "`go test
// ./hub/...` exits 1 here" and one that says "it exits 1 here because this
// coordinate cannot build it" — the second is actionable and the first is
// noise. So the recorder needs it, and there are only bad ways to get it except
// this one:
//
//   - Recomputing it in the recorder would re-run the filesystem probes on
//     every failure, and duplicate the one function whose answers must not
//     drift between two callers.
//   - Reordering the middleware so the advisor is outermost would put the
//     advisor's own stderr write inside the recorded span, and would still not
//     hand the value over.
//
// So the recorder puts a slot in the context on the way DOWN, and the advisor
// fills it on the way back up if — and only if — one is there. When the
// recorder is off there is no slot, the advisor's write is a nil check, and the
// cost is zero.

type diagKeyType struct{}

var diagKey diagKeyType

// diagSlot carries one command's diagnosis back to the recorder.
//
// No mutex: a slot belongs to exactly one command's handler chain, which is a
// single goroutine, and it is written at most once on the way back up.
type diagSlot struct {
	dimension string
	retryable bool
	filled    bool
}

// withDiagSlot attaches a fresh slot for the command about to run.
func withDiagSlot(ctx context.Context) (context.Context, *diagSlot) {
	s := &diagSlot{}
	return context.WithValue(ctx, diagKey, s), s
}

// diagFrom returns the slot for this command, or nil when nobody is collecting.
func diagFrom(ctx context.Context) *diagSlot {
	s, _ := ctx.Value(diagKey).(*diagSlot)
	return s
}

// recordDiagnosis is the advisor's side of the hand-off.
//
// It is called with whatever the advisor concluded, including nil — and nil is
// recorded as "classified, found nothing", which is a different claim from
// "never looked". Only the first write counts, so a later, less specific pass
// cannot overwrite a specific diagnosis.
func (s *diagSlot) recordDiagnosis(h *hint) {
	if s == nil || s.filled {
		return
	}
	s.filled = true
	if h == nil {
		return
	}
	s.dimension, s.retryable = h.dimension, h.retryable
}
