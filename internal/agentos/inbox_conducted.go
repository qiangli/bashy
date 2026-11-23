package agentos

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/weave"
)

// inboxExitTimeout is the distinguishable exit code returned when
// `bashy inbox --until-count N` reaches its `--wait` deadline before N matching
// messages arrive. Plain `--wait` without `--until-count` keeps exit 0.
const inboxExitTimeout = 3

type inboxExitError struct {
	code   int
	err    error
	silent bool
}

func (e *inboxExitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("inbox: exit %d", e.code)
	}
	return e.err.Error()
}

func (e *inboxExitError) Unwrap() error    { return e.err }
func (e *inboxExitError) ExitCode() int    { return e.code }
func (e *inboxExitError) SilentExit() bool { return e.silent }

func inboxCommandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return bus.ExitCode(err)
}

func inboxSilentExit(err error) bool {
	var s interface{ SilentExit() bool }
	return errors.As(err, &s) && s.SilentExit()
}

// conductedSprint describes an active ("doing") sprint whose conductor seat
// belongs to reader — either as lease holder (even if the 30m heartbeat TTL
// elapsed while waiting on a long worker run) or as unleased owner.
type conductedSprint struct {
	ID            int64
	RoleLabel     string
	RoleTopic     string
	ContactRef    string
	ContactHolder string
}

func conductedSprintsFor(reader string) []conductedSprint {
	reader = strings.TrimSpace(reader)
	if reader == "" {
		return nil
	}
	dir, err := weave.SprintStoreDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return nil
	}
	q, err := loadSprintWaitQueue(dir)
	if err != nil || q == nil {
		return nil
	}
	var out []conductedSprint
	seen := make(map[int64]bool)
	for _, s := range q.Stories {
		if s == nil || seen[s.ID] || !strings.EqualFold(strings.TrimSpace(s.Column), "doing") {
			continue
		}
		conductor := ""
		if s.Lease != nil && strings.TrimSpace(s.Lease.Holder) != "" {
			conductor = strings.TrimSpace(s.Lease.Holder)
		} else {
			conductor = strings.TrimSpace(s.Owner)
		}
		if conductor == "" || !strings.EqualFold(conductor, reader) {
			continue
		}
		seen[s.ID] = true
		cs := conductedSprint{
			ID:        s.ID,
			RoleLabel: fmt.Sprintf("conductor:%d", s.ID),
			RoleTopic: fmt.Sprintf("conductor.%d", s.ID),
		}
		if s.Contact != nil && (s.Contact.Kind == "" || strings.EqualFold(s.Contact.Kind, "meet")) {
			cs.ContactRef = strings.TrimSpace(s.Contact.Ref)
			cs.ContactHolder = strings.TrimSpace(s.Contact.Holder)
		}
		out = append(out, cs)
	}
	return out
}

func conductedMeetRoomSet(conducted []conductedSprint) map[string]bool {
	if len(conducted) == 0 {
		return nil
	}
	rooms := make(map[string]bool, len(conducted))
	for _, cs := range conducted {
		if cs.ContactRef != "" {
			rooms[strings.ToLower(cs.ContactRef)] = true
		}
	}
	return rooms
}

// memoizeHostRolesFor scopes one stable role table and inbox-read conductor
// audience membership to a single snapshot for reader. When reader conducts an
// open ("doing") sprint whose 30m heartbeat TTL elapsed while waiting on a
// worker, `weave.LiveSprintManagers` and `weave.conductorRoles` drop the seat
// at send time; on the READ side, reader still owns that sprint's obligations
// (`Audience{Role: "conductor"}`, `conductor:<id>` / `conductor.<id>`, and
// retained `role:conductor:<id>` buffers) and must never miss worker reports.
func memoizeHostRolesFor(reader string) (func(), []conductedSprint) {
	snapshotRolesMu.Lock()
	conducted := conductedSprintsFor(reader)
	prevRoles := bus.HostRoles
	prevSelect := bus.FleetSelect
	if prevRoles == nil && len(conducted) == 0 {
		return func() {
			snapshotRolesMu.Unlock()
		}, nil
	}
	var rolesOnce sync.Once
	var roles []bus.HostRole
	if prevRoles != nil || len(conducted) > 0 {
		bus.HostRoles = func() []bus.HostRole {
			rolesOnce.Do(func() {
				if prevRoles != nil {
					roles = append(roles, prevRoles()...)
				}
				if len(conducted) > 0 {
					byTopic := make(map[string]int, len(roles))
					for i, r := range roles {
						byTopic[strings.ToLower(strings.TrimSpace(r.Topic))] = i
					}
					for _, cs := range conducted {
						key := strings.ToLower(cs.RoleTopic)
						if idx, ok := byTopic[key]; ok {
							if strings.TrimSpace(roles[idx].Holder) == "" {
								roles[idx].Holder = reader
							}
							continue
						}
						byTopic[key] = len(roles)
						roles = append(roles, bus.HostRole{
							Label:  cs.RoleLabel,
							Topic:  cs.RoleTopic,
							Holder: reader,
						})
					}
				}
			})
			return roles
		}
	}
	if prevSelect != nil && len(conducted) > 0 {
		bus.FleetSelect = func(aud bus.Audience) ([]string, error) {
			names, err := prevSelect(aud)
			if err != nil {
				return nil, err
			}
			if aud.Validate() == nil && strings.EqualFold(strings.TrimSpace(aud.Role), "conductor") && !stringMember(names, reader) {
				names = append(append([]string(nil), names...), reader)
			}
			return names, nil
		}
	}
	return func() {
		bus.HostRoles = prevRoles
		bus.FleetSelect = prevSelect
		snapshotRolesMu.Unlock()
	}, conducted
}

func mbInboxTo(p bus.Post) string {
	if strings.TrimSpace(p.To) != "" {
		return bus.RoleLabelFor(p.To)
	}
	if !p.Broadcast() {
		return p.Audiences()
	}
	return ""
}

var (
	inboxRefPrefixRE   = regexp.MustCompile(`^\s*\[ref:[^\]]+\]\s*`)
	inboxWorkerEventRE = regexp.MustCompile(`^(DONE|DESIGN|BLOCKED|DECISION)\s+([0-9a-fA-F]{6,40})\b`)
	inboxSubmittedRE   = regexp.MustCompile(`\bsubmitted story\s+([0-9a-fA-F]{6,40})\s+for\b`)
)

// classifyInboxEvent shares `sprint await`'s event taxonomy with `bashy inbox`
// so manager watches see structured event_type, story_id, and suggested_verb on
// worker coordination reports and directed replies.
func classifyInboxEvent(body, to string) (eventType, storyID, suggestedVerb string) {
	trimmed := strings.TrimSpace(inboxRefPrefixRE.ReplaceAllString(body, ""))
	if m := inboxWorkerEventRE.FindStringSubmatch(trimmed); m != nil {
		id := strings.ToLower(m[2])
		switch m[1] {
		case "DONE":
			return "worker_done", id, "review"
		case "DESIGN":
			return "worker_design", id, "reply"
		case "BLOCKED":
			return "worker_blocked", id, "reply"
		case "DECISION":
			return "worker_decision", id, "reply"
		}
	}
	if m := inboxSubmittedRE.FindStringSubmatch(trimmed); m != nil {
		return "story_submitted", strings.ToLower(m[1]), "accept"
	}
	if strings.TrimSpace(to) != "" && !strings.EqualFold(strings.TrimSpace(to), "all") {
		return "manager_reply", "", "reply"
	}
	return "", "", ""
}
