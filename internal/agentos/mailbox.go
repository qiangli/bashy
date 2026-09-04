package agentos

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/lockfile"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/spf13/cobra"
)

const mailboxSchema = "bashy-mailbox-v1"

type mailboxSpec struct{ Key, Address, Kind string }
type mailboxMark struct {
	ReadAt    string `json:"read_at,omitempty"`
	AckedAt   string `json:"acked_at,omitempty"`
	Preserved bool   `json:"preserved,omitempty"`
	Project   string `json:"project,omitempty"`
	Status    string `json:"status,omitempty"`
}
type mailboxState struct {
	Schema string                 `json:"schema"`
	Marks  map[string]mailboxMark `json:"marks"`
}
type mailboxItem struct {
	Schema       string              `json:"schema"`
	ID           string              `json:"id"`
	Source       string              `json:"source"`
	Seq          int64               `json:"seq"`
	At           string              `json:"at,omitempty"`
	From         string              `json:"from,omitempty"`
	To           string              `json:"to,omitempty"`
	Topic        string              `json:"topic,omitempty"`
	Project      string              `json:"project,omitempty"`
	Status       string              `json:"status,omitempty"`
	Room         string              `json:"room,omitempty"`
	Body         string              `json:"body"`
	Read         bool                `json:"read"`
	Acknowledged bool                `json:"acknowledged"`
	Preserved    bool                `json:"preserved"`
	Origin       *unifiedInboxOrigin `json:"origin,omitempty"`
}

func currentHumanMailbox() (mailboxSpec, error) {
	u, err := user.Current()
	if err != nil {
		return mailboxSpec{}, fmt.Errorf("inbox human: resolve current OS user: %w", err)
	}
	if strings.TrimSpace(u.Username) == "" {
		return mailboxSpec{}, fmt.Errorf("inbox human: current OS user has no username")
	}
	name := u.Username
	return mailboxSpec{Key: "human:" + name, Address: name, Kind: "human"}, nil
}

func currentAgentMailbox(as string) (mailboxSpec, error) {
	name, err := resolveInboxReader(as)
	if err != nil {
		return mailboxSpec{}, err
	}
	return mailboxSpec{Key: "agent:" + name, Address: name, Kind: "agent"}, nil
}

func mailboxDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_MAILBOX_DIR")); d != "" {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
		_ = os.Chmod(d, 0o700)
		return d, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(u.HomeDir) == "" {
		return "", fmt.Errorf("inbox: current OS user has no home directory")
	}
	d := filepath.Join(u.HomeDir, ".bashy", "inbox")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(d, 0o700)
	return d, nil
}

func mailboxStatePath(spec mailboxSpec) (string, error) {
	d, err := mailboxDir()
	if err != nil {
		return "", err
	}
	name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(spec.Key)
	return filepath.Join(d, name+".json"), nil
}

func loadMailboxState(spec mailboxSpec) (mailboxState, error) {
	s := mailboxState{Schema: mailboxSchema, Marks: map[string]mailboxMark{}}
	p, err := mailboxStatePath(spec)
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("inbox: read mailbox state: %w", err)
	}
	if s.Marks == nil {
		s.Marks = map[string]mailboxMark{}
	}
	return s, nil
}

func saveMailboxState(spec mailboxSpec, s mailboxState) error {
	p, err := mailboxStatePath(spec)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	t := fmt.Sprintf("%s.tmp-%d", p, os.Getpid())
	if err = os.WriteFile(t, b, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(t, 0o600)
	if err = os.Rename(t, p); err != nil {
		_ = os.Remove(t)
		return err
	}
	return os.Chmod(p, 0o600)
}

func updateMailboxState(spec mailboxSpec, change func(*mailboxState) error) error {
	p, err := mailboxStatePath(spec)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	l, err := lockfile.Acquire(p+".lock", lockfile.Holder{Name: host, PID: os.Getpid(), Intent: "update mailbox state", Since: time.Now()})
	if err != nil {
		return err
	}
	defer l.Release()
	s, err := loadMailboxState(spec)
	if err != nil {
		return err
	}
	if err := change(&s); err != nil {
		return err
	}
	return saveMailboxState(spec, s)
}

func mailboxAccept(spec mailboxSpec, to string, broadcast bool) bool {
	if broadcast {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(to), strings.TrimSpace(spec.Address))
}

func snapshotMailbox(spec mailboxSpec) ([]mailboxItem, mailboxState, error) {
	state, err := loadMailboxState(spec)
	if err != nil {
		return nil, state, err
	}
	var out []mailboxItem
	posts, err := bus.Posts()
	if err != nil {
		return nil, state, err
	}
	for _, p := range posts {
		accept := mailboxAccept(spec, p.To, p.Broadcast())
		if spec.Kind == "agent" && !accept {
			accept = p.ForReader(spec.Address)
		}
		if !accept {
			continue
		}
		id := fmt.Sprintf("mb:%d", p.Seq)
		m := state.Marks[id]
		out = append(out, mailboxItem{Schema: mailboxSchema, ID: id, Source: "mb", Seq: p.Seq, At: p.At, From: p.From, To: p.Audiences(), Topic: p.Topic, Project: m.Project, Status: m.Status, Body: p.Body, Read: m.ReadAt != "", Acknowledged: m.AckedAt != "", Preserved: m.Preserved})
	}
	rooms, err := meet.Rooms()
	if err != nil {
		return nil, state, err
	}
	for _, r := range rooms {
		if !r.Board || !stringMember(r.Members, spec.Address) {
			continue
		}
		// Read full history as the mailbox principal. HistoryRecords applies the
		// same recipient isolation as native Meet delivery without consulting its
		// cursor, so another inbox consumer cannot make durable mailbox IDs and
		// their local marks disappear.
		d, o, _, e := meet.HistoryRecords(r.ID, spec.Address, 0)
		if e != nil {
			return nil, state, e
		}
		records := append(d, o...)
		sort.SliceStable(records, func(i, j int) bool { return records[i].Seq < records[j].Seq })
		for _, rec := range records {
			ev := rec.Event
			broadcast := strings.TrimSpace(ev.To) == ""
			if !mailboxAccept(spec, ev.To, broadcast) {
				continue
			}
			id := fmt.Sprintf("meet:%s:%d", r.ID, rec.Seq)
			m := state.Marks[id]
			item := mailboxItem{Schema: mailboxSchema, ID: id, Source: "meet", Seq: rec.Seq, At: ev.TS.Format(time.RFC3339Nano), From: ev.Speaker, To: emptyAs(ev.To, "all"), Topic: ev.Kind, Project: m.Project, Status: emptyAs(m.Status, ev.Status), Room: r.ID, Body: ev.Text, Read: m.ReadAt != "", Acknowledged: m.AckedAt != "", Preserved: m.Preserved}
			if item.Project == "" {
				item.Project = r.Topic
			}
			if ev.Origin != nil {
				item.Origin = &unifiedInboxOrigin{Source: ev.Origin.Source, Seq: ev.Origin.Seq}
			}
			out = append(out, item)
		}
	}
	events, err := room.Timeline(0)
	if err != nil {
		return nil, state, err
	}
	for _, e := range events {
		if e.Type != room.EventNotify || !mailboxAccept(spec, e.To, strings.TrimSpace(e.To) == "") {
			continue
		}
		id := fmt.Sprintf("bus:%d", e.Seq)
		m := state.Marks[id]
		out = append(out, mailboxItem{Schema: mailboxSchema, ID: id, Source: "bus", Seq: e.Seq, At: e.TS, From: e.Principal, To: emptyAs(e.To, "all"), Topic: e.Topic, Project: m.Project, Status: m.Status, Room: e.Room, Body: e.Body, Read: m.ReadAt != "", Acknowledged: m.AckedAt != "", Preserved: m.Preserved})
	}
	if spec.Kind == "agent" && bus.HostRoles != nil {
		for _, role := range bus.HostRoles() {
			if !strings.EqualFold(strings.TrimSpace(role.Holder), spec.Address) {
				continue
			}
			pending, e := bus.ReadPending(role.Topic)
			if e != nil {
				return nil, state, e
			}
			for _, p := range pending {
				id := fmt.Sprintf("role:%s:%d", role.Label, p.Seq)
				mark := state.Marks[id]
				out = append(out, mailboxItem{Schema: mailboxSchema, ID: id, Source: "role", Seq: p.Seq, At: p.TS, From: p.Principal, To: role.Label, Topic: p.Topic, Project: mark.Project, Status: mark.Status, Room: p.Room, Body: p.Body, Read: mark.ReadAt != "", Acknowledged: mark.AckedAt != "", Preserved: mark.Preserved})
			}
		}
	}
	// A Meet copy with structured MB provenance is the same message, not a second task.
	mb := map[int64]bool{}
	for _, i := range out {
		if i.Source == "mb" {
			mb[i.Seq] = true
		}
	}
	filtered := out[:0]
	for _, i := range out {
		if i.Source == "meet" && i.Origin != nil && strings.EqualFold(i.Origin.Source, "mb") && mb[i.Origin.Seq] {
			continue
		}
		filtered = append(filtered, i)
	}
	out = filtered
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Acknowledged != out[j].Acknowledged {
			return !out[i].Acknowledged
		}
		if out[i].Read != out[j].Read {
			return !out[i].Read
		}
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Seq < out[j].Seq
	})
	return out, state, nil
}

type mailboxFilter struct {
	source, topic, project, status, search, order string
	all                                           bool
	limit                                         int
}

func (f mailboxFilter) apply(in []mailboxItem) []mailboxItem {
	out := make([]mailboxItem, 0, len(in))
	for _, i := range in {
		if !f.all && i.Acknowledged {
			continue
		}
		if f.source != "" && !strings.EqualFold(i.Source, f.source) {
			continue
		}
		if f.topic != "" && !strings.EqualFold(i.Topic, f.topic) {
			continue
		}
		if f.project != "" && !strings.EqualFold(i.Project, f.project) {
			continue
		}
		if f.status != "" && !strings.EqualFold(i.Status, f.status) {
			continue
		}
		haystack := strings.Join([]string{i.Body, i.From, i.To, i.Topic, i.Project, i.Status, i.Room, i.Source}, " ")
		if f.search != "" && !strings.Contains(strings.ToLower(haystack), strings.ToLower(f.search)) {
			continue
		}
		out = append(out, i)
	}
	switch f.order {
	case "", "unread":
	case "newest":
		sort.SliceStable(out, func(i, j int) bool { return out[i].At > out[j].At })
	case "oldest":
		sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	case "source":
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Source == out[j].Source {
				return out[i].Seq < out[j].Seq
			}
			return out[i].Source < out[j].Source
		})
	}
	if f.limit > 0 && len(out) > f.limit {
		out = out[:f.limit]
	}
	return out
}

func addMailboxFilterFlags(cmd *cobra.Command, f *mailboxFilter) {
	p := cmd.Flags()
	p.StringVar(&f.source, "source", "", "filter by source (mb, meet, bus, role)")
	p.StringVar(&f.topic, "topic", "", "filter by exact topic")
	p.StringVar(&f.project, "project", "", "filter by project")
	p.StringVar(&f.status, "status", "", "filter by status")
	p.StringVar(&f.search, "search", "", "search body/sender/recipient/topic/project/status/room/source")
	p.StringVar(&f.order, "sort", "unread", "order: unread, newest, oldest, or source")
	p.BoolVar(&f.all, "all", false, "include acknowledged history")
	p.IntVarP(&f.limit, "limit", "n", 0, "maximum records (0 = all)")
}
func renderMailbox(cmd *cobra.Command, items []mailboxItem, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		for _, i := range items {
			if err := enc.Encode(i); err != nil {
				return err
			}
		}
		return nil
	}
	for _, i := range items {
		state := "unread"
		if i.Read {
			state = "read"
		}
		if i.Acknowledged {
			state = "acked"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] [%s] %s/%d %s → %s", i.ID, state, i.Source, i.Seq, emptyAs(i.From, "unknown"), emptyAs(i.To, "all"))
		if i.Topic != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", i.Topic)
		}
		if i.Project != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " project=%s", i.Project)
		}
		if i.Status != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " status=%s", i.Status)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n\n", i.Body)
	}
	return nil
}

func mailboxSpecFor(human bool, as string) (mailboxSpec, error) {
	if human {
		return currentHumanMailbox()
	}
	return currentAgentMailbox(as)
}
func newMailboxListCmd(human bool) *cobra.Command {
	var f mailboxFilter
	var as string
	var jsonOut bool
	c := &cobra.Command{Use: "list", Short: "list the durable mailbox without consuming it", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		switch f.order {
		case "unread", "newest", "oldest", "source":
		default:
			return fmt.Errorf("inbox list: --sort must be unread, newest, oldest, or source")
		}
		if f.limit < 0 {
			return fmt.Errorf("inbox list: --limit must not be negative")
		}
		s, e := mailboxSpecFor(human, as)
		if e != nil {
			return e
		}
		items, _, e := snapshotMailbox(s)
		if e != nil {
			return e
		}
		return renderMailbox(cmd, f.apply(items), jsonOut)
	}}
	addMailboxFilterFlags(c, &f)
	c.Flags().StringVar(&as, "as", "", "agent mailbox identity")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit "+mailboxSchema+" NDJSON")
	if human {
		_ = c.Flags().MarkHidden("as")
	}
	return c
}

func newMailboxReadCmd(human bool) *cobra.Command     { return newMailboxStateCmd("read", human) }
func newMailboxAckCmd(human bool) *cobra.Command      { return newMailboxStateCmd("ack", human) }
func newMailboxPreserveCmd(human bool) *cobra.Command { return newMailboxStateCmd("preserve", human) }

func newMailboxOrganizeCmd(human bool) *cobra.Command {
	var as, project, status string
	c := &cobra.Command{Use: "organize <id>...", Short: "label messages by project and status", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if project == "" && status == "" {
			return fmt.Errorf("inbox organize: pass --project and/or --status")
		}
		spec, err := mailboxSpecFor(human, as)
		if err != nil {
			return err
		}
		items, _, err := snapshotMailbox(spec)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, item := range items {
			known[item.ID] = true
		}
		for _, id := range args {
			if !known[id] {
				return fmt.Errorf("inbox organize: no message %q", id)
			}
		}
		return updateMailboxState(spec, func(state *mailboxState) error {
			for _, id := range args {
				mark := state.Marks[id]
				if project != "" {
					mark.Project = project
				}
				if status != "" {
					mark.Status = status
				}
				state.Marks[id] = mark
			}
			return nil
		})
	}}
	f := c.Flags()
	f.StringVar(&as, "as", "", "agent mailbox identity")
	f.StringVar(&project, "project", "", "project label")
	f.StringVar(&status, "status", "", "status label")
	if human {
		_ = f.MarkHidden("as")
	}
	return c
}
func newMailboxStateCmd(action string, human bool) *cobra.Command {
	var as string
	var jsonOut, peek bool
	c := &cobra.Command{Use: action + " <id>...", Short: map[string]string{"read": "open messages and keep them pending", "ack": "acknowledge messages explicitly", "preserve": "reopen and retain messages in the pending queue"}[action], Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		spec, e := mailboxSpecFor(human, as)
		if e != nil {
			return e
		}
		items, state, e := snapshotMailbox(spec)
		if e != nil {
			return e
		}
		by := map[string]mailboxItem{}
		for _, i := range items {
			by[i.ID] = i
		}
		now := time.Now().UTC().Format(time.RFC3339)
		var selected []mailboxItem
		for _, id := range args {
			i, ok := by[id]
			if !ok {
				return fmt.Errorf("inbox %s: no message %q", action, id)
			}
			selected = append(selected, i)
			if peek {
				continue
			}
			m := state.Marks[id]
			switch action {
			case "read":
				m.ReadAt = now
			case "ack":
				m.ReadAt = now
				m.AckedAt = now
				m.Preserved = false
			case "preserve":
				m.AckedAt = ""
				m.Preserved = true
			}
			state.Marks[id] = m
			switch action {
			case "read":
				selected[len(selected)-1].Read = true
			case "ack":
				selected[len(selected)-1].Read = true
				selected[len(selected)-1].Acknowledged = true
				selected[len(selected)-1].Preserved = false
			case "preserve":
				selected[len(selected)-1].Acknowledged = false
				selected[len(selected)-1].Preserved = true
			}
		}
		if !peek {
			changes := make(map[string]mailboxMark, len(args))
			for _, id := range args {
				changes[id] = state.Marks[id]
			}
			if e = updateMailboxState(spec, func(latest *mailboxState) error {
				for id, mark := range changes {
					current := latest.Marks[id]
					current.ReadAt = mark.ReadAt
					current.AckedAt = mark.AckedAt
					current.Preserved = mark.Preserved
					latest.Marks[id] = current
				}
				return nil
			}); e != nil {
				return e
			}
		}
		return renderMailbox(cmd, selected, jsonOut)
	}}
	c.Flags().StringVar(&as, "as", "", "agent mailbox identity")
	c.Flags().BoolVar(&jsonOut, "json", false, "emit "+mailboxSchema+" NDJSON")
	c.Flags().BoolVar(&peek, "peek", false, "show without changing read/ack state")
	if human {
		_ = c.Flags().MarkHidden("as")
	}
	return c
}

func newHumanMailboxCmd() *cobra.Command {
	c := &cobra.Command{Use: "human", Short: "the current OS user's dedicated durable mailbox", Long: `human selects the current OS user's mailbox inside the unified inbox.

Agents post concise status with a topic/project/status and stable shared reference:
  bashy inbox human send --topic posix-cert --project dhnt --status blocked --ref docs/status.md "Profile D needs review"
  bashy inbox human list --topic posix-cert
  bashy inbox human read mb:42
  bashy inbox human ack mb:42
  bashy inbox human organize mb:42 --project dhnt --status active

List and search never consume. Read marks opened but remains pending. Only ack
removes from the pending view; preserve reopens it. Agent inboxes have the same
list/read/ack/preserve/organize model at ` + "`bashy inbox list`" + `. State is owned by the
mailbox principal, so another authorized local agent sees the same human state.`, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return newMailboxListCmd(true).RunE(cmd, nil) }}
	c.AddCommand(newMailboxListCmd(true), newMailboxReadCmd(true), newMailboxAckCmd(true), newMailboxPreserveCmd(true), newMailboxOrganizeCmd(true), newHumanSendCmd())
	return c
}

func newHumanSendCmd() *cobra.Command {
	var topic, project, status, ref, as string
	var jsonOut bool
	c := &cobra.Command{Use: "send <concise-status>", Short: "send a durable status to the current human", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		spec, e := currentHumanMailbox()
		if e != nil {
			return e
		}
		from, e := bus.ResolveAuthoredActor(as)
		if e != nil {
			return e
		}
		body := args[0]
		if ref != "" {
			body += "\nref: " + ref
		}
		if e = bus.ValidateCoordinationBody(body); e != nil {
			return fmt.Errorf("inbox human send: %w", e)
		}
		seq, e := bus.PostMessageSeq(bus.Post{From: from, To: spec.Address, Topic: topic, Body: body})
		if e != nil {
			return e
		}
		id := fmt.Sprintf("mb:%d", seq)
		if project != "" || status != "" {
			if se := updateMailboxState(spec, func(state *mailboxState) error {
				m := state.Marks[id]
				m.Project = project
				m.Status = status
				state.Marks[id] = m
				return nil
			}); se != nil {
				return fmt.Errorf("message %s was posted but metadata state failed: %w", id, se)
			}
		}
		if jsonOut {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"schema": mailboxSchema, "id": id, "to": spec.Address, "topic": topic, "project": project, "status": status})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "sent %s to human %s\n", id, spec.Address)
		return nil
	}}
	f := c.Flags()
	f.StringVar(&topic, "topic", "", "interest/topic, for example posix-cert")
	f.StringVar(&project, "project", "", "project label")
	f.StringVar(&status, "status", "", "status label, for example blocked or complete")
	f.StringVar(&ref, "ref", "", "stable shared path, commit, issue, room, or artifact reference")
	f.StringVar(&as, "as", "", "sender identity (required for unattributed agent sessions)")
	f.BoolVar(&jsonOut, "json", false, "emit a machine-readable receipt")
	return c
}
