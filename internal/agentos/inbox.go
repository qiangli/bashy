package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/spf13/cobra"
)

const unifiedInboxSchema = "bashy-inbox-v1"

type unifiedInboxEvent struct {
	Schema string              `json:"schema"`
	Source string              `json:"source"`
	Seq    int64               `json:"seq"`
	At     string              `json:"at,omitempty"`
	From   string              `json:"from,omitempty"`
	To     string              `json:"to,omitempty"`
	Topic  string              `json:"topic,omitempty"`
	Room   string              `json:"room,omitempty"`
	Body   string              `json:"body"`
	Origin *unifiedInboxOrigin `json:"origin,omitempty"`
}

type unifiedInboxOrigin struct {
	Source string `json:"source"`
	Seq    int64  `json:"seq"`
}

type inboxBatch struct {
	events []unifiedInboxEvent
	acks   []func() error
	warns  []string
}

func newUnifiedInboxCmd() *cobra.Command {
	var as string
	var wait time.Duration
	var peek, jsonOut, watch bool
	var limit int
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "read or watch every inbound Bashy communication channel",
		Long: `inbox is one read-through view over the existing message board, Meet
boards, Bus pending buffers, and steward/conductor role addresses. It creates no
message store and keeps each source's own cursor.

  bashy inbox                         drain everything waiting for you
  bashy inbox --peek                  inspect without advancing any cursor
  bashy inbox --wait 15m              wait for one batch, then return
  bashy inbox --watch                 follow new inputs until interrupted
  bashy inbox --watch --wait 15m      follow for at most 15 minutes

For a model-driven coordination sentinel, assign one distinct registered Bashy
identity (verify with 'bashy agents show NAME'), invite it to assigned Meet
boards, route/subscribe its own inputs, and repeat
'bashy inbox --as NAME --wait 60s'. It returns on the first batch so the agent
can respond before re-entering until its assignment deadline. Reserve --watch
for a human or sidecar stream. Surface every request promptly;
prioritize directed, BLOCKED, CONFLICT, ownership, baseline, and merge inputs.
If action is not immediate, acknowledge receipt with owner, action, and ETA.
Never read as another identity, silently consume, duplicate claimed work, or
impersonate decision authority. A sentinel sees only sources routed to its own
identity; invite/subscribe/address it explicitly. Its reply must say the
sentinel routed the request and the supervisor has not read it. On expiry,
handoff processed/outstanding counts and last source sequences. See
'bashy skills show check-messages'.

NAME owns the address and cursor; NICK/aliases do not create another inbox.
Never share one registered NAME between agents. Separate concurrent topic
watchers need separate registered names. Keep messages short: request/decision,
priority, owner/expected response, and a stable repo-relative + commit/issue/
room/artifact reference; never send only an inaccessible temporary path.

A sentinel that exits must say monitoring ENDED, why/deadline, last processed
provenance, outstanding status, and who resumes coverage. It must never promise
continued monitoring after its process or assignment ends.

Bashy-owned agent sessions receive the same view once at each real turn
boundary. A session started outside Bashy has no authenticated control channel
to steer; use this command (or --watch) explicitly. Bashy never guesses a PID or
pretends such a session was adopted.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if wait < 0 {
				return fmt.Errorf("inbox: --wait must not be negative")
			}
			if limit < 0 {
				return fmt.Errorf("inbox: --limit must not be negative")
			}
			if watch && peek {
				return fmt.Errorf("inbox: --watch cannot be combined with --peek (the same unread batch would repeat forever)")
			}
			if watch && limit > 0 {
				return fmt.Errorf("inbox: --watch cannot be combined with --limit (a capped source intentionally remains unread)")
			}
			reader, err := resolveInboxReader(as)
			if err != nil {
				return err
			}
			return runUnifiedInbox(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), reader, limit, peek, jsonOut, watch, wait)
		},
	}
	f := cmd.Flags()
	f.StringVar(&as, "as", "", "read as this identity (required when an external agent session cannot be attributed)")
	f.DurationVar(&wait, "wait", 0, "wait up to this duration for input (with --watch: total watch bound)")
	f.BoolVar(&peek, "peek", false, "read without advancing any source cursor")
	f.BoolVar(&watch, "watch", false, "follow all inbound sources until interrupted")
	f.IntVarP(&limit, "limit", "n", 0, "show at most this many records per source (0 = no cap; a capped source remains unread)")
	f.BoolVar(&jsonOut, "json", false, "emit one "+unifiedInboxSchema+" object per line (NDJSON)")
	return cmd
}

// resolveInboxReader prevents an authenticated Bashy agent from borrowing a
// different identity's cursors. External sessions may name themselves, but the
// name must resolve to a registered agent rather than minting an arbitrary
// cursor by typo. Role backlog is reachable only through its current holder.
func resolveInboxReader(as string) (string, error) {
	principal := strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
	if strings.Contains(principal, "agent/") {
		self, err := bus.BoardIdentity("")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(as) == "" {
			return self, nil
		}
		requested, err := bus.BoardIdentity(as)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(self, requested) {
			return "", fmt.Errorf("inbox: authenticated agent %q cannot read as %q; each registered name owns its own cursors", self, requested)
		}
		return self, nil
	}
	if strings.TrimSpace(as) == "" {
		return bus.BoardIdentity("")
	}
	addr, kind, ok := bus.ResolveSendTarget(as)
	if !ok || kind != bus.TargetAgent {
		return "", fmt.Errorf("inbox: --as %q is not a registered Bashy agent; verify with `bashy agents list --all` and `bashy whois agent:%s`", as, as)
	}
	return addr, nil
}

func runUnifiedInbox(ctx context.Context, out, errOut io.Writer, reader string, limit int, peek, jsonOut, watch bool, bound time.Duration) error {
	deadline := time.Time{}
	if bound > 0 {
		deadline = time.Now().Add(bound)
	}
	for {
		batch, err := snapshotUnifiedInbox(reader, limit, true)
		if err != nil {
			return err
		}
		if len(batch.events) > 0 {
			if err := renderInboxBatch(out, errOut, batch, jsonOut); err != nil {
				return err
			}
			if !peek {
				for _, ack := range batch.acks {
					if err := ack(); err != nil {
						return fmt.Errorf("inbox: acknowledge rendered source: %w", err)
					}
				}
			}
			if !watch {
				return nil
			}
		} else if !watch && bound == 0 {
			fmt.Fprintf(errOut, "nothing new in any channel for %s\n", reader)
			return nil
		}
		if !watch && !deadline.IsZero() && !time.Now().Before(deadline) {
			fmt.Fprintln(errOut, "EMPTY (timeout)")
			return nil
		}
		if watch && !deadline.IsZero() && !time.Now().Before(deadline) {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if watch {
				return nil
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// snapshotUnifiedInbox only READS. Its ack closures are called after the whole
// rendered batch has reached stdout, so a broken pipe cannot silently consume a
// message. Each closure carries the exact per-source high-water mark observed.
func snapshotUnifiedInbox(reader string, limit int, includeBus bool) (inboxBatch, error) {
	var batch inboxBatch
	appendLimited := func(source string, events []unifiedInboxEvent, ack func() error) {
		shown := events
		capped := limit > 0 && len(events) > limit
		if capped {
			shown = events[:limit]
			batch.warns = append(batch.warns, fmt.Sprintf("%s: %d more record(s) remain unread because of --limit", source, len(events)-limit))
		}
		batch.events = append(batch.events, shown...)
		if len(shown) > 0 && !capped && ack != nil {
			batch.acks = append(batch.acks, ack)
		}
	}

	// Message board, including steward/conductor messages now addressed to the
	// stable role rather than copied into a role-specific store.
	directed, other, _, err := bus.Unseen(reader, 0)
	if err != nil {
		return batch, fmt.Errorf("message board: %w", err)
	}
	posts := append(directed, other...)
	mbEvents := make([]unifiedInboxEvent, 0, len(posts))
	var mbHigh int64
	for _, p := range posts {
		if p.Seq > mbHigh {
			mbHigh = p.Seq
		}
		mbEvents = append(mbEvents, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "mb", Seq: p.Seq, At: p.At, From: p.From, To: bus.RoleLabelFor(p.To), Topic: p.Topic, Body: p.Body})
	}
	appendLimited("mb", mbEvents, func() error { return bus.MarkSeen(reader, mbHigh) })

	// Every Meet board is a channel. Ordinary chaired meetings are deliberately
	// excluded: their transcript is a record, not a participant inbox.
	rooms, err := meet.Rooms()
	if err != nil {
		return batch, fmt.Errorf("meet rooms: %w", err)
	}
	for _, room := range rooms {
		if !room.Board || !stringMember(room.Members, reader) {
			continue
		}
		d, o, _, through, err := meet.UnreadRecords(room.ID, reader, 0)
		if err != nil {
			return batch, fmt.Errorf("meet room %s: %w", room.ID, err)
		}
		events := append(d, o...)
		out := make([]unifiedInboxEvent, 0, len(events))
		for _, record := range events {
			e := record.Event
			item := unifiedInboxEvent{Schema: unifiedInboxSchema, Source: "meet", Seq: record.Seq, At: e.TS.Format(time.RFC3339Nano), From: e.Speaker, To: e.To, Topic: e.Kind, Room: room.ID, Body: e.Text}
			if e.Origin != nil {
				item.Origin = &unifiedInboxOrigin{Source: e.Origin.Source, Seq: e.Origin.Seq}
			}
			out = append(out, item)
		}
		id := room.ID
		appendLimited("meet:"+id, out, func() error { return meet.MarkSeenThrough(id, reader, through) })
	}

	if includeBus {
		snapshot, err := bus.SnapshotInbox(reader)
		if err != nil {
			return batch, fmt.Errorf("bus notifications: %w", err)
		}
		appendLimited("bus", pendingEvents("bus", snapshot.Items), snapshot.Commit)
	}

	// Compatibility drain for pre-board role pings already durable in the old
	// role buffers. New role messages arrive through MB above; no sixth store is
	// created and this path can disappear after the retained backlog is empty.
	if bus.HostRoles != nil {
		for _, role := range bus.HostRoles() {
			if !strings.EqualFold(strings.TrimSpace(role.Holder), strings.TrimSpace(reader)) {
				continue
			}
			snapshot, err := bus.SnapshotInbox(role.Topic)
			if err != nil {
				return batch, fmt.Errorf("role %s: %w", role.Label, err)
			}
			out := pendingEvents("role:"+role.Label, snapshot.Items)
			appendLimited("role:"+role.Label, out, snapshot.Commit)
		}
	}
	batch.events = collapseProvenanceDuplicates(batch.events)
	return batch, nil
}

// collapseProvenanceDuplicates removes only a Meet copy whose structured
// origin names an MB record present in the same rendered batch. Rendered prose
// is never a key: independently repeated text remains independently visible.
// Source acknowledgements stay in the batch, so successful rendering advances
// both the MB and Meet watermarks; a render failure advances neither.
func collapseProvenanceDuplicates(events []unifiedInboxEvent) []unifiedInboxEvent {
	mb := make(map[int64]struct{})
	for _, event := range events {
		if event.Source == "mb" {
			mb[event.Seq] = struct{}{}
		}
	}
	out := events[:0]
	for _, event := range events {
		if event.Source == "meet" && event.Origin != nil && strings.EqualFold(event.Origin.Source, "mb") {
			if _, ok := mb[event.Origin.Seq]; ok {
				continue
			}
		}
		out = append(out, event)
	}
	return out
}

func pendingEvents(source string, items []bus.Pending) []unifiedInboxEvent {
	out := make([]unifiedInboxEvent, 0, len(items))
	for _, p := range items {
		out = append(out, unifiedInboxEvent{Schema: unifiedInboxSchema, Source: source, Seq: p.Seq, At: p.TS, From: p.Principal, To: p.To, Topic: p.Topic, Room: p.Room, Body: p.Body})
	}
	return out
}

func renderInboxBatch(out, errOut io.Writer, batch inboxBatch, jsonOut bool) error {
	var rendered bytes.Buffer
	if jsonOut {
		enc := json.NewEncoder(&rendered)
		for _, event := range batch.events {
			if err := enc.Encode(event); err != nil {
				return err
			}
		}
	} else {
		for _, event := range batch.events {
			where := event.Source
			if event.Room != "" {
				where += "/" + event.Room
			}
			fmt.Fprintf(&rendered, "[%s:%d] %s → %s", where, event.Seq, emptyAs(event.From, "unknown"), emptyAs(event.To, "all"))
			if event.Topic != "" {
				fmt.Fprintf(&rendered, " (%s)", event.Topic)
			}
			fmt.Fprintf(&rendered, "\n%s\n\n", event.Body)
		}
	}
	n, err := out.Write(rendered.Bytes())
	if err != nil {
		return err
	}
	if n != rendered.Len() {
		return io.ErrShortWrite
	}
	for _, warning := range batch.warns {
		fmt.Fprintln(errOut, warning)
	}
	return nil
}

func stringMember(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// unifiedTurnPreamble is wired into coreutils/chat. Bus pending is already
// drained by bus.TurnPreamble itself; this callback adds every other source in
// one bounded block and acknowledges only after rendering that block in memory.
func unifiedTurnPreamble(agent string) bus.PreparedPreamble {
	batch, err := snapshotUnifiedInbox(agent, 0, false)
	if err != nil {
		warning := fmt.Sprintf("[Bashy unified inbox warning]\nCould not read every inbound source: %v\nNo source cursor was advanced; run `bashy inbox --as %s`.", err, agent)
		return bus.NewPreparedPreamble(warning, nil)
	}
	if len(batch.events) == 0 {
		return bus.PreparedPreamble{}
	}
	var out bytes.Buffer
	if err := renderInboxBatch(&out, io.Discard, batch, false); err != nil {
		return bus.PreparedPreamble{}
	}
	ack := func() error {
		for _, ack := range batch.acks {
			if err := ack(); err != nil {
				return err
			}
		}
		return nil
	}
	text := "[Bashy unified inbox — read before the instruction below]\n" + strings.TrimSpace(out.String())
	return bus.NewPreparedPreamble(text, ack)
}
