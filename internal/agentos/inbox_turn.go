package agentos

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/admission"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/meet"
)

// unifiedTurnItems adapts the host-owned sources to Bus's bounded, per-record
// turn admission. The legacy whole-batch hook could exceed a PTY's input limit;
// its deferred acknowledgement then replayed the same role mail every turn.
func unifiedTurnItems(reader string) ([]admission.Item, error) {
	batch, err := snapshotUnifiedInbox(reader, 0, false)
	if err != nil {
		return nil, err
	}
	roleTopics := hostRoleTopics()
	items := make([]admission.Item, 0, len(batch.events))
	for _, event := range batch.events {
		e := event
		id := turnInboxID(e)
		priority := admission.PriorityInformational
		if e.To != "" {
			priority = admission.PriorityDirected
		}
		items = append(items, admission.Item{
			Source: e.Source, ID: strconv.FormatInt(e.Seq, 10), Sequence: e.Seq, From: e.From,
			To: e.To, Topic: e.Topic, Priority: priority, Body: e.Body,
			ArtifactRef: "bashy inbox read --as " + strconv.Quote(reader) + " " + strconv.Quote(id),
			OverflowRef: "bashy inbox --as " + strconv.Quote(reader) + " --peek",
			Acknowledge: func() error { return acknowledgeTurnInboxItem(reader, e, batch.rawEvents, roleTopics) },
		})
	}
	return items, nil
}

func hostRoleTopics() map[string]string {
	roleTopics := map[string]string{}
	if bus.HostRoles != nil {
		for _, role := range bus.HostRoles() {
			roleTopics["role:"+role.Label] = role.Topic
		}
	}
	return roleTopics
}

func turnInboxID(e unifiedInboxEvent) string {
	if e.Source == "meet" {
		return fmt.Sprintf("meet:%s:%d", e.Room, e.Seq)
	}
	return fmt.Sprintf("%s:%d", e.Source, e.Seq)
}

func acknowledgeTurnInboxItem(reader string, event unifiedInboxEvent, raw []unifiedInboxEvent, roles map[string]string) error {
	ids := []string{turnInboxID(event)}
	if event.Source == "mb" {
		for _, copy := range raw {
			if copy.Source == "meet" && copy.Origin != nil && strings.EqualFold(copy.Origin.Source, "mb") && copy.Origin.Seq == event.Seq {
				ids = append(ids, turnInboxID(copy))
			}
		}
	}
	spec := mailboxSpec{Key: "agent:" + reader, Address: reader, Kind: "agent"}
	if err := updateMailboxState(spec, func(state *mailboxState) error {
		now := time.Now().UTC().Format(time.RFC3339)
		for _, id := range ids {
			mark := state.Marks[id]
			if mark.AckedAt == "" {
				mark.ReadAt, mark.AckedAt = now, now
				mark.Preserved = false
				state.Marks[id] = mark
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if event.Source == "bus" {
		snapshot, err := bus.SnapshotInbox(reader)
		if err != nil {
			return err
		}
		return snapshot.CommitItem(event.Seq)
	}
	if strings.HasPrefix(event.Source, "role:") {
		topic := roles[event.Source]
		if topic == "" {
			return fmt.Errorf("inbox: role %s has no source topic", event.Source)
		}
		snapshot, err := bus.SnapshotInbox(topic)
		if err != nil {
			return err
		}
		return snapshot.CommitItem(event.Seq)
	}
	if err := advanceTurnSource(reader, event.Source, event.Room, raw, spec); err != nil {
		return err
	}
	if event.Source == "mb" {
		for _, copy := range raw {
			if copy.Source == "meet" && copy.Origin != nil && strings.EqualFold(copy.Origin.Source, "mb") && copy.Origin.Seq == event.Seq {
				if err := advanceTurnSource(reader, "meet", copy.Room, raw, spec); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Advance only the contiguous acknowledged source prefix. Admission may select
// a later urgent record before an earlier routine one, so the latter must keep
// the native cursor from jumping past it.
func advanceTurnSource(reader, source, roomID string, raw []unifiedInboxEvent, spec mailboxSpec) error {
	if source != "mb" && source != "meet" {
		return nil
	}
	state, err := loadMailboxState(spec)
	if err != nil {
		return err
	}
	var events []unifiedInboxEvent
	for _, e := range raw {
		if e.Source == source && (source != "meet" || e.Room == roomID) {
			events = append(events, e)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	var through int64
	for _, e := range events {
		if state.Marks[turnInboxID(e)].AckedAt == "" {
			break
		}
		through = e.Seq
	}
	if through == 0 {
		return nil
	}
	if source == "mb" {
		return bus.MarkSeen(reader, through)
	}
	return meet.MarkSeenThrough(roomID, reader, through)
}
