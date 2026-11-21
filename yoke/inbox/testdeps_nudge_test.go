package main

import (
	"fmt"
	"maps"
	"strings"
)

// unreadSummary is the attributable shape behind one hint: how many, from
// which sources, from whom — never bodies.
type unreadSummary struct {
	count      int
	incomplete bool
	bySource   []string // "mb:2", "meet/room:1", ... in first-seen order
	senders    []string // distinct non-empty From, capped, in first-seen order
}

// summarizeUnreadHint reduces a read-only snapshot to its attributable shape.
// Empty batches (no unread) report ok=false: silence, not "no mail".
func summarizeUnreadHint(batch inboxBatch) (unreadSummary, bool) {
	s := unreadSummary{incomplete: batch.hintIncomplete}
	counts := map[string]int{}
	var order []string
	seenSender := map[string]bool{}
	for _, e := range batch.events {
		s.count++
		src := e.Source
		if e.Room != "" {
			src += "/" + e.Room
		}
		if counts[src] == 0 {
			order = append(order, src)
		}
		counts[src]++
		from := strings.TrimSpace(e.From)
		if from == "" || strings.EqualFold(from, "unknown") {
			continue
		}
		if !seenSender[from] {
			seenSender[from] = true
			if len(s.senders) < 3 {
				s.senders = append(s.senders, from)
			}
		}
	}
	if s.count == 0 && !s.incomplete {
		return unreadSummary{}, false
	}
	for _, src := range order {
		s.bySource = append(s.bySource, fmt.Sprintf("%s:%d", src, counts[src]))
	}
	return s, true
}

// formatUnreadHint renders the concise model-visible line: unread count,
// source attribution, sender attribution, and the one reader to run.
func formatUnreadHint(s unreadSummary) string {
	var b strings.Builder
	if s.incomplete && s.count == 0 {
		return "you may have unread mail — run `bashy inbox`"
	}
	if s.incomplete {
		fmt.Fprintf(&b, "you have at least %d unread (%s)", s.count, strings.Join(s.bySource, ", "))
	} else {
		fmt.Fprintf(&b, "you have %d unread (%s)", s.count, strings.Join(s.bySource, ", "))
	}
	if len(s.senders) > 0 {
		fmt.Fprintf(&b, " from %s", strings.Join(s.senders, ", "))
	}
	b.WriteString(" — run `bashy inbox`")
	return b.String()
}

// liveUnreadSnapshot is the read-only unread view, shared with managed turn
// delivery. includeBus=false matches the turn preamble: bus pending is drained
// by bus.TurnPreamble itself and must not be double-reported here.
func liveUnreadSnapshot(reader string) (inboxBatch, error) {
	return snapshotUnifiedInbox(reader, 0, false)
}

// localUnreadSnapshot is liveUnreadSnapshot without the relay delivery pass:
// the per-command hint runs on every verb and must never wait on the network.
func localUnreadSnapshot(reader string) (inboxBatch, error) {
	return snapshotInboxWithHint(reader, 0, false, false, true)
}

// checkUnreadArrivals is the arrival-scoped gate. fp is the store fingerprint
// for reader (fpOK=false when it could not be sampled); snapshot reads unread
// mail without consuming it; refingerprint re-samples the stores after a read
// (nil skips this). It returns the hint text ("" when silent) and the next
// persisted state, plus whether a snapshot ran (the caller saves state only
// then, so a failed snapshot retries next turn instead of suppressing).
//
// The re-sample matters because the first read lazily materializes cursor
// files, which moves the fingerprint without any mail arriving: recording the
// pre-read generation would re-hint on the very next turn. The recorded
// generation is therefore the post-read one, re-reading (bounded) while the
// store keeps moving under us so a racing arrival is included, not missed.
func checkUnreadArrivals(reader string, fp uint64, fpOK bool, st inboxHintArrivals, snapshot func(string) (inboxBatch, error), refingerprint func(string) (uint64, bool)) (string, inboxHintArrivals, bool) {
	if fpOK && fp != 0 {
		if st.Hinted[reader] == fp || st.Probed[reader] == fp {
			return "", st, false // announced, or probed quiet since the last change
		}
	}
	pre, preOK := fp, fpOK
	var batch inboxBatch
	var err error
	for tries := 0; tries < 3; tries++ {
		if batch, err = snapshot(reader); err != nil {
			return "", st, false
		}
		if refingerprint == nil {
			break
		}
		post, postOK := refingerprint(reader)
		if !postOK || post == 0 || (preOK && post == pre) {
			break
		}
		pre, preOK = post, postOK
	}
	next := inboxHintArrivals{
		Probed: maps.Clone(st.Probed),
		Hinted: maps.Clone(st.Hinted),
	}
	if next.Probed == nil {
		next.Probed = map[string]uint64{}
	}
	if next.Hinted == nil {
		next.Hinted = map[string]uint64{}
	}
	if preOK && pre != 0 {
		next.Probed[reader] = pre
	}
	sum, ok := summarizeUnreadHint(batch)
	if !ok {
		return "", next, true
	}
	if preOK && pre != 0 {
		next.Hinted[reader] = pre
	}
	return formatUnreadHint(sum), next, true
}
